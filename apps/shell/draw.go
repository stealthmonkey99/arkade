// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func newCanvas(w, h int) display.Sprite {
	out := make(display.Sprite, h)
	for y := range out {
		out[y] = make([]display.Pixel, w)
	}
	return out
}

func spriteW(s display.Sprite) int {
	w := 0
	for _, row := range s {
		if len(row) > w {
			w = len(row)
		}
	}
	return w
}

// blitWindow composites src at (ox,oy), drawing only columns within
// [minX, maxX) — the clip window marquees scroll inside.
func blitWindow(dst, src display.Sprite, ox, oy, minX, maxX int) {
	for y, row := range src {
		dy := oy + y
		if dy < 0 || dy >= len(dst) {
			continue
		}
		for x, p := range row {
			dx := ox + x
			if dx < minX || dx >= maxX || dx < 0 || dx >= len(dst[dy]) || p.A == 0 {
				continue
			}
			dst[dy][dx] = display.Merge(dst[dy][dx], p)
		}
	}
}

func blit(dst, src display.Sprite, ox, oy int) {
	blitWindow(dst, src, ox, oy, 0, 1<<30)
}

func fillRect(dst display.Sprite, x, y, w, h int, p display.Pixel) {
	for dy := y; dy < y+h; dy++ {
		if dy < 0 || dy >= len(dst) {
			continue
		}
		for dx := x; dx < x+w; dx++ {
			if dx < 0 || dx >= len(dst[dy]) {
				continue
			}
			dst[dy][dx] = display.Merge(dst[dy][dx], p)
		}
	}
}

// drawQRModal centers a QR symbol on a white card over a dimming scrim, with an outline beyond the card.
func drawQRModal(out, symbol display.Sprite, outline display.Pixel) {
	if len(symbol) == 0 {
		return
	}
	fillRect(out, 0, 0, screenW, screenH, display.Pixel{A: 120})
	const pad = 2
	w := spriteW(symbol) + 2*pad
	h := len(symbol) + 2*pad
	x := (screenW - w) / 2
	y := (screenH - h) / 2
	fillRect(out, x, y, w, h, display.White)
	blit(out, symbol, x+pad, y+pad)
	ui.StrokeRect(&display.SpriteCanvas{Sprite: out}, x-1, y-1, w+2, h+2, display.ToUI(outline))
}

// The pixel marquee holds at the start, crawls left and wraps through a gap.
const (
	marqueeHoldMs  = 900
	marqueeMsPerPx = 55
	marqueeGap     = 16
)

func marqueeOff(textW, window int, ms float64) int {
	if textW <= window || ms < 0 {
		return 0
	}
	span := textW + marqueeGap
	cycle := marqueeHoldMs + float64(span)*marqueeMsPerPx
	t := math.Mod(ms, cycle)
	if t < marqueeHoldMs {
		return 0
	}
	return int((t-marqueeHoldMs)/marqueeMsPerPx) % span
}

// drawMarquee draws src at (x,y) scrolling within window px when too wide.
func drawMarquee(dst, src display.Sprite, x, y, window int, ms float64) {
	textW := spriteW(src)
	if textW <= window {
		blitWindow(dst, src, x, y, x, x+window)
		return
	}
	off := marqueeOff(textW, window, ms)
	span := textW + marqueeGap
	blitWindow(dst, src, x-off, y, x, x+window)
	blitWindow(dst, src, x-off+span, y, x, x+window)
}

const (
	lcdHoldMs = 1200
	lcdStepMs = 350
	lcdGap    = "   "
)

// lcdWindow returns the 16-column view of s at ms since the anchor, scrolling when it does not fit.
func lcdWindow(s string, ms float64) string {
	runes := []rune(s)
	if len(runes) <= hardware.LcdCols {
		return s
	}
	loop := append(append([]rune{}, runes...), []rune(lcdGap)...)
	n := len(loop)
	cycle := lcdHoldMs + float64(n)*lcdStepMs
	t := math.Mod(math.Max(ms, 0), cycle)
	off := 0
	if t >= lcdHoldMs {
		off = int((t-lcdHoldMs)/lcdStepMs) % n
	}
	doubled := append(append([]rune{}, loop...), loop...)
	return string(doubled[off : off+hardware.LcdCols])
}
