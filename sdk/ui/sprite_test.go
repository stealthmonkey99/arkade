// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "testing"

// disc5 is a 5x5 round hitbox mask with corners off.
var disc5 = Mask{W: 5, H: 5, Rows: []uint64{0x0E, 0x1F, 0x1F, 0x1F, 0x0E}}

func TestOpaqueMaskCollidesOnlyAtFullAlpha(t *testing.T) {
	const w, h = 3, 2
	alphas := []byte{255, 254, 0, 1, 255, 255}
	pix := make([]byte, w*h*4)
	for i, a := range alphas {
		pix[i*4+3] = a
	}
	m := OpaqueMask(w, h, pix)
	if m.W != w || m.H != h || len(m.Rows) != h {
		t.Fatalf("OpaqueMask gave %dx%d with %d rows, want %dx%d with %d", m.W, m.H, len(m.Rows), w, h, h)
	}
	for i, a := range alphas {
		if got := m.At(i%w, i/w); got != (a == 255) {
			t.Errorf("pixel (%d,%d) alpha %d: collidable %v", i%w, i/w, a, got)
		}
	}
}

func TestMaskAt(t *testing.T) {
	if disc5.At(0, 0) || disc5.At(4, 0) || disc5.At(0, 4) || disc5.At(4, 4) {
		t.Fatal("disc corners must not be collidable")
	}
	if !disc5.At(2, 0) || !disc5.At(0, 2) || !disc5.At(2, 2) || !disc5.At(4, 2) {
		t.Fatal("disc edges/center must be collidable")
	}
	if disc5.At(-1, 2) || disc5.At(5, 2) || disc5.At(2, -1) || disc5.At(2, 5) {
		t.Fatal("out of bounds must be false, not a panic")
	}
}

func TestMaskOverlapsCornerMiss(t *testing.T) {
	// Two round shapes whose bounding boxes overlap in a 1x1 corner cell
	// where both corners are cut: a rect test would hit, the mask must not.
	a, b := disc5, disc5
	if !a.Overlaps(0, 0, &b, 2, 2) {
		t.Fatal("heavily overlapping discs must collide")
	}
	if a.Overlaps(0, 0, &b, 4, 4) {
		t.Fatal("corner-only bounding overlap of two cut corners must NOT collide")
	}
	if a.Overlaps(0, 0, &b, 5, 0) || a.Overlaps(0, 0, &b, 0, 5) {
		t.Fatal("adjacent (touching, not overlapping) discs must not collide")
	}
	// Symmetry: swapping which mask is placed where cannot change the answer.
	for dx := -6; dx <= 6; dx++ {
		for dy := -6; dy <= 6; dy++ {
			if a.Overlaps(0, 0, &b, dx, dy) != b.Overlaps(dx, dy, &a, 0, 0) {
				t.Fatalf("Overlaps not symmetric at offset (%d,%d)", dx, dy)
			}
		}
	}
}

func TestMaskOverlapsFarApartAndHugeOffsets(t *testing.T) {
	a, b := disc5, disc5
	if a.Overlaps(0, 0, &b, 100, 0) || a.Overlaps(0, 0, &b, 0, -100) {
		t.Fatal("disjoint masks must not collide")
	}
	// offsets past the uint64 shift width must be false, never UB/panic
	if a.Overlaps(0, 0, &b, 70, 2) || a.Overlaps(70, 2, &b, 0, 0) {
		t.Fatal("offsets beyond 64 columns must be false")
	}
}

// spriteFromRows builds a tiny sprite whose pixels are opaque red where
// rows has bits set and semi-transparent green elsewhere, mirroring what
// OpaqueMask builds (mask == the A==255 pixels only).
func spriteFromRows(w, h int, rows []uint64) Sprite {
	pix := make([]byte, 0, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if rows[y]&(1<<uint(x)) != 0 {
				pix = append(pix, 255, 0, 0, 255)
			} else {
				pix = append(pix, 0, 255, 0, 200)
			}
		}
	}
	return Sprite{W: w, H: h, Pix: string(pix), Mask: Mask{W: w, H: h, Rows: rows}}
}

func TestSpriteDrawBlendsAndSkipsTransparent(t *testing.T) {
	s := spriteFromRows(3, 2, []uint64{0b101, 0b010})
	lay := NewLayer(8, 8)
	s.Draw(lay, 2, 3)
	if got := lay.At(2, 3); got != (Color{255, 0, 0, 255}) {
		t.Fatalf("opaque sprite pixel = %+v", got)
	}
	want := BlendOver(Transparent, Color{0, 255, 0, 200})
	if got := lay.At(3, 3); got != want {
		t.Fatalf("semi-transparent sprite pixel = %+v, want %+v", got, want)
	}
	if got := lay.At(5, 3); got != (Color{}) {
		t.Fatalf("pixel outside sprite = %+v, want transparent", got)
	}
}

func TestSpriteDrawMalformedPixIsNoop(t *testing.T) {
	s := Sprite{W: 4, H: 4, Pix: "short"}
	lay := NewLayer(8, 8)
	s.Draw(lay, 0, 0) // must not panic
	if got := lay.At(0, 0); got != (Color{}) {
		t.Fatalf("malformed sprite drew %+v, want nothing", got)
	}
}

func TestMaskWithFewerRowsThanHIsAShorterShape(t *testing.T) {
	short := Mask{W: 3, H: 5, Rows: []uint64{0x7, 0x7}}
	full := Mask{W: 3, H: 5, Rows: []uint64{0x7, 0x7, 0x7, 0x7, 0x7}}
	if !short.Overlaps(0, 0, &full, 0, 0) || !full.Overlaps(0, 0, &short, 0, 0) {
		t.Fatal("the rows a short mask does have did not collide")
	}
	if short.Overlaps(0, 0, &full, 0, 2) || full.Overlaps(0, 2, &short, 0, 0) {
		t.Fatal("rows past len(Rows) collided; they must read as empty")
	}
	if short.At(0, 2) {
		t.Fatal("At reported a pixel in a row the mask does not have")
	}
	var empty Mask
	if empty.Overlaps(0, 0, &full, 0, 0) || full.Overlaps(0, 0, &empty, 0, 0) {
		t.Fatal("the zero Mask collided")
	}
}
