// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// Highlight and Shadow are the semi-transparent overlay colors a beveled
// Border blends onto its top/left and bottom/right edges. DarkShadow is the
// deeper shadow BorderDarkEdge uses instead of Shadow.
var (
	Highlight  = Color{255, 255, 255, 70}
	Shadow     = Color{0, 0, 0, 70}
	DarkShadow = Color{0, 0, 0, 100}
)

// borderBase is the bevel's base color when Fill is transparent, so the bevel
// still shows instead of blending onto nothing.
var borderBase = Color{120, 120, 120, 255}

// BorderKind selects how Border draws its edge.
type BorderKind int

const (
	// BorderNone draws no edge, only the fill (if any).
	BorderNone BorderKind = iota
	// BorderColor draws a solid 1px Color edge.
	BorderColor
	// BorderEdge draws a beveled highlight/shadow edge (a "raised" look).
	BorderEdge
	// BorderDarkEdge draws a beveled highlight/dark-shadow edge (a deeper
	// "raised" look than BorderEdge).
	BorderDarkEdge
)

// Border is a filled rectangle with an optional 1px edge, solid or beveled.
// Rect covers a solid Stroke; Border exists for the bevel's asymmetric
// per-row pattern.
type Border struct {
	X, Y, W, H int // bounds, in pixels; nothing is drawn when W or H is not positive
	// Fill paints the interior and is the bevel's base color for BorderEdge
	// and BorderDarkEdge; a transparent Fill bevels over mid-gray.
	Fill Color
	Kind BorderKind // edge style; the zero value is [BorderNone]
	// Color is the edge color when Kind == BorderColor; unused otherwise.
	Color Color
}

// EdgeBorder returns a raised-bevel Border at the given geometry and fill.
func EdgeBorder(x, y, w, h int, fill Color) Border {
	return Border{X: x, Y: y, W: w, H: h, Fill: fill, Kind: BorderEdge}
}

// DarkEdgeBorder is EdgeBorder with a deeper shadow.
func DarkEdgeBorder(x, y, w, h int, fill Color) Border {
	return Border{X: x, Y: y, W: w, H: h, Fill: fill, Kind: BorderDarkEdge}
}

// SolidBorder returns a Border with a flat, single-color 1px edge.
func SolidBorder(x, y, w, h int, fill, edge Color) Border {
	return Border{X: x, Y: y, W: w, H: h, Fill: fill, Kind: BorderColor, Color: edge}
}

// edgeColors resolves the highlight and shadow for b's edge, blended over
// Fill (or mid-gray when Fill is transparent).
func (b Border) edgeColors() (highlight, shadow Color) {
	base := b.Fill
	if base.A == 0 {
		base = borderBase
	}
	switch b.Kind {
	case BorderEdge:
		return BlendOver(base, Highlight), BlendOver(base, Shadow)
	case BorderDarkEdge:
		return BlendOver(base, Highlight), BlendOver(base, DarkShadow)
	default:
		return b.Color, b.Color
	}
}

// Draw composites the fill and then, if Kind != BorderNone, the edge onto cv.
// The light source is upper-left:
//
//   - top row (when H > 3): highlight, full width
//   - bottom row: shadow, full width
//   - interior rows (when H > 3): left column highlight, right column shadow
//   - H <= 3: only the bottom row and each other row's right column get
//     shadow, and no highlight is drawn
func (b Border) Draw(cv Canvas) {
	if b.W <= 0 || b.H <= 0 {
		return
	}
	if b.Fill.A != 0 {
		FillRect(cv, b.X, b.Y, b.W, b.H, b.Fill)
	}
	if b.Kind == BorderNone {
		return
	}
	highlight, shadow := b.edgeColors()
	for row := 0; row < b.H; row++ {
		y := b.Y + row
		switch {
		case row == 0 && b.H > 3:
			HLine(cv, b.X, b.X+b.W-1, y, highlight)
		case row == b.H-1:
			HLine(cv, b.X, b.X+b.W-1, y, shadow)
		case b.H > 3:
			cv.Blend(b.X, y, highlight)
			cv.Blend(b.X+b.W-1, y, shadow)
		default:
			cv.Blend(b.X+b.W-1, y, shadow)
		}
	}
}
