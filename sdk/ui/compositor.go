// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// MaxLayers is how many layers one [Compositor] stacks.
const MaxLayers = 8

// Compositor holds an ordered stack of up to [MaxLayers] [Layer]s and
// flattens them onto a target [Canvas], bottom to top, scaling each layer's
// Opacity into pixel alpha and blending with [BlendOver]. The stack is a
// fixed array in add order, so compositing is deterministic and the
// compositor itself never allocates after its layers exist.
//
// The *[Layer] [Compositor.AddLayer] returns is the handle: keep it, draw into it, and set
// its Opacity and Visible fields directly.
type Compositor struct {
	w, h   int
	layers [MaxLayers]*Layer
	n      int
}

// NewCompositor allocates a compositor for a w×h pixel target. Call it during
// Init, never from a package init().
func NewCompositor(w, h int) *Compositor {
	return &Compositor{w: w, h: h}
}

// AddLayer creates a full-size Layer at the given opacity, places it on top
// of the stack, and returns it for the caller to draw into. It returns nil
// once [MaxLayers] are stacked. It allocates, so call it during Init.
func (c *Compositor) AddLayer(opacity float64) *Layer {
	if c.n >= MaxLayers {
		return nil
	}
	l := NewLayer(c.w, c.h)
	l.Opacity = opacity
	c.layers[c.n] = l
	c.n++
	return l
}

// RemoveLayer takes l out of the stack, closing the gap above it. A layer
// that is not in the stack, nil included, is left alone.
func (c *Compositor) RemoveLayer(l *Layer) {
	for i := 0; i < c.n; i++ {
		if c.layers[i] != l {
			continue
		}
		copy(c.layers[i:c.n], c.layers[i+1:c.n])
		c.n--
		c.layers[c.n] = nil
		return
	}
}

// Len returns how many layers are stacked.
func (c *Compositor) Len() int { return c.n }

// Composite blends every visible layer with positive opacity onto dst in
// stack order (first added is the bottom): each pixel's alpha is scaled by
// the layer's opacity and blended with [BlendOver]. Call it from the render
// phase; it allocates nothing. Layers larger than dst are clipped by dst's
// Blend.
func (c *Compositor) Composite(dst Canvas) {
	for _, l := range c.layers[:c.n] {
		if !l.Visible || l.Opacity <= 0 {
			continue
		}
		for y := 0; y < l.h; y++ {
			row := y * l.w
			for x := 0; x < l.w; x++ {
				px := l.pix[row+x]
				if px.A == 0 {
					continue
				}
				px = scaleAlpha(px, l.Opacity)
				if px.A == 0 {
					continue
				}
				dst.Blend(x, y, px)
			}
		}
	}
}

// scaleAlpha scales c's alpha by factor, truncating rather than rounding.
// factor >= 1 is a no-op; factor <= 0 zeroes alpha and keeps the color
// channels.
func scaleAlpha(c Color, factor float64) Color {
	if factor >= 1 {
		return c
	}
	if factor <= 0 {
		return Color{c.R, c.G, c.B, 0}
	}
	return Color{c.R, c.G, c.B, byte(float64(c.A) * factor)}
}

// Fade is a linear opacity ramp driven by caller-supplied elapsed time; it
// reads no clock. Call [Fade.Advance] with dtMicros once per frame from Update
// and assign the result to a [Layer]'s Opacity.
type Fade struct {
	// DurationMicros is the ramp length in microseconds; <= 0 completes
	// instantly.
	DurationMicros int64
	// Reverse ramps 1→0 instead of the default 0→1.
	Reverse bool

	elapsed int64
}

// NewFade builds a Fade of the given duration in microseconds, starting at
// its beginning (0 elapsed). It allocates, so call it from Init, or declare a
// Fade value instead.
func NewFade(durationMicros int64) *Fade {
	return &Fade{DurationMicros: durationMicros}
}

// Advance moves the ramp forward by dtMicros microseconds (negative values are
// ignored) and returns the resulting opacity.
func (f *Fade) Advance(dtMicros int64) float64 {
	if dtMicros > 0 {
		f.elapsed += dtMicros
	}
	if f.DurationMicros > 0 && f.elapsed > f.DurationMicros {
		f.elapsed = f.DurationMicros
	}
	return f.Opacity()
}

// Opacity returns the current ramp position in [0, 1] without advancing
// it: elapsed/duration for a forward Fade, or 1 - elapsed/duration when
// Reverse is set. A DurationMicros <= 0 is already at its end.
func (f *Fade) Opacity() float64 {
	var t float64
	if f.DurationMicros <= 0 {
		t = 1
	} else {
		t = float64(f.elapsed) / float64(f.DurationMicros)
		if t > 1 {
			t = 1
		}
		if t < 0 {
			t = 0
		}
	}
	if f.Reverse {
		return 1 - t
	}
	return t
}

// Done reports whether the ramp has reached its end.
func (f *Fade) Done() bool {
	return f.DurationMicros <= 0 || f.elapsed >= f.DurationMicros
}

// Reset restarts the ramp from zero elapsed time.
func (f *Fade) Reset() { f.elapsed = 0 }
