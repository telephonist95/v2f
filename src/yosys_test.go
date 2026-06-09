package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBitToNetID(t *testing.T) {
	cases := []struct {
		name      string
		in        interface{}
		wantID    int
		wantNumOK bool
	}{
		{"float", 5.0, 5, true},
		{"json.Number int", json.Number("42"), 42, true},
		{"json.Number invalid", json.Number("abc"), 0, false},
		{"string const 0", "0", 0, false},
		{"string const 1", "1", 0, false},
		{"string x", "x", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := BitToNetID(c.in)
			if id != c.wantID || ok != c.wantNumOK {
				t.Errorf("BitToNetID(%v) = (%d,%v); want (%d,%v)", c.in, id, ok, c.wantID, c.wantNumOK)
			}
		})
	}
}

func TestGetTopModuleDeterministic(t *testing.T) {
	d := &Design{
		Modules: map[string]*Module{
			"zzz_top": {Attributes: map[string]string{"top": "00000000000000000000000000000001"}},
			"aaa_lib": {Attributes: map[string]string{}},
			"mmm_lib": {Attributes: map[string]string{}},
		},
	}
	// Run many times; result must be identical.
	first, _ := GetTopModule(d)
	for i := 0; i < 50; i++ {
		name, mod := GetTopModule(d)
		if name != first || mod == nil {
			t.Fatalf("iter %d: non-deterministic top selection: got %q (first=%q)", i, name, first)
		}
		if name != "zzz_top" {
			t.Fatalf("iter %d: expected zzz_top (marked top), got %q", i, name)
		}
	}
}

func TestGetTopModuleFallbackSorted(t *testing.T) {
	d := &Design{
		Modules: map[string]*Module{
			"zzz": {Attributes: map[string]string{}},
			"aaa": {Attributes: map[string]string{}},
			"mmm": {Attributes: map[string]string{}},
		},
	}
	for i := 0; i < 30; i++ {
		name, _ := GetTopModule(d)
		if name != "aaa" {
			t.Fatalf("iter %d: expected fallback to alphabetically first module 'aaa', got %q", i, name)
		}
	}
}

func TestParseDesignMinimal(t *testing.T) {
	js := `{
		"creator": "test",
		"modules": {
			"foo": {
				"attributes": { "top": "1" },
				"ports": {
					"a": { "direction": "input", "bits": [ 2 ] },
					"y": { "direction": "output", "bits": [ 3 ] }
				},
				"cells": {},
				"netnames": {}
			}
		}
	}`
	tmpFile := t.TempDir() + "/d.json"
	if err := writeFile(tmpFile, []byte(js)); err != nil {
		t.Fatal(err)
	}
	d, err := ParseDesign(tmpFile)
	if err != nil {
		t.Fatalf("ParseDesign: %v", err)
	}
	if len(d.Modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(d.Modules))
	}
	mod := d.Modules["foo"]
	if mod == nil {
		t.Fatal("module foo missing")
	}
	if mod.Attributes["top"] != "1" {
		t.Errorf("Attributes[top] = %q; want \"1\"", mod.Attributes["top"])
	}
	if !strings.EqualFold(mod.Ports["a"].Direction, "input") {
		t.Errorf("port a direction = %q", mod.Ports["a"].Direction)
	}
}
