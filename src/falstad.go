package main

import (
	"fmt"
	"strings"
)

// Falstad element type constants.
const (
	FalstadAND      = "150"
	FalstadNAND     = "151"
	FalstadOR       = "152"
	FalstadNOR      = "153"
	FalstadXOR      = "154"
	FalstadDFF      = "155"
	FalstadCustom   = "208"
	FalstadInverter = "I"
	FalstadLogicIn  = "L"
	FalstadLogicOut = "M"
	FalstadRail     = "R"
	FalstadWire     = "w"
)

// CustomModel defines a Falstad Custom Logic chip model.
type CustomModel struct {
	Name    string
	Inputs  []string
	Outputs []string
	Rules   []string
	Info    string
}

// Dump returns the model definition line for the Falstad circuit text.
// Format: ! escapedName flags escapedInputs escapedOutputs escapedInfo escapedRules
func (m *CustomModel) Dump() string {
	inputs := strings.Join(m.Inputs, ",")
	outputs := strings.Join(m.Outputs, ",")
	info := m.Info
	if info == "" {
		info = "\\0"
	} else {
		info = falstadEscape(info)
	}

	var escapedRules []string
	for _, r := range m.Rules {
		escapedRules = append(escapedRules, falstadEscape(r))
	}
	rules := strings.Join(escapedRules, "\\n")

	return fmt.Sprintf("! %s 0 %s %s %s %s", m.Name, inputs, outputs, info, rules)
}

// falstadEscape escapes special characters for Falstad custom logic model serialization.
// Custom-logic rules use +, =, #, & as delimiters, so these are escaped too.
func falstadEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, " ", "\\s")
	s = strings.ReplaceAll(s, "+", "\\p")
	s = strings.ReplaceAll(s, "=", "\\q")
	s = strings.ReplaceAll(s, "#", "\\h")
	s = strings.ReplaceAll(s, "&", "\\a")
	return s
}

// falstadEscapeText escapes only token separators for inline text annotations
// (the "x ..." element). Unlike falstadEscape, it leaves +, =, #, & alone so
// they render literally in the label.
func falstadEscapeText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, " ", "\\s")
	return s
}

// Predefined custom logic models for Yosys cell types without native Falstad equivalents.
var PredefinedModels = map[string]*CustomModel{
	// 2-input gates
	"XNOR":   {Name: "XNOR", Inputs: []string{"A", "B"}, Outputs: []string{"Y"}, Rules: []string{"00=1", "01=0", "10=0", "11=1"}},
	"ANDNOT": {Name: "ANDNOT", Inputs: []string{"A", "B"}, Outputs: []string{"Y"}, Rules: []string{"00=0", "01=0", "10=1", "11=0"}},
	"ORNOT":  {Name: "ORNOT", Inputs: []string{"A", "B"}, Outputs: []string{"Y"}, Rules: []string{"00=1", "01=0", "10=1", "11=1"}},

	// Buffer
	"BUF": {Name: "BUF", Inputs: []string{"A"}, Outputs: []string{"Y"}, Rules: []string{"0=0", "1=1"}},

	// Muxes
	"MUX":  {Name: "MUX", Inputs: []string{"A", "B", "S"}, Outputs: []string{"Y"}, Rules: []string{"a?0=a", "?b1=b"}},
	"NMUX": {Name: "NMUX", Inputs: []string{"A", "B", "S"}, Outputs: []string{"Y"}, Rules: []string{"000=1", "010=1", "100=0", "110=0", "001=1", "011=0", "101=1", "111=0"}},
	"MUX4": {Name: "MUX4", Inputs: []string{"A", "B", "C", "D", "S", "T"}, Outputs: []string{"Y"},
		Rules: []string{"a???00=a", "?b??10=b", "??c?01=c", "???d11=d"}},

	// AOI / OAI
	// AOI3: Y = ~((A & B) | C)
	"AOI3": {Name: "AOI3", Inputs: []string{"A", "B", "C"}, Outputs: []string{"Y"},
		Rules: []string{"000=1", "001=0", "010=1", "011=0", "100=1", "101=0", "110=0", "111=0"}},
	// OAI3: Y = ~((A | B) & C)
	"OAI3": {Name: "OAI3", Inputs: []string{"A", "B", "C"}, Outputs: []string{"Y"},
		Rules: []string{"000=1", "001=1", "010=1", "011=0", "100=1", "101=0", "110=1", "111=0"}},
	// AOI4: Y = ~((A & B) | (C & D))
	"AOI4": {Name: "AOI4", Inputs: []string{"A", "B", "C", "D"}, Outputs: []string{"Y"},
		Rules: []string{
			"0000=1", "0001=1", "0010=1", "0011=0",
			"0100=1", "0101=1", "0110=1", "0111=0",
			"1000=1", "1001=1", "1010=1", "1011=0",
			"1100=0", "1101=0", "1110=0", "1111=0",
		}},
	// OAI4: Y = ~((A | B) & (C | D))
	"OAI4": {Name: "OAI4", Inputs: []string{"A", "B", "C", "D"}, Outputs: []string{"Y"},
		Rules: []string{
			"0000=1", "0001=1", "0010=1", "0011=1",
			"0100=1", "0101=0", "0110=0", "0111=0",
			"1000=1", "1001=0", "1010=0", "1011=0",
			"1100=1", "1101=0", "1110=0", "1111=0",
		}},
}

// DumpHeader returns the Falstad circuit file header.
func DumpHeader() string {
	return "$ 1 0.000005 10.20027730826997 50 5 43"
}

// DumpInverter generates a Falstad inverter dump line.
// Pin: input at (x1,y1), output at (x2,y2).
func DumpInverter(x1, y1, x2, y2 int) string {
	return fmt.Sprintf("I %d %d %d %d 0 0.5 5", x1, y1, x2, y2)
}

// DumpGate generates a Falstad gate dump line.
// gateType is "150"(AND), "151"(NAND), "152"(OR), "153"(NOR), "154"(XOR).
// Pins: input A at (x1, y1-16), input B at (x1, y1+16), output at (x2, y2).
func DumpGate(gateType string, x1, y1, x2, y2, inputCount int) string {
	return fmt.Sprintf("%s %d %d %d %d 0 %d 0 5", gateType, x1, y1, x2, y2, inputCount)
}

// DumpCustomLogic generates a Falstad custom logic chip dump line.
// Pin positions depend on the model. outputCount specifies how many output voltage
// values to append (one per output pin, required by CustomLogicElm constructor).
func DumpCustomLogic(x1, y1, x2, y2 int, modelName string, outputCount int) string {
	s := fmt.Sprintf("208 %d %d %d %d 0 %s", x1, y1, x2, y2, modelName)
	for i := 0; i < outputCount; i++ {
		s += " 0"
	}
	return s
}

// DumpLogicInput generates a Falstad logic input (switch) dump line.
// Single pin (output) at (x1, y1). Label extends toward (x2, y2).
func DumpLogicInput(x1, y1, x2, y2 int) string {
	return fmt.Sprintf("L %d %d %d %d 0 0 false 5 0", x1, y1, x2, y2)
}

// DumpClock generates a Falstad clock rail dump line.
// Single pin (output) at (x1, y1). Label extends toward (x2, y2).
// Generates a 100 Hz square wave 0-5V.
func DumpClock(x1, y1, x2, y2 int) string {
	return fmt.Sprintf("R %d %d %d %d 1 2 100 2.5 2.5 0 0.5", x1, y1, x2, y2)
}

// DumpLogicOutput generates a Falstad logic output (LED/probe) dump line.
// Single pin (input) at (x1, y1). Indicator extends toward (x2, y2).
func DumpLogicOutput(x1, y1, x2, y2 int) string {
	return fmt.Sprintf("M %d %d %d %d 0 2.5", x1, y1, x2, y2)
}

// DumpText generates a Falstad text annotation (no electrical connections).
func DumpText(x, y int, text string) string {
	return fmt.Sprintf("x %d %d %d %d 4 14 %s", x, y, x+1, y, falstadEscapeText(text))
}

// DumpScope generates a Falstad scope (oscilloscope) line for an element.
// elmIdx is the 0-based index of the element among all element lines
// (components + text annotations + wires; excludes header, model defs, and scope lines).
// label is shown as the scope title.
func DumpScope(elmIdx int, label string) string {
	// speed=20 → 2 ms/div (maxTimeStep=5μs, gridStep = smallest 1-2-5 ≥ 5e-6*20*20 = 2e-3)
	// value=0 (voltage), flags=4102 (FLAG_PLOTS|showV|!showMax),
	// vscale=5, ascale=1, position=0, plotcount=1, label at end
	return fmt.Sprintf("o %d 20 0 4102 5 1 0 1 %s", elmIdx, falstadEscape(label))
}

// DumpWire generates a Falstad wire dump line.
func DumpWire(x1, y1, x2, y2 int) string {
	return fmt.Sprintf("w %d %d %d %d 0", x1, y1, x2, y2)
}

// DumpLabeledNode generates a Falstad LabeledNodeElm.
// CircuitJS dump-type for LabeledNodeElm is 207. All elements with the same
// label text are electrically connected.
//
// Flag bits:
//
//	FLAG_INTERNAL = 1 → silent / ground-like internal node (no text shown);
//	                    internal nodes do NOT unify with external labels of
//	                    the same name.
//	FLAG_ESCAPE   = 4 → text uses CircuitJS escape sequences; the modern
//	                    dump always sets this so the text token can survive
//	                    whitespace and special chars.
//
// (x1,y1) is the pin; (x2,y2) is where the text/box is drawn.
func DumpLabeledNode(x1, y1, x2, y2 int, internal bool, label string) string {
	flags := 4 // FLAG_ESCAPE — always emit modern, escape-aware format
	if internal {
		flags |= 1 // FLAG_INTERNAL
	}
	return fmt.Sprintf("207 %d %d %d %d %d %s", x1, y1, x2, y2, flags, falstadEscape(label))
}
