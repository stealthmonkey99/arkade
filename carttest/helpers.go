// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"context"
	"testing"
)

// Key ordinals for InputRecord.Key, matching the wire table.
const (
	KeyBrightness uint8 = 0
	KeyRotate     uint8 = 1
	KeyQuit       uint8 = 2
	KeyEscape     uint8 = 3
	KeyEnter      uint8 = 4
	KeySpace      uint8 = 5
	KeyUp         uint8 = 6
	KeyDown       uint8 = 7
	KeyLeft       uint8 = 8
	KeyRight      uint8 = 9
	KeyHome       uint8 = 10
	KeyEnd        uint8 = 11
	KeyJoyA       uint8 = 12
	KeyJoyB       uint8 = 13
	KeyJoyC       uint8 = 14
	KeyJoyD       uint8 = 15
	KeyL1         uint8 = 16
	KeyR1         uint8 = 17
	KeyL2         uint8 = 18
	KeyR2         uint8 = 19
	KeyJoyAux1    uint8 = 20
	KeyJoyAux2    uint8 = 21
	KeyJoyAux3    uint8 = 22
	KeyJoyAux4    uint8 = 23
	KeySelect     uint8 = 24
	KeyStart      uint8 = 25
	KeyTurbo      uint8 = 26
	KeyClear      uint8 = 27
)

// SplashMicros is one load-phase tick long enough to complete any custom
// cartlib.Loader (typically ready at 2.5s) and cartlib.MinSplashMicros
// (500ms). The game is not ticked during load, so it never sees this dt.
const SplashMicros = 3_000_000

// AdvancePastSplash consumes cartlib's load phase with one SplashMicros
// update, then renders once so the guest leaves the load phase.
func AdvancePastSplash(t *testing.T, cart *Cartridge) {
	t.Helper()
	ctx := context.Background()
	if err := cart.Update(ctx, SplashMicros); err != nil {
		t.Fatalf("Update (splash skip): %v", err)
	}
	if err := cart.Render(ctx); err != nil {
		t.Fatalf("Render (splash skip): %v", err)
	}
}

// MatrixFrame converts one recorded blit (row-major, DefaultWidth*DefaultHeight
// pixels) into a Matrix, failing the test on a wrong-sized frame.
func MatrixFrame(t *testing.T, pixels []Pixel) Matrix {
	t.Helper()
	if len(pixels) != DefaultWidth*DefaultHeight {
		t.Fatalf("blit frame has %d pixels, want %d", len(pixels), DefaultWidth*DefaultHeight)
	}
	m := make(Matrix, DefaultHeight)
	for y := range m {
		m[y] = pixels[y*DefaultWidth : (y+1)*DefaultWidth]
	}
	return m
}

// PixelAt indexes a row-major DefaultWidth-wide blit frame.
func PixelAt(pixels []Pixel, x, y int) Pixel {
	return pixels[y*DefaultWidth+x]
}

// LastBlit returns the most recent host_blit frame, failing the test if
// none was recorded.
func (h *FakeHost) LastBlit(t *testing.T) []Pixel {
	t.Helper()
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if len(h.Blits) == 0 {
		t.Fatal("no blit recorded")
	}
	return h.Blits[len(h.Blits)-1]
}

// PlayCount counts host_play_sound calls.
func (h *FakeHost) PlayCount() int {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	n := 0
	for _, s := range h.Sounds {
		if s.Action == "play" {
			n++
		}
	}
	return n
}

// LastLCDForSeat returns the most recent host_set_text call addressed to
// seat, failing the test if that seat's LCD was never written.
func (h *FakeHost) LastLCDForSeat(t *testing.T, seat int) TextSetCall {
	t.Helper()
	h.Mu.Lock()
	defer h.Mu.Unlock()
	for i := len(h.LCDTexts) - 1; i >= 0; i-- {
		if h.LCDTexts[i].Player == seat {
			return h.LCDTexts[i]
		}
	}
	t.Fatalf("no host_set_text recorded for seat %d", seat)
	return TextSetCall{}
}

// LastLCDLine1 returns line1 of the most recent host_set_text call
// addressed to player, or "" if none was. It does not lock Mu.
func LastLCDLine1(host *FakeHost, player int) string {
	for i := len(host.LCDTexts) - 1; i >= 0; i-- {
		if host.LCDTexts[i].Player == player {
			return host.LCDTexts[i].Line1
		}
	}
	return ""
}

// SoundLoadedID returns the id host_load_sound assigned to path, or 0 if it
// was never loaded. It does not lock Mu.
func SoundLoadedID(host *FakeHost, path string) uint32 {
	for _, c := range host.Sounds {
		if c.Action == "load" && c.Path == path {
			return c.ID
		}
	}
	return 0
}

// SoundPlayed reports whether id was ever the target of a host_play_sound
// call. It does not lock Mu.
func SoundPlayed(host *FakeHost, id uint32) bool {
	if id == 0 {
		return false
	}
	for _, c := range host.Sounds {
		if c.Action == "play" && c.ID == id {
			return true
		}
	}
	return false
}

// LastLCD returns the most recent host_set_text call to any seat, failing
// the test if none was recorded.
func (h *FakeHost) LastLCD(t *testing.T) TextSetCall {
	t.Helper()
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if len(h.LCDTexts) == 0 {
		t.Fatal("no host_set_text recorded")
	}
	return h.LCDTexts[len(h.LCDTexts)-1]
}

// SawLCDLine1 reports whether any host_set_text call's line1 ever equaled
// want.
func SawLCDLine1(host *FakeHost, want string) bool {
	host.Mu.Lock()
	defer host.Mu.Unlock()
	for _, lt := range host.LCDTexts {
		if lt.Line1 == want {
			return true
		}
	}
	return false
}

// SawLCDLine2 is SawLCDLine1 for line2.
func SawLCDLine2(host *FakeHost, want string) bool {
	host.Mu.Lock()
	defer host.Mu.Unlock()
	for _, lt := range host.LCDTexts {
		if lt.Line2 == want {
			return true
		}
	}
	return false
}

// SamePixels reports whether two blit frames agree on every pixel.
func SamePixels(a, b []Pixel) bool {
	if len(a) != len(b) || b == nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SameRect reports whether two DefaultWidth-wide blits agree on every
// pixel inside the (x,y,w,h) rectangle.
func SameRect(a, b []Pixel, x, y, w, h int) bool {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			i := yy*DefaultWidth + xx
			if i < 0 || i >= len(a) || i >= len(b) || a[i] != b[i] {
				return false
			}
		}
	}
	return true
}

// RandConsumed reports how many values the cartridge has drawn from the
// RandScript so far, capped at its length.
func (h *FakeHost) RandConsumed() int {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	return h.randIdx
}

// AssertBlitsIdentical fails unless two hosts recorded byte-identical
// host_blit sequences, the determinism check between two fresh runs.
func AssertBlitsIdentical(t *testing.T, a, b *FakeHost) {
	t.Helper()
	if len(a.Blits) != len(b.Blits) {
		t.Fatalf("runs produced %d vs %d frames", len(a.Blits), len(b.Blits))
	}
	for i := range a.Blits {
		if len(a.Blits[i]) != len(b.Blits[i]) {
			t.Fatalf("frame %d: %d vs %d pixels", i, len(a.Blits[i]), len(b.Blits[i]))
		}
		for j := range a.Blits[i] {
			if a.Blits[i][j] != b.Blits[i][j] {
				t.Fatalf("frame %d differs at pixel %d: %+v vs %+v", i, j, a.Blits[i][j], b.Blits[i][j])
			}
		}
	}
}
