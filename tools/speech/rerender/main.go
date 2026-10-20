// SPDX-License-Identifier: GPL-2.0-or-later

// Command rerender speaks every speech asset under the given roots again
// from its own recipe, so a build can check the committed audio still
// matches what the recipe makes. A speech asset is a sound envelope whose
// recipe is a lone node with params kind "speech"; it is found by its
// envelope, not by its name. Each render is written under -out, and the
// committed/rendered pairs are printed one per line for
// `assetdiff -tolerance`:
//
//	go run ./tools/speech/rerender -out /tmp/speech ../arkade-cartridges/cartridges
//
// A speech node inside a mix is reported and skipped: only soundlab's
// mixer renders those.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/tools/sfxlib"
	"github.com/stealthmonkey99/arkade/tools/speech"
)

type asset struct {
	path   string
	rate   int
	params speech.Params
}

func main() {
	out := flag.String("out", "", "directory the renders are written to (required)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: rerender -out <dir> <root> [<root> ...]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *out == "" || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*out, flag.Args()); err != nil {
		fmt.Fprintf(os.Stderr, "rerender: %v\n", err)
		os.Exit(1)
	}
}

func run(out string, roots []string) error {
	var found []asset
	for _, root := range roots {
		a, err := findSpeech(root)
		if err != nil {
			return err
		}
		found = append(found, a...)
	}
	if len(found) == 0 {
		fmt.Fprintln(os.Stderr, "rerender: no speech assets found")
		return nil
	}
	r := speech.NewRenderer()
	defer r.Close()
	for i, a := range found {
		samples, err := a.params.Render(r, a.rate)
		if err != nil {
			return fmt.Errorf("%s: %w", a.path, err)
		}
		dir := filepath.Join(out, fmt.Sprintf("%03d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.Base(a.path))
		if err := sfxlib.WriteFile(dst, samples, a.rate); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "rerender: %s (%q)\n", a.path, a.params.Text)
		fmt.Printf("%s %s\n", a.path, dst)
	}
	return nil
}

// findSpeech walks root for speech assets, in lexical order.
func findSpeech(root string) ([]asset, error) {
	var out []asset
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if kind, ok := assetfile.KindFromFilename(d.Name()); !ok || kind != assetfile.KindSound {
			return nil
		}
		if strings.ContainsAny(path, " \t\n") {
			return fmt.Errorf("%q: whitespace in a path would break the pair list", path)
		}
		a, ok, err := speechAsset(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if ok {
			out = append(out, a)
		}
		return nil
	})
	return out, err
}

func speechAsset(path string) (asset, bool, error) {
	env, err := assetfile.Read(path)
	if err != nil {
		return asset{}, false, err
	}
	if env.Kind != assetfile.KindSound {
		return asset{}, false, nil
	}
	recipe, err := env.SoundRecipe()
	if err != nil || recipe == nil {
		return asset{}, false, err
	}
	var lines []*speech.Params
	for _, n := range recipe.Compose {
		p, err := speech.UnmarshalParams(n.Params)
		if err != nil {
			return asset{}, false, err
		}
		if p != nil {
			lines = append(lines, p)
		}
	}
	switch {
	case len(lines) == 0:
		return asset{}, false, nil
	case recipe.Mix != nil || len(recipe.Compose) != 1:
		fmt.Fprintf(os.Stderr, "rerender: SKIP %s: speech inside a mix renders only through soundlab\n", path)
		return asset{}, false, nil
	case env.Render == nil:
		return asset{}, false, errors.New("sound envelope has no render block")
	}
	return asset{path: path, rate: env.Render.SampleRate, params: *lines[0]}, true, nil
}
