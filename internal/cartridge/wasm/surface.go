// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"

	"github.com/stealthmonkey99/arkade/internal/display"
)

type surfaceImports struct {
	host interface {
		Surface
		Logger
	}
	dims frameDims
}

func (s surfaceImports) blit(_ context.Context, mod api.Module, ptr, length uint32) {
	want := uint32(s.dims.cols * s.dims.rows * 4)
	if want == 0 || length != want || length > maxBlitBytes {
		hostWarnf(s.host, "host_blit: length %d, want %d", length, want)
		return
	}
	raw, ok := mod.Memory().Read(ptr, length)
	if !ok {
		hostWarnf(s.host, "host_blit: ptr %#x/%d out of bounds", ptr, length)
		return
	}
	pixels := make([]display.Pixel, s.dims.cols*s.dims.rows)
	for i := range pixels {
		o := i * 4
		pixels[i] = display.Pixel{R: raw[o], G: raw[o+1], B: raw[o+2], A: raw[o+3]}
	}
	s.host.Blit(pixels)
}

// drawPixel is host_draw_pixel: it sets one pixel to a packed 0xRRGGBBAA
// colour, ignoring a point off the canvas.
func (s surfaceImports) drawPixel(_ context.Context, _ api.Module, x, y int32, rgba uint32) {
	if int(x) < 0 || int(y) < 0 || int(x) >= s.dims.cols || int(y) >= s.dims.rows {
		return
	}
	s.host.DrawPixel(int(x), int(y), decodeColor(rgba))
}

// fillRect is host_fill_rect: it fills a rectangle with a packed 0xRRGGBBAA
// colour, clipped to the canvas; a negative size warns and draws nothing.
func (s surfaceImports) fillRect(_ context.Context, _ api.Module, x, y, w, h int32, rgba uint32) {
	if w < 0 || h < 0 {
		hostWarnf(s.host, "host_fill_rect: negative size %dx%d", w, h)
		return
	}
	s.host.FillRect(int(x), int(y), int(w), int(h), decodeColor(rgba))
}

// loadSprite is host_load_sprite: it decodes up to 1 MiB of PNG bytes into a
// sprite and returns its handle, or 0 on failure.
func (s surfaceImports) loadSprite(_ context.Context, mod api.Module, ptr, length uint32) uint32 {
	png, ok := guestBytes(s.host, mod, "host_load_sprite:", ptr, length, maxSpriteBytes)
	if !ok {
		return 0
	}
	id, err := s.host.LoadSprite(png)
	if err != nil {
		hostWarnf(s.host, "host_load_sprite: %v", err)
		return 0
	}
	return id
}

// drawSprite is host_draw_sprite: it draws the sprite loaded as id at x, y;
// an unknown handle, 0 included, draws nothing.
func (s surfaceImports) drawSprite(_ context.Context, _ api.Module, id uint32, x, y int32) {
	s.host.DrawSprite(id, int(x), int(y))
}

func decodeColor(rgba uint32) display.Pixel {
	return display.Pixel{
		R: uint8(rgba >> 24),
		G: uint8(rgba >> 16),
		B: uint8(rgba >> 8),
		A: uint8(rgba),
	}
}
