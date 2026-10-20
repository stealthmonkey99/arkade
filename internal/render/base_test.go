// SPDX-License-Identifier: GPL-2.0-or-later

package render_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// square2x2 builds a 2×2 matrix whose four pixels are distinguishable, so a
// rotation is observable pixel-for-pixel.
func square2x2() display.Matrix {
	return display.Matrix{
		{{R: 1, A: 255}, {R: 2, A: 255}},
		{{R: 3, A: 255}, {R: 4, A: 255}},
	}
}

// TestBaseOrientationComposesWithPhysicalAndVirtual checks that the base,
// physical and virtual layers sum into the final rotation, and that all three
// at their defaults leave the frame unrotated.
func TestBaseOrientationComposesWithPhysicalAndVirtual(t *testing.T) {
	cases := []struct {
		name     string
		physical int // number of RotateRight calls
		base     display.Orientation
		virtual  display.Orientation
		want     display.Orientation
	}{
		{"all upright", 0, display.Upright, display.Upright, display.Upright},
		{"base only", 0, display.Clockwise, display.Upright, display.Clockwise},
		{"base+virtual", 0, display.Clockwise, display.Clockwise, display.UpsideDown},
		{"physical+base+virtual", 1, display.Clockwise, display.Clockwise, display.CounterClockwise},
		{"wraps mod 4", 2, display.UpsideDown, display.Upright, display.Upright},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := render.NewCapture(true)
			for i := 0; i < tc.physical; i++ {
				c.RotateRight()
			}
			c.SetBaseOrientation(tc.base)
			c.VirtualRotate(tc.virtual)

			in := square2x2()
			c.Paint(in, clock.FrameTime{})

			got := c.LastFrame().Data
			want := display.Rotate(square2x2(), tc.want)
			if !display.MatrixEquals(got, want) {
				t.Errorf("combined rotation mismatch: got %v, want the frame rotated %v", got, tc.want)
			}
		})
	}
}

func TestBaseOrientationReadback(t *testing.T) {
	c := render.NewCapture(false)
	c.RotateRight()                     // physical = Clockwise
	c.VirtualRotate(display.UpsideDown) // virtual
	c.SetBaseOrientation(display.CounterClockwise)

	if got := c.BaseOrientation(); got != display.CounterClockwise {
		t.Errorf("BaseOrientation() = %v, want CounterClockwise", got)
	}
	if got := c.Orientation(); got != display.Clockwise {
		t.Errorf("SetBaseOrientation disturbed the physical layer: %v", got)
	}
	if got := c.VirtualOrientation(); got != display.UpsideDown {
		t.Errorf("SetBaseOrientation disturbed the virtual layer: %v", got)
	}
}

// The getters take the lock; under -race this pins that they do.
func TestOrientationGettersAreSafeAlongsideRotation(t *testing.T) {
	var r render.Renderer = render.NewCapture(false)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			r.RotateRight()
			r.SetBaseOrientation(display.Orientation(i % 4))
			r.VirtualRotate(display.Orientation(i % 4))
			r.DimDown()
		}
	}()
	for polling := true; polling; {
		select {
		case <-done:
			polling = false
		default:
			_, _, _, _ = r.Orientation(), r.BaseOrientation(), r.VirtualOrientation(), r.Brightness()
		}
	}
}
