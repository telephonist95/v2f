package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const MaxTruthTableInputs = 12

// ffDetail holds extracted info about a single flip-flop cell.
type ffDetail struct {
	name     string
	cell     *Cell
	baseType string
	params   string
	clkNet   int
	rstNet   int
	enNet    int
	dNet     int
	qNet     int
}

func extractFF(name string, cell *Cell) *ffDetail {
	typeName := strings.TrimPrefix(cell.Type, "$_")
	typeName = strings.TrimSuffix(typeName, "_")
	idx := strings.LastIndex(typeName, "_")
	if idx < 0 {
		return nil
	}

	fd := &ffDetail{
		name: name, cell: cell,
		baseType: typeName[:idx], params: typeName[idx+1:],
		rstNet: -1, enNet: -1, dNet: -1, qNet: -1, clkNet: -1,
	}

	getNet := func(port string) int {
		if bits, ok := cell.Connections[port]; ok && len(bits) > 0 {
			if id, ok := BitToNetID(bits[0]); ok {
				return id
			}
		}
		return -1
	}

	fd.qNet = getNet("Q")
	fd.dNet = getNet("D")

	switch fd.baseType {
	case "DFF":
		fd.clkNet = getNet("C")
		if len(fd.params) >= 3 {
			fd.rstNet = getNet("R")
		}
	case "DFFE":
		fd.clkNet = getNet("C")
		fd.enNet = getNet("E")
		if len(fd.params) >= 4 {
			fd.rstNet = getNet("R")
		}
	case "SDFF":
		fd.clkNet = getNet("C")
		fd.rstNet = getNet("R")
	case "SDFFE", "SDFFCE":
		fd.clkNet = getNet("C")
		fd.rstNet = getNet("R")
		fd.enNet = getNet("E")
	case "DLATCH":
		fd.clkNet = getNet("E")
		if len(fd.params) >= 3 {
			fd.rstNet = getNet("R")
		}
	case "SR":
		// SR latch: S stored in dNet (acts as "data" input),
		// R stored in rstNet. No clock — clkNet stays -1.
		fd.dNet = getNet("S")
		fd.rstNet = getNet("R")
	default:
		return nil
	}
	return fd
}

type regGroup struct {
	baseType string
	params   string
	clkNet   int
	rstNet   int
	enNet    int
	ffs      []*ffDetail
	dNets    []int
	qNets    []int
}

func groupRegisters(ffs []*ffDetail) []*regGroup {
	type key struct {
		baseType, params      string
		clkNet, rstNet, enNet int
	}
	groups := map[key]*regGroup{}
	var result []*regGroup
	for _, fd := range ffs {
		// SR cells encode S into dNet — bits cannot be grouped by R alone
		// since they may have different S inputs. Keep each SR in its own group.
		if fd.baseType == "SR" {
			g := &regGroup{
				baseType: fd.baseType, params: fd.params,
				clkNet: fd.clkNet, rstNet: fd.rstNet, enNet: fd.enNet,
				ffs: []*ffDetail{fd}, dNets: []int{fd.dNet}, qNets: []int{fd.qNet},
			}
			result = append(result, g)
			continue
		}
		k := key{fd.baseType, fd.params, fd.clkNet, fd.rstNet, fd.enNet}
		g, ok := groups[k]
		if !ok {
			g = &regGroup{baseType: fd.baseType, params: fd.params, clkNet: fd.clkNet, rstNet: fd.rstNet, enNet: fd.enNet}
			groups[k] = g
		}
		g.ffs = append(g.ffs, fd)
	}
	for _, g := range groups {
		sort.Slice(g.ffs, func(i, j int) bool { return g.ffs[i].qNet < g.ffs[j].qNet })
		for _, fd := range g.ffs {
			g.dNets = append(g.dNets, fd.dNet)
			g.qNets = append(g.qNets, fd.qNet)
		}
		result = append(result, g)
	}
	sort.Slice(result, func(i, j int) bool {
		if len(result[i].qNets) > 0 && len(result[j].qNets) > 0 {
			return result[i].qNets[0] < result[j].qNets[0]
		}
		return i < j
	})
	return result
}

func captureVars(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + i)
	}
	return string(b)
}

func generateRegisterModel(reg *regGroup, idx int) *CustomModel {
	w := len(reg.ffs)
	edge := func(c byte) string {
		if c == 'P' {
			return "+"
		}
		return "-"
	}
	pol := func(c byte) string {
		if c == 'P' {
			return "1"
		}
		return "0"
	}
	inv := func(c byte) string {
		if c == 'P' {
			return "0"
		}
		return "1"
	}

	dcN := strings.Repeat("?", w)
	capN := captureVars(w)
	p := reg.params

	var inputs []string
	var rules []string

	switch reg.baseType {
	case "DFF":
		if len(p) == 1 {
			inputs = append(inputs, "C")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			rules = []string{edge(p[0]) + capN + "=" + capN}
		} else if len(p) == 3 {
			inputs = append(inputs, "C", "R")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			rstVal := strings.Repeat(string(p[2]), w)
			rules = []string{
				"?" + pol(p[1]) + dcN + "=" + rstVal,
				edge(p[0]) + inv(p[1]) + capN + "=" + capN,
			}
		}
	case "DFFE":
		if len(p) == 2 {
			inputs = append(inputs, "C")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			inputs = append(inputs, "E")
			rules = []string{edge(p[0]) + capN + pol(p[1]) + "=" + capN}
		} else if len(p) == 4 {
			inputs = append(inputs, "C", "R")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			inputs = append(inputs, "E")
			rstVal := strings.Repeat(string(p[2]), w)
			rules = []string{
				"?" + pol(p[1]) + dcN + "?=" + rstVal,
				edge(p[0]) + inv(p[1]) + capN + pol(p[3]) + "=" + capN,
			}
		}
	case "SDFF":
		if len(p) == 3 {
			inputs = append(inputs, "C", "R")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			rstVal := strings.Repeat(string(p[2]), w)
			rules = []string{
				edge(p[0]) + pol(p[1]) + dcN + "=" + rstVal,
				edge(p[0]) + inv(p[1]) + capN + "=" + capN,
			}
		}
	case "SDFFE":
		if len(p) == 4 {
			inputs = append(inputs, "C", "R")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			inputs = append(inputs, "E")
			rstVal := strings.Repeat(string(p[2]), w)
			rules = []string{
				edge(p[0]) + pol(p[1]) + dcN + "?=" + rstVal,
				edge(p[0]) + inv(p[1]) + capN + pol(p[3]) + "=" + capN,
			}
		}
	case "SDFFCE":
		if len(p) == 4 {
			inputs = append(inputs, "C", "R")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			inputs = append(inputs, "E")
			rstVal := strings.Repeat(string(p[2]), w)
			rules = []string{
				edge(p[0]) + pol(p[1]) + dcN + pol(p[3]) + "=" + rstVal,
				edge(p[0]) + inv(p[1]) + capN + pol(p[3]) + "=" + capN,
			}
		}
	case "DLATCH":
		// Level-sensitive — uses pol(E), not edge.
		if len(p) == 1 {
			inputs = append(inputs, "E")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			rules = []string{pol(p[0]) + capN + "=" + capN}
		} else if len(p) == 3 {
			inputs = append(inputs, "E", "R")
			for i := 0; i < w; i++ {
				inputs = append(inputs, fmt.Sprintf("D%d", i))
			}
			rstVal := strings.Repeat(string(p[2]), w)
			rules = []string{
				"?" + pol(p[1]) + dcN + "=" + rstVal,
				pol(p[0]) + inv(p[1]) + capN + "=" + capN,
			}
		}
	case "SR":
		// SR is always grouped as width=1 (see groupRegisters).
		if w == 1 && len(p) == 2 {
			inputs = []string{"S", "R"}
			sEdge := edge(p[0])
			rEdge := edge(p[1])
			rules = []string{
				"?" + rEdge + "=0",
				sEdge + "?=1",
			}
		}
	}

	if len(inputs) == 0 {
		return nil
	}

	var outputs []string
	for i := 0; i < w; i++ {
		outputs = append(outputs, fmt.Sprintf("Q%d", i))
	}

	modelName := fmt.Sprintf("REG_%s_%s_%d_%d", reg.baseType, reg.params, w, idx)
	return &CustomModel{Name: modelName, Inputs: inputs, Outputs: outputs, Rules: rules}
}

// evalCombCell evaluates a single combinational cell.
func evalCombCell(cellType string, pv map[string]int) int {
	a, b, c, d, s, t := pv["A"], pv["B"], pv["C"], pv["D"], pv["S"], pv["T"]
	switch cellType {
	case "$_NOT_":
		return 1 - a
	case "$_BUF_":
		return a
	case "$_AND_":
		return a & b
	case "$_OR_":
		return a | b
	case "$_XOR_":
		return a ^ b
	case "$_NAND_":
		if a&b == 1 {
			return 0
		}
		return 1
	case "$_NOR_":
		if a|b == 1 {
			return 0
		}
		return 1
	case "$_XNOR_":
		if a^b == 1 {
			return 0
		}
		return 1
	case "$_ANDNOT_":
		return a & (1 - b)
	case "$_ORNOT_":
		if a == 1 || b == 0 {
			return 1
		}
		return 0
	case "$_MUX_":
		if s == 0 {
			return a
		}
		return b
	case "$_NMUX_":
		if s == 0 {
			return 1 - a
		}
		return 1 - b
	case "$_MUX4_":
		switch s + t*2 {
		case 0:
			return a
		case 1:
			return b
		case 2:
			return c
		case 3:
			return d
		}
	case "$_AOI3_":
		if (a&b)|c == 1 {
			return 0
		}
		return 1
	case "$_OAI3_":
		if (a|b)&c == 1 {
			return 0
		}
		return 1
	case "$_AOI4_":
		if (a&b)|(c&d) == 1 {
			return 0
		}
		return 1
	case "$_OAI4_":
		if (a|b)&(c|d) == 1 {
			return 0
		}
		return 1
	}
	return 0
}

// ConvertRTL builds a synthetic Module with RTL-level blocks and passes it
// through the existing Convert() for proper placement and routing.
func ConvertRTL(mod *Module) (*Circuit, error) {
	// Classify cells
	type combEntry struct {
		name string
		cell *Cell
	}
	var combs []combEntry
	var ffs []*ffDetail

	var cellNames []string
	for n := range mod.Cells {
		cellNames = append(cellNames, n)
	}
	sort.Strings(cellNames)

	for _, name := range cellNames {
		cell := mod.Cells[name]
		if IsSequentialCell(cell.Type) {
			if fd := extractFF(name, cell); fd != nil {
				ffs = append(ffs, fd)
			}
		} else {
			combs = append(combs, combEntry{name, cell})
		}
	}

	regs := groupRegisters(ffs)

	// Identify Q nets and control nets
	qNetSet := map[int]bool{}
	for _, r := range regs {
		for _, n := range r.qNets {
			qNetSet[n] = true
		}
	}

	// Collect input port net IDs for filtering
	inputPortNetSet := map[int]bool{}
	for _, port := range mod.Ports {
		if port.Direction == "input" {
			for _, b := range port.Bits {
				if id, ok := BitToNetID(b); ok {
					inputPortNetSet[id] = true
				}
			}
		}
	}

	// Collect unique control nets (clk, rst, en).
	// Only nets driven by INPUT PORTS are pass-through candidates (capture-vars).
	// Control nets driven by combinational logic (e.g. computed enable)
	// must be COMB outputs instead, not pass-through.
	controlNetSet := map[int]bool{}  // all control nets (excluded from data inputs)
	passthroughSet := map[int]bool{} // only port-driven control nets (capture-vars)
	var passthroughNets []int        // port-driven, in order
	var combDrivenCtrlNets []int     // control nets driven by comb logic → COMB outputs

	for _, r := range regs {
		for _, nid := range []int{r.clkNet, r.rstNet, r.enNet} {
			if nid < 0 || controlNetSet[nid] {
				continue
			}
			controlNetSet[nid] = true
			if inputPortNetSet[nid] {
				passthroughSet[nid] = true
				passthroughNets = append(passthroughNets, nid)
			} else {
				combDrivenCtrlNets = append(combDrivenCtrlNets, nid)
			}
		}
	}

	// Cloud interface
	// Data inputs: non-control input port nets + register Q nets
	// Pass-through inputs: control nets (clk, rst, en)
	// Data outputs: register D nets + non-Q output port nets
	// Pass-through outputs: same control nets (forwarded to register)
	var cloudDataInputNets []int
	cloudInSet := map[int]bool{}

	var inPortNames []string
	for name, port := range mod.Ports {
		if port.Direction == "input" {
			inPortNames = append(inPortNames, name)
		}
	}
	sort.Strings(inPortNames)
	for _, name := range inPortNames {
		for _, b := range mod.Ports[name].Bits {
			if id, ok := BitToNetID(b); ok && !controlNetSet[id] && !cloudInSet[id] {
				cloudInSet[id] = true
				cloudDataInputNets = append(cloudDataInputNets, id)
			}
		}
	}
	for _, r := range regs {
		for _, qn := range r.qNets {
			if !cloudInSet[qn] {
				cloudInSet[qn] = true
				cloudDataInputNets = append(cloudDataInputNets, qn)
			}
		}
	}

	var cloudOutputNets []int
	cloudOutSet := map[int]bool{}
	for _, r := range regs {
		for _, dn := range r.dNets {
			if !cloudOutSet[dn] {
				cloudOutSet[dn] = true
				cloudOutputNets = append(cloudOutputNets, dn)
			}
		}
	}
	// Add comb-driven control nets (computed enable/reset) as COMB outputs
	for _, nid := range combDrivenCtrlNets {
		if !cloudOutSet[nid] {
			cloudOutSet[nid] = true
			cloudOutputNets = append(cloudOutputNets, nid)
		}
	}
	var outPortNames []string
	for name, port := range mod.Ports {
		if port.Direction == "output" {
			outPortNames = append(outPortNames, name)
		}
	}
	sort.Strings(outPortNames)
	for _, name := range outPortNames {
		for _, b := range mod.Ports[name].Bits {
			if id, ok := BitToNetID(b); ok && !qNetSet[id] && !cloudOutSet[id] {
				cloudOutSet[id] = true
				cloudOutputNets = append(cloudOutputNets, id)
			}
		}
	}

	// Topological sort of comb cells for truth table evaluation
	combByName := map[string]*combEntry{}
	netDriverName := map[int]string{}
	for i := range combs {
		c := &combs[i]
		combByName[c.name] = c
		for port, dir := range c.cell.PortDirections {
			if dir == "output" {
				for _, b := range c.cell.Connections[port] {
					if id, ok := BitToNetID(b); ok {
						netDriverName[id] = c.name
					}
				}
			}
		}
	}
	visited := map[string]bool{}
	var topoOrder []string
	var visit func(string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		c := combByName[name]
		if c == nil {
			return
		}
		for port, dir := range c.cell.PortDirections {
			if dir == "input" {
				for _, b := range c.cell.Connections[port] {
					if id, ok := BitToNetID(b); ok {
						if drv, ok := netDriverName[id]; ok {
							visit(drv)
						}
					}
				}
			}
		}
		topoOrder = append(topoOrder, name)
	}
	for _, c := range combs {
		visit(c.name)
	}

	// Generate register models first (needed for output ordering)
	var regModels []*CustomModel
	for i, r := range regs {
		regModels = append(regModels, generateRegisterModel(r, i))
	}

	// Build COMB model. Order COMB outputs to match REG inputs vertically
	// (C, R, D0..Dn, E) so adjacent-channel wires are horizontal, not crossing.
	nDataIn := len(cloudDataInputNets)
	nPT := len(passthroughNets)
	var combModel *CustomModel

	// Build ordered COMB output list matching register input pin order
	var combOutNetsOrdered []int
	var combOutNamesOrdered []string
	addedOutNets := map[int]bool{}

	for ri, r := range regs {
		rm := regModels[ri]
		if rm == nil {
			continue
		}
		dataIdx := 0
		for _, pn := range rm.Inputs {
			var nid int
			var name string
			switch {
			case pn == "C":
				nid = r.clkNet
				name = fmt.Sprintf("PTO_C%d", ri)
			case pn == "R":
				nid = r.rstNet
				name = fmt.Sprintf("O_R%d", ri)
			case pn == "E":
				nid = r.enNet
				name = fmt.Sprintf("O_E%d", ri)
			case pn == "S":
				// SR latch S-input — driven from COMB like a data input.
				if len(r.dNets) > 0 {
					nid = r.dNets[0]
				}
				name = fmt.Sprintf("O%d_S", ri)
			default: // D0, D1, ...
				if dataIdx < len(r.dNets) {
					nid = r.dNets[dataIdx]
					name = fmt.Sprintf("O%d_%d", ri, dataIdx)
					dataIdx++
				}
			}
			if nid >= 0 && !addedOutNets[nid] {
				addedOutNets[nid] = true
				combOutNetsOrdered = append(combOutNetsOrdered, nid)
				combOutNamesOrdered = append(combOutNamesOrdered, name)
			}
		}
	}
	// Append output port nets not yet added
	for _, nid := range cloudOutputNets {
		if !addedOutNets[nid] {
			addedOutNets[nid] = true
			combOutNetsOrdered = append(combOutNetsOrdered, nid)
			combOutNamesOrdered = append(combOutNamesOrdered, fmt.Sprintf("O_P%d", len(combOutNamesOrdered)))
		}
	}
	cloudOutputNets = combOutNetsOrdered

	nDataOut := len(cloudOutputNets)
	needComb := (nDataIn > 0 && nDataOut > 0) || nPT > 0
	if needComb {
		var combInNames []string
		for i := range cloudDataInputNets {
			combInNames = append(combInNames, fmt.Sprintf("I%d", i))
		}
		for i := range passthroughNets {
			combInNames = append(combInNames, fmt.Sprintf("PT%d", i))
		}
		combOutNames := combOutNamesOrdered

		// Capture variable suffix for pass-through (a, b, c, ...)
		ptCapture := captureVars(nPT)

		if nDataIn > 0 && nDataOut > 0 && nDataIn <= MaxTruthTableInputs {
			var rules []string
			for combo := 0; combo < (1 << nDataIn); combo++ {
				netVals := map[int]int{}
				inPat := ""
				for bit := 0; bit < nDataIn; bit++ {
					val := (combo >> bit) & 1
					netVals[cloudDataInputNets[bit]] = val
					if val == 1 {
						inPat += "1"
					} else {
						inPat += "0"
					}
				}
				for _, name := range topoOrder {
					c := combByName[name]
					pv := map[string]int{}
					for port, dir := range c.cell.PortDirections {
						if dir == "input" {
							if bits := c.cell.Connections[port]; len(bits) > 0 {
								if id, ok := BitToNetID(bits[0]); ok {
									pv[port] = netVals[id]
								} else if s, ok := bits[0].(string); ok && s == "1" {
									pv[port] = 1
								}
							}
						}
					}
					outVal := evalCombCell(c.cell.Type, pv)
					for port, dir := range c.cell.PortDirections {
						if dir == "output" {
							for _, b := range c.cell.Connections[port] {
								if id, ok := BitToNetID(b); ok {
									netVals[id] = outVal
								}
							}
						}
					}
				}
				// Build output pattern matching cloudOutputNets order:
				// pass-through positions get capture vars, all other positions
				// (data + comb-driven control) get computed 0/1.
				outPat := ""
				ptIdx := 0
				for _, nid := range cloudOutputNets {
					if passthroughSet[nid] {
						outPat += string(rune('a' + ptIdx))
						ptIdx++
					} else {
						if netVals[nid] == 1 {
							outPat += "1"
						} else {
							outPat += "0"
						}
					}
				}
				rules = append(rules, inPat+ptCapture+"="+outPat)
			}
			combModel = &CustomModel{Name: "COMB", Inputs: combInNames, Outputs: combOutNames, Rules: rules}
		} else if nDataIn > 0 && nDataOut > 0 {
			// Too many data inputs — visual block, pass-through only for control.
			fmt.Fprintf(os.Stderr,
				"warning: COMB has %d data inputs (>%d) — truth table omitted, data outputs left as '?'.\n",
				nDataIn, MaxTruthTableInputs)
			outTemplate := ""
			ptIdx := 0
			for _, nid := range cloudOutputNets {
				if passthroughSet[nid] {
					outTemplate += string(rune('a' + ptIdx))
					ptIdx++
				} else {
					outTemplate += "?"
				}
			}
			var rules []string
			if nPT > 0 {
				rules = []string{strings.Repeat("?", nDataIn) + ptCapture + "=" + outTemplate}
			}
			combModel = &CustomModel{Name: "COMB", Inputs: combInNames, Outputs: combOutNames, Rules: rules}
		} else {
			// No data logic, only pass-through.
			outTemplate := ""
			ptIdx := 0
			for _, nid := range cloudOutputNets {
				if passthroughSet[nid] {
					outTemplate += string(rune('a' + ptIdx))
					ptIdx++
				} else {
					outTemplate += "?"
				}
			}
			rules := []string{ptCapture + "=" + outTemplate}
			combModel = &CustomModel{Name: "COMB", Inputs: combInNames, Outputs: combOutNames, Rules: rules}
		}
	}

	// Allocate new net IDs for COMB outputs to avoid driver conflicts.
	// A net can appear as both COMB input (from port/Q) and COMB output (to D/port),
	// which would register two drivers for the same net in Convert(). Fix: COMB outputs
	// use fresh net IDs; register D inputs and output ports are remapped accordingly.
	maxNetID := 0
	for _, port := range mod.Ports {
		for _, b := range port.Bits {
			if id, ok := BitToNetID(b); ok && id > maxNetID {
				maxNetID = id
			}
		}
	}
	for _, cell := range mod.Cells {
		for _, bits := range cell.Connections {
			for _, b := range bits {
				if id, ok := BitToNetID(b); ok && id > maxNetID {
					maxNetID = id
				}
			}
		}
	}

	combOutRemap := map[int]int{} // old cloudOutputNet → new net ID
	for _, oldNet := range cloudOutputNets {
		maxNetID++
		combOutRemap[oldNet] = maxNetID
	}

	// Build synthetic Module with remapped ports
	synMod := &Module{
		Ports:    map[string]*Port{},
		Cells:    map[string]*Cell{},
		NetNames: mod.NetNames,
	}

	// Copy input ports unchanged, remap output ports
	for name, port := range mod.Ports {
		if port.Direction == "output" {
			newBits := make([]interface{}, len(port.Bits))
			for i, b := range port.Bits {
				if id, ok := BitToNetID(b); ok {
					if newID, exists := combOutRemap[id]; exists {
						newBits[i] = float64(newID)
					} else {
						newBits[i] = b
					}
				} else {
					newBits[i] = b
				}
			}
			synMod.Ports[name] = &Port{Direction: port.Direction, Bits: newBits}
		} else {
			synMod.Ports[name] = port
		}
	}

	// Allocate new net IDs for pass-through outputs too
	ptOutRemap := map[int]int{} // control net → new net after pass-through COMB
	for _, nid := range passthroughNets {
		maxNetID++
		ptOutRemap[nid] = maxNetID
	}

	// Add comb block as a single cell (non-sequential name → placed in comb columns)
	if combModel != nil {
		PredefinedModels[combModel.Name] = combModel
		cellMapping["$_RTL_COMB_"] = cellMap{FalstadCustom, combModel.Name, combModel.Inputs, combModel.Outputs}

		combCell := &Cell{
			Type:           "$_RTL_COMB_",
			PortDirections: map[string]string{},
			Connections:    map[string][]interface{}{},
		}
		// Inputs: data + pass-through
		for i, nid := range cloudDataInputNets {
			pn := fmt.Sprintf("I%d", i)
			combCell.PortDirections[pn] = "input"
			combCell.Connections[pn] = []interface{}{float64(nid)}
		}
		for i, nid := range passthroughNets {
			pn := fmt.Sprintf("PT%d", i)
			combCell.PortDirections[pn] = "input"
			combCell.Connections[pn] = []interface{}{float64(nid)}
		}
		// Outputs: ordered to match REG inputs (data + control interleaved)
		for i, nid := range cloudOutputNets {
			pn := combOutNamesOrdered[i]
			combCell.PortDirections[pn] = "output"
			// Use remap: pass-through nets use ptOutRemap, others use combOutRemap
			if newID, ok := ptOutRemap[nid]; ok {
				combCell.Connections[pn] = []interface{}{float64(newID)}
			} else if newID, ok := combOutRemap[nid]; ok {
				combCell.Connections[pn] = []interface{}{float64(newID)}
			} else {
				combCell.Connections[pn] = []interface{}{float64(nid)}
			}
		}

		synMod.Cells["rtl_comb"] = combCell
	}

	// Add register cells (name contains "DFF" → IsSequentialCell returns true → FF column)
	for ri, r := range regs {
		m := regModels[ri]
		if m == nil {
			continue
		}

		PredefinedModels[m.Name] = m
		regType := fmt.Sprintf("$_RTL_DFF_%d_", ri)
		cellMapping[regType] = cellMap{FalstadCustom, m.Name, m.Inputs, m.Outputs}

		regCell := &Cell{
			Type:           regType,
			PortDirections: map[string]string{},
			Connections:    map[string][]interface{}{},
		}

		// Helper: resolve control net — pass-through remap OR comb output remap
		resolveCtrl := func(nid int) float64 {
			if newID, ok := ptOutRemap[nid]; ok {
				return float64(newID)
			}
			if newID, ok := combOutRemap[nid]; ok {
				return float64(newID)
			}
			return float64(nid)
		}

		for _, pn := range m.Inputs {
			regCell.PortDirections[pn] = "input"
			switch {
			case pn == "C":
				regCell.Connections[pn] = []interface{}{resolveCtrl(r.clkNet)}
			case pn == "R":
				regCell.Connections[pn] = []interface{}{resolveCtrl(r.rstNet)}
			case pn == "E":
				regCell.Connections[pn] = []interface{}{resolveCtrl(r.enNet)}
			case pn == "S":
				// SR latch S-input — sourced from COMB output (data path).
				sn := r.dNets[0]
				if newID, ok := combOutRemap[sn]; ok {
					regCell.Connections[pn] = []interface{}{float64(newID)}
				} else {
					regCell.Connections[pn] = []interface{}{float64(sn)}
				}
			default: // D0, D1, ...
				var dIdx int
				fmt.Sscanf(pn, "D%d", &dIdx)
				dn := r.dNets[dIdx]
				if newID, ok := combOutRemap[dn]; ok {
					regCell.Connections[pn] = []interface{}{float64(newID)}
				} else {
					regCell.Connections[pn] = []interface{}{float64(dn)}
				}
			}
		}
		for _, pn := range m.Outputs {
			regCell.PortDirections[pn] = "output"
			var qIdx int
			fmt.Sscanf(pn, "Q%d", &qIdx)
			regCell.Connections[pn] = []interface{}{float64(r.qNets[qIdx])}
		}

		synMod.Cells[fmt.Sprintf("rtl_reg_%d", ri)] = regCell
	}

	// Use the existing layout engine
	return Convert(synMod)
}
