// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

func TestLayerIsCanvas(t *testing.T) {
	var _ Canvas = NewLayer(1, 1)
}

func TestLayerBounds(t *testing.T) {
	l := NewLayer(5, 3)
	if w, h := l.Bounds(); w != 5 || h != 3 {
		t.Fatalf("Bounds() = %d,%d want 5,3", w, h)
	}
}

func TestLayerDefaults(t *testing.T) {
	l := NewLayer(2, 2)
	if l.Opacity != 1 {
		t.Errorf("new layer Opacity = %v, want 1", l.Opacity)
	}
	if !l.Visible {
		t.Errorf("new layer Visible = false, want true")
	}
	if got := l.At(0, 0); got != Transparent {
		t.Errorf("new layer pixel = %v, want Transparent", got)
	}
}

func TestLayerBlendUsesBlendOver(t *testing.T) {
	l := NewLayer(2, 2)
	l.Blend(0, 0, Black)
	half := Color{255, 255, 255, 128}
	l.Blend(0, 0, half)
	want := BlendOver(Black, half)
	if got := l.At(0, 0); got != want {
		t.Errorf("Blend result = %v, want %v (BlendOver(Black, half))", got, want)
	}
}

func TestLayerBlendOutOfBounds(t *testing.T) {
	l := NewLayer(2, 2)
	l.Blend(-1, 0, White)
	l.Blend(0, -1, White)
	l.Blend(2, 0, White)
	l.Blend(0, 2, White)
	if got := l.At(-1, 0); got != Transparent {
		t.Errorf("At() out of bounds = %v, want Transparent", got)
	}
}

func TestLayerFillAndClear(t *testing.T) {
	l := NewLayer(2, 2)
	l.Fill(White)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if got := l.At(x, y); got != White {
				t.Fatalf("At(%d,%d) after Fill = %v, want White", x, y, got)
			}
		}
	}
	l.Clear()
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if got := l.At(x, y); got != Transparent {
				t.Fatalf("At(%d,%d) after Clear = %v, want Transparent", x, y, got)
			}
		}
	}
}
