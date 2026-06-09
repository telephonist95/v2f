package main

import (
	"testing"
)

const sampleVCD = `$date
	Sun May 24 20:43:08 2026
$end
$version
	Icarus Verilog
$end
$timescale
	1s
$end
$scope module tb_counter $end
$var wire 4 ! count [3:0] $end
$var reg 1 " clk $end
$var reg 1 # en $end
$var reg 1 $ rst_n $end
$scope module u_counter $end
$var wire 1 " clk $end
$var wire 1 # en $end
$var wire 1 $ rst_n $end
$var reg 4 % count [3:0] $end
$upscope $end
$upscope $end
$enddefinitions $end
$dumpvars
bx %
0$
0#
0"
bx !
$end
#5
b0 !
b0 %
1"
#10
0"
#15
1"
#20
0"
1#
1$
#25
b1 !
b1 %
1"
`

func TestParseVCDHeader(t *testing.T) {
	f, err := ParseVCD(sampleVCD)
	if err != nil {
		t.Fatalf("ParseVCD: %v", err)
	}
	if f.Timescale != "1s" {
		t.Errorf("Timescale=%q; want %q", f.Timescale, "1s")
	}
	if f.Version == "" {
		t.Errorf("Version not extracted")
	}
}

func TestParseVCDSignals(t *testing.T) {
	f, err := ParseVCD(sampleVCD)
	if err != nil {
		t.Fatalf("ParseVCD: %v", err)
	}
	if len(f.Signals) != 8 {
		t.Fatalf("expected 8 signals, got %d", len(f.Signals))
	}
	// First signal: count[3:0] in tb_counter scope
	if f.Signals[0].ID != "!" {
		t.Errorf("Signals[0].ID=%q; want '!'", f.Signals[0].ID)
	}
	if f.Signals[0].Name != "count [3:0]" {
		t.Errorf("Signals[0].Name=%q; want \"count [3:0]\"", f.Signals[0].Name)
	}
	if f.Signals[0].Width != 4 {
		t.Errorf("Signals[0].Width=%d; want 4", f.Signals[0].Width)
	}
	if len(f.Signals[0].Scope) != 1 || f.Signals[0].Scope[0] != "tb_counter" {
		t.Errorf("Signals[0].Scope=%v; want [tb_counter]", f.Signals[0].Scope)
	}
	// Last signal: count [3:0] in u_counter scope (nested)
	last := f.Signals[len(f.Signals)-1]
	if len(last.Scope) != 2 || last.Scope[0] != "tb_counter" || last.Scope[1] != "u_counter" {
		t.Errorf("last signal Scope=%v; want [tb_counter u_counter]", last.Scope)
	}
}

func TestParseVCDChanges(t *testing.T) {
	f, err := ParseVCD(sampleVCD)
	if err != nil {
		t.Fatalf("ParseVCD: %v", err)
	}
	if len(f.Changes) == 0 {
		t.Fatal("expected non-empty changes")
	}
	// Time markers seen: 0 (dumpvars), 5, 10, 15, 20, 25.
	if f.EndTime != 25 {
		t.Errorf("EndTime=%d; want 25", f.EndTime)
	}
	// Look for the b0 ! change at time=5.
	found := false
	for _, c := range f.Changes {
		if c.Time == 5 && c.ID == "!" && c.Value == "0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected change {time:5 id:! val:0} in changes")
	}
}

func TestParseVCDScalarSequence(t *testing.T) {
	// Multiple scalar changes on one line: "1\" 0# 1$".
	src := `$timescale 1ns $end
$scope module top $end
$var reg 1 a clk $end
$var reg 1 b en $end
$var reg 1 c rst $end
$upscope $end
$enddefinitions $end
#0
0a 0b 0c
#10
1a
1b
1c
`
	f, err := ParseVCD(src)
	if err != nil {
		t.Fatalf("ParseVCD: %v", err)
	}
	if len(f.Signals) != 3 {
		t.Errorf("Signals len=%d; want 3", len(f.Signals))
	}
	// We expect 6 changes total (3 at t=0, 3 at t=10).
	if len(f.Changes) != 6 {
		t.Errorf("Changes len=%d; want 6 — got %+v", len(f.Changes), f.Changes)
	}
}

func TestDetectTestbench(t *testing.T) {
	src := `
module dut(input a, output b);
endmodule
module tb;
initial begin
    $dumpvars(0, tb);
end
endmodule
`
	got := detectTestbench(src)
	if got != "tb" {
		t.Errorf("detectTestbench=%q; want %q", got, "tb")
	}
}

func TestDetectTestbenchNone(t *testing.T) {
	src := `module dut(input a); endmodule`
	got := detectTestbench(src)
	if got != "" {
		t.Errorf("expected empty (no testbench), got %q", got)
	}
}

func TestGenerateAutoTestbenchSimple(t *testing.T) {
	mod := &Module{
		Ports: map[string]*Port{
			"clk":   {Direction: "input", Bits: []interface{}{2.0}},
			"rst_n": {Direction: "input", Bits: []interface{}{3.0}},
			"en":    {Direction: "input", Bits: []interface{}{4.0}},
			"count": {Direction: "output", Bits: []interface{}{5.0, 6.0, 7.0, 8.0}},
		},
	}
	src, err := generateAutoTestbench("counter", mod, "__auto_tb")
	if err != nil {
		t.Fatalf("generateAutoTestbench: %v", err)
	}
	// Sanity checks: it must mention $dumpvars, the DUT name, and our wires.
	for _, want := range []string{
		"module __auto_tb;",
		"$dumpvars",
		"counter dut",
		".clk(clk)",
		".rst_n(rst_n)",
		"$finish",
	} {
		if !contains(src, want) {
			t.Errorf("auto-testbench missing %q\n--- source ---\n%s", want, src)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
