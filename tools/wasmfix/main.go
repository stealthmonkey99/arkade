// SPDX-License-Identifier: GPL-2.0-or-later

// Command wasmfix rewrites a TinyGo-built cartridge's cart_manifest export into a
// real two-result wasm function. TinyGo lowers any Go function with more than one
// result through a hidden sret pointer parameter, even with +multivalue forced on,
// but the cart_manifest() -> (ptr, len) ABI in docs/CARTRIDGE.md and the wasm host
// require genuine (i32, i32) results.
//
// wasmpatch checks that the export's body stores exactly two compile-time constants
// into the sret slot, appends a new () -> (i32, i32) function returning them, and
// repoints the export. Any other body shape fails loudly rather than emitting a
// wrong manifest.
//
//	wasmfix -in cart.wasm -out cart.wasm -export cart_manifest
//
// in and out may be the same path. It works for any TinyGo export with exactly two
// i32 results.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stealthmonkey99/arkade/tools/wasmfix/wasmpatch"
)

func main() {
	in := flag.String("in", "", "input .wasm path")
	out := flag.String("out", "", "output .wasm path")
	exportName := flag.String("export", "cart_manifest", "exported function name to fix up into a real 2xi32-result function")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: wasmfix -in cart.wasm -out cart.wasm [-export cart_manifest]")
		os.Exit(2)
	}
	if err := run(*in, *out, *exportName); err != nil {
		fmt.Fprintf(os.Stderr, "wasmfix: %v\n", err)
		os.Exit(1)
	}
}

func run(inPath, outPath, exportName string) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inPath, err)
	}
	patched, ptr, length, err := wasmpatch.PatchSretExport(data, exportName)
	if err != nil {
		return fmt.Errorf("patch %q: %w", exportName, err)
	}
	if err := os.WriteFile(outPath, patched, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	fmt.Printf("wasmfix: %q now returns (ptr=%#x, len=%d) as a genuine 2xi32 wasm export (%s -> %s)\n", exportName, ptr, length, inPath, outPath)
	return nil
}
