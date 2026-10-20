// SPDX-License-Identifier: GPL-2.0-or-later

// Command rcartpack builds a .rcart cartridge bundle from a project
// directory: manifest.json, the compiled .wasm and an optional assets/ tree,
// tarred uncompressed into <slug>.rcart (docs/CARTRIDGE.md, ".rcart bundle
// format"). `make rcart DIR=...` is the usual entry point.
//
// The built tar is fed back through the loader's own validation before
// anything is written, so a bundle the cabinet would reject, or could only
// mount degraded, fails at pack time instead. The rules are package pack's.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stealthmonkey99/arkade/sdk/tools/rcartpack/pack"
)

func main() {
	var opts pack.Options
	flag.StringVar(&opts.Dir, "dir", "", "cartridge project directory (must contain manifest.json and a game .wasm; an assets/ subdirectory is optional)")
	flag.StringVar(&opts.Wasm, "wasm", "", "path to the compiled cartridge .wasm (default: <dir>/game.wasm, else <dir>/<manifest slug>.wasm)")
	flag.StringVar(&opts.Out, "out", "", "output .rcart path (defaults to <dir>/<manifest slug>.rcart)")
	flag.Parse()
	if opts.Dir == "" {
		fmt.Fprintln(os.Stderr, "usage: rcartpack -dir <cartridge project dir> [-wasm game.wasm] [-out out.rcart]")
		os.Exit(2)
	}
	res, err := pack.Pack(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rcartpack: %v\n", err)
		os.Exit(1)
	}
	for _, w := range res.Cosmetic {
		fmt.Fprintf(os.Stderr, "rcartpack: warning: %s\n", w)
	}
	fmt.Println("rcartpack:", res)
}
