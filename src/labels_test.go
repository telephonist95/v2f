package main

import (
	"strconv"
	"strings"
	"testing"
)

func makeMiniCircuit() *Circuit {
	// Two input ports → AND gate → one output port.
	clkPin := Pin{Pos: Point{0, 0}, NetID: 10, IsOutput: true, PortName: "clk"}
	rstPin := Pin{Pos: Point{0, 16}, NetID: 11, IsOutput: true, PortName: "rst_n"}
	outPin := Pin{Pos: Point{200, 0}, NetID: 12, IsOutput: false, PortName: "y"}
	clkComp := &PlacedComponent{Name: "clk", FalstadType: FalstadLogicIn, Pos1: Point{20, 0}, Pos2: Point{-20, 0}, Pins: []Pin{clkPin}}
	rstComp := &PlacedComponent{Name: "rst_n", FalstadType: FalstadLogicIn, Pos1: Point{20, 16}, Pos2: Point{-20, 16}, Pins: []Pin{rstPin}}
	outComp := &PlacedComponent{Name: "y", FalstadType: FalstadLogicOut, Pos1: Point{200, 0}, Pos2: Point{240, 0}, Pins: []Pin{outPin}}
	gate := &PlacedComponent{
		Name:        "u_and",
		FalstadType: FalstadAND,
		Pos1:        Point{100, 0},
		Pos2:        Point{164, 0},
		Pins: []Pin{
			{Pos: Point{100, -16}, NetID: 10, IsOutput: false, PortName: "A"},
			{Pos: Point{100, 16}, NetID: 11, IsOutput: false, PortName: "B"},
			{Pos: Point{164, 0}, NetID: 12, IsOutput: true, PortName: "Y"},
		},
	}
	return &Circuit{Components: []*PlacedComponent{clkComp, rstComp, gate, outComp}}
}

func TestBuildNetLabelsUsesPortNames(t *testing.T) {
	cir := makeMiniCircuit()
	labels := BuildNetLabels(cir)
	if labels[10] != "clk" {
		t.Errorf("labels[10]=%q; want \"clk\"", labels[10])
	}
	if labels[11] != "rst_n" {
		t.Errorf("labels[11]=%q; want \"rst_n\"", labels[11])
	}
	if labels[12] != "y" {
		t.Errorf("labels[12]=%q; want \"y\"", labels[12])
	}
}

func TestNetLabelOrFallback(t *testing.T) {
	labels := map[int]string{10: "clk"}
	if got := netLabelOr(labels, 10); got != "clk" {
		t.Errorf("netLabelOr(labels, 10)=%q; want \"clk\"", got)
	}
	if got := netLabelOr(labels, 42); got != "N42" {
		t.Errorf("netLabelOr(labels, 42)=%q; want \"N42\"", got)
	}
	if got := netLabelOr(nil, 7); got != "N7" {
		t.Errorf("netLabelOr(nil, 7)=%q; want \"N7\"", got)
	}
}

func TestEmitFalstadLabeledContainsLabeledNodes(t *testing.T) {
	cir := makeMiniCircuit()
	out := EmitFalstadLabeled(cir)
	if !strings.Contains(out, "207 ") {
		t.Error("expected at least one LabeledNodeElm (207 …) line")
	}
	// AND gate pins must produce three labels: clk, rst_n, y (one per pin).
	for _, want := range []string{" clk", " rst_n", " y"} {
		if !strings.Contains(out, "207 ") || !strings.Contains(out, want) {
			t.Errorf("missing %q in labeled output:\n%s", want, out)
		}
	}
	// And no plain wires.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "w ") {
			t.Errorf("labeled emitter unexpectedly produced wire: %q", line)
		}
	}
}

func TestEmitFalstadLabeledKeepsHeaderAndComponents(t *testing.T) {
	cir := makeMiniCircuit()
	out := EmitFalstadLabeled(cir)
	if !strings.HasPrefix(out, "$ ") {
		t.Errorf("missing Falstad header: %q", out[:30])
	}
	// Logic input + output + AND (150) must still be present.
	for _, want := range []string{"L 20 0", "150 100 0 164 0", "M 200 0"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing component %q:\n%s", want, out)
		}
	}
}

func TestEmitFalstadLabeledAllExternal(t *testing.T) {
	// Regression: every labeled-node must be external — i.e. FLAG_INTERNAL=1
	// bit must NOT be set, otherwise labels won't unify by name in Falstad.
	// FLAG_ESCAPE=4 IS always set in the modern dump format.
	cir := makeMiniCircuit()
	out := EmitFalstadLabeled(cir)
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "207 ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			t.Errorf("malformed labeled-node line: %q", line)
			continue
		}
		flag, err := strconv.Atoi(fields[5])
		if err != nil {
			t.Errorf("non-numeric flag in %q", line)
			continue
		}
		if flag&1 != 0 { // FLAG_INTERNAL
			t.Errorf("labeled-node %q has FLAG_INTERNAL set; want external", line)
		}
		if flag&4 == 0 { // FLAG_ESCAPE must be set in modern dump
			t.Errorf("labeled-node %q missing FLAG_ESCAPE", line)
		}
	}
}

func TestDumpLabeledNodeInternalFlag(t *testing.T) {
	external := DumpLabeledNode(0, 0, 16, 0, false, "clk")
	internal := DumpLabeledNode(0, 0, 16, 0, true, "clk")
	// External: type=207, coords=0 0 16 0, flags=4 (FLAG_ESCAPE)
	if !strings.HasPrefix(external, "207 0 0 16 0 4 ") {
		t.Errorf("external label dump wrong: %q", external)
	}
	// Internal: flags=5 (FLAG_ESCAPE|FLAG_INTERNAL)
	if !strings.HasPrefix(internal, "207 0 0 16 0 5 ") {
		t.Errorf("internal label dump wrong: %q", internal)
	}
}

func TestParseFlipFlopParams(t *testing.T) {
	cases := []struct{ in, base, params string }{
		{"DFF_P", "DFF", "P"},
		{"DFF_PN0", "DFF", "PN0"},
		{"DFFE_PN0P", "DFFE", "PN0P"},
		{"SDFFE_PN0P", "SDFFE", "PN0P"},
		{"DLATCH_P", "DLATCH", "P"},
		{"SR_PP", "SR", "PP"},
		{"REG_SDFFE_PN0P_4_0", "SDFFE", "PN0P"}, // RTL synthetic
		{"COMB", "", ""},                        // not a FF
		{"BUF", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		b, p := parseFlipFlopParams(c.in)
		if b != c.base || p != c.params {
			t.Errorf("parseFlipFlopParams(%q) = (%q,%q); want (%q,%q)", c.in, b, p, c.base, c.params)
		}
	}
}

func TestPortIndicatorsDFF(t *testing.T) {
	// DFFE_PN0P: C posedge, R active-low (low → reset), R-value 0, E active-high
	dyn, inv := portIndicators("DFFE", "PN0P", "C", false)
	if !dyn || inv {
		t.Errorf("clock C of PN0P: dyn=%v inv=%v; want true,false", dyn, inv)
	}
	dyn, inv = portIndicators("DFFE", "PN0P", "R", false)
	if dyn || !inv {
		t.Errorf("reset R of PN0P: dyn=%v inv=%v; want false,true", dyn, inv)
	}
	dyn, inv = portIndicators("DFFE", "PN0P", "E", false)
	if dyn || inv {
		t.Errorf("enable E of PN0P: dyn=%v inv=%v; want false,false (E=P active-high)", dyn, inv)
	}
	// Output Q — no indicators
	dyn, inv = portIndicators("DFFE", "PN0P", "Q", true)
	if dyn || inv {
		t.Errorf("output Q: dyn=%v inv=%v; want false,false", dyn, inv)
	}
}

func TestPortIndicatorsNegedge(t *testing.T) {
	// DFF_N: clock by negedge → dynamic + inverted
	dyn, inv := portIndicators("DFF", "N", "C", false)
	if !dyn || !inv {
		t.Errorf("negedge clock: dyn=%v inv=%v; want true,true", dyn, inv)
	}
}

func TestPortLabelGOST(t *testing.T) {
	cases := []struct {
		ffBase, pin string
		isOut       bool
		want        string
	}{
		{"DFFE", "C", false, "C1"},
		{"DFFE", "D", false, "1D"},
		{"DFFE", "R", false, "R"},
		{"DFFE", "E", false, "E"},
		{"DFFE", "Q", true, "Q"},
		{"SDFFE", "D0", false, "1D0"},
		{"", "A", false, "A"}, // combinational pin unchanged
	}
	for _, c := range cases {
		got := portLabelGOST(c.ffBase, c.pin, c.isOut)
		if got != c.want {
			t.Errorf("portLabelGOST(%q, %q, %v) = %q; want %q", c.ffBase, c.pin, c.isOut, got, c.want)
		}
	}
}
