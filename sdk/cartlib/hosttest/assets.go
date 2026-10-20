// SPDX-License-Identifier: GPL-2.0-or-later

package hosttest

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MountAssets serves every sprite sheet under dir/assets the way the
// cabinet serves a mounted bundle: each .png carrying an arkade sprite
// envelope becomes loadable by its bundle-relative path
// ("assets/title.arkadesprite.png"), and each of its cels, anchor
// included, lifts with Sheet.Cel. A .png without an envelope is skipped,
// as the cabinet refuses it. Call it before [Host.Init], usually with "."
// from the cartridge's own directory; a missing assets directory mounts
// nothing. It is carttest.FakeHost.MountAssets for an in-process test.
//
// It returns an error for a sheet the cabinet would refuse to load: an
// envelope that does not parse, a cel outside the image or over 64 pixels
// on a side.
func (h *Host) MountAssets(dir string) error {
	root := filepath.Join(dir, "assets")
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".png") {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		cels, err := sheetCels(data)
		if err != nil {
			return fmt.Errorf("hosttest: %s: %w", rel, err)
		}
		if cels == nil {
			return nil
		}
		path := filepath.ToSlash(rel)
		if h.cels[path] == nil {
			h.cels[path] = map[string]Cel{}
		}
		for name, c := range cels {
			h.cels[path][name] = c
		}
		return nil
	})
}

type spriteEnvelope struct {
	Kind string `json:"kind"`
	Cels []struct {
		Name   string `json:"name"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		W      int    `json:"w"`
		H      int    `json:"h"`
		Anchor [2]int `json:"anchor"`
	} `json:"cels"`
}

// sheetCels cuts a sprite sheet into its named cels as straight-alpha RGBA,
// or returns nil for a PNG with no sprite envelope.
func sheetCels(data []byte) (map[string]Cel, error) {
	js, err := envelopeJSON(data)
	if err != nil || js == nil {
		return nil, err
	}
	var env spriteEnvelope
	if err := json.Unmarshal(js, &env); err != nil {
		return nil, fmt.Errorf("arkade envelope: %w", err)
	}
	if env.Kind != "sprite" {
		return nil, nil
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		nrgba = image.NewNRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				nrgba.Set(x, y, img.At(x, y))
			}
		}
	}
	cels := map[string]Cel{}
	for _, c := range env.Cels {
		if c.W < 1 || c.H < 1 || c.W > 64 || c.H > 64 {
			return nil, fmt.Errorf("cel %q is %dx%d, outside 1 to 64 pixels a side", c.Name, c.W, c.H)
		}
		if c.X < 0 || c.Y < 0 || c.X+c.W > b.Dx() || c.Y+c.H > b.Dy() {
			return nil, fmt.Errorf("cel %q at (%d, %d) %dx%d leaves the %dx%d image", c.Name, c.X, c.Y, c.W, c.H, b.Dx(), b.Dy())
		}
		rgba := make([]byte, 0, c.W*c.H*4)
		for y := 0; y < c.H; y++ {
			row := nrgba.PixOffset(b.Min.X+c.X, b.Min.Y+c.Y+y)
			rgba = append(rgba, nrgba.Pix[row:row+c.W*4]...)
		}
		cels[c.Name] = Cel{W: c.W, H: c.H, AnchorX: clampU16(c.Anchor[0]), AnchorY: clampU16(c.Anchor[1]), RGBA: rgba}
	}
	return cels, nil
}

// clampU16 is the cabinet's range for an anchor, which host_cel_info
// reports as a u16.
func clampU16(v int) int { return min(max(v, 0), 0xFFFF) }

// envelopeJSON returns the "arkade" tEXt or zTXt chunk's text, or nil when
// the PNG has none.
func envelopeJSON(data []byte) ([]byte, error) {
	const sig = "\x89PNG\r\n\x1a\n"
	if !bytes.HasPrefix(data, []byte(sig)) {
		return nil, errors.New("not a PNG")
	}
	for at := len(sig); at+8 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[at:]))
		typ := string(data[at+4 : at+8])
		if n < 0 || at+12+n > len(data) {
			return nil, errors.New("truncated PNG chunk")
		}
		body := data[at+8 : at+8+n]
		at += 12 + n
		kw, rest, ok := bytes.Cut(body, []byte{0})
		if !ok || string(kw) != "arkade" {
			continue
		}
		switch typ {
		case "tEXt":
			return rest, nil
		case "zTXt":
			if len(rest) < 1 || rest[0] != 0 {
				return nil, errors.New("arkade zTXt: unknown compression method")
			}
			zr, err := zlib.NewReader(bytes.NewReader(rest[1:]))
			if err != nil {
				return nil, fmt.Errorf("arkade zTXt: %w", err)
			}
			return io.ReadAll(zr)
		}
	}
	return nil, nil
}
