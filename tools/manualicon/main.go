// SPDX-License-Identifier: GPL-2.0-or-later

// Command manualicon writes <cartridge>/manual-icon.png for every cartridge
// in the library named by ARKADE_CARTRIDGE_ROOTS (comma-separated cartridge
// roots): the 8x8 icon (assets/icon.arkadesprite.png, else assets/icon.png)
// upscaled to 128x128 with nearest-neighbor, for the cartridge's MANUAL.md.
// GitHub markdown strips style attributes, so the upscale is baked in. The
// file sits at the cartridge root so bundle staging, which copies assets/*,
// never packs it.
//
//	ARKADE_CARTRIDGE_ROOTS=<library>/cartridges go run ./tools/manualicon
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"

	"github.com/stealthmonkey99/arkade/tools/fleet"
)

const scale = 16 // 8x8 -> 128x128

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `usage: %s=<root>[,<root>...] go run ./tools/manualicon

Writes manual-icon.png beside each cartridge's MANUAL.md from its 8x8 icon.
Each root is a directory of cartridge directories, e.g. arkade-cartridges/cartridges.
`, fleet.RootsEnv)
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	dirs, slugs, err := fleet.Dirs(repoRoot)
	if err != nil {
		fatal(err)
	}
	made := 0
	for _, slug := range slugs {
		src := filepath.Join(dirs[slug], "assets", "icon.arkadesprite.png")
		if _, err := os.Stat(src); err != nil {
			src = filepath.Join(dirs[slug], "assets", "icon.png")
		}
		dst := filepath.Join(dirs[slug], "manual-icon.png")
		if err := upscale(src, dst); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("skip %s: no assets/icon.arkadesprite.png or assets/icon.png\n", slug)
				continue
			}
			fatal(fmt.Errorf("%s: %w", slug, err))
		}
		fmt.Printf("wrote %s\n", dst)
		made++
	}
	fmt.Printf("%d manual icons\n", made)
}

func upscale(srcPath, dstPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return err
	}
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale))
	for y := 0; y < b.Dy()*scale; y++ {
		for x := 0; x < b.Dx()*scale; x++ {
			out.Set(x, y, src.At(b.Min.X+x/scale, b.Min.Y+y/scale))
		}
	}
	g, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer g.Close()
	return png.Encode(g, out)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "manualicon:", err)
	os.Exit(1)
}
