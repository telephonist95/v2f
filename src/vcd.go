package main

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// VCDSignal describes a named signal extracted from VCD $var declarations.
// ID is the short VCD identifier code (the printable character soup like "!", "#",
// "%", etc. used in the time-value section).
type VCDSignal struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Width int      `json:"width"`
	Scope []string `json:"scope"`
	Kind  string   `json:"kind"` // wire / reg / parameter / ...
}

// VCDChange is one signal transition at a given simulation time.
// Value is a string holding either a binary representation (no leading "b") for
// multi-bit signals, "0"/"1"/"x"/"z" for single-bit, or a real number prefixed
// with "r" for $var real signals.
type VCDChange struct {
	Time  int64  `json:"time"`
	ID    string `json:"id"`
	Value string `json:"value"`
}

// VCDFile holds the structured form of a VCD file: timescale, signal table,
// and changes sorted by time.
type VCDFile struct {
	Timescale string      `json:"timescale"`
	Date      string      `json:"date,omitempty"`
	Version   string      `json:"version,omitempty"`
	Signals   []VCDSignal `json:"signals"`
	Changes   []VCDChange `json:"changes"`
	EndTime   int64       `json:"endTime"`
}

// ParseVCD parses a VCD text into a VCDFile structure.
//
// The parser is intentionally minimal — it understands $date, $version,
// $timescale, $scope/$upscope, $var declarations, $dumpvars/$dumpall blocks,
// time markers (#<n>), single-bit values (0/1/x/z directly followed by id),
// and vector values (b<bits> id, r<real> id). $comment and $end tokens are
// recognised but their contents are ignored.
func ParseVCD(text string) (*VCDFile, error) {
	f := &VCDFile{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	// VCD lines can be long when signals are wide.
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

	var scope []string
	var pendingSection string // accumulates content until $end
	var pendingBuf strings.Builder

	var currentTime int64

	flushPending := func() {
		switch pendingSection {
		case "$timescale":
			f.Timescale = strings.TrimSpace(pendingBuf.String())
		case "$date":
			f.Date = strings.TrimSpace(pendingBuf.String())
		case "$version":
			f.Version = strings.TrimSpace(pendingBuf.String())
		}
		pendingSection = ""
		pendingBuf.Reset()
	}

	processVar := func(fields []string) error {
		// $var <kind> <width> <id> <name> [bit_select] $end
		if len(fields) < 6 {
			return fmt.Errorf("malformed $var: %q", strings.Join(fields, " "))
		}
		kind := fields[1]
		width, err := strconv.Atoi(fields[2])
		if err != nil {
			return fmt.Errorf("bad $var width %q", fields[2])
		}
		id := fields[3]
		name := fields[4]
		// Append bit select (e.g. "[3:0]") if present, before the $end.
		for i := 5; i < len(fields); i++ {
			if fields[i] == "$end" {
				break
			}
			name += " " + fields[i]
		}
		sig := VCDSignal{ID: id, Name: name, Width: width, Kind: kind}
		sig.Scope = append(sig.Scope, scope...)
		f.Signals = append(f.Signals, sig)
		return nil
	}

	processValueChange := func(token string) {
		switch token[0] {
		case 'b', 'B':
			// b<bits> <id>
			parts := strings.SplitN(token[1:], " ", 2)
			if len(parts) == 2 {
				f.Changes = append(f.Changes, VCDChange{
					Time:  currentTime,
					ID:    parts[1],
					Value: parts[0],
				})
			}
		case 'r', 'R':
			parts := strings.SplitN(token[1:], " ", 2)
			if len(parts) == 2 {
				f.Changes = append(f.Changes, VCDChange{
					Time:  currentTime,
					ID:    parts[1],
					Value: "r" + parts[0],
				})
			}
		case '0', '1', 'x', 'X', 'z', 'Z':
			if len(token) >= 2 {
				f.Changes = append(f.Changes, VCDChange{
					Time:  currentTime,
					ID:    token[1:],
					Value: string(token[0]),
				})
			}
		}
	}

	inDumpBlock := false

	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		// Accumulate content of multi-line declarations like $date, $timescale.
		if pendingSection != "" {
			if line == "$end" {
				flushPending()
				continue
			}
			if pendingBuf.Len() > 0 {
				pendingBuf.WriteByte(' ')
			}
			pendingBuf.WriteString(line)
			continue
		}

		// Tokenize per whitespace; a single line may contain multiple value changes.
		fields := strings.Fields(line)
		i := 0
		for i < len(fields) {
			tok := fields[i]
			switch {
			case tok == "$date" || tok == "$version" || tok == "$timescale" || tok == "$comment":
				pendingSection = tok
				pendingBuf.Reset()
				// If the closing $end is on the same line, flush immediately.
				closeIdx := -1
				for k := i + 1; k < len(fields); k++ {
					if fields[k] == "$end" {
						closeIdx = k
						break
					}
				}
				if closeIdx > 0 {
					if closeIdx-1 > i {
						pendingBuf.WriteString(strings.Join(fields[i+1:closeIdx], " "))
					}
					flushPending()
					i = closeIdx + 1
				} else {
					if i+1 < len(fields) {
						pendingBuf.WriteString(strings.Join(fields[i+1:], " "))
					}
					i = len(fields)
				}

			case tok == "$scope":
				// $scope <type> <name> $end
				if i+3 < len(fields) {
					scope = append(scope, fields[i+2])
					i += 4
				} else {
					i = len(fields)
				}

			case tok == "$upscope":
				if len(scope) > 0 {
					scope = scope[:len(scope)-1]
				}
				// consume "$end" if present
				if i+1 < len(fields) && fields[i+1] == "$end" {
					i += 2
				} else {
					i++
				}

			case tok == "$var":
				// rest of fields on this line is the declaration
				if err := processVar(fields[i:]); err != nil {
					return nil, err
				}
				i = len(fields)

			case tok == "$enddefinitions":
				// consume "$end"
				if i+1 < len(fields) && fields[i+1] == "$end" {
					i += 2
				} else {
					i++
				}

			case tok == "$dumpvars" || tok == "$dumpall" || tok == "$dumpon" || tok == "$dumpoff":
				inDumpBlock = true
				i++

			case tok == "$end":
				inDumpBlock = false
				i++

			case strings.HasPrefix(tok, "#"):
				t, err := strconv.ParseInt(tok[1:], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("bad time marker %q", tok)
				}
				currentTime = t
				if t > f.EndTime {
					f.EndTime = t
				}
				i++

			case tok[0] == 'b' || tok[0] == 'B' || tok[0] == 'r' || tok[0] == 'R':
				// Vector: needs next token as identifier.
				if i+1 < len(fields) {
					processValueChange(tok + " " + fields[i+1])
					i += 2
				} else {
					i++
				}

			default:
				// Scalar value change: "0!", "1$", "x#"…
				if len(tok) >= 2 && isScalarValue(tok[0]) {
					processValueChange(tok)
				}
				i++
			}
		}
		_ = inDumpBlock // currently unused; reserved for stricter parsing
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan VCD: %w", err)
	}

	sort.SliceStable(f.Changes, func(i, j int) bool {
		return f.Changes[i].Time < f.Changes[j].Time
	})
	return f, nil
}

func isScalarValue(b byte) bool {
	switch b {
	case '0', '1', 'x', 'X', 'z', 'Z':
		return true
	}
	return false
}
