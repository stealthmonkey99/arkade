// SPDX-License-Identifier: GPL-2.0-or-later

package evening

import (
	"math"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

const (
	barColumns   = 16
	barPitch     = 4 // three lit pixels and a one-pixel gap
	barWidth     = 3
	barMinHeight = 2
	barMaxHeight = 32
	barTipRows   = 2
	barTipAlpha  = 0.45
	// barFloorAlpha is the always-lit bottom row, so the panel never reads as crashed.
	barFloorAlpha = 0.5

	// barRelease is how long a column takes to fall; the rise is immediate
	// because smoothing the attack made the cabinet ignore 50 ms knocks.
	barRelease = 600 * time.Millisecond

	// barRangeDb is how far above its quiet floor a band fills its column.
	barRangeDb = 30
	// barQuietFloor sits about 10 dB over the microphone's own noise in the
	// quietest band, so an empty band stays at the floor instead of amplifying hiss.
	barQuietFloor = 1e-7
	// barSeedRatio starts each band's floor below the first reading. The app
	// loads because the room is loud, so seeding at it would flatten the
	// columns until the first pause.
	barSeedRatio = 10
)

var (
	barFloorColor = display.RGB(31, 26, 140)
	barTipColor   = display.RGB(191, 77, 140)
)

// barsEl is the equalizer: sixteen columns from eight microphone bands, each
// normalised against its own rising quiet floor.
type barsEl struct {
	element.Base

	level [barColumns]float64
	quiet [hardware.MicBandCount]float64
	have  bool
}

func newBars() *barsEl {
	w, h := 64, 64
	b := &barsEl{}
	b.InitBase(element.Options{Width: &w, Height: &h})
	b.Impl = b.renderImpl
	b.MarkDirty()
	return b
}

// advance folds one band reading into the columns; ok=false lets them fall to the floor.
func (b *barsEl) advance(stats clock.FrameTime, bands [hardware.MicBandCount]float64, ok bool) {
	dt := stats.ElapsedMs / 1000
	if dt <= 0 {
		return
	}
	var target [hardware.MicBandCount]float64
	if ok {
		target = b.bandTargets(dt, bands)
	}
	for c := range b.level {
		want := interpolate(target, c)
		if want >= b.level[c] {
			b.level[c] = want
			continue
		}
		b.level[c] += (1 - math.Exp(-dt/barRelease.Seconds())) * (want - b.level[c])
	}
	b.MarkDirty()
}

// bandTargets converts band powers to column heights on a decibel scale,
// through each band's rising quiet floor (the rule tracker uses).
func (b *barsEl) bandTargets(dt float64, bands [hardware.MicBandCount]float64) [hardware.MicBandCount]float64 {
	if !b.have {
		b.have = true
		for i, v := range bands {
			b.quiet[i] = v / barSeedRatio
		}
	}
	var out [hardware.MicBandCount]float64
	rise := math.Exp(dt / quietRiseTau.Seconds())
	for i, v := range bands {
		if v < b.quiet[i] {
			b.quiet[i] = v
		} else {
			b.quiet[i] *= rise
		}
		b.quiet[i] = math.Max(b.quiet[i], barQuietFloor)
		if v <= b.quiet[i] {
			continue
		}
		out[i] = math.Min(1, 10*math.Log10(v/b.quiet[i])/barRangeDb)
	}
	return out
}

func interpolate(bands [hardware.MicBandCount]float64, col int) float64 {
	pos := float64(col) * float64(hardware.MicBandCount-1) / float64(barColumns-1)
	lo := int(pos)
	if lo >= hardware.MicBandCount-1 {
		return bands[hardware.MicBandCount-1]
	}
	f := pos - float64(lo)
	return bands[lo]*(1-f) + bands[lo+1]*f
}

func (b *barsEl) renderImpl(clock.FrameTime) display.Sprite {
	sprite := display.NewMatrix(64, 64, display.Transparent)
	for x := 0; x < 64; x++ {
		sprite[63][x] = barFloorColor.ScaleAlpha(barFloorAlpha)
	}
	for c := 0; c < barColumns; c++ {
		h := barMinHeight + int(b.level[c]*float64(barMaxHeight-barMinHeight)+0.5)
		x0 := c * barPitch
		for y := 0; y < h; y++ {
			k := float64(y) / float64(barMaxHeight)
			alpha := 0.6 + 0.4*(1-k)
			if y >= h-barTipRows {
				alpha *= barTipAlpha
			}
			px := lerpPixel(barFloorColor, barTipColor, k).ScaleAlpha(alpha)
			for dx := 0; dx < barWidth; dx++ {
				sprite[63-y][x0+dx] = px
			}
		}
	}
	return sprite
}

func lerpPixel(a, b display.Pixel, k float64) display.Pixel {
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*k + 0.5) }
	return display.RGB(lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B))
}
