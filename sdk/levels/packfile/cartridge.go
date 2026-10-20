// SPDX-License-Identifier: GPL-2.0-or-later

package packfile

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The cartridge layout every build and preflight assume. A cartridge's pack
// files live in AssetsDir beside its sprites and sounds, and compile to
// GenFile beside its other sources; the packer leaves them out of the .rcart,
// since nothing reads a pack at run time. Only files ending in [ArkadeExt] or
// [Ext] count as packs. A cartridge that wants generation-time checks
// ([Hook], [IndexedHook], [PackHook]) puts its own generator, whose main
// calls [Main], in GeneratorDir; without one, tools/genlevelpack generates
// with no checks.
const (
	AssetsDir    = "assets"
	GenFile      = "levels_gen.go"
	GeneratorDir = "tools/genlevels"
)

// HasPacks reports whether cartDir's [AssetsDir] holds at least one pack
// file. A missing directory is not an error: the cartridge simply has no
// levels.
func HasPacks(cartDir string) (bool, error) {
	entries, err := os.ReadDir(filepath.Join(cartDir, AssetsDir))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if !e.IsDir() && HasExt(e.Name()) {
			return true, nil
		}
	}
	return false, nil
}

// HasGenerator reports whether cartDir carries its own level generator in
// [GeneratorDir].
func HasGenerator(cartDir string) bool {
	info, err := os.Stat(filepath.Join(cartDir, filepath.FromSlash(GeneratorDir)))
	return err == nil && info.IsDir()
}

// CartridgeConfig is the [Config] for cartDir under the standard layout, with
// no hooks: the packs in [AssetsDir] in, [GenFile] out, package main.
func CartridgeConfig(cartDir string) Config {
	return Config{
		Dir: filepath.Join(cartDir, AssetsDir),
		Out: filepath.Join(cartDir, GenFile),
		Pkg: "main",
	}
}

// An Output is one generated file: where it goes and the bytes it must hold.
type Output struct {
	Path string // file to write
	Src  []byte // exact contents it must hold
}

// An Extra renders a further generated file from the validated packs, for a
// generator whose cartridge compiles more than the levels.Pack literals (an
// id table, a derived attract-mode script). It runs only after every hook has
// passed.
type Extra func(files []*File) (Output, error)

// Sync brings each output's file to its Src. A file already holding exactly
// Src is not rewritten. With check set it writes nothing and instead returns
// an error naming every output that is missing or differs.
func Sync(check bool, outs ...Output) error {
	var stale []string
	for _, o := range outs {
		have, err := os.ReadFile(o.Path)
		switch {
		case err == nil && bytes.Equal(have, o.Src):
			continue
		case err != nil && !errors.Is(err, os.ErrNotExist):
			return err
		case check && err != nil:
			stale = append(stale, "no "+filepath.Base(o.Path))
		case check:
			stale = append(stale, filepath.Base(o.Path)+" differs")
		default:
			if err := os.WriteFile(o.Path, o.Src, 0o644); err != nil {
				return err
			}
		}
	}
	if len(stale) > 0 {
		return &StaleError{Files: stale}
	}
	return nil
}

// StaleError is what [Sync], [Check] and a generator's -check report: the
// generated files that are missing or no longer what the pack files generate,
// whether because a pack changed since the last build or because someone
// edited the generated Go.
type StaleError struct {
	Files   []string // "no levels_gen.go" or "levels_gen.go differs", one per file
	Sources []string // the pack files they are generated from, as assets/<name>
}

// Error lists the stale files and their sources.
func (e *StaleError) Error() string {
	msg := strings.Join(e.Files, ", ")
	if len(e.Sources) > 0 {
		msg += " from " + strings.Join(e.Sources, ", ")
	}
	return msg + " (stale or hand-edited)"
}

// Check renders cfg and its extras exactly as a build would and reports a
// [*StaleError] when cfg.Out or any extra's file is missing or differs. It
// writes nothing. A hook error or an unparseable pack is returned as is.
func Check(cfg Config, extras ...Extra) error {
	return run(cfg, true, extras)
}

// Main is the whole main function of a cartridge's own generator in
// [GeneratorDir]: it fills an empty cfg.Dir, cfg.Out and cfg.Pkg from
// [CartridgeConfig] of the working directory, renders the packs through the
// hooks, and writes cfg.Out and every extra's file, leaving up-to-date files
// untouched. Run with -check, it writes nothing and exits 1 when any of them
// is stale. The build runs it from the cartridge directory before TinyGo.
//
// A hook failure, an unparseable pack, or a stale file under -check prints
// the error and exits 1, having written nothing.
func Main(cfg Config, extras ...Extra) {
	check := flag.Bool("check", false, "write nothing; exit 1 if a generated file is missing or stale")
	flag.Parse()
	prog := filepath.Base(os.Args[0])
	if wd, err := os.Getwd(); err == nil {
		def := CartridgeConfig(wd)
		if cfg.Dir == "" {
			cfg.Dir = def.Dir
		}
		if cfg.Out == "" {
			cfg.Out = def.Out
		}
	}
	if err := run(cfg, *check, extras); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", prog, err)
		os.Exit(1)
	}
}

func run(cfg Config, check bool, extras []Extra) error {
	if cfg.Out == "" {
		return errors.New("packfile: Config.Out is required")
	}
	files, src, err := Render(cfg)
	if err != nil {
		return err
	}
	outs := []Output{{Path: cfg.Out, Src: src}}
	for _, x := range extras {
		o, err := x(files)
		if err != nil {
			return err
		}
		outs = append(outs, o)
	}
	err = Sync(check, outs...)
	var stale *StaleError
	if errors.As(err, &stale) {
		for _, f := range files {
			stale.Sources = append(stale.Sources, filepath.ToSlash(filepath.Join(filepath.Base(cfg.Dir), filepath.Base(f.Path))))
		}
	}
	return err
}
