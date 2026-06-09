package main

import "strings"

// ResolveCell returns Falstad mapping info for a Yosys cell type.
// It first checks the static cellMapping, then tries to generate
// a CustomLogic model dynamically for FF/latch cell types.
// Returns ok=false if the cell type is unsupported.
func ResolveCell(yosysType string) (cm cellMap, ok bool) {
	if m, found := cellMapping[yosysType]; found {
		return m, true
	}

	// Try dynamic generation for FF/latch types.
	model, inPorts, outPorts, generated := generateSequentialCell(yosysType)
	if !generated {
		return cellMap{}, false
	}

	// Register the generated model and mapping.
	PredefinedModels[model.Name] = model
	cm = cellMap{FalstadCustom, model.Name, inPorts, outPorts}
	cellMapping[yosysType] = cm
	return cm, true
}

// generateSequentialCell creates a Falstad CustomLogic model for Yosys
// DFF, DFFE, SDFF, SDFFE, SDFFCE, DLATCH, and SR cell types.
func generateSequentialCell(yosysType string) (model *CustomModel, inPorts, outPorts []string, ok bool) {
	name := strings.TrimPrefix(yosysType, "$_")
	name = strings.TrimSuffix(name, "_")

	idx := strings.LastIndex(name, "_")
	if idx < 0 {
		return nil, nil, nil, false
	}
	base := name[:idx]
	params := name[idx+1:]

	edge := func(c byte) string {
		if c == 'P' {
			return "+"
		}
		return "-"
	}
	pol := func(c byte) string {
		if c == 'P' {
			return "1"
		}
		return "0"
	}
	inv := func(c byte) string {
		if c == 'P' {
			return "0"
		}
		return "1"
	}

	var inputs []string
	var rules []string

	switch base {
	case "DFF":
		switch len(params) {
		case 1:
			// $_DFF_<C>_: Inputs: C, D. On clock edge, Q=D.
			inputs = []string{"C", "D"}
			rules = []string{edge(params[0]) + "a=a"}
		case 3:
			// $_DFF_<C><R><V>_: Inputs: C, R, D. Async reset.
			inputs = []string{"C", "R", "D"}
			rules = []string{
				"?" + pol(params[1]) + "?=" + string(params[2]),
				edge(params[0]) + inv(params[1]) + "a=a",
			}
		}

	case "DFFE":
		switch len(params) {
		case 2:
			// $_DFFE_<C><E>_: Inputs: C, D, E. Enable.
			inputs = []string{"C", "D", "E"}
			rules = []string{edge(params[0]) + "a" + pol(params[1]) + "=a"}
		case 4:
			// $_DFFE_<C><R><V><E>_: Inputs: C, R, D, E. Async reset + enable.
			inputs = []string{"C", "R", "D", "E"}
			rules = []string{
				"?" + pol(params[1]) + "??=" + string(params[2]),
				edge(params[0]) + inv(params[1]) + "a" + pol(params[3]) + "=a",
			}
		}

	case "SDFF":
		if len(params) == 3 {
			// $_SDFF_<C><R><V>_: Inputs: C, R, D. Sync reset.
			inputs = []string{"C", "R", "D"}
			rules = []string{
				edge(params[0]) + pol(params[1]) + "?=" + string(params[2]),
				edge(params[0]) + inv(params[1]) + "a=a",
			}
		}

	case "SDFFE":
		if len(params) == 4 {
			// $_SDFFE_<C><R><V><E>_: Inputs: C, R, D, E.
			// Sync reset has priority over enable.
			inputs = []string{"C", "R", "D", "E"}
			rules = []string{
				edge(params[0]) + pol(params[1]) + "??=" + string(params[2]),
				edge(params[0]) + inv(params[1]) + "a" + pol(params[3]) + "=a",
			}
		}

	case "SDFFCE":
		if len(params) == 4 {
			// $_SDFFCE_<C><R><V><E>_: Inputs: C, R, D, E.
			// Clock enable gates everything including sync reset.
			inputs = []string{"C", "R", "D", "E"}
			rules = []string{
				edge(params[0]) + pol(params[1]) + "?" + pol(params[3]) + "=" + string(params[2]),
				edge(params[0]) + inv(params[1]) + "a" + pol(params[3]) + "=a",
			}
		}

	case "DLATCH":
		switch len(params) {
		case 1:
			// $_DLATCH_<E>_: Inputs: E, D. Transparent latch.
			inputs = []string{"E", "D"}
			rules = []string{
				pol(params[0]) + "0=0",
				pol(params[0]) + "1=1",
			}
		case 3:
			// $_DLATCH_<E><R><V>_: Inputs: E, R, D. Latch + async reset.
			inputs = []string{"E", "R", "D"}
			rules = []string{
				"?" + pol(params[1]) + "?=" + string(params[2]),
				pol(params[0]) + inv(params[1]) + "0=0",
				pol(params[0]) + inv(params[1]) + "1=1",
			}
		}

	case "SR":
		if len(params) == 2 {
			// $_SR_<S><R>_: Inputs: S, R. Edge-triggered SR latch, R has priority.
			inputs = []string{"S", "R"}
			sEdge := edge(params[0])
			rEdge := edge(params[1])
			rules = []string{
				"?" + rEdge + "=0",
				sEdge + "?=1",
			}
		}

	default:
		return nil, nil, nil, false
	}

	if len(inputs) == 0 {
		return nil, nil, nil, false
	}

	model = &CustomModel{
		Name:    name,
		Inputs:  inputs,
		Outputs: []string{"Q"},
		Rules:   rules,
	}
	return model, inputs, []string{"Q"}, true
}

// IsSequentialCell returns true if the Yosys cell type is a sequential element
// (flip-flop, latch, or SR) that should be placed in the FF column.
func IsSequentialCell(cellType string) bool {
	for _, s := range []string{"DFF", "SDFF", "DLATCH", "_SR_", "_FF_"} {
		if strings.Contains(cellType, s) {
			return true
		}
	}
	return false
}
