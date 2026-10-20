// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

const frame = uint64(16_667) // ~60fps tick, matching the engine's cadence

// tickUntil runs Update until it returns true or maxMicros elapses,
// returning the elapsed time and whether it fired.
func tickUntil(h *HoldConfirm, maxMicros uint64) (uint64, bool) {
	elapsed := uint64(0)
	for elapsed < maxMicros {
		if h.Update(frame) {
			return elapsed, true
		}
		elapsed += frame
	}
	return elapsed, false
}

func TestHoldConfirmRealJourney(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.KeyDown() // opening press, still held

	if _, fired := tickUntil(&h, 2_000_000); fired {
		t.Fatal("opening press alone must never confirm")
	}
	if p := h.Progress(); p != 0 {
		t.Fatalf("progress %.2f during unarmed opening hold, want 0", p)
	}

	h.KeyUp() // release arms
	if !h.Armed() {
		t.Fatal("KeyUp should arm the hold")
	}

	h.KeyDown() // the deliberate second press
	elapsed := uint64(0)
	for !h.Confirmed() {
		h.Update(frame)
		elapsed += frame
		if elapsed > 1_200_000 {
			t.Fatalf("not confirmed after %dus of armed hold", elapsed)
		}
	}
	if elapsed < 950_000 || elapsed > 1_100_000 {
		t.Errorf("confirmed after %dus, want ~1,000,000", elapsed)
	}

	if _, fired := tickUntil(&h, 2_000_000); fired {
		t.Fatal("Update fired while B still held after blink")
	}

	h.KeyUp()
	if !h.Update(frame) {
		t.Fatal("Update should fire once B is released after the blink")
	}
	if h.Update(frame) {
		t.Error("Update must fire exactly once")
	}
}

func TestHoldConfirmReleaseDrains(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.KeyDown()
	h.KeyUp() // arm
	h.KeyDown()
	for i := 0; i < 30; i++ { // ~500ms held
		h.Update(frame)
	}
	half := h.Progress()
	if half < 0.4 || half > 0.6 {
		t.Fatalf("progress after ~500ms hold = %.2f, want ~0.5", half)
	}
	h.KeyUp()
	h.Update(150_000) // drains 150ms*3
	got := h.Progress()
	want := half - 0.45
	if got < want-0.02 || got > want+0.02 {
		t.Errorf("progress after 150ms release = %.3f, want ~%.2f (3x drain)", got, want)
	}
	if !h.Active() {
		t.Error("overlay must stay open after release")
	}
	for i := 0; i < 60; i++ {
		h.Update(frame)
	}
	if p := h.Progress(); p != 0 {
		t.Errorf("progress %.2f should fully drain to 0", p)
	}
}

func TestHoldConfirmVirtualJourney(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.Pulse() // opening tap
	if _, fired := tickUntil(&h, 2_000_000); fired {
		t.Fatal("a single virtual tap must not confirm")
	}
	if !h.Armed() {
		t.Fatal("opening tap's credit expiry should have armed the widget")
	}
	if p := h.Progress(); p != 0 {
		t.Fatalf("progress %.2f after lone opening tap, want 0", p)
	}

	elapsed := uint64(0)
	for !h.Confirmed() {
		h.Pulse()
		h.Update(frame)
		elapsed += frame
		if elapsed > 2_000_000 {
			t.Fatalf("pulse train never confirmed after %dus", elapsed)
		}
	}
	for i := 0; i < 60; i++ {
		h.Pulse()
		if h.Update(frame) {
			t.Fatal("Update fired while virtual credit still live")
		}
	}
	if _, fired := tickUntil(&h, 1_500_000); !fired {
		t.Fatal("Update never fired after virtual release")
	}
}

// A browser host sends keydown plus a virtual keypress on press and keyup on
// release: the release must end the hold at once, not PulseMicros later.
func TestHoldConfirmKeyUpDropsVirtualCredit(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.KeyDown()
	h.Pulse()
	h.KeyUp()
	if h.Holding() {
		t.Fatal("KeyUp must clear the opening press's virtual credit")
	}
	if !h.Armed() {
		t.Fatal("KeyUp should arm the hold")
	}

	h.KeyDown()
	h.Pulse()
	for i := 0; i < 30; i++ {
		h.Update(frame)
		h.Pulse() // auto-repeat while physically held
	}
	half := h.Progress()
	if half < 0.4 || half > 0.6 {
		t.Fatalf("progress after ~500ms browser hold = %.2f, want ~0.5", half)
	}

	h.KeyUp()
	if h.Holding() {
		t.Error("Holding() must be false the instant a real keyup arrives")
	}
	h.Update(frame)
	if p := h.Progress(); p >= half {
		t.Errorf("progress %.3f did not drain on the frame after keyup (was %.3f)", p, half)
	}
}

func TestHoldConfirmPulseBridgesRepeatDelay(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.KeyUp() // arm directly (simplest arming path)
	h.Pulse()
	h.Update(400_000) // silence shorter than the 600ms credit
	if p := h.Progress(); p < 0.39 {
		t.Errorf("progress %.2f drained during the auto-repeat gap", p)
	}
}

func TestHoldConfirmOpenIdempotent(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.KeyUp()
	h.KeyDown()
	h.Update(500_000)
	h.Open()
	if p := h.Progress(); p < 0.45 {
		t.Errorf("re-Open reset progress to %.2f", p)
	}
	if !h.Armed() {
		t.Error("re-Open cleared the armed state")
	}
}

func TestHoldConfirmClose(t *testing.T) {
	var h HoldConfirm
	h.Open()
	h.KeyUp()
	h.KeyDown()
	h.Update(1_200_000)
	if !h.Confirmed() {
		t.Fatal("expected confirmed")
	}
	h.Close()
	if h.Active() || h.Progress() != 0 || h.Armed() || h.Confirmed() || h.Update(frame) {
		t.Error("Close must clear active/progress/armed/confirm state")
	}
}

func TestHoldConfirmDraw(t *testing.T) {
	lay := NewLayer(64, 64)
	lay.Fill(Color{0, 200, 0, 255})
	var h HoldConfirm
	h.W, h.H = 64, 64
	h.Draw(lay)
	if got := lay.At(2, 2); got != (Color{0, 200, 0, 255}) {
		t.Fatalf("inactive Draw touched pixels: %v", got)
	}

	h.Open()
	h.Draw(lay)
	if corner := lay.At(2, 2); corner.G >= 200 {
		t.Error("active Draw should dim the backdrop")
	}
	if center := lay.At(32, 32); center == (Color{0, 200, 0, 255}) {
		t.Error("active Draw should paint the panel over the center")
	}

	countRed := func() int {
		n := 0
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				c := lay.At(x, y)
				if c.R > 180 && c.G < 90 && c.B < 90 {
					n++
				}
			}
		}
		return n
	}
	empty := countRed()
	h.KeyUp()
	h.KeyDown()
	h.Update(999_000)
	h.Draw(lay)
	full := countRed()
	if full <= empty {
		t.Errorf("full-hold draw should add red fill pixels: empty=%d full=%d", empty, full)
	}

	h.Update(2_000)
	if !h.Confirmed() {
		t.Fatal("expected confirmed for blink test")
	}
	h.Draw(lay) // blinkAccum ~0 → blink ON (white row)
	whiteOn := countRed()
	h.Update(holdBlinkPeriod + 2_000) // advance into the OFF half-cycle
	h.Draw(lay)
	whiteOff := countRed()
	if whiteOn == whiteOff {
		t.Errorf("blink phases should differ in red pixel count: on=%d off=%d", whiteOn, whiteOff)
	}
}

func TestHoldConfirmQuarterTurnsDraw(t *testing.T) {
	render := func(q int) *Layer {
		lay := NewLayer(64, 64)
		lay.Fill(Color{0, 0, 0, 255})
		h := HoldConfirm{W: 64, H: 64, QuarterTurns: q}
		h.Open()
		h.Draw(lay)
		return lay
	}

	plain := render(0)
	for q := 1; q <= 3; q++ {
		got := render(q)
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				// Where the plain pixel (x, y) lands after q CW turns.
				rx, ry := x, y
				for i := 0; i < q; i++ {
					rx, ry = 63-ry, rx
				}
				if got.At(rx, ry) != plain.At(x, y) {
					t.Fatalf("q=%d: pixel (%d,%d) -> (%d,%d) = %v, want %v (not an exact rotation)", q, x, y, rx, ry, got.At(rx, ry), plain.At(x, y))
				}
			}
		}
	}

	again := render(0)
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if again.At(x, y) != plain.At(x, y) {
				t.Fatalf("q=0 render not deterministic at (%d,%d)", x, y)
			}
		}
	}
}

// TestHoldConfirmAvoidsTheNameEntry pins that the quit confirm never covers
// the band it was told to avoid. The centered panel owns rows 18-45 of a
// 64x64 canvas, where entries commonly sit.
func TestHoldConfirmAvoidsTheNameEntry(t *testing.T) {
	for _, entryY := range []int{27, 30, 34, 40, 42, 44, 45, 50} {
		lay := NewLayer(64, 64)
		lay.Fill(Color{0, 0, 0, 255})

		entry := NewNameEntry(8, entryY, 0)
		h := HoldConfirm{W: 64, H: 64}
		h.Open()
		h.Avoid(entry.LetterBand())
		h.Draw(lay)

		// The bevel is one pixel of a different shade on each edge, so the
		// plate run understates the panel by a row top and bottom.
		top, bottom := -1, -1
		for y := 0; y < 64; y++ {
			if lay.At(32, y) == (Color{26, 26, 32, 255}) {
				if top < 0 {
					top = y
				}
				bottom = y
			}
		}
		if top < 0 {
			t.Fatalf("entryY=%d: no panel drawn at all", entryY)
		}
		top, bottom = top-1, bottom+1
		if top < 0 || bottom >= 64 {
			t.Errorf("entryY=%d: panel spans rows %d..%d, which runs off the canvas", entryY, top, bottom)
		}
		if top <= entryY+ledGlyphRows-1 && bottom >= entryY {
			t.Errorf("entryY=%d: panel spans rows %d..%d and still covers the entry's rows %d..%d",
				entryY, top, bottom, entryY, entryY+ledGlyphRows-1)
		}
	}
}

func drawnConfirm(setup func(h *HoldConfirm)) *Layer {
	lay := NewLayer(64, 64)
	lay.Fill(Color{0, 0, 0, 255})
	h := HoldConfirm{W: 64, H: 64}
	setup(&h)
	h.Draw(lay)
	return lay
}

func layersEqual(a, b *Layer) bool {
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

func TestHoldConfirmAvoidLastsOneOpening(t *testing.T) {
	entry := NewNameEntry(8, 44, 0)
	plain := drawnConfirm(func(h *HoldConfirm) { h.Open() })
	avoided := drawnConfirm(func(h *HoldConfirm) {
		h.Open()
		h.Avoid(entry.LetterBand())
	})
	if layersEqual(plain, avoided) {
		t.Fatal("Avoid over an entry at row 44 left the centred panel where it was")
	}

	cases := map[string]func(h *HoldConfirm){
		"reopened after Close": func(h *HoldConfirm) {
			h.Open()
			h.Avoid(entry.LetterBand())
			h.Close()
			h.Open()
		},
		"Avoid before Open": func(h *HoldConfirm) {
			h.Avoid(entry.LetterBand())
			h.Open()
		},
		"cleared with zero rows": func(h *HoldConfirm) {
			h.Open()
			h.Avoid(entry.LetterBand())
			h.Avoid(0, 0)
		},
		"an entry drawn but not avoided": func(h *HoldConfirm) {
			entry.Draw(NewLayer(64, 64))
			h.Open()
		},
	}
	for name, setup := range cases {
		if !layersEqual(plain, drawnConfirm(setup)) {
			t.Errorf("%s: the panel moved off centre", name)
		}
	}
}
