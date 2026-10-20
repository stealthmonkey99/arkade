// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
)

// CelRGBA extracts the w×h rectangle at (x0, y0) of img as straight-alpha
// RGBA bytes, row-major, len w*h*4, matching ui.Sprite.Pix. The build-time
// sprite generator and the runtime host both call it so their bytes agree.
// Pixels outside img's bounds read as transparent.
func CelRGBA(img image.Image, x0, y0, w, h int) []byte {
	pix := make([]byte, 0, w*h*4)
	nrgba, isNRGBA := img.(*image.NRGBA)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a uint8
			if isNRGBA {
				c := nrgba.NRGBAAt(x0+x, y0+y)
				r, g, b, a = c.R, c.G, c.B, c.A
			} else {
				pr, pg, pb, pa := img.At(x0+x, y0+y).RGBA()
				if pa == 0 {
					r, g, b, a = 0, 0, 0, 0
				} else {
					r = uint8((pr * 0xFFFF / pa) >> 8)
					g = uint8((pg * 0xFFFF / pa) >> 8)
					b = uint8((pb * 0xFFFF / pa) >> 8)
					a = uint8(pa >> 8)
				}
			}
			pix = append(pix, r, g, b, a)
		}
	}
	return pix
}

// Sheet is a decoded sprite sheet: its validated envelope and pixels.
type Sheet struct {
	Envelope *Envelope
	Image    image.Image
	Width    int
	Height   int
}

// DecodeSheet reads a sprite sheet's PNG bytes: the envelope validated
// against the decoded image bounds, plus the pixels. name is used only for
// the naming rule, as in ReadBytes.
func DecodeSheet(name string, data []byte) (*Sheet, error) {
	env, err := ReadBytes(name, data)
	if err != nil {
		return nil, err
	}
	if env.Kind != KindSprite {
		return nil, fmt.Errorf("assetfile: %s is a %q envelope, not a sprite sheet", name, env.Kind)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("assetfile: decode %s pixels: %w", name, err)
	}
	b := img.Bounds()
	// A lying IHDR would otherwise let a cel rectangle past the check.
	if err := Validate(env, KindSprite, b.Dx(), b.Dy()); err != nil {
		return nil, err
	}
	return &Sheet{Envelope: env, Image: img, Width: b.Dx(), Height: b.Dy()}, nil
}

// Cel returns the named cel's rectangle, offset by the image's bounds origin
// so it feeds CelRGBA directly, or ok=false when the sheet has no such cel.
func (s *Sheet) Cel(name string) (x, y, w, h int, ok bool) {
	b := s.Image.Bounds()
	for _, c := range s.Envelope.Cels {
		if c.Name == name {
			return b.Min.X + c.X, b.Min.Y + c.Y, c.W, c.H, true
		}
	}
	return 0, 0, 0, 0, false
}

// SheetHeader reports a PNG's pixel dimensions and whether it is interlaced,
// read from the IHDR without decoding. Callers cap the dimensions before
// decoding, and refuse Adam7 because the browser host's decoder cannot read
// it.
func SheetHeader(data []byte) (w, h int, interlaced bool, err error) {
	chunks, w, h, err := loadPNGChunks(data)
	if err != nil {
		return 0, 0, false, err
	}
	// The interlace flag is IHDR byte 12; loadPNGChunks checked only the
	// first eight.
	if len(chunks[0].Data) < 13 {
		return 0, 0, false, errors.New("assetfile: PNG IHDR is truncated")
	}
	return w, h, chunks[0].Data[12] != 0, nil
}
