// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// Panel timings for the power-off sequence, from the instant PowerOff was
// called: hold, collapse to a line, shrink to a dot, glow out (a CRT shutdown).
const (
	powerOffHoldMs     = 400
	powerOffCollapseMs = 600
	powerOffShrinkMs   = 400
	powerOffGlowMs     = 700

	powerOffShrinkAtMs = powerOffHoldMs + powerOffCollapseMs
	powerOffGlowAtMs   = powerOffShrinkAtMs + powerOffShrinkMs
	powerOffDeadMs     = powerOffGlowAtMs + powerOffGlowMs
)

// powerOffSpark is phosphor white with a trace of blue.
var powerOffSpark = [3]float64{0.9, 0.92, 1}

// Collapse brightness boost and white lift; blue lifts harder so the picture
// cools toward the spark colour.
const (
	powerOffBoost = 2.2
	powerOffLift  = 0.25
	powerOffLiftB = 0.3
)

// powerOffFrame renders the animation over last, the frame on the panel when
// PowerOff was called, at elapsedMs since. Past powerOffDeadMs it is black.
// Pure function of its inputs.
func powerOffFrame(last display.Matrix, elapsedMs float64) display.Matrix {
	h := len(last)
	w := 0
	if h > 0 {
		w = len(last[0])
	}
	if elapsedMs < powerOffHoldMs {
		return display.CloneMatrix(last)
	}

	out := display.NewMatrix(w, h, display.Black)
	switch p := easeInQuad((elapsedMs - powerOffHoldMs) / powerOffCollapseMs); {
	case p < 1:
		powerOffCollapse(out, last, p)
	case elapsedMs < powerOffGlowAtMs:
		powerOffLine(out, EaseInOut.at((elapsedMs-powerOffShrinkAtMs)/powerOffShrinkMs))
	default:
		powerOffDot(out, clamp01((elapsedMs-powerOffGlowAtMs)/powerOffGlowMs))
	}
	return out
}

// powerOffCollapse maps the source rows onto a band closing on the middle row,
// brightening as it narrows.
func powerOffCollapse(out, last display.Matrix, p float64) {
	h, w := len(out), 0
	if h > 0 {
		w = len(out[0])
	}
	scale := math.Max(1/float64(h), 1-p)
	boost := 1 + powerOffBoost*p
	mid, centre := float64(h)/2, (float64(h)-1)/2
	for y := 0; y < h; y++ {
		sy := mid + (float64(y)-centre)/scale
		if sy < 0 || sy >= float64(h) {
			continue
		}
		src := last[int(sy)]
		for x := 0; x < w && x < len(src); x++ {
			out[y][x] = display.Pixel{
				R: burn(src[x].R, boost, powerOffLift*p),
				G: burn(src[x].G, boost, powerOffLift*p),
				B: burn(src[x].B, boost, powerOffLiftB*p),
				A: 255,
			}
		}
	}
}

func powerOffLine(out display.Matrix, p float64) {
	h, w := len(out), 0
	if h > 0 {
		w = len(out[0])
	}
	half := float64(w)/2*(1-p) + 1
	centre := (float64(w) - 1) / 2
	for x := 0; x < w; x++ {
		d := math.Abs(float64(x) - centre)
		if d >= half {
			continue
		}
		a := 0.6 + 0.4*(1-d/half)
		spark(out, x, h/2-1, a*0.5)
		spark(out, x, h/2, a)
	}
}

func powerOffDot(out display.Matrix, p float64) {
	h, w := len(out), 0
	if h > 0 {
		w = len(out[0])
	}
	v := (1 - p) * (1 - p)
	const radius = 3
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			d2 := float64(dx*dx + dy*dy)
			if d2 > radius*radius+1 {
				continue
			}
			spark(out, w/2+dx, h/2+dy, v/(1+1.6*d2))
		}
	}
}

// spark adds the phosphor colour at alpha, clipping rather than blending.
func spark(out display.Matrix, x, y int, alpha float64) {
	if alpha <= 0 || y < 0 || y >= len(out) || x < 0 || x >= len(out[y]) {
		return
	}
	p := out[y][x]
	out[y][x] = display.Pixel{
		R: addByte(p.R, powerOffSpark[0]*alpha),
		G: addByte(p.G, powerOffSpark[1]*alpha),
		B: addByte(p.B, powerOffSpark[2]*alpha),
		A: 255,
	}
}

func burn(v uint8, boost, lift float64) uint8 {
	return byteOf(float64(v)/255*boost + lift)
}

func addByte(v uint8, add float64) uint8 {
	return byteOf(float64(v)/255 + add)
}

func byteOf(v float64) uint8 {
	return uint8(math.Round(clamp01(v) * 255))
}

func easeInQuad(p float64) float64 {
	p = clamp01(p)
	return p * p
}
