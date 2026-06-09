package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// simulationOutput is the raw result of running iverilog+vvp on a design.
type simulationOutput struct {
	VCDText string // raw VCD contents
	Log     string // combined stdout+stderr from iverilog/vvp
	TopTB   string // testbench top module name (auto-generated or detected)
	AutoTB  bool   // true if testbench was synthesized by ver2fal
}

// RunSimulation compiles and runs the SystemVerilog source through Icarus
// Verilog (iverilog + vvp) and returns the resulting VCD waveform.
//
// If the source already contains a $dumpvars-bearing module, that module is
// used as the simulation top. Otherwise a minimal auto-testbench is generated
// from the synthesized module ports.
//
// timeout caps the overall iverilog+vvp wall time.
func RunSimulation(source, dutTop string, mod *Module, timeout time.Duration) (*simulationOutput, error) {
	tb := detectTestbench(source)
	autoTB := false
	finalSource := source
	tbTop := tb
	if tb == "" {
		autoTB = true
		tbTop = "__auto_tb"
		gen, err := generateAutoTestbench(dutTop, mod, tbTop)
		if err != nil {
			return nil, fmt.Errorf("auto-testbench: %w", err)
		}
		finalSource = source + "\n\n" + gen
	}

	tmp, err := os.MkdirTemp("", "ver2fal-sim-*")
	if err != nil {
		return nil, fmt.Errorf("mkdir temp: %w", err)
	}
	defer os.RemoveAll(tmp)

	srcPath := filepath.Join(tmp, "design.sv")
	vvpPath := filepath.Join(tmp, "sim.vvp")
	vcdPath := filepath.Join(tmp, "sim.vcd")

	if err := os.WriteFile(srcPath, []byte(finalSource), 0o600); err != nil {
		return nil, fmt.Errorf("write source: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out := &simulationOutput{TopTB: tbTop, AutoTB: autoTB}
	var logBuf strings.Builder

	// Compile with iverilog.
	compileCmd := exec.CommandContext(ctx, "iverilog",
		"-g2012", "-o", vvpPath, "-s", tbTop, srcPath)
	compileCmd.Dir = tmp
	compileOut, err := compileCmd.CombinedOutput()
	logBuf.WriteString("=== iverilog ===\n")
	logBuf.Write(compileOut)
	if ctx.Err() == context.DeadlineExceeded {
		out.Log = logBuf.String()
		return out, fmt.Errorf("iverilog timed out")
	}
	if err != nil {
		out.Log = logBuf.String()
		return out, fmt.Errorf("iverilog failed: %w", err)
	}

	// Run with vvp; intercept $dumpfile to redirect to vcdPath.
	// vvp resolves $dumpfile relative to CWD, so we set Dir=tmp.
	// If the user-supplied testbench passed a different dumpfile name, we still
	// recover whichever .vcd file actually got created.
	runCmd := exec.CommandContext(ctx, "vvp", vvpPath)
	runCmd.Dir = tmp
	// Force $dumpfile-less testbenches to dump to a known path via DUMPFILE env.
	runCmd.Env = append(os.Environ(), "VCD_OUT="+vcdPath)
	runOut, err := runCmd.CombinedOutput()
	logBuf.WriteString("\n=== vvp ===\n")
	logBuf.Write(runOut)
	out.Log = logBuf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("vvp timed out")
	}
	if err != nil {
		return out, fmt.Errorf("vvp failed: %w", err)
	}

	// Locate VCD: prefer sim.vcd, else first *.vcd in tmp.
	vcdData, vcdErr := os.ReadFile(vcdPath)
	if vcdErr != nil {
		matches, _ := filepath.Glob(filepath.Join(tmp, "*.vcd"))
		for _, m := range matches {
			if data, err := os.ReadFile(m); err == nil {
				vcdData = data
				vcdErr = nil
				break
			}
		}
	}
	if vcdErr != nil || len(vcdData) == 0 {
		return out, fmt.Errorf("simulation produced no VCD output (did the testbench call $dumpvars?)")
	}
	out.VCDText = string(vcdData)
	return out, nil
}

// dumpvarsRE detects a module that calls $dumpvars (i.e. is a testbench).
var dumpvarsRE = regexp.MustCompile(`\$dumpvars\b`)

// detectTestbench returns the name of a module that contains $dumpvars,
// or empty string if no such module exists.
func detectTestbench(source string) string {
	// Walk modules: find module ... endmodule blocks; if $dumpvars appears inside, that module is a testbench.
	type modSpan struct {
		name       string
		start, end int
	}
	var spans []modSpan
	startRE := regexp.MustCompile(`(?m)^\s*module\s+([A-Za-z_][A-Za-z0-9_$]*)\b`)
	endRE := regexp.MustCompile(`(?m)^\s*endmodule\b`)

	i := 0
	for i < len(source) {
		mStart := startRE.FindStringSubmatchIndex(source[i:])
		if mStart == nil {
			break
		}
		nameStart, nameEnd := i+mStart[2], i+mStart[3]
		name := source[nameStart:nameEnd]
		bodyStart := i + mStart[1]
		mEnd := endRE.FindStringIndex(source[bodyStart:])
		if mEnd == nil {
			break
		}
		bodyEnd := bodyStart + mEnd[1]
		spans = append(spans, modSpan{name: name, start: bodyStart, end: bodyEnd})
		i = bodyEnd
	}

	for _, s := range spans {
		body := source[s.start:s.end]
		if dumpvarsRE.MatchString(body) {
			return s.name
		}
	}
	return ""
}

// generateAutoTestbench builds a minimal testbench that:
//   - declares a reg for every DUT input (special-cased clk, rst*, en*)
//   - wires a wire for every DUT output
//   - instantiates the DUT
//   - drives clk, asserts reset for a few cycles, releases, varies data inputs
//   - calls $dumpvars and $finish after a fixed time budget
//
// It returns the testbench source code (a single module).
func generateAutoTestbench(dutTop string, mod *Module, tbName string) (string, error) {
	if mod == nil {
		return "", fmt.Errorf("module info required for auto-testbench")
	}

	type portInfo struct {
		name         string
		width        int
		isInput      bool
		isClock      bool
		isReset      bool
		rstActiveLow bool
		isEnable     bool
	}

	var ports []portInfo

	var portNames []string
	for n := range mod.Ports {
		portNames = append(portNames, n)
	}
	sort.Strings(portNames)

	for _, name := range portNames {
		p := mod.Ports[name]
		width := len(p.Bits)
		if width < 1 {
			width = 1
		}
		lower := strings.ToLower(name)
		pi := portInfo{
			name:    name,
			width:   width,
			isInput: p.Direction == "input",
			isClock: strings.Contains(lower, "clk") || strings.Contains(lower, "clock"),
			isReset: strings.Contains(lower, "rst") || strings.Contains(lower, "reset"),
			isEnable: lower == "en" || strings.HasPrefix(lower, "en_") ||
				strings.HasSuffix(lower, "_en") || strings.Contains(lower, "enable"),
		}
		if pi.isReset {
			// _n suffix → active-low (rst_n, reset_n, etc.)
			pi.rstActiveLow = strings.HasSuffix(lower, "_n") || strings.HasSuffix(lower, "n")
		}
		ports = append(ports, pi)
	}

	var lines []string
	lines = append(lines,
		fmt.Sprintf("// Auto-generated testbench for module %s", dutTop),
		fmt.Sprintf("module %s;", tbName),
	)

	// Declare reg/wire for each port.
	for _, p := range ports {
		decl := "reg"
		if !p.isInput {
			decl = "wire"
		}
		if p.width > 1 {
			lines = append(lines, fmt.Sprintf("    %s [%d:0] %s;", decl, p.width-1, p.name))
		} else {
			lines = append(lines, fmt.Sprintf("    %s %s;", decl, p.name))
		}
	}

	// Instantiate DUT with .port(port) connections.
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("    %s dut (", dutTop))
	var conns []string
	for _, p := range ports {
		conns = append(conns, fmt.Sprintf("        .%s(%s)", p.name, p.name))
	}
	lines = append(lines, strings.Join(conns, ",\n"))
	lines = append(lines, "    );")

	// Clock generator.
	hasClock := false
	for _, p := range ports {
		if p.isInput && p.isClock {
			hasClock = true
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("    initial %s = 0;", p.name))
			lines = append(lines, fmt.Sprintf("    always #5 %s = ~%s;", p.name, p.name))
		}
	}

	// Initial block with stimuli.
	lines = append(lines, "")
	lines = append(lines, "    initial begin")
	lines = append(lines, `        $dumpfile("sim.vcd");`)
	lines = append(lines, fmt.Sprintf("        $dumpvars(0, %s);", tbName))

	// Initial values for non-clock inputs.
	for _, p := range ports {
		if !p.isInput || p.isClock {
			continue
		}
		switch {
		case p.isReset && p.rstActiveLow:
			lines = append(lines, fmt.Sprintf("        %s = 1'b0; // assert reset", p.name))
		case p.isReset && !p.rstActiveLow:
			lines = append(lines, fmt.Sprintf("        %s = 1'b1; // assert reset", p.name))
		case p.isEnable:
			lines = append(lines, fmt.Sprintf("        %s = 1'b0;", p.name))
		default:
			lines = append(lines, fmt.Sprintf("        %s = 0;", p.name))
		}
	}

	if hasClock {
		lines = append(lines, "        #20;")
	} else {
		lines = append(lines, "        #1;")
	}

	// Release reset, enable.
	for _, p := range ports {
		if !p.isInput {
			continue
		}
		if p.isReset {
			if p.rstActiveLow {
				lines = append(lines, fmt.Sprintf("        %s = 1'b1; // release reset", p.name))
			} else {
				lines = append(lines, fmt.Sprintf("        %s = 1'b0; // release reset", p.name))
			}
		}
		if p.isEnable {
			lines = append(lines, fmt.Sprintf("        %s = 1'b1;", p.name))
		}
	}

	// Drive data inputs through several patterns.
	stepDelay := "#10"
	if !hasClock {
		stepDelay = "#5"
	}

	dataPorts := []portInfo{}
	for _, p := range ports {
		if p.isInput && !p.isClock && !p.isReset && !p.isEnable {
			dataPorts = append(dataPorts, p)
		}
	}

	if len(dataPorts) > 0 {
		// Sweep simple patterns.
		patterns := []string{"0", "1", "'h5A", "'hFF", "'h00"}
		for _, pat := range patterns {
			for _, p := range dataPorts {
				v := pat
				if p.width == 1 {
					switch pat {
					case "0":
						v = "1'b0"
					case "1":
						v = "1'b1"
					default:
						v = "1'b1"
					}
				} else {
					v = fmt.Sprintf("%d'b0 | %d%s", p.width, p.width, pat)
					// simpler: just truncated literal
					v = fmt.Sprintf("%d%s", p.width, pat)
				}
				lines = append(lines, fmt.Sprintf("        %s = %s;", p.name, v))
			}
			lines = append(lines, fmt.Sprintf("        %s;", stepDelay))
		}
	}

	// Final settle + finish.
	if hasClock {
		lines = append(lines, "        #50;")
	} else {
		lines = append(lines, "        #20;")
	}
	lines = append(lines, "        $finish;")
	lines = append(lines, "    end")
	lines = append(lines, "")

	// Safety watchdog: hard finish at 10000 time units to avoid runaway designs.
	lines = append(lines, "    initial begin")
	lines = append(lines, "        #10000;")
	lines = append(lines, `        $display("auto-tb watchdog: forced $finish");`)
	lines = append(lines, "        $finish;")
	lines = append(lines, "    end")

	lines = append(lines, "endmodule")
	return strings.Join(lines, "\n") + "\n", nil
}
