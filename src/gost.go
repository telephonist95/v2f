package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type gostDiagram struct {
	Title  string         `json:"title"`
	Nodes  []gostNode     `json:"nodes"`
	Wires  []gostWire     `json:"wires"`
	Bounds gostBounds     `json:"bounds"`
	Stats  map[string]int `json:"stats"`
}

type gostBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type gostNode struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Kind     string     `json:"kind"`
	Type     string     `json:"type"`
	Function string     `json:"function"`
	Inverted bool       `json:"inverted"`
	X        int        `json:"x"`
	Y        int        `json:"y"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	Ports    []gostPort `json:"ports"`
}

type gostPort struct {
	Name      string `json:"name"`
	NetID     int    `json:"netId"`
	Direction string `json:"direction"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	// ГОСТ 2.743-91 табл. 3 — указатели на выводах УГО.
	// Dynamic=true рисует треугольник внутри УГО (динамический вход — фронт).
	// Inverted=true рисует кружок снаружи УГО (инверсный вход — active-low).
	// Если оба true — динамический вход по СПАДУ (negedge clock).
	Dynamic  bool `json:"dynamic,omitempty"`
	Inverted bool `json:"inverted,omitempty"`
}

type gostWire struct {
	ID       string        `json:"id"`
	Label    string        `json:"label"`
	Width    int           `json:"width"`
	Bus      bool          `json:"bus"`
	NetIDs   []int         `json:"netIds"`
	Segments []gostSegment `json:"segments"`
	Taps     []gostBusTap  `json:"taps,omitempty"`
}

type gostSegment struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

type gostBusTap struct {
	Label    string        `json:"label"`
	NetID    int           `json:"netId"`
	Segments []gostSegment `json:"segments"`
	LabelX   int           `json:"labelX"`
	LabelY   int           `json:"labelY"`
}

const (
	gostScaleX = 0.72
	gostScaleY = 0.52
)

// BuildGostDiagram converts a placed and routed circuit into a GOST-oriented
// drawing model. The wire geometry is taken from the same orthogonal router
// that emits Falstad wires, so the web renderer does not invent extra curved
// routes and crossings on the client side.
//
// Bus rendering is disabled by design: every net becomes an independent
// single-bit wire (Bus=false, NetIDs=[id]), and every component — including
// multi-bit input/output terminals — is rendered as its own GOST block.
func BuildGostDiagram(cir *Circuit, title string) gostDiagram {
	segmentsByNet := map[int][]gostSegment{}
	for _, ws := range cir.WireSegments {
		seg := gostSegment{X1: ws.From.X, Y1: ws.From.Y, X2: ws.To.X, Y2: ws.To.Y}
		segmentsByNet[ws.NetID] = append(segmentsByNet[ws.NetID], seg)
	}

	var nodes []gostNode
	for _, comp := range cir.Components {
		nodes = append(nodes, buildGostNode(comp, fmt.Sprintf("n%d", len(nodes))))
	}

	netLabels := collectNetLabels(nodes)

	var netIDs []int
	for id := range segmentsByNet {
		netIDs = append(netIDs, id)
	}
	sort.Ints(netIDs)

	var wires []gostWire
	for _, id := range netIDs {
		label := netLabels[id]
		if label == "" {
			label = fmt.Sprintf("N%d", id)
		}
		wires = append(wires, gostWire{
			ID:       fmt.Sprintf("w%d", len(wires)),
			Label:    label,
			Width:    1,
			Bus:      false,
			NetIDs:   []int{id},
			Segments: cloneSegments(segmentsByNet[id]),
		})
	}

	scaleGostDiagram(nodes, wires)
	bounds := computeGostBounds(nodes, wires)
	return gostDiagram{
		Title:  title,
		Nodes:  nodes,
		Wires:  wires,
		Bounds: bounds,
		Stats: map[string]int{
			"nodes": len(nodes),
			"edges": len(wires),
			"wires": len(wires),
			"nets":  len(segmentsByNet),
			"buses": countGostBuses(wires),
		},
	}
}

func scaleGostDiagram(nodes []gostNode, wires []gostWire) {
	scaleX := func(v int) int { return scaleInt(v, gostScaleX) }
	scaleY := func(v int) int { return scaleInt(v, gostScaleY) }

	for i := range nodes {
		nodes[i].X = scaleX(nodes[i].X)
		nodes[i].Y = scaleY(nodes[i].Y)
		nodes[i].Width = scaleX(nodes[i].Width)
		nodes[i].Height = scaleY(nodes[i].Height)
		for j := range nodes[i].Ports {
			nodes[i].Ports[j].X = scaleX(nodes[i].Ports[j].X)
			nodes[i].Ports[j].Y = scaleY(nodes[i].Ports[j].Y)
		}
	}
	for i := range wires {
		for j := range wires[i].Segments {
			scaleSegment(&wires[i].Segments[j], scaleX, scaleY)
		}
		for j := range wires[i].Taps {
			wires[i].Taps[j].LabelX = scaleX(wires[i].Taps[j].LabelX)
			wires[i].Taps[j].LabelY = scaleY(wires[i].Taps[j].LabelY)
			for k := range wires[i].Taps[j].Segments {
				scaleSegment(&wires[i].Taps[j].Segments[k], scaleX, scaleY)
			}
		}
	}
}

func scaleSegment(seg *gostSegment, scaleX, scaleY func(int) int) {
	seg.X1 = scaleX(seg.X1)
	seg.Y1 = scaleY(seg.Y1)
	seg.X2 = scaleX(seg.X2)
	seg.Y2 = scaleY(seg.Y2)
}

func scaleInt(v int, factor float64) int {
	if v < 0 {
		return int(float64(v)*factor - 0.5)
	}
	return int(float64(v)*factor + 0.5)
}

// parseFlipFlopParams extracts the base name and parameter string from a
// flip-flop / latch / SR cell model name. Returns ("", "") if the model is
// not a recognised sequential element.
//
// Examples:
//
//	"DFF_P"            → ("DFF",   "P")
//	"DFF_PN0"          → ("DFF",   "PN0")
//	"DFFE_PN0P"        → ("DFFE",  "PN0P")
//	"SDFFE_PN0P"       → ("SDFFE", "PN0P")
//	"DLATCH_P"         → ("DLATCH","P")
//	"SR_PP"            → ("SR",    "PP")
//	"REG_SDFFE_PN0P_4_0" → ("SDFFE","PN0P")   (synthetic RTL register — strip prefix/suffix)
var rtlRegisterSuffixRE = regexp.MustCompile(`_\d+_\d+$`)

func parseFlipFlopParams(model string) (base, params string) {
	if model == "" {
		return "", ""
	}
	name := strings.ToUpper(model)
	// Strip RTL synthetic register prefix and trailing _<width>_<idx>.
	name = strings.TrimPrefix(name, "REG_")
	name = rtlRegisterSuffixRE.ReplaceAllString(name, "")
	// Now expect "<BASE>_<PARAMS>" where BASE is one of DFF, DFFE, SDFF,
	// SDFFE, SDFFCE, DLATCH, SR.
	idx := strings.LastIndex(name, "_")
	if idx < 0 {
		return "", ""
	}
	b := name[:idx]
	p := name[idx+1:]
	switch b {
	case "DFF", "DFFE", "SDFF", "SDFFE", "SDFFCE", "DLATCH", "SR":
		return b, p
	}
	return "", ""
}

// portIndicators returns the ГОСТ 2.743-91 table 3 indicator flags for a pin
// of a sequential element.
//
//	Dynamic=true  → треугольник внутри УГО (динамический вход)
//	Inverted=true → кружок снаружи УГО (инверсный статический вход)
//	Both true     → инверсный динамический (вход по СПАДУ тактового)
//
// Parameter encoding (Yosys):
//
//	DFF_<C>_:      C edge (P=posedge, N=negedge)
//	DFF_<C><R><V>_:    C edge, R reset polarity (P=high-active, N=low-active), V reset value
//	DFFE_<C><E>_:      C edge, E enable polarity
//	DFFE_<C><R><V><E>_:full
//	SDFF/SDFFE/SDFFCE: same scheme but with synchronous reset
//	DLATCH_<E>_ / _<E><R><V>_: E is the level-sensitive enable (not edge)
//	SR_<S><R>_:        S and R polarity
func portIndicators(base, params, pin string, isOutput bool) (dyn, inv bool) {
	if isOutput || base == "" {
		return false, false
	}
	pol := func(i int) byte {
		if i < 0 || i >= len(params) {
			return 'P'
		}
		return params[i]
	}
	switch base {
	case "DFF":
		if pin == "C" {
			dyn = true
			inv = pol(0) == 'N'
		} else if pin == "R" && len(params) >= 3 {
			inv = pol(1) == 'N'
		}
	case "DFFE":
		if pin == "C" {
			dyn = true
			inv = pol(0) == 'N'
		} else if pin == "E" {
			// 2-char params: <C><E>; 4-char params: <C><R><V><E>
			if len(params) == 2 {
				inv = pol(1) == 'N'
			} else if len(params) == 4 {
				inv = pol(3) == 'N'
			}
		} else if pin == "R" && len(params) >= 4 {
			inv = pol(1) == 'N'
		}
	case "SDFF":
		if pin == "C" {
			dyn = true
			inv = pol(0) == 'N'
		} else if pin == "R" {
			inv = pol(1) == 'N'
		}
	case "SDFFE", "SDFFCE":
		if pin == "C" {
			dyn = true
			inv = pol(0) == 'N'
		} else if pin == "R" {
			inv = pol(1) == 'N'
		} else if pin == "E" {
			inv = pol(3) == 'N'
		}
	case "DLATCH":
		if pin == "E" {
			// Level-sensitive enable — no triangle, but polarity may be inverted.
			inv = pol(0) == 'N'
		} else if pin == "R" && len(params) >= 3 {
			inv = pol(1) == 'N'
		}
	case "SR":
		if pin == "S" {
			inv = pol(0) == 'N'
		} else if pin == "R" {
			inv = pol(1) == 'N'
		}
	}
	return
}

// portLabelGOST converts a Yosys port name to the ГОСТ 2.743-91 marker used
// inside the УГО. For combinational gates the pin name is kept unchanged;
// for flip-flop / latch / SR elements the metki follow table 12 (triggers):
//
//	C → C1  (clock that controls 1D)
//	D → 1D  (data dependent on C1)
//	Q → Q   (or its number from the body)
//	R, S, E — kept as-is (they already match table 4)
func portLabelGOST(ffBase, pin string, isOutput bool) string {
	if ffBase == "" {
		return pin
	}
	if isOutput {
		// Output of FF — usually Q. Some Yosys cells use Q0/Q1/… for RTL register groups.
		return pin
	}
	switch pin {
	case "C":
		return "C1"
	case "D":
		return "1D"
	}
	if strings.HasPrefix(pin, "D") && len(pin) > 1 { // D0, D1, … from RTL register
		return "1" + pin // → 1D0, 1D1
	}
	return pin
}

func buildGostNode(comp *PlacedComponent, id string) gostNode {
	x, y, w, h := componentGostBounds(comp)
	node := gostNode{
		ID:       id,
		Label:    componentLabel(comp),
		Kind:     componentKind(comp),
		Type:     comp.YosysType,
		Function: componentGostFunction(comp),
		Inverted: componentHasInvertedOutput(comp),
		X:        x,
		Y:        y,
		Width:    w,
		Height:   h,
	}
	ffBase, ffParams := parseFlipFlopParams(comp.ModelName)
	for _, pin := range comp.Pins {
		dir := "in"
		if pin.IsOutput {
			dir = "out"
		}
		dyn, inv := portIndicators(ffBase, ffParams, pin.PortName, pin.IsOutput)
		name := portLabelGOST(ffBase, pin.PortName, pin.IsOutput)
		node.Ports = append(node.Ports, gostPort{
			Name:      name,
			NetID:     pin.NetID,
			Direction: dir,
			X:         pin.Pos.X,
			Y:         pin.Pos.Y,
			Dynamic:   dyn,
			Inverted:  inv,
		})
	}
	sort.SliceStable(node.Ports, func(i, j int) bool {
		if node.Ports[i].Y != node.Ports[j].Y {
			return node.Ports[i].Y < node.Ports[j].Y
		}
		return node.Ports[i].Name < node.Ports[j].Name
	})
	return node
}

func componentGostBounds(comp *PlacedComponent) (int, int, int, int) {
	if len(comp.Pins) == 1 {
		p := comp.Pins[0].Pos
		if comp.FalstadType == FalstadLogicIn || comp.FalstadType == FalstadRail {
			return p.X - 56, p.Y - 14, 56, 28
		}
		if comp.FalstadType == FalstadLogicOut {
			return p.X, p.Y - 14, 56, 28
		}
	}

	minX, maxX := comp.Pos1.X, comp.Pos2.X
	minY, maxY := comp.Pos1.Y, comp.Pos2.Y
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	for _, p := range comp.Pins {
		if p.Pos.X < minX {
			minX = p.Pos.X
		}
		if p.Pos.X > maxX {
			maxX = p.Pos.X
		}
		if p.Pos.Y < minY {
			minY = p.Pos.Y
		}
		if p.Pos.Y > maxY {
			maxY = p.Pos.Y
		}
	}

	w := maxX - minX
	if w < 64 {
		w = 64
	}
	topPad := 30
	bottomPad := 10
	h := maxY - minY + topPad + bottomPad
	if h < 50 {
		h = 50
	}
	return minX, minY - topPad, w, h
}

func componentLabel(comp *PlacedComponent) string {
	switch comp.FalstadType {
	case FalstadLogicIn, FalstadRail, FalstadLogicOut:
		return comp.Name
	case FalstadCustom:
		if comp.ModelName != "" {
			if strings.HasPrefix(comp.ModelName, "REG_") {
				if width := modelWidth(comp.ModelName); width > 1 {
					return fmt.Sprintf("REG[%d:0]", width-1)
				}
				return "REG"
			}
			return comp.ModelName
		}
	}
	if comp.YosysType != "" {
		return strings.Trim(comp.YosysType, "$_")
	}
	return comp.Name
}

func modelWidth(name string) int {
	parts := strings.Split(name, "_")
	for i := len(parts) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(parts[i])
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func componentKind(comp *PlacedComponent) string {
	switch comp.FalstadType {
	case FalstadLogicIn, FalstadRail:
		return "input"
	case FalstadLogicOut:
		return "output"
	case FalstadCustom:
		if strings.Contains(comp.ModelName, "REG") || strings.Contains(comp.ModelName, "DFF") {
			return "register"
		}
		if comp.ModelName == "COMB" {
			return "comb"
		}
	}
	if IsSequentialCell(comp.YosysType) {
		return "register"
	}
	return "logic"
}

func componentGostFunction(comp *PlacedComponent) string {
	switch comp.FalstadType {
	case FalstadLogicIn:
		return "IN"
	case FalstadRail:
		return "CLK"
	case FalstadLogicOut:
		return "OUT"
	case FalstadInverter:
		return "1"
	case FalstadAND, FalstadNAND:
		return "&"
	case FalstadOR, FalstadNOR:
		return ">=1"
	case FalstadXOR:
		return "=1"
	case FalstadCustom:
		name := strings.ToUpper(comp.ModelName)
		switch {
		// "RG" (регистр) — только для синтетических групповых регистров RTL.
		// Одиночные триггеры (вентильный уровень) — "T" по ГОСТ 2.743-91.
		case strings.HasPrefix(name, "REG_"):
			return "RG"
		case strings.Contains(name, "DFF") || strings.Contains(name, "LATCH") ||
			strings.Contains(name, "_FF_") || strings.HasPrefix(name, "SR_"):
			return "T"
		case strings.Contains(name, "MUX"):
			return "MUX"
		case name == "XNOR":
			return "=1"
		case strings.Contains(name, "AND"):
			return "&"
		case strings.Contains(name, "OR"):
			return ">=1"
		case name == "BUF":
			return "1"
		case name == "COMB":
			return "F"
		case name != "":
			return name
		}
	}
	return "F"
}

func componentHasInvertedOutput(comp *PlacedComponent) bool {
	switch comp.FalstadType {
	case FalstadInverter, FalstadNAND, FalstadNOR:
		return true
	case FalstadCustom:
		name := strings.ToUpper(comp.ModelName)
		return name == "XNOR" || name == "NMUX" || strings.HasPrefix(name, "AOI") || strings.HasPrefix(name, "OAI")
	}
	return false
}

func collectNetLabels(nodes []gostNode) map[int]string {
	labels := map[int]string{}
	score := map[int]int{}
	for _, node := range nodes {
		for _, port := range node.Ports {
			if port.NetID == 0 || port.Name == "" {
				continue
			}
			nextScore := 1
			if node.Kind == "input" || node.Kind == "output" {
				nextScore = 10
			} else if !isGenericPinBase(pinBase(port.Name)) {
				nextScore = 4
			}
			if nextScore > score[port.NetID] || (nextScore == score[port.NetID] && len(port.Name) > len(labels[port.NetID])) {
				labels[port.NetID] = port.Name
				score[port.NetID] = nextScore
			}
		}
	}
	return labels
}

func cloneSegments(segments []gostSegment) []gostSegment {
	out := append([]gostSegment(nil), segments...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].X1 != out[j].X1 {
			return out[i].X1 < out[j].X1
		}
		if out[i].Y1 != out[j].Y1 {
			return out[i].Y1 < out[j].Y1
		}
		if out[i].X2 != out[j].X2 {
			return out[i].X2 < out[j].X2
		}
		return out[i].Y2 < out[j].Y2
	})
	return out
}

func computeGostBounds(nodes []gostNode, wires []gostWire) gostBounds {
	if len(nodes) == 0 {
		return gostBounds{X: 0, Y: 0, Width: 800, Height: 500}
	}
	minX, minY := nodes[0].X, nodes[0].Y
	maxX, maxY := nodes[0].X+nodes[0].Width, nodes[0].Y+nodes[0].Height
	add := func(x, y int) {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	for _, node := range nodes {
		add(node.X, node.Y)
		add(node.X+node.Width, node.Y+node.Height)
		for _, port := range node.Ports {
			add(port.X, port.Y)
		}
	}
	for _, wire := range wires {
		for _, seg := range wire.Segments {
			add(seg.X1, seg.Y1)
			add(seg.X2, seg.Y2)
		}
	}
	margin := 64
	minX -= margin
	minY -= margin
	maxX += margin
	maxY += margin
	return gostBounds{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

func countGostBuses(wires []gostWire) int {
	n := 0
	for _, wire := range wires {
		if wire.Bus {
			n++
		}
	}
	return n
}

var indexSuffixRE = regexp.MustCompile(`^(.+)\[([0-9]+)]$`)
var digitSuffixRE = regexp.MustCompile(`[0-9]+$`)
var underscoreIndexRE = regexp.MustCompile(`_[0-9]+$`)

func pinBase(name string) string {
	name = indexSuffixRE.ReplaceAllString(name, "$1")
	name = underscoreIndexRE.ReplaceAllString(name, "")
	return digitSuffixRE.ReplaceAllString(name, "")
}

func isGenericPinBase(name string) bool {
	switch name {
	case "A", "B", "C", "D", "E", "I", "O", "PT", "PTO", "S", "T", "Y", "Q":
		return true
	}
	return false
}
