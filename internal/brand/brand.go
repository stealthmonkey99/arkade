// SPDX-License-Identifier: GPL-2.0-or-later

// Package brand holds the Årkade wordmark's reusable artwork: its 5×7
// letterforms, the hand-drawn Å ring, the marquee bulb layout, and the
// additive canvas that paints them.
package brand

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// Point is a pixel coordinate in the wordmark's own layout space.
type Point struct{ X, Y int }

// RGB is a colour as additive light, one component per channel in 0..1.
type RGB [3]float64

// PixelRGB converts a display.Pixel to the 0..1 RGB a Canvas accumulates.
func PixelRGB(p display.Pixel) RGB {
	return RGB{float64(p.R) / 255, float64(p.G) / 255, float64(p.B) / 255}
}

// Amber is the wordmark's colour; wordGlow is the deeper orange of its halo.
var (
	Amber    = RGB{1, 173.0 / 255, 64.0 / 255}
	wordGlow = RGB{1, 0.55, 0.12}
	bulbCore = RGB{1, 0.86, 0.62}
	bulbHalo = RGB{1, 0.7, 0.4}
)

// The wordmark's letterforms, 5×7 and heavier than the engine's 4px LED font.
var (
	glyphA = []string{".###.", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"}
	glyphR = []string{"####.", "#...#", "#...#", "####.", "#.#..", "#..#.", "#...#"}
	glyphK = []string{"#...#", "#..#.", "#.#..", "##...", "#.#..", "#..#.", "#...#"}
	glyphD = []string{"###..", "#..#.", "#...#", "#...#", "#...#", "#..#.", "###.."}
	glyphE = []string{"#####", "#....", "#....", "####.", "#....", "#....", "#####"}
)

// ÅRKADE: the Å is the plain A with its ring drawn separately (RingPixels),
// because animations start from the ring.
var wordGlyphs = [][]string{glyphA, glyphR, glyphK, glyphA, glyphD, glyphE}

const (
	wordX     = 15
	wordY     = 26
	wordPitch = 6 // a 5px glyph and a 1px gap
	bulbPitch = 4
)

// RingCenter is the point animations expand from; RingPixels are the ring
// pixels the wordmark lights.
var (
	RingCenter  = Point{17, 23}
	RingPixels  = []Point{{17, 22}, {16, 23}, {18, 23}, {17, 24}}
	marqueeRect = [4]int{8, 15, 56, 39} // sides are multiples of bulbPitch so a bulb lands on every corner
)

// Bulb is one marquee lamp; Chase is its distance from the top centre going
// either way round, 0 at the top centre and 1 at the bottom.
type Bulb struct {
	Point
	Chase float64
}

type glowSpill struct {
	Point
	weight float64
}

// WordPixels are the lit pixels of every letter, laid out once at init.
// MarqueeBulbs are the marquee's lamps, laid out clockwise from its
// top-left corner.
var (
	WordPixels   []Point
	MarqueeBulbs []Bulb
	glowSpills   []glowSpill
)

func init() {
	for i, glyph := range wordGlyphs {
		for y, row := range glyph {
			for x, ch := range row {
				if ch == '#' {
					WordPixels = append(WordPixels, Point{wordX + i*wordPitch + x, wordY + y})
				}
			}
		}
	}

	MarqueeBulbs = layOutBulbs(marqueeRect)

	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			d2 := dx*dx + dy*dy
			if d2 == 0 || d2 > 10 {
				continue
			}
			glowSpills = append(glowSpills, glowSpill{Point{dx, dy}, 0.16 / float64(1+d2)})
		}
	}
}

// layOutBulbs walks the rectangle clockwise from its top-left corner,
// dropping a bulb every bulbPitch pixels.
func layOutBulbs(r [4]int) []Bulb {
	w, h := r[2]-r[0], r[3]-r[1]
	perimeter := 2 * (w + h)
	topCenter := float64(w) / 2
	var out []Bulb
	for s := 0; s < perimeter; s += bulbPitch {
		var p Point
		switch {
		case s < w:
			p = Point{r[0] + s, r[1]}
		case s < w+h:
			p = Point{r[2], r[1] + s - w}
		case s < 2*w+h:
			p = Point{r[2] - (s - w - h), r[3]}
		default:
			p = Point{r[0], r[3] - (s - 2*w - h)}
		}
		d := math.Abs(float64(s) - topCenter)
		out = append(out, Bulb{p, math.Min(d, float64(perimeter)-d) / (float64(perimeter) / 2)})
	}
	return out
}

// Canvas accumulates light additively in floats, so overlapping highlights
// sum.
type Canvas struct {
	w, h int
	px   []float64
}

// NewCanvas returns a transparent w×h canvas ready to paint into.
func NewCanvas(w, h int) *Canvas {
	return &Canvas{w: w, h: h, px: make([]float64, w*h*3)}
}

// Bounds is the canvas's width and height.
func (c *Canvas) Bounds() (w, h int) { return c.w, c.h }

// Add accumulates col at a into the pixel at p, silently ignoring an
// out-of-range point or a non-positive alpha.
func (c *Canvas) Add(p Point, col RGB, a float64) {
	if a <= 0 || p.X < 0 || p.Y < 0 || p.X >= c.w || p.Y >= c.h {
		return
	}
	i := (p.Y*c.w + p.X) * 3
	c.px[i] += col[0] * a
	c.px[i+1] += col[1] * a
	c.px[i+2] += col[2] * a
}

// AddBulb paints one marquee lamp at intensity v: a bright core with a soft
// halo bleeding onto its four neighbours.
func (c *Canvas) AddBulb(b Point, v float64) {
	c.Add(b, bulbCore, v)
	for _, d := range [...]Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		c.Add(Point{b.X + d.X, b.Y + d.Y}, bulbHalo, v*0.18)
	}
}

const (
	// GlowRest is the wordmark halo's resting level, the centre of its breath.
	GlowRest = 0.45
	// GlowBreath is how far the resting halo swings either side of GlowRest.
	GlowBreath    = 0.1
	glowBreathMs  = 320 // a 2s breath
	bulbTwinkleMs = 260
)

// RestingGlow is the wordmark halo strength at phaseMs into its breath.
func RestingGlow(phaseMs float64) float64 {
	return GlowRest + GlowBreath*math.Sin(phaseMs/glowBreathMs)
}

// BulbTwinkle is the intensity of marquee bulb i (its index in MarqueeBulbs)
// once lit, at phaseMs.
func BulbTwinkle(phaseMs float64, i int) float64 {
	return 0.72 + 0.28*math.Sin(phaseMs/bulbTwinkleMs+float64(i)*0.9)
}

// RestingWordmark draws the wordmark as it rests after the boot splash has
// settled: full strength, breathing halo, twinkling marquee.
func (c *Canvas) RestingWordmark(phaseMs float64) {
	c.Wordmark(1, RestingGlow(phaseMs))
	for i, b := range MarqueeBulbs {
		c.AddBulb(b.Point, BulbTwinkle(phaseMs, i))
	}
}

// Wordmark draws the letterforms and the Å ring at alpha, with an optional
// halo spilling outward from every lit pixel at glow.
func (c *Canvas) Wordmark(alpha, glow float64) {
	for _, p := range WordPixels {
		c.Add(p, Amber, alpha)
	}
	for _, p := range RingPixels {
		c.Add(p, Amber, alpha)
	}
	if glow <= 0 {
		return
	}
	for _, p := range WordPixels {
		for _, s := range glowSpills {
			c.Add(Point{p.X + s.X, p.Y + s.Y}, wordGlow, glow*s.weight)
		}
	}
}

// Matrix converts the accumulated light to a display.Matrix, leaving
// untouched pixels transparent.
func (c *Canvas) Matrix() display.Matrix {
	out := display.NewMatrix(c.w, c.h, display.Transparent)
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			i := (y*c.w + x) * 3
			r, g, b := Channel(c.px[i]), Channel(c.px[i+1]), Channel(c.px[i+2])
			if r|g|b != 0 {
				out[y][x] = display.Pixel{R: r, G: g, B: b, A: 255}
			}
		}
	}
	return out
}

// Channel converts one accumulated light value to an 8-bit colour channel,
// clamping to 0..255.
func Channel(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(v*255 + 0.5)
}
