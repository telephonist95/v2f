package main

import (
	"os"
	"testing"
)

// writeFile is a tiny helper used by other test files in this package.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

func TestModuleNames(t *testing.T) {
	src := `
module foo (input a, output b);
endmodule
module tb_foo;
endmodule
module bar (input x);
endmodule
`
	got := moduleNames(src)
	want := []string{"foo", "tb_foo", "bar"}
	if len(got) != len(want) {
		t.Fatalf("moduleNames returned %v; want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("moduleNames[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestModuleNamesDeduplicates(t *testing.T) {
	src := `module foo; endmodule
module foo; endmodule`
	got := moduleNames(src)
	if len(got) != 1 || got[0] != "foo" {
		t.Errorf("expected single 'foo', got %v", got)
	}
}

func TestInferTopModuleSkipsTestbench(t *testing.T) {
	src := `
module tb_top;
endmodule
module dut (input a, output b);
endmodule
`
	if got := inferTopModule(src); got != "dut" {
		t.Errorf("inferTopModule chose %q; want \"dut\" (skipping tb_*)", got)
	}
}

func TestInferTopModuleSkipsTest(t *testing.T) {
	src := `
module test_runner;
endmodule
module real_design;
endmodule
`
	if got := inferTopModule(src); got != "real_design" {
		t.Errorf("inferTopModule chose %q; want \"real_design\"", got)
	}
}

func TestInferTopModuleFallbackToFirst(t *testing.T) {
	src := `module tb_only; endmodule`
	if got := inferTopModule(src); got != "tb_only" {
		t.Errorf("expected fallback to first ('tb_only'), got %q", got)
	}
}

func TestInferTopModuleEmpty(t *testing.T) {
	if got := inferTopModule("// no modules here"); got != "" {
		t.Errorf("expected empty string for source with no modules, got %q", got)
	}
}
