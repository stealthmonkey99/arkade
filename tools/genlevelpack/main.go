// SPDX-License-Identifier: GPL-2.0-or-later

// Command genlevelpack compiles a cartridge's level packs, assets/*.arkadelevels,
// into levels_gen.go, the levels.Pack literals the cartridge builds in. Every
// cartridge build runs it before TinyGo; the generated file is committed build
// output, like manifest_gen.go, and is never edited by hand.
//
//	genlevelpack [<cartridge dir>...]          # regenerate; "." when none is named
//	genlevelpack -check [<cartridge dir>...]   # exit 1 naming each stale file
//
// A cartridge with no pack in assets/ is skipped. One with its own
// generator in tools/genlevels (sdk/levels/packfile's Main, carrying the
// cartridge's hooks) is handed to it: `go run ./tools/genlevels [-check]` from
// the cartridge directory. Every other cartridge is generated here with no
// per-game checks. An up-to-date file is left untouched.
//
//	genlevelpack -dir <dir> -out <file> [-pkg <name>] [-check]
//
// compiles one directory of packs into one file for a package that is not a
// cartridge, such as sdk/levels/example.
//
//	genlevelpack -fmt <cartridge dir>/assets/foo.arkadelevels
//
// rewrites one pack file in place to the canonical shape and refuses an
// unparseable pack, writing nothing. It is idempotent.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

func main() {
	check := flag.Bool("check", false, "write nothing; exit 1 if a generated file is missing or stale")
	dir := flag.String("dir", "", "with -out: a directory of pack files that is not a cartridge's assets/")
	out := flag.String("out", "", "with -dir: the generated .go file")
	pkg := flag.String("pkg", "main", "with -dir: package name for the generated file")
	fmtFile := flag.String("fmt", "", "canonicalize this one pack file in place, then exit")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: genlevelpack [-check] [<cartridge dir>...]\n       genlevelpack -dir <dir> -out <file> [-pkg <name>] [-check]\n       genlevelpack -fmt <pack file>")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *fmtFile != "" {
		if err := runFmt(*fmtFile); err != nil {
			fmt.Fprintln(os.Stderr, "genlevelpack:", err)
			os.Exit(1)
		}
		return
	}

	if *dir != "" || *out != "" {
		if *dir == "" || *out == "" {
			fmt.Fprintln(os.Stderr, "genlevelpack: -dir and -out go together")
			os.Exit(2)
		}
		cfg := packfile.Config{Dir: *dir, Out: *out, Pkg: *pkg}
		if err := generate(cfg, *check); err != nil {
			fmt.Fprintf(os.Stderr, "genlevelpack: %s: %v\n", *out, err)
			os.Exit(1)
		}
		return
	}

	carts := flag.Args()
	if len(carts) == 0 {
		carts = []string{"."}
	}
	failed := false
	for _, cart := range carts {
		if err := cartridge(cart, *check); err != nil {
			fmt.Fprintf(os.Stderr, "genlevelpack: %s: %v\n", cart, err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

func cartridge(dir string, check bool) error {
	ok, err := packfile.HasPacks(dir)
	if err != nil || !ok {
		return err
	}
	if packfile.HasGenerator(dir) {
		return runGenerator(dir, check)
	}
	return generate(packfile.CartridgeConfig(dir), check)
}

func generate(cfg packfile.Config, check bool) error {
	if check {
		return packfile.Check(cfg)
	}
	_, err := packfile.Generate(cfg)
	return err
}

// runGenerator hands a cartridge to its own generator, which lives in the
// cartridge's module and so cannot be linked in here.
func runGenerator(dir string, check bool) error {
	args := []string{"run", "./" + packfile.GeneratorDir}
	if check {
		args = append(args, "-check")
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", packfile.GeneratorDir, err)
	}
	return nil
}

// runFmt canonicalizes path in place, writing nothing if the pack does not parse.
func runFmt(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := packfile.Format(path, src)
	if err != nil {
		return err
	}
	if string(out) == string(src) {
		fmt.Printf("genlevelpack: %s is already canonical\n", path)
		return nil
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}
	fmt.Printf("genlevelpack: formatted %s\n", path)
	return nil
}
