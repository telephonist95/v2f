package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	rtlMode := flag.Bool("rtl", false, "emit RTL-level Falstad circuit")
	serveMode := flag.Bool("serve", false, "start HTTP server")
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	staticDir := flag.String("static", "vkr/dist", "static web assets directory")
	flag.Parse()

	if *serveMode {
		if err := ServeHTTP(*addr, *staticDir); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: ver2fal [-rtl] <yosys_json_file>\n       ver2fal -serve [-addr 127.0.0.1:8080] [-static vkr/dist]\n")
		os.Exit(1)
	}

	filename := args[0]

	design, err := ParseDesign(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing design: %v\n", err)
		os.Exit(1)
	}

	_, mod := GetTopModule(design)
	if mod == nil {
		fmt.Fprintf(os.Stderr, "Error: no modules found in design\n")
		os.Exit(1)
	}

	var cir *Circuit
	if *rtlMode {
		cir, err = ConvertRTL(mod)
	} else {
		cir, err = Convert(mod)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error converting: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(EmitFalstad(cir))
}
