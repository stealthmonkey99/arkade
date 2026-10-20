// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io/fs"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// SpriteEl displays a fixed pixel grid loaded from data, a PNG, or a
// JSON array of pixel rows. (Named SpriteEl to avoid clashing with
// display.Sprite, the pixel-grid type.)
type SpriteEl struct {
	Base
	data display.Sprite
}

// SpriteOptions configures a sprite element.
type SpriteOptions struct {
	Options
	Data display.Sprite
}

// NewSprite builds a sprite element from raw pixel data.
func NewSprite(opts SpriteOptions) *SpriteEl {
	s := &SpriteEl{data: opts.Data}
	if s.data == nil {
		s.data = display.Sprite{{}}
	}
	s.InitBase(opts.Options)
	s.Impl = s.renderImpl
	return s
}

func (s *SpriteEl) renderImpl(clock.FrameTime) display.Sprite { return s.data }

// SetData swaps the sprite's pixels.
func (s *SpriteEl) SetData(data display.Sprite) {
	s.data = data
	s.dirty = true
}

// Data returns the sprite's pixels.
func (s *SpriteEl) Data() display.Sprite { return s.data }

// DecodePNG converts PNG bytes into a pixel grid.
func DecodePNG(raw []byte) (display.Sprite, error) {
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}
	bounds := img.Bounds()
	data := make(display.Sprite, bounds.Dy())
	for y := 0; y < bounds.Dy(); y++ {
		row := make([]display.Pixel, bounds.Dx())
		for x := 0; x < bounds.Dx(); x++ {
			c := img.At(bounds.Min.X+x, bounds.Min.Y+y)
			r, g, b, a := c.RGBA()
			if a == 0 {
				row[x] = display.Transparent
			} else {
				// RGBA() returns premultiplied 16-bit; unpremultiply.
				row[x] = display.Pixel{
					R: uint8((r * 0xffff / a) >> 8),
					G: uint8((g * 0xffff / a) >> 8),
					B: uint8((b * 0xffff / a) >> 8),
					A: uint8(a >> 8),
				}
			}
		}
		data[y] = row
	}
	return data, nil
}

// jsonPixel is one pixel of the JSON sprite format ({red,green,blue,alpha}).
type jsonPixel struct {
	Red   uint8 `json:"red"`
	Green uint8 `json:"green"`
	Blue  uint8 `json:"blue"`
	Alpha uint8 `json:"alpha"`
}

// DecodeJSONSprite converts JSON sprite bytes into a pixel grid.
func DecodeJSONSprite(raw []byte) (display.Sprite, error) {
	var rows [][]jsonPixel
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("decode json sprite: %w", err)
	}
	data := make(display.Sprite, len(rows))
	for y, row := range rows {
		out := make([]display.Pixel, len(row))
		for x, p := range row {
			out[x] = display.Pixel{R: p.Red, G: p.Green, B: p.Blue, A: p.Alpha}
		}
		data[y] = out
	}
	return data, nil
}

// SpriteFromPNG loads a sprite element from a PNG in an fs.FS (embed.FS).
func SpriteFromPNG(fsys fs.FS, path string, opts Options) (*SpriteEl, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	data, err := DecodePNG(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return NewSprite(SpriteOptions{Options: opts, Data: data}), nil
}

// SpriteFromJSON loads a sprite element from JSON in an fs.FS.
func SpriteFromJSON(fsys fs.FS, path string, opts Options) (*SpriteEl, error) {
	raw, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	data, err := DecodeJSONSprite(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return NewSprite(SpriteOptions{Options: opts, Data: data}), nil
}
