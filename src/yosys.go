package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Design is the top-level Yosys JSON structure.
type Design struct {
	Creator string             `json:"creator"`
	Modules map[string]*Module `json:"modules"`
}

// Module represents a single Yosys module.
type Module struct {
	Attributes map[string]string   `json:"attributes"`
	Ports      map[string]*Port    `json:"ports"`
	Cells      map[string]*Cell    `json:"cells"`
	NetNames   map[string]*NetName `json:"netnames"`
}

// Port represents a module I/O port.
type Port struct {
	Direction string        `json:"direction"`
	Bits      []interface{} `json:"bits"`
}

// Cell represents a logic cell instance.
type Cell struct {
	HideName       int                      `json:"hide_name"`
	Type           string                   `json:"type"`
	Parameters     map[string]interface{}   `json:"parameters"`
	Attributes     map[string]interface{}   `json:"attributes"`
	PortDirections map[string]string        `json:"port_directions"`
	Connections    map[string][]interface{} `json:"connections"`
}

// NetName represents a named net.
type NetName struct {
	HideName   int                    `json:"hide_name"`
	Bits       []interface{}          `json:"bits"`
	Attributes map[string]interface{} `json:"attributes"`
}

// ParseDesign reads and parses a Yosys JSON file.
func ParseDesign(filename string) (*Design, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var design Design
	if err := json.Unmarshal(data, &design); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return &design, nil
}

// BitToNetID extracts an integer net ID from a JSON bits element.
// Returns the net ID and true if it's a numeric net ID,
// or 0 and false if it's a string constant ("0","1","x","z").
func BitToNetID(bit interface{}) (int, bool) {
	switch v := bit.(type) {
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	}
	return 0, false
}

// GetTopModule returns the module marked top (attribute "top" is non-empty).
// If multiple modules are marked top or none are, returns the alphabetically
// first match — this keeps the choice deterministic across runs.
func GetTopModule(d *Design) (string, *Module) {
	names := make([]string, 0, len(d.Modules))
	for name := range d.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		mod := d.Modules[name]
		if mod.Attributes["top"] != "" {
			return name, mod
		}
	}
	if len(names) > 0 {
		first := names[0]
		return first, d.Modules[first]
	}
	return "", nil
}
