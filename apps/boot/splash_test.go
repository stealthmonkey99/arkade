// SPDX-License-Identifier: GPL-2.0-or-later

package boot

import (
	"math"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/brand"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/testkit"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func splashLit(ms float64) map[brand.Point]display.Pixel {
	out := map[brand.Point]display.Pixel{}
	for y, row := range splashFrame(panelW, panelH, ms) {
		for x, p := range row {
			if p.A != 0 {
				out[brand.Point{X: x, Y: y}] = p
			}
		}
	}
	return out
}

func TestSplashPilotLightIsAloneBeforeTheChord(t *testing.T) {
	for _, ms := range []float64{0, 200, stingerAtMs - 1} {
		lit := splashLit(ms)
		if len(lit) != len(brand.RingPixels) {
			t.Errorf("%.0fms: %d pixels lit, want only the Å's ring (%d)", ms, len(lit), len(brand.RingPixels))
		}
		for _, p := range brand.RingPixels {
			if _, ok := lit[p]; !ok {
				t.Errorf("%.0fms: ring pixel %+v is dark", ms, p)
			}
		}
	}
}

func TestSplashRingExpandsOnSchedule(t *testing.T) {
	for _, sinceOnset := range []float64{0, 225, 450} {
		want := sinceOnset / ringTravelMs * ringReachPx
		c := brand.NewCanvas(panelW, panelH)
		ring(c, sinceOnset, voiceTints[0])
		lit := 0
		for y, row := range c.Matrix() {
			for x, p := range row {
				if p.A == 0 {
					continue
				}
				lit++
				d := math.Hypot(float64(x-brand.RingCenter.X), float64(y-brand.RingCenter.Y))
				if math.Abs(d-want) >= 1 {
					t.Errorf("%.0fms in: pixel (%d,%d) is %.2f from the centre, want within 1px of %.1f",
						sinceOnset, x, y, d, want)
				}
			}
		}
		if lit == 0 {
			t.Errorf("%.0fms in: the ring painted nothing", sinceOnset)
		}
	}
}

func TestSplashRingsCarryTheLampColours(t *testing.T) {
	lamps := [bloomVoices]display.Pixel{
		display.FromUI(ui.ButtonGreen), display.FromUI(ui.ButtonRed), display.FromUI(ui.ButtonYellow),
		display.FromUI(ui.ButtonBlue), display.White,
	}
	for voice, lamp := range lamps {
		c := brand.NewCanvas(panelW, panelH)
		ring(c, 0, voiceTints[voice])
		got := c.Matrix()[brand.RingCenter.Y][brand.RingCenter.X]
		want := display.Pixel{
			R: brand.Channel(float64(lamp.R) / 255 * 0.85),
			G: brand.Channel(float64(lamp.G) / 255 * 0.85),
			B: brand.Channel(float64(lamp.B) / 255 * 0.85),
			A: 255,
		}
		if got != want {
			t.Errorf("voice %d ring = %+v, want its lamp colour at 0.85 (%+v)", voice, got, want)
		}
	}
}

func TestSplashWordmarkSchedule(t *testing.T) {
	for _, tc := range []struct{ ms, want float64 }{
		{0, 0}, {wordFadeAtMs, 0}, {wordFadeAtMs + wordFadeMs, 1}, {splashMs, 1},
	} {
		if got := wordAlphaAt(tc.ms); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("wordmark alpha at %.0fms = %.3f, want %.3f", tc.ms, got, tc.want)
		}
	}
	for _, tc := range []struct{ ms, want float64 }{
		{0, 0}, {glowRiseAtMs, 0}, {glowPeakAtMs, glowPeak}, {glowRestAtMs, brand.GlowRest},
	} {
		if got := glowAt(tc.ms); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("glow at %.0fms = %.3f, want %.3f", tc.ms, got, tc.want)
		}
	}
	if got := glowAt(splashMs); got < brand.GlowRest-brand.GlowBreath || got > brand.GlowRest+brand.GlowBreath {
		t.Errorf("glow at the end of the splash = %.3f, want it breathing around %.2f", got, brand.GlowRest)
	}

	lit := splashLit(wordFadeAtMs + wordFadeMs)
	for _, p := range brand.WordPixels {
		got, ok := lit[p]
		if !ok || got.R != 255 || got.G < brand.Channel(brand.Amber[1]) {
			t.Errorf("wordmark pixel %+v = %+v, want at least the amber", p, got)
		}
	}
}

func TestSplashMarqueeChase(t *testing.T) {
	first, last := 0, 0
	for i, b := range brand.MarqueeBulbs {
		if b.Chase < brand.MarqueeBulbs[first].Chase {
			first = i
		}
		if b.Chase > brand.MarqueeBulbs[last].Chase {
			last = i
		}
	}

	// Rings cross the marquee pixels, so read the chase off the marquee alone.
	chaseLit := func(ms float64, b brand.Bulb) bool {
		c := brand.NewCanvas(panelW, panelH)
		marquee(c, ms)
		return c.Matrix()[b.Y][b.X].A != 0
	}
	if chaseLit(bulbChaseAtMs, brand.MarqueeBulbs[first]) {
		t.Errorf("the top-centre bulb was lit at %dms, before its cue", bulbChaseAtMs)
	}
	if !chaseLit(bulbChaseAtMs+bulbRiseMs, brand.MarqueeBulbs[first]) {
		t.Errorf("the top-centre bulb is still dark %dms after the chase began", bulbRiseMs)
	}
	lastCue := bulbChaseAtMs + brand.MarqueeBulbs[last].Chase*bulbChaseMs
	if chaseLit(lastCue-1, brand.MarqueeBulbs[last]) {
		t.Error("the last bulb lit before the chase reached it")
	}
	if !chaseLit(lastCue+bulbRiseMs, brand.MarqueeBulbs[last]) {
		t.Error("the last bulb never lit")
	}
	if lastCue > splashMs {
		t.Errorf("the chase reaches its last bulb at %.0fms, after the splash ends", lastCue)
	}

	end := splashLit(splashMs)
	for i, b := range brand.MarqueeBulbs {
		if _, ok := end[b.Point]; !ok {
			t.Errorf("bulb %d (%+v) is dark at the end of the splash", i, b.Point)
		}
	}
	for _, p := range brand.WordPixels {
		if _, ok := end[p]; !ok {
			t.Errorf("wordmark pixel %+v is dark at the end of the splash", p)
		}
	}
}

func TestBootPaintsTheSplashFrame(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{FPS: 100})
	const msPerTick = 10
	ticked := 0
	for _, tick := range []int{1, 70, 180} {
		rig.Tick(tick - ticked)
		ticked = tick
		want := splashFrame(panelW, panelH, float64(tick*msPerTick))
		for _, row := range want {
			for x, p := range row {
				if p.A == 0 {
					row[x] = display.Black
				}
			}
		}
		if !display.MatrixEquals(rig.Frame(), want) {
			t.Errorf("panel at tick %d (%dms) is not splashFrame's output", tick, tick*msPerTick)
		}
	}
}

func TestSplashGoldenFrames(t *testing.T) {
	for _, tc := range []struct {
		name string
		ms   float64
	}{
		{"splash-0ms", 0},
		{"splash-700ms", 700},
		{"splash-1800ms", 1800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testkit.GoldenFrame(t, tc.name, splashFrame(panelW, panelH, tc.ms))
		})
	}
}
