// SPDX-License-Identifier: GPL-2.0-or-later

// Command gen-sheet packs Catcher's hand-editable PNGs into the single
// sprite sheet the cartridge loads from its bundle at runtime. Run it from
// the cartridge directory:
//
//	go run ./tools/gen-sheet
//	make sheet                      # the same thing
//
// It reads assets/sprites/*.png, the editable source of truth, and writes
// assets/catcher.arkadesprite.png: one image carrying every cel, plus an
// "arkade" envelope naming the cel rectangles and the walk animation
// (docs/ASSETS.md, "The envelope"). The cartridge loads the sheet from its
// bundle by cel name (docs/ASSETS.md, "Sprites from the bundle").
//
// Cels are laid out left to right in one row, in a fixed order, so unchanged
// PNGs rewrite byte-identical output. This is host Go, never compiled into
// the .wasm.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// sources lists the cels in sheet order. A strip contributes one cel per
// frame, cut at celW. Cel names are looked up by the cartridge ("catcher.0",
// "fruit"), so renaming one here means renaming it there.
var sources = []struct {
	file  string // under assets/sprites
	name  string // cel name, or the prefix for a strip's "<name>.<i>"
	cels  int    // 1 for a plain sprite, N for a strip
	celW  int    // cel width; 0 means "the whole image"
	celH  int
	strip bool
}{
	{file: "catcher.png", name: "catcher", cels: 3, celW: 7, celH: 9, strip: true},
	{file: "fruit.png", name: "fruit", cels: 1},
}

// walk is the walk cycle: frames name a cel and a hold, so "catcher.0"
// appears twice and is still one drawing.
var walk = assetfile.Animation{
	Name: "walk",
	Frames: []assetfile.Frame{
		{Cel: "catcher.0", HoldMs: 130},
		{Cel: "catcher.1", HoldMs: 130},
		{Cel: "catcher.0", HoldMs: 130},
		{Cel: "catcher.2", HoldMs: 130},
	},
	Loop: assetfile.LoopCycle,
}

const (
	srcDir = "assets/sprites"
	outPNG = "assets/catcher.arkadesprite.png"
)

func main() {
	s, err := buildSheet(".")
	if err != nil {
		log.Fatalf("gen-sheet: %v", err)
	}
	if err := os.WriteFile(outPNG, s.file, 0o644); err != nil {
		log.Fatalf("gen-sheet: %v", err)
	}
	fmt.Printf("gen-sheet: wrote %s (%dx%d, %d cels, %d bytes)\n", outPNG, s.w, s.h, s.cels, len(s.file))
}

type packedSheet struct {
	file       []byte
	w, h, cels int
}

// buildSheet packs the sources under root into the sheet file's bytes.
func buildSheet(root string) (packedSheet, error) {
	type piece struct {
		img  image.Image
		name string
		w, h int
	}
	var pieces []piece
	for _, s := range sources {
		img, err := readPNG(filepath.Join(root, srcDir, s.file))
		if err != nil {
			return packedSheet{}, err
		}
		b := img.Bounds()
		celW, celH := s.celW, s.celH
		if celW == 0 {
			celW, celH = b.Dx(), b.Dy()
		}
		if s.strip && celW*s.cels != b.Dx() {
			return packedSheet{}, fmt.Errorf("%s is %dpx wide, but %d cels of %dpx need %d",
				s.file, b.Dx(), s.cels, celW, celW*s.cels)
		}
		for i := 0; i < s.cels; i++ {
			name := s.name
			if s.strip {
				name = fmt.Sprintf("%s.%d", s.name, i)
			}
			sub := image.Rect(b.Min.X+i*celW, b.Min.Y, b.Min.X+i*celW+celW, b.Min.Y+celH)
			pieces = append(pieces, piece{
				img:  cropped{img, sub},
				name: name,
				w:    celW,
				h:    celH,
			})
		}
	}

	sheetW, sheetH := 0, 0
	for _, p := range pieces {
		sheetW += p.w
		if p.h > sheetH {
			sheetH = p.h
		}
	}
	sheet := image.NewNRGBA(image.Rect(0, 0, sheetW, sheetH))
	cels := make([]assetfile.Cel, 0, len(pieces))
	x := 0
	for _, p := range pieces {
		draw.Draw(sheet, image.Rect(x, 0, x+p.w, p.h), p.img, p.img.Bounds().Min, draw.Src)
		cels = append(cels, assetfile.Cel{
			Name: p.name, X: x, Y: 0, W: p.w, H: p.h,
			// Catcher draws from the top-left corner, so the anchor is the origin
			Anchor: [2]int{0, 0},
		})
		x += p.w
	}

	var base bytes.Buffer
	if err := png.Encode(&base, sheet); err != nil {
		return packedSheet{}, fmt.Errorf("encode sheet: %w", err)
	}

	env := &assetfile.Envelope{
		Format:      assetfile.FormatVersion,
		Kind:        assetfile.KindSprite,
		Name:        "catcher",
		Description: "Catcher's whole cast: the three-cel walk strip and the fruit.",
		Tags:        []string{"tutorial"},
		Generator:   assetfile.Generator{Tool: "gen-sheet", Version: "1.0.0"},
		SpritePayload: assetfile.SpritePayload{
			Cels:       cels,
			Animations: []assetfile.Animation{walk},
		},
	}
	out, err := assetfile.WriteBytes(filepath.Base(outPNG), env, base.Bytes())
	if err != nil {
		return packedSheet{}, fmt.Errorf("write envelope: %w", err)
	}
	// decode it the way the host will, so an unloadable sheet is never written
	if _, err := assetfile.DecodeSheet(filepath.Base(outPNG), out); err != nil {
		return packedSheet{}, fmt.Errorf("verify %s: %w", outPNG, err)
	}
	return packedSheet{file: out, w: sheetW, h: sheetH, cels: len(cels)}, nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return img, nil
}

// cropped narrows an image to one rectangle without depending on the
// concrete type's own SubImage method.
type cropped struct {
	image.Image
	r image.Rectangle
}

func (c cropped) Bounds() image.Rectangle { return c.r }
