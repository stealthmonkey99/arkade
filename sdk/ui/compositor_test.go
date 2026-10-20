// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// TestCompositeAtFullOpacityMatchesDirectDraw: a widget composited from a
// layer at opacity 1.0 onto opaque black equals the same widget drawn
// directly.
func TestCompositeAtFullOpacityMatchesDirectDraw(t *testing.T) {
	const w, h = 48, 48

	direct := newGridCanvas(w, h)
	paintScene(direct)

	comp := NewCompositor(w, h)
	layer := comp.AddLayer(1.0)
	paintScene(layer)

	target := newGridCanvas(w, h)
	comp.Composite(target)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if direct.at(x, y) != target.at(x, y) {
				t.Fatalf("pixel (%d,%d): direct=%v composited=%v", x, y, direct.at(x, y), target.at(x, y))
			}
		}
	}
}

// TestCompositeOpacityFloorScales: opacity 0.5 over opaque black truncates
// alpha 255*0.5 to 127 (not 128), so a white layer yields {127,127,127,255}.
func TestCompositeOpacityFloorScales(t *testing.T) {
	const w, h = 2, 2

	comp := NewCompositor(w, h)
	layer := comp.AddLayer(0.5)
	layer.Fill(White)

	target := newGridCanvas(w, h) // opaque black, like a fresh framebuffer
	comp.Composite(target)

	want := Color{127, 127, 127, 255}
	if got := target.at(0, 0); got != want {
		t.Fatalf("0.5-opacity white over black = %v, want %v (floor-scaled, matches display.ScaleAlpha+Merge)", got, want)
	}

	scaled := scaleAlpha(White, 0.5)
	if scaled.A != 127 {
		t.Fatalf("scaleAlpha(White, 0.5).A = %d, want 127", scaled.A)
	}
	if got := BlendOver(Black, scaled); got != want {
		t.Fatalf("BlendOver(Black, scaleAlpha(White,0.5)) = %v, want %v", got, want)
	}
}

// TestCompositeOpacityZeroAndOneEndpoints: opacity <= 0 contributes nothing
// and opacity >= 1 passes pixels through with no truncation.
func TestCompositeOpacityZeroAndOneEndpoints(t *testing.T) {
	const w, h = 1, 1

	zero := NewCompositor(w, h)
	l := zero.AddLayer(0)
	l.Fill(White)
	target := newGridCanvas(w, h)
	zero.Composite(target)
	if got := target.at(0, 0); got != Black {
		t.Errorf("opacity 0 layer changed target: got %v, want unchanged Black", got)
	}

	one := NewCompositor(w, h)
	l2 := one.AddLayer(1)
	l2.Fill(Color{10, 20, 30, 200})
	target2 := newGridCanvas(w, h)
	one.Composite(target2)
	want := BlendOver(Black, Color{10, 20, 30, 200})
	if got := target2.at(0, 0); got != want {
		t.Errorf("opacity 1 layer = %v, want %v (unscaled BlendOver)", got, want)
	}
}

// TestCompositeLayerOrder: the last-added layer is on top, and reversing
// insertion order flips the winner.
func TestCompositeLayerOrder(t *testing.T) {
	const w, h = 1, 1

	comp := NewCompositor(w, h)
	bottom := comp.AddLayer(1)
	bottom.Fill(Color{255, 0, 0, 255}) // opaque red
	top := comp.AddLayer(1)
	top.Fill(Color{0, 0, 255, 255}) // opaque blue

	target := newGridCanvas(w, h)
	comp.Composite(target)
	if got := target.at(0, 0); got != (Color{0, 0, 255, 255}) {
		t.Fatalf("top-over-bottom = %v, want opaque blue", got)
	}

	comp2 := NewCompositor(w, h)
	t2 := comp2.AddLayer(1)
	t2.Fill(Color{0, 0, 255, 255})
	b2 := comp2.AddLayer(1)
	b2.Fill(Color{255, 0, 0, 255})
	target2 := newGridCanvas(w, h)
	comp2.Composite(target2)
	if got := target2.at(0, 0); got != (Color{255, 0, 0, 255}) {
		t.Fatalf("insertion-order-reversed winner = %v, want opaque red", got)
	}
}

func TestCompositorIsBoundedAndRemoveClosesTheGap(t *testing.T) {
	comp := NewCompositor(1, 1)
	var layers [MaxLayers]*Layer
	for i := range layers {
		layers[i] = comp.AddLayer(1)
		if layers[i] == nil {
			t.Fatalf("AddLayer %d of %d returned nil", i+1, MaxLayers)
		}
	}
	if comp.AddLayer(1) != nil {
		t.Fatal("a layer past MaxLayers was accepted")
	}
	if comp.Len() != MaxLayers {
		t.Fatalf("Len = %d, want %d", comp.Len(), MaxLayers)
	}

	layers[0].Fill(Color{255, 0, 0, 255})
	layers[3].Fill(Color{0, 255, 0, 255})
	layers[7].Fill(Color{0, 0, 255, 255})
	comp.RemoveLayer(layers[7])
	comp.RemoveLayer(layers[7]) // already gone
	comp.RemoveLayer(nil)
	if comp.Len() != MaxLayers-1 {
		t.Fatalf("Len after one removal = %d", comp.Len())
	}
	target := newGridCanvas(1, 1)
	comp.Composite(target)
	if got := target.at(0, 0); got != (Color{0, 255, 0, 255}) {
		t.Fatalf("after removing the top layer the winner = %v, want green from layer 3", got)
	}

	comp.RemoveLayer(layers[3])
	if comp.AddLayer(0.5) == nil {
		t.Fatal("a freed slot could not be reused")
	}
	if n := testing.AllocsPerRun(20, func() { comp.Composite(target) }); n != 0 {
		t.Errorf("Composite allocated %v times per run", n)
	}
}

func TestCompositorOpacityIsTheLayersField(t *testing.T) {
	comp := NewCompositor(2, 2)
	l := comp.AddLayer(1)
	l.Fill(White)
	l.Opacity = 0

	target := newGridCanvas(2, 2)
	comp.Composite(target)
	if got := target.at(0, 0); got != Black {
		t.Fatalf("after Opacity = 0: target = %v, want unchanged Black", got)
	}
}

func TestCompositorInvisibleLayerSkipped(t *testing.T) {
	comp := NewCompositor(1, 1)
	l := comp.AddLayer(1)
	l.Fill(White)
	l.Visible = false

	target := newGridCanvas(1, 1)
	comp.Composite(target)
	if got := target.at(0, 0); got != Black {
		t.Fatalf("invisible layer changed target: got %v, want unchanged Black", got)
	}
}

func TestFadeForwardRamp(t *testing.T) {
	f := NewFade(1000)
	if got := f.Opacity(); got != 0 {
		t.Fatalf("fresh Fade Opacity() = %v, want 0", got)
	}
	if got := f.Advance(500); got != 0.5 {
		t.Fatalf("Advance(500) of 1000 = %v, want 0.5", got)
	}
	if got := f.Advance(500); got != 1 {
		t.Fatalf("Advance to full duration = %v, want 1", got)
	}
	if !f.Done() {
		t.Fatalf("Done() after reaching duration = false, want true")
	}
	if got := f.Advance(1000); got != 1 {
		t.Fatalf("Advance past duration = %v, want clamped 1", got)
	}
}

func TestFadeReverseRamp(t *testing.T) {
	f := &Fade{DurationMicros: 1000, Reverse: true}
	if got := f.Opacity(); got != 1 {
		t.Fatalf("fresh Reverse Fade Opacity() = %v, want 1", got)
	}
	if got := f.Advance(1000); got != 0 {
		t.Fatalf("Reverse Advance to full duration = %v, want 0", got)
	}
}

func TestFadeZeroDurationCompletesInstantly(t *testing.T) {
	f := NewFade(0)
	if !f.Done() {
		t.Fatalf("zero-duration Fade Done() = false, want true")
	}
	if got := f.Opacity(); got != 1 {
		t.Fatalf("zero-duration Fade Opacity() = %v, want 1", got)
	}
}

func TestFadeReset(t *testing.T) {
	f := NewFade(1000)
	f.Advance(700)
	f.Reset()
	if got := f.Opacity(); got != 0 {
		t.Fatalf("Opacity() after Reset = %v, want 0", got)
	}
}

func TestFadeIgnoresNegativeAdvance(t *testing.T) {
	f := NewFade(1000)
	f.Advance(500)
	if got := f.Advance(-100); got != 0.5 {
		t.Fatalf("Advance(negative) changed opacity to %v, want unchanged 0.5", got)
	}
}
