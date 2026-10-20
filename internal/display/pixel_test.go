// SPDX-License-Identifier: GPL-2.0-or-later

package display

import "testing"

func TestMergeOpaqueSrcWins(t *testing.T) {
	got := Merge(Red, Blue)
	if got != Blue {
		t.Errorf("opaque src should win, got %+v", got)
	}
}

func TestMergeTransparentSrcKeepsDst(t *testing.T) {
	got := Merge(Red, Transparent)
	if got != Red {
		t.Errorf("transparent src should keep dst, got %+v", got)
	}
}

func TestMergeHighlightOverBlack(t *testing.T) {
	// 70/255 alpha white over opaque black.
	got := Merge(Black, Highlight)
	// aOut = 70/255 + 1*(1-70/255) = 1 → channels = 255 * (70/255) = 70
	want := Pixel{70, 70, 70, 255}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestMergeShadowOverColor(t *testing.T) {
	got := Merge(Pixel{200, 100, 50, 255}, Shadow)
	// black at 70/255 over opaque color: floor(c * 185/255)
	want := Pixel{145, 72, 36, 255}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestMergeOntoTransparent(t *testing.T) {
	src := Pixel{100, 150, 200, 128}
	got := Merge(Transparent, src)
	// dst alpha 0 → result color = src color, alpha = src alpha (within
	// floor rounding).
	if got.A != 128 && got.A != 127 {
		t.Errorf("alpha should stay ~128, got %d", got.A)
	}
	if got.R < 99 || got.R > 100 || got.G < 149 || got.G > 150 || got.B < 199 || got.B > 200 {
		t.Errorf("color should survive merge onto transparent, got %+v", got)
	}
}

func TestScaleAlphaFloors(t *testing.T) {
	p := Pixel{10, 20, 30, 255}
	got := p.ScaleAlpha(0.5)
	if got.A != 127 {
		t.Errorf("expected floor(255*0.5)=127, got %d", got.A)
	}
	if got.R != 10 || got.G != 20 || got.B != 30 {
		t.Errorf("color channels must not change, got %+v", got)
	}
}

func TestRotateClockwise(t *testing.T) {
	m := Matrix{
		{Red, Green},
		{Blue, Yellow},
	}
	got := Rotate(m, Clockwise)
	want := Matrix{
		{Blue, Red},
		{Yellow, Green},
	}
	if !MatrixEquals(got, want) {
		t.Errorf("clockwise rotation wrong: %+v", got)
	}
}

func TestRotateRoundTrip(t *testing.T) {
	m := NewMatrix(4, 4, Black)
	m[0][1] = Red
	cw := Rotate(m, Clockwise)
	back := Rotate(cw, CounterClockwise)
	if !MatrixEquals(m, back) {
		t.Error("CW then CCW should round-trip")
	}
}
