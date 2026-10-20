// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// Mask is a bit-per-pixel collision mask: Rows[y] bit x (1<<x) is set when
// local pixel (x, y) is collidable. W is at most 64, the uint64 row width.
// Rows holds one entry per row of H; a row past len(Rows) has no collidable
// pixels, so a Mask built with fewer rows than H is a shorter shape, not an
// invalid one. A Mask can back a [Sprite] or stand alone as a shape, such as
// a round hitbox for a procedurally drawn character.
type Mask struct {
	W, H int      // size in pixels; W must not exceed 64
	Rows []uint64 // one row per entry, bit x set when pixel x is collidable
}

// rows is the height that has collidable data: H capped at len(Rows).
func (m *Mask) rows() int { return min(m.H, len(m.Rows)) }

// At reports whether local pixel (x, y) is collidable. Out-of-bounds is
// simply false, so callers can probe without pre-clamping. It allocates
// nothing.
func (m *Mask) At(x, y int) bool {
	if x < 0 || y < 0 || x >= m.W || y >= m.rows() {
		return false
	}
	return m.Rows[y]&(1<<uint(x)) != 0
}

// Overlaps reports whether any collidable pixel of m placed at (mx, my)
// coincides with a collidable pixel of o placed at (ox, oy), with positions in
// pixels. It allocates nothing, so it is safe to call every Update.
func (m *Mask) Overlaps(mx, my int, o *Mask, ox, oy int) bool {
	x0, x1 := max(mx, ox), min(mx+m.W, ox+o.W)
	y0, y1 := max(my, oy), min(my+m.rows(), oy+o.rows())
	if x0 >= x1 || y0 >= y1 {
		return false
	}
	d := ox - mx // shift aligning o's bit space onto m's
	for y := y0; y < y1; y++ {
		a := m.Rows[y-my]
		b := o.Rows[y-oy]
		if d >= 0 {
			b <<= uint(d)
		} else {
			b >>= uint(-d)
		}
		if a&b != 0 {
			return true
		}
	}
	return false
}

// OpaqueMask builds the collision mask of w×h straight-alpha RGBA pixels
// (row-major, 4 bytes each): a pixel is collidable exactly when its alpha is
// 255. This is the rule behind every [Sprite.Mask] cartlib lifts from a
// bundled sheet, and the one to follow when building a Sprite by hand. w must be
// at most 64 and pix at least w*h*4 bytes long, or OpaqueMask panics. It
// allocates the rows, so call it from Init, not per frame.
func OpaqueMask(w, h int, pix []byte) Mask {
	rows := make([]uint64, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if pix[(y*w+x)*4+3] == 255 {
				rows[y] |= 1 << uint(x)
			}
		}
	}
	return Mask{W: w, H: h, Rows: rows}
}

// Sprite is drawable pixel art plus its collision mask, usually lifted from a
// bundled sheet by cartlib's Sheet.Cel in Init (docs/ASSETS.md, "Sprites
// from the bundle"). Pix holds straight-alpha RGBA bytes, row-major, len
// W*H*4, as a string so the art is immutable once built.
//
// A pixel with A==255 is part of the collidable Mask; anything lower draws
// blended but never collides (see [OpaqueMask]).
type Sprite struct {
	W, H int    // size, in pixels
	Pix  string // straight-alpha RGBA, row-major, 4 bytes per pixel
	Mask Mask   // collision shape; see [OpaqueMask]
}

// Draw blends the sprite onto cv with its top-left at (x, y). Pixels with
// A==0 are skipped and the rest go through [Canvas.Blend]. A Pix shorter than
// W*H*4 draws nothing, silently. Call it from the render phase; it allocates
// nothing.
func (s *Sprite) Draw(cv Canvas, x, y int) {
	if len(s.Pix) < s.W*s.H*4 {
		return // malformed literal: draw nothing rather than panic
	}
	i := 0
	for py := 0; py < s.H; py++ {
		for px := 0; px < s.W; px++ {
			if a := s.Pix[i+3]; a != 0 {
				cv.Blend(x+px, y+py, Color{R: s.Pix[i], G: s.Pix[i+1], B: s.Pix[i+2], A: a})
			}
			i += 4
		}
	}
}
