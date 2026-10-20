// SPDX-License-Identifier: GPL-2.0-or-later

package boot

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/brand"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The panel size the artwork is laid out for; splashFrame clips to the actual size.
const (
	panelW = 64
	panelH = 64
)

// Timeline in ms from splash start.
const (
	pilotUntilMs = 520
	pilotPeriod  = 140

	ringTravelMs = 900
	ringReachPx  = 60
	ringFadePx   = 62 // radius at which a ring's alpha reaches zero

	wordFadeAtMs = 550
	wordFadeMs   = 450

	glowRiseAtMs = 600
	glowPeakAtMs = 950
	glowRestAtMs = 1600
	glowPeak     = 1.3

	bulbChaseAtMs = 1000
	bulbChaseMs   = 1100
	bulbRiseMs    = 220
)

// voiceTints are the ring colours, one per chord voice in bloom order.
var voiceTints = [bloomVoices]brand.RGB{
	brand.PixelRGB(display.FromUI(ui.ButtonGreen)),
	brand.PixelRGB(display.FromUI(ui.ButtonRed)),
	brand.PixelRGB(display.FromUI(ui.ButtonYellow)),
	brand.PixelRGB(display.FromUI(ui.ButtonBlue)),
	brand.PixelRGB(display.White),
}

// splashFrame paints the splash at elapsedMs since it began: a pilot ring,
// five voice rings, the wordmark glow and the marquee chase. Pure in time.
func splashFrame(w, h int, elapsedMs float64) display.Matrix {
	c := brand.NewCanvas(w, h)
	t := math.Max(0, elapsedMs)

	if t < pilotUntilMs {
		pilot := 0.22 + 0.12*math.Sin(t/pilotPeriod)
		for _, p := range brand.RingPixels {
			c.Add(p, brand.Amber, pilot)
		}
	}

	for voice := 0; voice < bloomVoices; voice++ {
		ring(c, t-float64(stingerAtMs+voice*bloomStepMs), voiceTints[voice])
	}

	if word := wordAlphaAt(t); word > 0 {
		c.Wordmark(word, glowAt(t)*word)
	}

	marquee(c, t)
	return c.Matrix()
}

// ring expands one voice's antialiased 1px ring, dimming as it grows.
// sinceOnset < 0 means the voice has not entered.
func ring(c *brand.Canvas, sinceOnset float64, tint brand.RGB) {
	if sinceOnset < 0 {
		return
	}
	r := sinceOnset / ringTravelMs * ringReachPx
	if r > ringFadePx {
		return
	}
	alpha := math.Pow(1-r/ringFadePx, 1.3) * 0.85
	w, h := c.Bounds()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			edge := math.Abs(math.Hypot(float64(x-brand.RingCenter.X), float64(y-brand.RingCenter.Y)) - r)
			if edge < 1 {
				c.Add(brand.Point{X: x, Y: y}, tint, alpha*(1-edge))
			}
		}
	}
}

func marquee(c *brand.Canvas, t float64) {
	for i, b := range brand.MarqueeBulbs {
		lit := t - (bulbChaseAtMs + b.Chase*bulbChaseMs)
		if lit < 0 {
			continue
		}
		v := easeOutCubic(lit / bulbRiseMs)
		if lit > bulbRiseMs {
			v *= brand.BulbTwinkle(t, i)
		}
		c.AddBulb(b.Point, v)
	}
}

func wordAlphaAt(t float64) float64 { return easeOutCubic((t - wordFadeAtMs) / wordFadeMs) }

// glowAt is the wordmark halo strength: an overshoot settling into a breath.
func glowAt(t float64) float64 {
	switch {
	case t <= glowRiseAtMs:
		return 0
	case t < glowPeakAtMs:
		return glowPeak * smoothstep((t-glowRiseAtMs)/(glowPeakAtMs-glowRiseAtMs))
	case t < glowRestAtMs:
		return glowPeak + (brand.GlowRest-glowPeak)*smoothstep((t-glowPeakAtMs)/(glowRestAtMs-glowPeakAtMs))
	default:
		return brand.RestingGlow(t - glowRestAtMs)
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func easeOutCubic(t float64) float64 { return 1 - math.Pow(1-clamp01(t), 3) }

func smoothstep(t float64) float64 { t = clamp01(t); return t * t * (3 - 2*t) }
