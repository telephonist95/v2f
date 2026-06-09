package main

import (
	"strings"
	"testing"
)

func TestEvalCombCellBasics(t *testing.T) {
	cases := []struct {
		t   string
		pv  map[string]int
		out int
	}{
		{"$_NOT_", map[string]int{"A": 0}, 1},
		{"$_NOT_", map[string]int{"A": 1}, 0},
		{"$_BUF_", map[string]int{"A": 0}, 0},
		{"$_BUF_", map[string]int{"A": 1}, 1},
		{"$_AND_", map[string]int{"A": 1, "B": 1}, 1},
		{"$_AND_", map[string]int{"A": 1, "B": 0}, 0},
		{"$_OR_", map[string]int{"A": 0, "B": 1}, 1},
		{"$_OR_", map[string]int{"A": 0, "B": 0}, 0},
		{"$_XOR_", map[string]int{"A": 1, "B": 0}, 1},
		{"$_XOR_", map[string]int{"A": 1, "B": 1}, 0},
		{"$_NAND_", map[string]int{"A": 1, "B": 1}, 0},
		{"$_NAND_", map[string]int{"A": 1, "B": 0}, 1},
		{"$_NOR_", map[string]int{"A": 0, "B": 0}, 1},
		{"$_NOR_", map[string]int{"A": 1, "B": 0}, 0},
		{"$_XNOR_", map[string]int{"A": 1, "B": 1}, 1},
		{"$_XNOR_", map[string]int{"A": 1, "B": 0}, 0},
		{"$_ANDNOT_", map[string]int{"A": 1, "B": 0}, 1},
		{"$_ANDNOT_", map[string]int{"A": 0, "B": 0}, 0},
		{"$_ORNOT_", map[string]int{"A": 0, "B": 0}, 1},
		{"$_ORNOT_", map[string]int{"A": 0, "B": 1}, 0},
		{"$_MUX_", map[string]int{"A": 1, "B": 0, "S": 0}, 1},
		{"$_MUX_", map[string]int{"A": 1, "B": 0, "S": 1}, 0},
		{"$_NMUX_", map[string]int{"A": 1, "B": 0, "S": 0}, 0},
		{"$_AOI3_", map[string]int{"A": 1, "B": 1, "C": 0}, 0},
		{"$_AOI3_", map[string]int{"A": 0, "B": 0, "C": 0}, 1},
		{"$_OAI3_", map[string]int{"A": 1, "B": 0, "C": 1}, 0},
		{"$_AOI4_", map[string]int{"A": 1, "B": 1, "C": 0, "D": 0}, 0},
		{"$_OAI4_", map[string]int{"A": 1, "B": 0, "C": 0, "D": 1}, 0},
		{"$_MUX4_", map[string]int{"A": 1, "B": 0, "C": 0, "D": 0, "S": 0, "T": 0}, 1},
		{"$_MUX4_", map[string]int{"A": 0, "B": 0, "C": 0, "D": 1, "S": 1, "T": 1}, 1},
	}
	for _, c := range cases {
		got := evalCombCell(c.t, c.pv)
		if got != c.out {
			t.Errorf("evalCombCell(%s, %v) = %d; want %d", c.t, c.pv, got, c.out)
		}
	}
}

func TestExtractFFDFF(t *testing.T) {
	cell := &Cell{
		Type:        "$_DFF_P_",
		Connections: map[string][]interface{}{"C": {2.0}, "D": {3.0}, "Q": {4.0}},
	}
	fd := extractFF("ff0", cell)
	if fd == nil {
		t.Fatal("extractFF returned nil for $_DFF_P_")
	}
	if fd.baseType != "DFF" || fd.params != "P" {
		t.Errorf("baseType=%q params=%q", fd.baseType, fd.params)
	}
	if fd.clkNet != 2 || fd.dNet != 3 || fd.qNet != 4 {
		t.Errorf("nets clk=%d d=%d q=%d", fd.clkNet, fd.dNet, fd.qNet)
	}
	if fd.rstNet != -1 || fd.enNet != -1 {
		t.Errorf("expected rst/en = -1, got rst=%d en=%d", fd.rstNet, fd.enNet)
	}
}

func TestExtractFFSDFFE(t *testing.T) {
	cell := &Cell{
		Type:        "$_SDFFE_PN0P_",
		Connections: map[string][]interface{}{"C": {1.0}, "R": {2.0}, "D": {3.0}, "E": {4.0}, "Q": {5.0}},
	}
	fd := extractFF("ff", cell)
	if fd == nil {
		t.Fatal("extractFF returned nil for $_SDFFE_PN0P_")
	}
	if fd.baseType != "SDFFE" || fd.params != "PN0P" {
		t.Errorf("baseType=%q params=%q", fd.baseType, fd.params)
	}
	if fd.clkNet != 1 || fd.rstNet != 2 || fd.dNet != 3 || fd.enNet != 4 || fd.qNet != 5 {
		t.Errorf("nets clk=%d rst=%d d=%d en=%d q=%d", fd.clkNet, fd.rstNet, fd.dNet, fd.enNet, fd.qNet)
	}
}

func TestExtractFFSR(t *testing.T) {
	cell := &Cell{
		Type:        "$_SR_PP_",
		Connections: map[string][]interface{}{"S": {7.0}, "R": {8.0}, "Q": {9.0}},
	}
	fd := extractFF("sr", cell)
	if fd == nil {
		t.Fatal("extractFF returned nil for $_SR_PP_ (regression: SR was previously dropped)")
	}
	if fd.baseType != "SR" || fd.params != "PP" {
		t.Errorf("baseType=%q params=%q", fd.baseType, fd.params)
	}
	if fd.dNet != 7 {
		t.Errorf("S → dNet = %d; want 7", fd.dNet)
	}
	if fd.rstNet != 8 {
		t.Errorf("R → rstNet = %d; want 8", fd.rstNet)
	}
	if fd.clkNet != -1 {
		t.Errorf("SR should have clkNet=-1, got %d", fd.clkNet)
	}
}

func TestGenerateSequentialCellDFF(t *testing.T) {
	m, in, out, ok := generateSequentialCell("$_DFF_P_")
	if !ok || m == nil {
		t.Fatal("generateSequentialCell failed for $_DFF_P_")
	}
	if !equalStrings(in, []string{"C", "D"}) {
		t.Errorf("inputs=%v; want [C D]", in)
	}
	if !equalStrings(out, []string{"Q"}) {
		t.Errorf("outputs=%v; want [Q]", out)
	}
	// rule should be "+a=a"
	if len(m.Rules) != 1 || m.Rules[0] != "+a=a" {
		t.Errorf("rules=%v; want [+a=a]", m.Rules)
	}
}

func TestGenerateSequentialCellSR(t *testing.T) {
	m, in, out, ok := generateSequentialCell("$_SR_PP_")
	if !ok || m == nil {
		t.Fatal("generateSequentialCell failed for $_SR_PP_")
	}
	if !equalStrings(in, []string{"S", "R"}) {
		t.Errorf("inputs=%v; want [S R]", in)
	}
	if !equalStrings(out, []string{"Q"}) {
		t.Errorf("outputs=%v; want [Q]", out)
	}
	if len(m.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d: %v", len(m.Rules), m.Rules)
	}
}

func TestGenerateRegisterModelDLATCH(t *testing.T) {
	// DLATCH was previously not handled in generateRegisterModel — must return non-nil now.
	ffs := []*ffDetail{
		{baseType: "DLATCH", params: "P", clkNet: 1, dNet: 2, qNet: 3, rstNet: -1, enNet: -1},
	}
	regs := groupRegisters(ffs)
	if len(regs) != 1 {
		t.Fatalf("expected 1 group, got %d", len(regs))
	}
	m := generateRegisterModel(regs[0], 0)
	if m == nil {
		t.Fatal("generateRegisterModel returned nil for DLATCH — regression")
	}
	if !equalStrings(m.Inputs, []string{"E", "D0"}) {
		t.Errorf("DLATCH inputs=%v; want [E D0]", m.Inputs)
	}
}

func TestGenerateRegisterModelSR(t *testing.T) {
	ffs := []*ffDetail{
		{baseType: "SR", params: "PP", clkNet: -1, dNet: 10, qNet: 11, rstNet: 12, enNet: -1},
	}
	regs := groupRegisters(ffs)
	if len(regs) != 1 {
		t.Fatalf("expected 1 group, got %d", len(regs))
	}
	m := generateRegisterModel(regs[0], 0)
	if m == nil {
		t.Fatal("generateRegisterModel returned nil for SR — regression")
	}
	if !equalStrings(m.Inputs, []string{"S", "R"}) {
		t.Errorf("SR inputs=%v; want [S R]", m.Inputs)
	}
}

func TestGroupRegistersSRIsolated(t *testing.T) {
	// Two SR latches sharing R-net must not be merged — they have distinct S inputs.
	ffs := []*ffDetail{
		{baseType: "SR", params: "PP", clkNet: -1, dNet: 1, qNet: 100, rstNet: 50, enNet: -1},
		{baseType: "SR", params: "PP", clkNet: -1, dNet: 2, qNet: 101, rstNet: 50, enNet: -1},
	}
	regs := groupRegisters(ffs)
	if len(regs) != 2 {
		t.Fatalf("expected 2 distinct SR groups, got %d", len(regs))
	}
}

func TestIsSequentialCell(t *testing.T) {
	pos := []string{"$_DFF_P_", "$_SDFF_PN0_", "$_DFFE_PP_", "$_DLATCH_P_", "$_SR_PP_"}
	for _, s := range pos {
		if !IsSequentialCell(s) {
			t.Errorf("IsSequentialCell(%q) = false; want true", s)
		}
	}
	neg := []string{"$_NOT_", "$_AND_", "$_XOR_", "$_MUX4_"}
	for _, s := range neg {
		if IsSequentialCell(s) {
			t.Errorf("IsSequentialCell(%q) = true; want false", s)
		}
	}
}

func TestResolveCellRegistersDynamicModel(t *testing.T) {
	before := len(PredefinedModels)
	cm, ok := ResolveCell("$_DFF_N_")
	if !ok {
		t.Fatal("ResolveCell failed for $_DFF_N_")
	}
	if cm.falstadType != FalstadCustom {
		t.Errorf("falstadType=%q; want %q", cm.falstadType, FalstadCustom)
	}
	if cm.modelName == "" {
		t.Error("expected a non-empty model name")
	}
	if _, ok := PredefinedModels[cm.modelName]; !ok {
		t.Errorf("PredefinedModels missing dynamic entry %q", cm.modelName)
	}
	if len(PredefinedModels) <= before {
		t.Errorf("PredefinedModels did not grow")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCaptureVars(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, ""}, {1, "a"}, {3, "abc"}, {7, "abcdefg"},
	}
	for _, c := range cases {
		got := captureVars(c.n)
		if got != c.want {
			t.Errorf("captureVars(%d)=%q; want %q", c.n, got, c.want)
		}
	}
}

func TestFalstadCustomRuleHasNoUndefinedCaptures(t *testing.T) {
	// Regression for Bug #1: combDrivenCtrlNets should not become RHS capture vars.
	// We can't exercise full ConvertRTL trivially here without a full design,
	// but we lock the invariant: a generated COMB rule must use only capture
	// letters that appear on the LHS.
	rule := "????????ab=ab??????"
	lhs, rhs, ok := splitRule(rule)
	if !ok {
		t.Fatalf("test rule malformed: %q", rule)
	}
	lhsCaps := captureLetters(lhs)
	for _, ch := range rhs {
		if ch >= 'a' && ch <= 'z' && !lhsCaps[ch] {
			t.Errorf("RHS uses undefined capture %q (rule=%q)", string(ch), rule)
		}
	}
}

// splitRule extracts (LHS, RHS) from a rule "x=y".
func splitRule(rule string) (string, string, bool) {
	parts := strings.SplitN(rule, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func captureLetters(s string) map[rune]bool {
	m := map[rune]bool{}
	for _, ch := range s {
		if ch >= 'a' && ch <= 'z' {
			m[ch] = true
		}
	}
	return m
}
