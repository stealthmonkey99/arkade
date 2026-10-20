// SPDX-License-Identifier: GPL-2.0-or-later

package attract

import (
	"math"
	"math/rand"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
)

// Warp tuning for the starfield, paced slow-to-medium.
const (
	starCount = 40

	// depthMax is where a star spawns (near the centre) and depthMin where it
	// respawns, past the edge. Radius is focal/depth, so it accelerates
	// outward without depth reaching zero.
	depthMax = 30.0
	depthMin = 2.0

	// focal maps depth to a pixel radius: distFar (~45px, the corners) is
	// focal/depthMin and distNear (~3px) is focal/depthMax.
	focal    = 90.0
	distFar  = focal / depthMin
	distNear = focal / depthMax

	// warpRate is the base closing speed in depth units per second, scaled
	// per star by star.speed.
	warpRate = 4.0
)

// dimPalette is soft, low-saturation hues so the screensaver recedes.
var dimPalette = []display.Pixel{
	{R: 70, G: 90, B: 190, A: 255},  // periwinkle
	{R: 110, G: 70, B: 180, A: 255}, // violet
	{R: 50, G: 140, B: 160, A: 255}, // teal
	{R: 160, G: 80, B: 130, A: 255}, // rose
}

// star is one warp particle. angle is fixed for its lifetime and speed varies
// per star so the field does not respawn in unison.
type star struct {
	angle float64
	depth float64
	speed float64
	color display.Pixel
}

// starfield is the ambient element: stars stream outward from the centre and
// respawn there past the edge.
type starfield struct {
	element.Base
	stars []star
}

// newStarfield lays out the field with the engine's seeded RNG so golden
// frames reproduce. Initial depths are staggered so the first frame is
// already in motion.
func newStarfield(rng *rand.Rand) *starfield {
	w, h := 64, 64
	f := &starfield{}
	f.InitBase(element.Options{Width: &w, Height: &h})
	f.Impl = f.renderImpl
	f.stars = make([]star, starCount)
	for i := range f.stars {
		f.stars[i] = newStar(rng, true)
	}
	f.MarkDirty()
	return f
}

// newStar rolls a fresh star. staggered spreads the depth across the range;
// respawns start at depthMax.
func newStar(rng *rand.Rand, staggered bool) star {
	depth := depthMax
	if staggered {
		depth = depthMin + rng.Float64()*(depthMax-depthMin)
	}
	return star{
		angle: rng.Float64() * 2 * math.Pi,
		depth: depth,
		speed: 0.8 + rng.Float64()*0.5,
		color: dimPalette[rng.Intn(len(dimPalette))],
	}
}

// Advance moves every star by the elapsed time and respawns those past the
// edge. It is the only place the element mutates state or draws randomness.
func (f *starfield) Advance(stats clock.FrameTime, rng *rand.Rand) {
	dt := stats.ElapsedMs / 1000
	if dt > 0 {
		for i := range f.stars {
			s := &f.stars[i]
			s.depth -= warpRate * s.speed * dt
			if s.depth <= depthMin {
				f.stars[i] = newStar(rng, false)
			}
		}
	}
	f.MarkDirty()
}

// renderImpl projects each star to a pixel at radius focal/depth. Brightness
// grows with radius but is capped low.
func (f *starfield) renderImpl(clock.FrameTime) display.Sprite {
	sprite := display.NewMatrix(64, 64, display.Transparent)
	const cx, cy = 32.0, 32.0
	for _, s := range f.stars {
		dist := focal / s.depth
		x := int(cx + dist*math.Cos(s.angle))
		y := int(cy + dist*math.Sin(s.angle))
		if x < 0 || x >= 64 || y < 0 || y >= 64 {
			continue
		}
		t := (dist - distNear) / (distFar - distNear)
		t = math.Max(0, math.Min(1, t))
		sprite[y][x] = s.color.WithAlpha(uint8(20 + t*120))
	}
	return sprite
}
