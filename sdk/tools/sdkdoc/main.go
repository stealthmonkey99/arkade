// SPDX-License-Identifier: GPL-2.0-or-later

// Command sdkdoc generates the SDK's API reference from its doc comments:
// one Markdown page per SDK package under sdk/docs/api, and the import and
// export tables inside sdk/docs/abi.md. The doc comments are the source;
// the pages are never edited by hand.
//
//	sdkdoc [-sdk <dir>]          # regenerate (make sdk-docs)
//	sdkdoc -check [-sdk <dir>]   # exit 1 naming each stale or missing page
//
// -sdk is the SDK root, the directory holding cartlib/ and docs/; it
// defaults to ./sdk, which is right from public/arkade.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stealthmonkey99/arkade/sdk/tools/sdkdoc/gen"
)

func main() {
	check := flag.Bool("check", false, "report stale or missing pages instead of writing them")
	sdk := flag.String("sdk", "sdk", "the SDK root directory")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: sdkdoc [-check] [-sdk <dir>]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}
	run := gen.Write
	if *check {
		run = gen.Check
	}
	if err := run(*sdk); err != nil {
		fmt.Fprintf(os.Stderr, "sdkdoc: %v\n", err)
		os.Exit(1)
	}
}
