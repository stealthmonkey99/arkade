// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

// FBWidth and FBHeight are the LED matrix's size in pixels, the only size
// either host uses; cart_init's (cols, rows) always carry them, which is
// why [Game.Init] does not pass them on.
const (
	FBWidth  = 64
	FBHeight = 64
)

// Framebuffer is a cartridge's local RGBA pixel buffer, sent whole to the
// host once per frame after Render (or Attract, or the splash) returns. Pix
// is row-major, interleaved R,G,B,A bytes per pixel, exactly host_blit's
// wire format.
//
// The zero value is transparent black. cartlib owns the one Framebuffer
// every frame is drawn into and does not clear it between frames, so a
// gameplay frame should start with [Framebuffer.Clear]; an attract frame
// may stay transparent where the engine's starfield should show.
type Framebuffer struct {
	// Pix is the pixel data, 4 bytes (R, G, B, A) per pixel, row-major.
	Pix [FBWidth * FBHeight * 4]byte
}

// SetPixel writes one opaque pixel. Coordinates outside the frame do
// nothing.
func (fb *Framebuffer) SetPixel(x, y int, r, g, b byte) {
	if x < 0 || y < 0 || x >= FBWidth || y >= FBHeight {
		return
	}
	o := (y*FBWidth + x) * 4
	fb.Pix[o] = r
	fb.Pix[o+1] = g
	fb.Pix[o+2] = b
	fb.Pix[o+3] = 0xFF
}

// FillRect fills the rectangle [x, x+w) x [y, y+h), clipped to the frame,
// with one opaque color.
func (fb *Framebuffer) FillRect(x, y, w, h int, r, g, b byte) {
	for py := y; py < y+h; py++ {
		for px := x; px < x+w; px++ {
			fb.SetPixel(px, py, r, g, b)
		}
	}
}

// Clear fills the whole frame with one opaque color.
func (fb *Framebuffer) Clear(r, g, b byte) {
	for i := 0; i < len(fb.Pix); i += 4 {
		fb.Pix[i] = r
		fb.Pix[i+1] = g
		fb.Pix[i+2] = b
		fb.Pix[i+3] = 0xFF
	}
}

// ClearTransparent clears the whole frame to transparent black. Use it in
// [Attractor.Attract], where transparent pixels show the engine's starfield;
// a gameplay frame should be opaque, so use [Framebuffer.Clear] there.
func (fb *Framebuffer) ClearTransparent() {
	for i := range fb.Pix {
		fb.Pix[i] = 0
	}
}

func (fb *Framebuffer) blit() {
	host.Blit(fb.Pix[:])
}
