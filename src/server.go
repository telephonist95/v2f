package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var convertMu sync.Mutex

type convertRequest struct {
	Source string `json:"source"`
	Top    string `json:"top"`
}

type convertResponse struct {
	Top        string            `json:"top"`
	YosysLog   string            `json:"yosysLog"`
	YosysJSON  string            `json:"yosysJson"`
	Gate       circuitView       `json:"gate"`
	RTL        circuitView       `json:"rtl"`
	ModuleList []string          `json:"moduleList"`
	Simulation *simulationResult `json:"simulation,omitempty"`
}

// simulationResult is the API-facing form of a behavioural simulation run.
// VCDText is the raw .vcd content (so the user can download it directly);
// Parsed is the structured form for the in-browser waveform viewer.
type simulationResult struct {
	OK      bool     `json:"ok"`
	TopTB   string   `json:"topTB"`
	AutoTB  bool     `json:"autoTB"`
	Log     string   `json:"log"`
	Error   string   `json:"error,omitempty"`
	VCDText string   `json:"vcdText,omitempty"`
	Parsed  *VCDFile `json:"parsed,omitempty"`
}

type circuitView struct {
	Falstad        string         `json:"falstad"`
	FalstadLabeled string         `json:"falstadLabeled"`
	Gost           gostDiagram    `json:"gost"`
	NetLabels      map[int]string `json:"netLabels,omitempty"`
}

type exampleInfo struct {
	Name   string `json:"name"`
	Top    string `json:"top"`
	Source string `json:"source"`
}

func ServeHTTP(addr, staticDir string) error {
	root := repoRoot()
	if !filepath.IsAbs(staticDir) {
		staticDir = filepath.Join(root, staticDir)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/convert", withCORS(handleConvert(root)))
	mux.HandleFunc("/api/examples", withCORS(handleExamples(root)))
	mux.Handle("/", spaHandler(staticDir))

	fmt.Printf("ver2fal web server listening on http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// maxConvertRequestBytes caps the /api/convert request body to prevent
// memory exhaustion via large source payloads.
const maxConvertRequestBytes = 1 << 20 // 1 MiB

func handleConvert(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxConvertRequestBytes)

		var req convertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON request: %w", err), "")
			return
		}
		req.Source = strings.TrimSpace(req.Source)
		if req.Source == "" {
			writeJSONError(w, http.StatusBadRequest, fmt.Errorf("source is empty"), "")
			return
		}

		convertMu.Lock()
		defer convertMu.Unlock()

		resp, err := synthesizeAndConvert(root, req)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err, resp.YosysLog)
			return
		}
		writeJSON(w, resp)
	}
}

func handleExamples(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET required", http.StatusMethodNotAllowed)
			return
		}
		paths, err := filepath.Glob(filepath.Join(root, "verilog", "*.sv"))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err, "")
			return
		}
		sort.Strings(paths)
		var examples []exampleInfo
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			source := string(data)
			examples = append(examples, exampleInfo{
				Name:   filepath.Base(p),
				Top:    inferTopModule(source),
				Source: source,
			})
		}
		writeJSON(w, examples)
	}
}

func synthesizeAndConvert(root string, req convertRequest) (convertResponse, error) {
	resp := convertResponse{}
	resp.ModuleList = moduleNames(req.Source)
	resp.Top = strings.TrimSpace(req.Top)
	if resp.Top == "" {
		resp.Top = inferTopModule(req.Source)
	}
	if resp.Top == "" {
		return resp, fmt.Errorf("cannot infer top module; specify it manually")
	}

	tmp, err := os.MkdirTemp("", "ver2fal-*")
	if err != nil {
		return resp, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	srcFile := filepath.Join(tmp, "input.sv")
	jsonFile := filepath.Join(tmp, "netlist.json")
	// Strip testbench-only modules (those that call $dumpvars / forever / etc.)
	// before sending to yosys, since yosys -synth refuses non-synthesizable
	// constructs. The original source is preserved separately for simulation.
	synthSource := stripTestbenchModules(req.Source)
	if err := os.WriteFile(srcFile, []byte(synthSource), 0o600); err != nil {
		return resp, fmt.Errorf("writing source file: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "yosys", "-c", filepath.Join(root, "synth.tcl"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"SRC="+srcFile,
		"TOP="+resp.Top,
		"OUT="+jsonFile,
	)
	out, err := cmd.CombinedOutput()
	resp.YosysLog = string(out)
	if ctx.Err() == context.DeadlineExceeded {
		return resp, fmt.Errorf("yosys timed out")
	}
	if err != nil {
		return resp, fmt.Errorf("yosys failed: %w", err)
	}

	jsonData, err := os.ReadFile(jsonFile)
	if err != nil {
		return resp, fmt.Errorf("reading synthesized JSON: %w", err)
	}
	resp.YosysJSON = string(jsonData)

	design, err := ParseDesign(jsonFile)
	if err != nil {
		return resp, err
	}
	_, mod := GetTopModule(design)
	if mod == nil {
		return resp, fmt.Errorf("no top module in synthesized design")
	}

	gateCircuit, err := Convert(mod)
	if err != nil {
		return resp, fmt.Errorf("gate-level conversion failed: %w", err)
	}
	rtlCircuit, err := ConvertRTL(mod)
	if err != nil {
		return resp, fmt.Errorf("RTL conversion failed: %w", err)
	}

	resp.Gate = circuitView{
		Falstad:        EmitFalstad(gateCircuit),
		FalstadLabeled: EmitFalstadLabeled(gateCircuit),
		Gost:           BuildGostDiagram(gateCircuit, "Вентильная схема"),
		NetLabels:      BuildNetLabels(gateCircuit),
	}
	resp.RTL = circuitView{
		Falstad:        EmitFalstad(rtlCircuit),
		FalstadLabeled: EmitFalstadLabeled(rtlCircuit),
		Gost:           BuildGostDiagram(rtlCircuit, "RTL-схема"),
		NetLabels:      BuildNetLabels(rtlCircuit),
	}

	// Behavioural simulation via Icarus Verilog. Failures here do not abort
	// the request — instead the error is reported in the simulation payload.
	resp.Simulation = runBehaviouralSimulation(req.Source, resp.Top, mod)
	return resp, nil
}

// runBehaviouralSimulation runs iverilog+vvp, parses VCD, and returns an
// API-shaped result. The function never returns nil; on failure it returns
// a result with OK=false and an Error explanation.
func runBehaviouralSimulation(source, dutTop string, mod *Module) *simulationResult {
	res := &simulationResult{}
	out, err := RunSimulation(source, dutTop, mod, 30*time.Second)
	if out != nil {
		res.Log = out.Log
		res.TopTB = out.TopTB
		res.AutoTB = out.AutoTB
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.VCDText = out.VCDText
	parsed, perr := ParseVCD(out.VCDText)
	if perr != nil {
		res.Error = "VCD parse failed: " + perr.Error()
		return res
	}
	res.Parsed = parsed
	res.OK = true
	return res
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "synth.tcl")); err == nil {
			return wd
		}
		next := filepath.Dir(wd)
		if next == wd {
			return "."
		}
		wd = next
	}
}

var moduleDeclRE = regexp.MustCompile(`(?m)^\s*module\s+([A-Za-z_][A-Za-z0-9_$]*)\b`)

// nonsynthRE detects constructs typical of behavioural testbenches that
// yosys cannot synthesise — used to identify modules to skip for yosys.
var nonsynthRE = regexp.MustCompile(`\$dumpvars|\$dumpfile|\$finish|\$display|\$monitor|\bforever\b|\binitial\b`)

// stripTestbenchModules returns source with testbench-only modules removed.
// A module is considered a testbench if its body contains $dumpvars / $finish /
// forever / initial — none of which yosys synthesises.
func stripTestbenchModules(source string) string {
	endRE := regexp.MustCompile(`(?m)^\s*endmodule\b`)
	var out strings.Builder
	i := 0
	for i < len(source) {
		m := moduleDeclRE.FindStringSubmatchIndex(source[i:])
		if m == nil {
			out.WriteString(source[i:])
			break
		}
		// Copy text before module declaration.
		out.WriteString(source[i : i+m[0]])

		bodyStart := i + m[0]
		mEnd := endRE.FindStringIndex(source[bodyStart:])
		if mEnd == nil {
			// Unterminated module — keep as is and stop.
			out.WriteString(source[bodyStart:])
			break
		}
		bodyEnd := bodyStart + mEnd[1]
		body := source[bodyStart:bodyEnd]
		if !nonsynthRE.MatchString(body) {
			out.WriteString(body)
		}
		i = bodyEnd
	}
	return out.String()
}

func moduleNames(source string) []string {
	matches := moduleDeclRE.FindAllStringSubmatch(source, -1)
	var names []string
	seen := map[string]bool{}
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

func inferTopModule(source string) string {
	names := moduleNames(source)
	for _, name := range names {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "tb") && !strings.Contains(lower, "test") {
			return name
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, err error, log string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
		"log":   log,
	})
}

func spaHandler(staticDir string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(staticDir))
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(index); err == nil {
			http.ServeFile(w, r, index)
			return
		}
		http.Error(w, "frontend is not built; run npm run build in vkr", http.StatusNotFound)
	}
}
