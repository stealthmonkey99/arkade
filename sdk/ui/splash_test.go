// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// TestSplashFillsBackground checks the default (opaque Black) and an explicit
// Background at a corner pixel far from any glyph.
func TestSplashFillsBackground(t *testing.T) {
	const w, h = 64, 32

	def := Splash{W: w, H: h, Title: "X"}
	got := newByteCanvas(w, h)
	def.Draw(got)
	if c := got.at(0, 0); c != Black {
		t.Errorf("default background corner = %v, want opaque Black", c)
	}

	bg := Color{10, 20, 30, 255}
	explicit := Splash{W: w, H: h, Title: "X", Background: bg}
	got2 := newByteCanvas(w, h)
	explicit.Draw(got2)
	if c := got2.at(0, 0); c != bg {
		t.Errorf("explicit background corner = %v, want %v", c, bg)
	}
}

// TestSplashCentersTitle checks the leftmost lit column against
// (W - TextWidth)/2. ARKADE is used because its 'A' lights its own leftmost
// column.
func TestSplashCentersTitle(t *testing.T) {
	const w, h = 64, 32
	bg := Color{10, 20, 30, 255}
	txt := Color{200, 200, 200, 255}
	title := "ARKADE"
	s := Splash{W: w, H: h, Title: title, Background: bg, Color: txt}

	cv := newByteCanvas(w, h)
	s.Draw(cv)

	font := FontLED{}
	wantX := (w - font.TextWidth(title, 1)) / 2
	lineH := font.Height(1)
	top := (h - lineH) / 2

	gotX := -1
	for x := 0; x < w && gotX == -1; x++ {
		for y := top; y < top+lineH; y++ {
			if cv.at(x, y) != bg {
				gotX = x
				break
			}
		}
	}
	if gotX != wantX {
		t.Errorf("title left edge = %d, want %d", gotX, wantX)
	}
}

// TestSplashSubtitleBelowTitle compares against an independently built
// canvas of direct FontLED.DrawText calls rather than re-deriving Splash's
// math.
func TestSplashSubtitleBelowTitle(t *testing.T) {
	const w, h = 64, 40
	txt := White
	s := Splash{W: w, H: h, Title: "ARKADE", Subtitle: "LOADING", Color: txt}

	got := newByteCanvas(w, h)
	s.Draw(got)

	font := FontLED{}
	lineH := font.Height(1)
	blockH := lineH + splashGap + lineH
	top := (h - blockH) / 2
	titleX := (w - font.TextWidth("ARKADE", 1)) / 2
	subX := (w - font.TextWidth("LOADING", 1)) / 2

	want := newByteCanvas(w, h)
	FillRect(want, 0, 0, w, h, Black) // default Background
	font.DrawText(want, titleX, top, "ARKADE", 1, txt)
	font.DrawText(want, subX, top+lineH+splashGap, "LOADING", 1, txt)

	assertCanvasEqual(t, got, want)
}

func TestSplashNoTitleOrSubtitle(t *testing.T) {
	const w, h = 16, 16
	s := Splash{W: w, H: h}

	got := newByteCanvas(w, h)
	s.Draw(got)

	want := newByteCanvas(w, h)
	FillRect(want, 0, 0, w, h, Black)

	assertCanvasEqual(t, got, want)
}
