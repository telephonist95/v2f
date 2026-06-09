package main

import (
	"strings"
	"testing"
)

func TestFalstadEscape(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"a b", `a\sb`},
		{"a+b", `a\pb`},
		{"x=1", `x\q1`},
		{"a#b", `a\hb`},
		{"a&b", `a\ab`},
		{"a\nb", `a\nb`},
		{`a\b`, `a\\b`},
	}
	for _, c := range cases {
		got := falstadEscape(c.in)
		if got != c.want {
			t.Errorf("falstadEscape(%q)=%q; want %q", c.in, got, c.want)
		}
	}
}

func TestFalstadEscapeTextKeepsOperators(t *testing.T) {
	// Text-annotation escape must leave +, =, #, & alone.
	in := "a+b=c#d&e f"
	got := falstadEscapeText(in)
	want := `a+b=c#d&e\sf`
	if got != want {
		t.Errorf("falstadEscapeText(%q)=%q; want %q", in, got, want)
	}
}

func TestDumpHeaderFormat(t *testing.T) {
	h := DumpHeader()
	if !strings.HasPrefix(h, "$ ") {
		t.Errorf("DumpHeader missing leading $: %q", h)
	}
}

func TestDumpWire(t *testing.T) {
	got := DumpWire(10, 20, 30, 40)
	want := "w 10 20 30 40 0"
	if got != want {
		t.Errorf("DumpWire = %q; want %q", got, want)
	}
}

func TestDumpInverter(t *testing.T) {
	got := DumpInverter(0, 0, 64, 0)
	want := "I 0 0 64 0 0 0.5 5"
	if got != want {
		t.Errorf("DumpInverter = %q; want %q", got, want)
	}
}

func TestDumpGate(t *testing.T) {
	got := DumpGate(FalstadAND, 0, 0, 64, 0, 2)
	want := "150 0 0 64 0 0 2 0 5"
	if got != want {
		t.Errorf("DumpGate AND = %q; want %q", got, want)
	}
}

func TestDumpCustomLogicOutputCount(t *testing.T) {
	got := DumpCustomLogic(0, 0, 96, 0, "MUX", 1)
	if !strings.HasPrefix(got, "208 0 0 96 0 0 MUX") {
		t.Errorf("DumpCustomLogic prefix wrong: %q", got)
	}
	if !strings.HasSuffix(got, " 0") {
		t.Errorf("DumpCustomLogic missing trailing voltage values: %q", got)
	}

	// 3 outputs → three trailing "0" tokens.
	got = DumpCustomLogic(0, 0, 96, 0, "REG", 3)
	parts := strings.Fields(got)
	trailing := 0
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "0" {
			trailing++
			continue
		}
		break
	}
	if trailing < 3 {
		t.Errorf("expected ≥3 trailing zero tokens, got %d in %q", trailing, got)
	}
}

func TestCustomModelDumpFormat(t *testing.T) {
	m := &CustomModel{
		Name:    "AND2",
		Inputs:  []string{"A", "B"},
		Outputs: []string{"Y"},
		Rules:   []string{"00=0", "11=1"},
	}
	got := m.Dump()
	if !strings.HasPrefix(got, "! AND2 0 A,B Y ") {
		t.Errorf("CustomModel.Dump unexpected: %q", got)
	}
	if !strings.Contains(got, `00\q0\n11\q1`) {
		t.Errorf("CustomModel.Dump missing escaped rules: %q", got)
	}
}
