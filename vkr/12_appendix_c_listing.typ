#import "@docs/gost732-2017:0.5.0": *

#let _signed = sys.inputs.at("signed", default: "") == "true"

#ненумерованный_заголовок(содержание: [ПРИЛОЖЕНИЕ В. Исходный текст модуля])[Приложение В]

#metadata("cover") <appendix-cover>

#align(center)[
  #text(size: 14pt, weight: "bold")[Исходный текст модуля]

  Листов 2
]

#pagebreak()
#metadata("content-start") <appendix-content-start>

#set heading(outlined: false)
#set figure(numbering: (.., n) => [В.#n])

#page(align(left, image(if _signed { "/img/signed_full/prilozhenie_v.jpg" } else { "img/v_titul_croped.jpg" }, width: 100%)), margin: if _signed { 0pt } else { (left: 3cm, right: 1.5cm, top: 2cm, bottom: 2cm) }, footer: if _signed { none } else { align(center)[#text(fill: white)[1]] })

Исходный текст модуля yosys.go, выполняющего десериализацию списка соединений в формате Yosys JSON и предоставляющего основные структуры данных для работы с описанием цифрового устройства, приведён в #ref(<list:yosys-go>, supplement: [листинге]).

#листинг(```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

// Design is the top-level Yosys JSON structure.
type Design struct {
    Creator string              `json:"creator"`
    Modules map[string]*Module  `json:"modules"`
}

// Module represents a single Yosys module.
type Module struct {
    Attributes map[string]string    `json:"attributes"`
    Ports      map[string]*Port     `json:"ports"`
    Cells      map[string]*Cell     `json:"cells"`
    NetNames   map[string]*NetName  `json:"netnames"`
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

// GetTopModule returns the top module of the design.
func GetTopModule(d *Design) (string, *Module) {
    for name, mod := range d.Modules {
        if mod.Attributes["top"] != "" {
            return name, mod
        }
    }
    for name, mod := range d.Modules {
        return name, mod
    }
    return "", nil
}
```)[Текст модуля yosys.go] <list:yosys-go>
