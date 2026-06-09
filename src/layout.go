package main

import (
	"fmt"
	"sort"
	"strings"
)

// Layout grid constants (matching Falstad's internal layout).
const (
	Grid           = 16  // Falstad grid step
	GateLen        = 64  // native gate length (x1 to x2)
	GateHS         = 16  // gate pin half-spacing (gheight at gsize=2)
	ChipPinSpacing = 32  // chip pin spacing (cspc2 = 2*cspc = 2*8*csize = 32 for csize=2)
	ChipExtent     = 96  // chip total width from west post to east post: (sizeX+1)*cspc2 = 3*32
	ColWidth       = 96  // column width (max of GateLen and ChipExtent)
	RowSpacing     = 128 // vertical spacing between component rows
	TrackSpacing   = 12
)

// Point on the Falstad grid.
type Point struct{ X, Y int }

// Pin is a connection point on a placed component.
type Pin struct {
	Pos      Point
	NetID    int
	IsOutput bool
	PortName string
}

// PlacedComponent is a component with assigned coordinates.
type PlacedComponent struct {
	Name, YosysType, FalstadType, ModelName string
	Pos1, Pos2                              Point
	Pins                                    []Pin
	Column, Row                             int
}

// Net tracks a signal: one driver, zero or more sinks.
type Net struct {
	ID     int
	Driver *PinRef
	Sinks  []*PinRef
}

// PinRef references a pin on a placed component.
type PinRef struct {
	Comp     *PlacedComponent
	PinIndex int
}

// WireSegment is one routed orthogonal wire segment with its logical net.
type WireSegment struct {
	NetID    int
	From, To Point
}

// Circuit holds the complete placed/routed circuit.
type Circuit struct {
	Components   []*PlacedComponent
	Wires        []string
	WireSegments []WireSegment
	Models       []*CustomModel
}

// Cell type → Falstad mapping table

type cellMap struct {
	falstadType string
	modelName   string
	inPorts     []string
	outPorts    []string
}

var cellMapping = map[string]cellMap{
	// Native Falstad gates
	"$_NOT_":  {FalstadInverter, "", []string{"A"}, []string{"Y"}},
	"$_AND_":  {FalstadAND, "", []string{"A", "B"}, []string{"Y"}},
	"$_OR_":   {FalstadOR, "", []string{"A", "B"}, []string{"Y"}},
	"$_XOR_":  {FalstadXOR, "", []string{"A", "B"}, []string{"Y"}},
	"$_NAND_": {FalstadNAND, "", []string{"A", "B"}, []string{"Y"}},
	"$_NOR_":  {FalstadNOR, "", []string{"A", "B"}, []string{"Y"}},
	// Custom logic combinational
	"$_BUF_":    {FalstadCustom, "BUF", []string{"A"}, []string{"Y"}},
	"$_XNOR_":   {FalstadCustom, "XNOR", []string{"A", "B"}, []string{"Y"}},
	"$_ANDNOT_": {FalstadCustom, "ANDNOT", []string{"A", "B"}, []string{"Y"}},
	"$_ORNOT_":  {FalstadCustom, "ORNOT", []string{"A", "B"}, []string{"Y"}},
	"$_MUX_":    {FalstadCustom, "MUX", []string{"A", "B", "S"}, []string{"Y"}},
	"$_NMUX_":   {FalstadCustom, "NMUX", []string{"A", "B", "S"}, []string{"Y"}},
	"$_MUX4_":   {FalstadCustom, "MUX4", []string{"A", "B", "C", "D", "S", "T"}, []string{"Y"}},
	"$_AOI3_":   {FalstadCustom, "AOI3", []string{"A", "B", "C"}, []string{"Y"}},
	"$_OAI3_":   {FalstadCustom, "OAI3", []string{"A", "B", "C"}, []string{"Y"}},
	"$_AOI4_":   {FalstadCustom, "AOI4", []string{"A", "B", "C", "D"}, []string{"Y"}},
	"$_OAI4_":   {FalstadCustom, "OAI4", []string{"A", "B", "C", "D"}, []string{"Y"}},
}

// Convert: Yosys module → Falstad circuit

func Convert(mod *Module) (*Circuit, error) {
	cir := &Circuit{}

	// Classify cells
	type cellInfo struct {
		name    string
		cell    *Cell
		isComb  bool
		depth   int
		column  int
		row     int
		outNets []int
		inNets  []int // combinational dependency nets
	}

	var combCells, ffCells []*cellInfo
	for name, cell := range mod.Cells {
		ci := &cellInfo{name: name, cell: cell}
		ci.isComb = !IsSequentialCell(cell.Type)

		for port, dir := range cell.PortDirections {
			for _, b := range cell.Connections[port] {
				id, ok := BitToNetID(b)
				if !ok {
					continue
				}
				if dir == "output" {
					ci.outNets = append(ci.outNets, id)
				}
			}
		}

		if ci.isComb {
			for port, dir := range cell.PortDirections {
				if dir == "input" {
					for _, b := range cell.Connections[port] {
						if id, ok := BitToNetID(b); ok {
							ci.inNets = append(ci.inNets, id)
						}
					}
				}
			}
		} else {
			if bits, ok := cell.Connections["D"]; ok {
				for _, b := range bits {
					if id, ok := BitToNetID(b); ok {
						ci.inNets = append(ci.inNets, id)
					}
				}
			}
		}

		if ci.isComb {
			combCells = append(combCells, ci)
		} else {
			ffCells = append(ffCells, ci)
		}
	}

	// Net driver map
	netDriver := map[int]*cellInfo{}
	for _, ci := range append(combCells, ffCells...) {
		for _, nid := range ci.outNets {
			netDriver[nid] = ci
		}
	}

	// Source nets (input ports + FF outputs)
	sourceNets := map[int]bool{}
	for _, port := range mod.Ports {
		if port.Direction == "input" {
			for _, b := range port.Bits {
				if id, ok := BitToNetID(b); ok {
					sourceNets[id] = true
				}
			}
		}
	}
	for _, ci := range ffCells {
		for _, nid := range ci.outNets {
			sourceNets[nid] = true
		}
	}

	// Topological depth
	depthCache := map[string]int{}
	var computeDepth func(*cellInfo) int
	computeDepth = func(ci *cellInfo) int {
		if d, ok := depthCache[ci.name]; ok {
			return d
		}
		depthCache[ci.name] = 0
		maxD := 0
		for _, nid := range ci.inNets {
			if sourceNets[nid] {
				continue
			}
			if drv, ok := netDriver[nid]; ok && drv.isComb {
				if d := computeDepth(drv) + 1; d > maxD {
					maxD = d
				}
			}
		}
		depthCache[ci.name] = maxD
		ci.depth = maxD
		return maxD
	}
	for _, ci := range combCells {
		computeDepth(ci)
	}

	// Sort cells: by depth then output net ID for visual alignment
	firstOutNet := func(ci *cellInfo) int {
		if len(ci.outNets) > 0 {
			return ci.outNets[0]
		}
		return 9999
	}
	sort.Slice(combCells, func(i, j int) bool {
		if combCells[i].depth != combCells[j].depth {
			return combCells[i].depth < combCells[j].depth
		}
		return firstOutNet(combCells[i]) < firstOutNet(combCells[j])
	})
	sort.Slice(ffCells, func(i, j int) bool {
		return firstOutNet(ffCells[i]) < firstOutNet(ffCells[j])
	})

	// Column / row assignment
	maxCombDepth := 0
	for _, ci := range combCells {
		if ci.depth > maxCombDepth {
			maxCombDepth = ci.depth
		}
	}
	ffCol := maxCombDepth + 2
	outCol := ffCol + 1
	numColumns := outCol + 1

	combByDepth := map[int][]*cellInfo{}
	for _, ci := range combCells {
		ci.column = ci.depth + 1
		combByDepth[ci.depth] = append(combByDepth[ci.depth], ci)
	}
	for _, grp := range combByDepth {
		for i, ci := range grp {
			ci.row = i
		}
	}
	for i, ci := range ffCells {
		ci.column = ffCol
		ci.row = i
	}

	// Input / output ports
	// Expand multi-bit ports into individual bits so each gets its own LogicInput/Output.
	type portBit struct {
		name    string
		netID   int
		isClock bool
	}
	var inputBits, outputBits []portBit

	// Collect and sort port names for deterministic order.
	var inPortNames, outPortNames []string
	for name, port := range mod.Ports {
		if port.Direction == "input" {
			inPortNames = append(inPortNames, name)
		} else {
			outPortNames = append(outPortNames, name)
		}
	}
	sort.Strings(inPortNames)
	sort.Strings(outPortNames)

	for _, name := range inPortNames {
		port := mod.Ports[name]
		isClock := strings.Contains(strings.ToLower(name), "clk") || strings.Contains(strings.ToLower(name), "clock")
		for i, b := range port.Bits {
			if id, ok := BitToNetID(b); ok {
				label := name
				if len(port.Bits) > 1 {
					label = fmt.Sprintf("%s[%d]", name, i)
				}
				inputBits = append(inputBits, portBit{label, id, isClock})
			}
		}
	}
	for _, name := range outPortNames {
		port := mod.Ports[name]
		for i, b := range port.Bits {
			if id, ok := BitToNetID(b); ok {
				label := name
				if len(port.Bits) > 1 {
					label = fmt.Sprintf("%s[%d]", name, i)
				}
				outputBits = append(outputBits, portBit{label, id, false})
			}
		}
	}

	// Pre-count channel tracks for dynamic channel widths
	// For each net, determine driver column and sink columns, then simulate
	// routing decisions to count how many tracks each channel needs.
	type netColInfo struct {
		driverCol int
		sinkCols  []int
	}
	netColMap := map[int]*netColInfo{}
	getNetCol := func(nid int) *netColInfo {
		if nci, ok := netColMap[nid]; ok {
			return nci
		}
		nci := &netColInfo{driverCol: -1}
		netColMap[nid] = nci
		return nci
	}

	// Input port bits drive from column 0.
	for _, ib := range inputBits {
		nci := getNetCol(ib.netID)
		nci.driverCol = 0
	}
	// Cell outputs are drivers, cell inputs are sinks.
	for _, ci := range append(combCells, ffCells...) {
		cm, ok := ResolveCell(ci.cell.Type)
		if !ok {
			continue
		}
		for _, pn := range cm.outPorts {
			for _, b := range ci.cell.Connections[pn] {
				if nid, ok := BitToNetID(b); ok {
					nci := getNetCol(nid)
					nci.driverCol = ci.column
				}
			}
		}
		for _, pn := range cm.inPorts {
			for _, b := range ci.cell.Connections[pn] {
				if nid, ok := BitToNetID(b); ok {
					nci := getNetCol(nid)
					nci.sinkCols = append(nci.sinkCols, ci.column)
				}
			}
		}
	}
	// Output port bits are sinks at outCol.
	for _, ob := range outputBits {
		nci := getNetCol(ob.netID)
		nci.sinkCols = append(nci.sinkCols, outCol)
	}

	// Count tracks per channel.
	chanTrackCount := make([]int, numColumns)
	busNets := 0
	clampCh := func(ch int) int {
		if ch < 0 {
			return 0
		}
		if ch >= numColumns-1 {
			return numColumns - 2
		}
		return ch
	}
	for _, nci := range netColMap {
		if nci.driverCol < 0 || len(nci.sinkCols) == 0 {
			continue
		}
		allAdjacent := true
		for _, sc := range nci.sinkCols {
			if sc != nci.driverCol+1 {
				allAdjacent = false
				break
			}
		}
		if allAdjacent && nci.driverCol+1 < numColumns {
			chanTrackCount[nci.driverCol]++
		} else {
			busNets++
			srcCh := clampCh(nci.driverCol)
			chanTrackCount[srcCh]++
			seenCh := map[int]bool{}
			for _, sc := range nci.sinkCols {
				ch := clampCh(sc - 1)
				if !seenCh[ch] {
					seenCh[ch] = true
					chanTrackCount[ch]++
				}
			}
		}
	}

	// Compute column X positions with dynamic channel widths.
	// Each channel needs space for adjacent tracks (left) + gap + bus drop tracks (right).
	colInX := make([]int, numColumns)
	colOutX := make([]int, numColumns)
	colInX[0] = ColWidth
	colOutX[0] = ColWidth
	for c := 1; c < numColumns; c++ {
		ch := c - 1
		tracks := chanTrackCount[ch]
		// Double the track count + gap to separate adjacent from bus drop tracks
		chanWidth := (tracks+4)*TrackSpacing + 2*TrackSpacing // extra 2 tracks as separator
		if chanWidth < 5*TrackSpacing {
			chanWidth = 5 * TrackSpacing
		}
		colInX[c] = colOutX[c-1] + chanWidth
		colOutX[c] = colInX[c] + ColWidth
	}

	// Compute per-column max component height for dynamic spacing.
	colMaxPins := make([]int, numColumns)
	globalMaxPins := 0
	for _, ci := range combCells {
		cm, ok := ResolveCell(ci.cell.Type)
		if !ok {
			continue
		}
		n := len(cm.inPorts)
		if len(cm.outPorts) > n {
			n = len(cm.outPorts)
		}
		if n > colMaxPins[ci.column] {
			colMaxPins[ci.column] = n
		}
		if n > globalMaxPins {
			globalMaxPins = n
		}
	}
	for _, ci := range ffCells {
		cm, ok := ResolveCell(ci.cell.Type)
		if !ok {
			continue
		}
		n := len(cm.inPorts)
		if len(cm.outPorts) > n {
			n = len(cm.outPorts)
		}
		if n > colMaxPins[ci.column] {
			colMaxPins[ci.column] = n
		}
		if n > globalMaxPins {
			globalMaxPins = n
		}
	}

	// Per-column row spacing: based on tallest component in that column
	colRowSpacing := make([]int, numColumns)
	for c := 0; c < numColumns; c++ {
		h := colMaxPins[c]*ChipPinSpacing + 32
		if h < RowSpacing {
			h = RowSpacing
		}
		colRowSpacing[c] = h
	}

	// Bus gap: based on globally tallest component half-height
	maxHalfHeight := 48
	if h := (globalMaxPins - 1) * ChipPinSpacing / 2; h > maxHalfHeight {
		maxHalfHeight = h
	}

	// Bus tracks above components.
	busSlots := busNets
	if busSlots < 1 {
		busSlots = 1
	}
	busGap := maxHalfHeight + 48
	baseY := busSlots*TrackSpacing + busGap + Grid

	nets := map[int]*Net{}
	getNet := func(id int) *Net {
		if n, ok := nets[id]; ok {
			return n
		}
		n := &Net{ID: id}
		nets[id] = n
		return n
	}

	rowY := func(col, r int) int { return baseY + r*colRowSpacing[col] }

	registerPin := func(comp *PlacedComponent, pi int, nid int, isOut bool) {
		n := getNet(nid)
		ref := &PinRef{comp, pi}
		if isOut {
			n.Driver = ref
		} else {
			n.Sinks = append(n.Sinks, ref)
		}
	}

	// Place input ports (one component per bit)
	for i, ib := range inputBits {
		y := rowY(0, i)
		pinX := colOutX[0]
		ftype := FalstadLogicIn
		if ib.isClock {
			ftype = FalstadRail
		}
		comp := &PlacedComponent{
			Name:        ib.name,
			YosysType:   "input",
			FalstadType: ftype,
			Pos1:        Point{pinX, y},
			Pos2:        Point{pinX - GateLen, y},
			Column:      0, Row: i,
		}
		comp.Pins = append(comp.Pins, Pin{Point{pinX, y}, ib.netID, true, ib.name})
		registerPin(comp, 0, ib.netID, true)
		cir.Components = append(cir.Components, comp)
	}

	// Place combinational cells
	for _, ci := range combCells {
		cm, ok := ResolveCell(ci.cell.Type)
		if !ok {
			return nil, fmt.Errorf("unsupported cell type: %s", ci.cell.Type)
		}
		cy := rowY(ci.column, ci.row) // center Y of this row
		x1 := colInX[ci.column]
		nIn := len(cm.inPorts)
		nOut := len(cm.outPorts)

		comp := &PlacedComponent{
			Name: ci.name, YosysType: ci.cell.Type,
			FalstadType: cm.falstadType, ModelName: cm.modelName,
			Column: ci.column, Row: ci.row,
		}

		if cm.falstadType == FalstadCustom {
			// ChipElm: pins start at y_top, go down by ChipPinSpacing.
			// Output at x1 + ChipExtent.
			sizeY := nIn
			if nOut > sizeY {
				sizeY = nOut
			}
			yTop := cy - (sizeY-1)*ChipPinSpacing/2
			comp.Pos1 = Point{x1, yTop}
			comp.Pos2 = Point{x1 + ChipExtent, yTop}
			for i, pn := range cm.inPorts {
				pp := Point{x1, yTop + ChipPinSpacing*i}
				nid := firstNet(ci.cell.Connections[pn])
				comp.Pins = append(comp.Pins, Pin{pp, nid, false, pn})
				if nid != 0 {
					registerPin(comp, len(comp.Pins)-1, nid, false)
				}
			}
			for i, pn := range cm.outPorts {
				pp := Point{x1 + ChipExtent, yTop + ChipPinSpacing*i}
				nid := firstNet(ci.cell.Connections[pn])
				comp.Pins = append(comp.Pins, Pin{pp, nid, true, pn})
				if nid != 0 {
					registerPin(comp, len(comp.Pins)-1, nid, true)
				}
			}
		} else {
			// GateElm / InverterElm: input at x1, output at x1+GateLen.
			// Gate pins: pin 0 at y+GateHS (below center), pin 1 at y-GateHS (above).
			x2 := x1 + GateLen
			comp.Pos1 = Point{x1, cy}
			comp.Pos2 = Point{x2, cy}
			for i, pn := range cm.inPorts {
				var pp Point
				if cm.falstadType == FalstadInverter {
					pp = Point{x1, cy}
				} else {
					// Falstad: offset = -(idx - (nIn-1)/2) * GateHS*2
					off := -(float64(i) - float64(nIn-1)/2.0) * float64(GateHS*2)
					pp = Point{x1, cy + int(off)}
				}
				nid := firstNet(ci.cell.Connections[pn])
				comp.Pins = append(comp.Pins, Pin{pp, nid, false, pn})
				if nid != 0 {
					registerPin(comp, len(comp.Pins)-1, nid, false)
				}
			}
			for _, pn := range cm.outPorts {
				nid := firstNet(ci.cell.Connections[pn])
				comp.Pins = append(comp.Pins, Pin{Point{x2, cy}, nid, true, pn})
				if nid != 0 {
					registerPin(comp, len(comp.Pins)-1, nid, true)
				}
			}
		}
		cir.Components = append(cir.Components, comp)
	}

	// Place flip-flops (ChipElm)
	for _, ci := range ffCells {
		cm, ok := ResolveCell(ci.cell.Type)
		if !ok {
			return nil, fmt.Errorf("unsupported FF type: %s", ci.cell.Type)
		}
		cy := rowY(ci.column, ci.row)
		x1 := colInX[ci.column]
		nIn := len(cm.inPorts)
		nOut := len(cm.outPorts)
		sizeY := nIn
		if nOut > sizeY {
			sizeY = nOut
		}
		yTop := cy - (sizeY-1)*ChipPinSpacing/2
		comp := &PlacedComponent{
			Name: ci.name, YosysType: ci.cell.Type,
			FalstadType: cm.falstadType, ModelName: cm.modelName,
			Pos1: Point{x1, yTop}, Pos2: Point{x1 + ChipExtent, yTop},
			Column: ci.column, Row: ci.row,
		}
		for i, pn := range cm.inPorts {
			pp := Point{x1, yTop + ChipPinSpacing*i}
			nid := firstNet(ci.cell.Connections[pn])
			comp.Pins = append(comp.Pins, Pin{pp, nid, false, pn})
			if nid != 0 {
				registerPin(comp, len(comp.Pins)-1, nid, false)
			}
		}
		for i, pn := range cm.outPorts {
			pp := Point{x1 + ChipExtent, yTop + ChipPinSpacing*i}
			nid := firstNet(ci.cell.Connections[pn])
			comp.Pins = append(comp.Pins, Pin{pp, nid, true, pn})
			if nid != 0 {
				registerPin(comp, len(comp.Pins)-1, nid, true)
			}
		}
		cir.Components = append(cir.Components, comp)
	}

	// Place output ports
	for i, ob := range outputBits {
		y := rowY(outCol, i)
		pinX := colInX[outCol]
		comp := &PlacedComponent{
			Name: ob.name, YosysType: "output", FalstadType: FalstadLogicOut,
			Pos1: Point{pinX, y}, Pos2: Point{pinX + GateLen, y},
			Column: outCol, Row: i,
		}
		comp.Pins = append(comp.Pins, Pin{Point{pinX, y}, ob.netID, false, ob.name})
		registerPin(comp, 0, ob.netID, false)
		cir.Components = append(cir.Components, comp)
	}

	// Collect custom logic models
	usedModels := map[string]bool{}
	for _, comp := range cir.Components {
		if comp.FalstadType == FalstadCustom && comp.ModelName != "" {
			usedModels[comp.ModelName] = true
		}
	}
	var modelNames []string
	for name := range usedModels {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)
	for _, name := range modelNames {
		if m, ok := PredefinedModels[name]; ok {
			cir.Models = append(cir.Models, m)
		}
	}

	// Wire routing
	type trackAllocator struct {
		adjTracks map[int]int // channel idx → next offset (from LEFT of channel)
		busTracks map[int]int // channel idx → next offset (from RIGHT of channel)
		busTrackY int         // current bus Y (decreasing)
	}
	ta := &trackAllocator{
		adjTracks: map[int]int{},
		busTracks: map[int]int{},
		busTrackY: baseY - busGap,
	}
	// Adjacent tracks: allocated from LEFT side of channel (near source column output)
	allocChanTrack := func(ch int) int {
		off := ta.adjTracks[ch]
		ta.adjTracks[ch] = off + TrackSpacing
		return colOutX[ch] + TrackSpacing + off
	}
	// Bus drop tracks: allocated from RIGHT side of channel (near sink column input)
	allocBusDropTrack := func(ch int) int {
		off := ta.busTracks[ch]
		ta.busTracks[ch] = off + TrackSpacing
		return colInX[ch+1] - TrackSpacing - off
	}
	allocBusY := func() int {
		y := ta.busTrackY
		ta.busTrackY -= TrackSpacing
		return y
	}

	addWire := func(nid, x1, y1, x2, y2 int) {
		if x1 == x2 && y1 == y2 {
			return // zero-length
		}
		cir.Wires = append(cir.Wires, DumpWire(x1, y1, x2, y2))
		cir.WireSegments = append(cir.WireSegments, WireSegment{
			NetID: nid,
			From:  Point{x1, y1},
			To:    Point{x2, y2},
		})
	}

	// Sort net IDs for deterministic output.
	var netIDs []int
	for id := range nets {
		netIDs = append(netIDs, id)
	}
	sort.Ints(netIDs)

	// For each net, decide routing strategy.
	for _, nid := range netIDs {
		net := nets[nid]
		if net.Driver == nil || len(net.Sinks) == 0 {
			continue
		}
		srcPin := net.Driver.Comp.Pins[net.Driver.PinIndex]
		srcCol := net.Driver.Comp.Column

		// Collect sink info.
		type sinkInfo struct {
			pos Point
			col int
		}
		var sinks []sinkInfo
		for _, s := range net.Sinks {
			sp := s.Comp.Pins[s.PinIndex]
			sinks = append(sinks, sinkInfo{sp.Pos, s.Comp.Column})
		}

		// Determine if ALL sinks are in exactly srcCol+1.
		allAdjacent := true
		for _, s := range sinks {
			if s.col != srcCol+1 {
				allAdjacent = false
				break
			}
		}

		if allAdjacent && srcCol+1 < numColumns {
			// Adjacent channel routing
			ch := srcCol
			trackX := allocChanTrack(ch)
			src := srcPin.Pos

			// Source → channel track (horizontal).
			addWire(nid, src.X, src.Y, trackX, src.Y)

			// Collect all Y-junctions: source Y + all sink Ys.
			ys := []int{src.Y}
			for _, s := range sinks {
				ys = append(ys, s.pos.Y)
			}
			sort.Ints(ys)
			ys = uniqueInts(ys)

			// Vertical trunk segments.
			for i := 0; i < len(ys)-1; i++ {
				addWire(nid, trackX, ys[i], trackX, ys[i+1])
			}
			// Horizontal branches to sinks.
			for _, s := range sinks {
				addWire(nid, trackX, s.pos.Y, s.pos.X, s.pos.Y)
			}
		} else {
			// Bus routing for ALL sinks
			busY := allocBusY()
			src := srcPin.Pos

			// Source → channel (RIGHT side, near source column output).
			srcCh := srcCol
			if srcCh >= numColumns-1 {
				srcCh = numColumns - 2
			}
			srcTrackX := allocBusDropTrack(srcCh)
			addWire(nid, src.X, src.Y, srcTrackX, src.Y)

			// Vertical up to bus.
			addWire(nid, srcTrackX, src.Y, srcTrackX, busY)

			// Group sinks by destination drop-down channel (RIGHT side).
			type dropGroup struct {
				ch     int
				trackX int
				sinks  []sinkInfo
			}
			dgMap := map[int]*dropGroup{}
			for _, s := range sinks {
				ch := s.col - 1
				if ch < 0 {
					ch = 0
				}
				if ch >= numColumns-1 {
					ch = numColumns - 2
				}
				dg, ok := dgMap[ch]
				if !ok {
					tx := allocBusDropTrack(ch)
					dg = &dropGroup{ch: ch, trackX: tx}
					dgMap[ch] = dg
				}
				dg.sinks = append(dg.sinks, s)
			}

			// Horizontal bus segments between all junction X positions.
			busXs := []int{srcTrackX}
			for _, dg := range dgMap {
				busXs = append(busXs, dg.trackX)
			}
			sort.Ints(busXs)
			busXs = uniqueInts(busXs)
			for i := 0; i < len(busXs)-1; i++ {
				addWire(nid, busXs[i], busY, busXs[i+1], busY)
			}

			// Drop-down from bus to each sink group (sorted by channel for determinism).
			var dgKeys []int
			for ch := range dgMap {
				dgKeys = append(dgKeys, ch)
			}
			sort.Ints(dgKeys)
			for _, ch := range dgKeys {
				dg := dgMap[ch]
				sort.Slice(dg.sinks, func(i, j int) bool {
					return dg.sinks[i].pos.Y < dg.sinks[j].pos.Y
				})
				ys := []int{busY}
				for _, s := range dg.sinks {
					ys = append(ys, s.pos.Y)
				}
				sort.Ints(ys)
				ys = uniqueInts(ys)
				for i := 0; i < len(ys)-1; i++ {
					addWire(nid, dg.trackX, ys[i], dg.trackX, ys[i+1])
				}
				for _, s := range dg.sinks {
					addWire(nid, dg.trackX, s.pos.Y, s.pos.X, s.pos.Y)
				}
			}
		}
	}

	return cir, nil
}

// firstNet extracts the first net ID from a connection bits slice.
func firstNet(bits []interface{}) int {
	if len(bits) > 0 {
		id, _ := BitToNetID(bits[0])
		return id
	}
	return 0
}

// EmitFalstad generates the complete Falstad circuit text.
func EmitFalstad(cir *Circuit) string {
	var lines []string
	lines = append(lines, DumpHeader())

	for _, m := range cir.Models {
		lines = append(lines, m.Dump())
	}

	// Element index counter (Falstad counts all element lines:
	// components, text annotations, wires — but NOT header, models, scopes).
	elmIdx := 0
	addElm := func(line string) {
		lines = append(lines, line)
		elmIdx++
	}

	// Track indices and names of input/output elements for scopes.
	type scopeEntry struct {
		idx  int
		name string
	}
	var scopes []scopeEntry

	for _, comp := range cir.Components {
		var line string
		switch comp.FalstadType {
		case FalstadInverter:
			line = DumpInverter(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y)
		case FalstadAND, FalstadNAND, FalstadOR, FalstadNOR, FalstadXOR:
			nIn := 0
			for _, p := range comp.Pins {
				if !p.IsOutput {
					nIn++
				}
			}
			line = DumpGate(comp.FalstadType, comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y, nIn)
		case FalstadCustom:
			nOut := 0
			for _, p := range comp.Pins {
				if p.IsOutput {
					nOut++
				}
			}
			line = DumpCustomLogic(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y, comp.ModelName, nOut)
		case FalstadLogicIn:
			scopes = append(scopes, scopeEntry{elmIdx, comp.Name})
			addElm(DumpLogicInput(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y))
			addElm(DumpText(comp.Pos2.X-40, comp.Pos2.Y, comp.Name))
			continue
		case FalstadRail:
			scopes = append(scopes, scopeEntry{elmIdx, comp.Name})
			addElm(DumpClock(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y))
			addElm(DumpText(comp.Pos2.X-40, comp.Pos2.Y, comp.Name))
			continue
		case FalstadLogicOut:
			scopes = append(scopes, scopeEntry{elmIdx, comp.Name})
			addElm(DumpLogicOutput(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y))
			addElm(DumpText(comp.Pos2.X+8, comp.Pos2.Y, comp.Name))
			continue
		}
		if line != "" {
			addElm(line)
		}
	}

	for _, w := range cir.Wires {
		addElm(w)
	}

	// Append scope lines for all inputs and outputs.
	for _, s := range scopes {
		lines = append(lines, DumpScope(s.idx, s.name))
	}

	return strings.Join(lines, "\n")
}

// BuildNetLabels returns a short, human-friendly label for every net in the
// circuit (used for the "address-method" / labeled view in both GOST and
// Falstad). Nets touching a LogicInput, LogicOutput or clock Rail component
// adopt the port's name (clk, rst_n, count[3], …); the rest fall back to
// "N<id>". The map only contains nets that have at least one named anchor;
// callers should default to fmt.Sprintf("N%d", netID) for missing entries.
func BuildNetLabels(cir *Circuit) map[int]string {
	labels := map[int]string{}
	for _, comp := range cir.Components {
		switch comp.FalstadType {
		case FalstadLogicIn, FalstadLogicOut, FalstadRail:
			for _, pin := range comp.Pins {
				if pin.NetID == 0 {
					continue
				}
				if _, ok := labels[pin.NetID]; !ok {
					labels[pin.NetID] = comp.Name
				}
			}
		}
	}
	return labels
}

// netLabelOr returns the recorded label for netID, or a synthetic "N<id>".
func netLabelOr(labels map[int]string, netID int) string {
	if s, ok := labels[netID]; ok && s != "" {
		return s
	}
	return fmt.Sprintf("N%d", netID)
}

// EmitFalstadLabeled produces a Falstad text that uses LabeledNodeElm (188)
// instead of explicit wires to join pins. Every pin on every placed component
// gets its own labeled-node anchor — pins sharing a net share the same label
// text and are therefore electrically connected by Falstad at simulation time.
//
// Important: in CircuitJS, LabeledNodeElm.flag bit 1 means *internal* (an
// anonymous ground-like node) — internal nodes do not unify with external
// labels of the same name and do not display the text. We therefore always
// emit external labels (flag=0). For Logic inputs/outputs/rails we skip the
// separate text annotation so we don't duplicate the net name next to the
// switch/lamp/clock body.
func EmitFalstadLabeled(cir *Circuit) string {
	netLabels := BuildNetLabels(cir)

	var lines []string
	lines = append(lines, DumpHeader())
	for _, m := range cir.Models {
		lines = append(lines, m.Dump())
	}

	elmIdx := 0
	addElm := func(line string) {
		lines = append(lines, line)
		elmIdx++
	}

	type scopeEntry struct {
		idx  int
		name string
	}
	var scopes []scopeEntry

	// Labeled-node geometry: a short horizontal stub from the pin position
	// outward — to the right of an output pin, to the left of an input pin.
	const labelStub = 24

	emitLabelsFor := func(comp *PlacedComponent) {
		for _, pin := range comp.Pins {
			if pin.NetID == 0 {
				continue
			}
			label := netLabelOr(netLabels, pin.NetID)
			off := labelStub
			if !pin.IsOutput {
				off = -labelStub
			}
			addElm(DumpLabeledNode(pin.Pos.X, pin.Pos.Y, pin.Pos.X+off, pin.Pos.Y, false, label))
		}
	}

	for _, comp := range cir.Components {
		var line string
		switch comp.FalstadType {
		case FalstadInverter:
			line = DumpInverter(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y)
		case FalstadAND, FalstadNAND, FalstadOR, FalstadNOR, FalstadXOR:
			nIn := 0
			for _, p := range comp.Pins {
				if !p.IsOutput {
					nIn++
				}
			}
			line = DumpGate(comp.FalstadType, comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y, nIn)
		case FalstadCustom:
			nOut := 0
			for _, p := range comp.Pins {
				if p.IsOutput {
					nOut++
				}
			}
			line = DumpCustomLogic(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y, comp.ModelName, nOut)
		case FalstadLogicIn:
			scopes = append(scopes, scopeEntry{elmIdx, comp.Name})
			addElm(DumpLogicInput(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y))
			emitLabelsFor(comp)
			continue
		case FalstadRail:
			scopes = append(scopes, scopeEntry{elmIdx, comp.Name})
			addElm(DumpClock(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y))
			emitLabelsFor(comp)
			continue
		case FalstadLogicOut:
			scopes = append(scopes, scopeEntry{elmIdx, comp.Name})
			addElm(DumpLogicOutput(comp.Pos1.X, comp.Pos1.Y, comp.Pos2.X, comp.Pos2.Y))
			emitLabelsFor(comp)
			continue
		}
		if line != "" {
			addElm(line)
			emitLabelsFor(comp)
		}
	}

	for _, s := range scopes {
		lines = append(lines, DumpScope(s.idx, s.name))
	}
	return strings.Join(lines, "\n")
}

func uniqueInts(s []int) []int {
	if len(s) == 0 {
		return s
	}
	r := []int{s[0]}
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			r = append(r, s[i])
		}
	}
	return r
}
