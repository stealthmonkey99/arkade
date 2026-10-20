// SPDX-License-Identifier: GPL-2.0-or-later

// Command manifestgen writes each cartridge directory's manifest_gen.go,
// the compiled-in runtime manifest cart_manifest returns, from its
// manifest.json (sdk/docs/packaging.md, "One manifest, two forms"). Every
// cartridge build runs it before `tinygo build`; the generated file is
// committed so a clean clone builds without it.
//
//	manifestgen [<dir>...]          # regenerate; "." when no directory is named
//	manifestgen -check [<dir>...]   # exit 1 naming each missing or stale file
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stealthmonkey99/arkade/sdk/tools/manifestgen/gen"
)

func main() {
	check := flag.Bool("check", false, "report stale or missing files instead of writing them")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: manifestgen [-check] [<cartridge dir>...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	dirs := flag.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}

	failed := false
	for _, dir := range dirs {
		if *check {
			if err := gen.Check(dir); err != nil {
				fmt.Fprintf(os.Stderr, "manifestgen: %s: %v\n", dir, err)
				failed = true
			}
			continue
		}
		changed, err := gen.Write(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "manifestgen: %s: %v\n", dir, err)
			failed = true
			continue
		}
		if changed {
			fmt.Printf("manifestgen: wrote %s/%s\n", dir, gen.FileName)
		}
	}
	if failed {
		os.Exit(1)
	}
}
