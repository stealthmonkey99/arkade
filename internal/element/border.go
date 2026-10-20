// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// BorderKind is ui.BorderKind: how a shape's edge is drawn.
type BorderKind = ui.BorderKind

const (
	BorderNone     = ui.BorderNone
	BorderColor    = ui.BorderColor
	BorderEdge     = ui.BorderEdge
	BorderDarkEdge = ui.BorderDarkEdge
)

// Border is a shape's edge: a kind and, for BorderColor, its colour.
type Border struct {
	Kind  BorderKind
	Color display.Pixel
}

// NoBorder is the empty border.
var NoBorder = Border{Kind: BorderNone}

// EdgeBorder bevels using the fill color.
var EdgeBorder = Border{Kind: BorderEdge}

// DarkEdgeBorder bevels with a deeper shadow.
var DarkEdgeBorder = Border{Kind: BorderDarkEdge}

// BorderOf builds a solid-color border.
func BorderOf(p display.Pixel) Border { return Border{Kind: BorderColor, Color: p} }

// edgeColors resolves the highlight and shadow pixels for a border over a
// fill, as ui.Border bevels them.
func (b Border) edgeColors(fill display.Pixel) (highlight, shadow display.Pixel) {
	base := fill
	if base.A == 0 {
		base = display.Gray
	}
	switch b.Kind {
	case BorderEdge:
		return display.Merge(base, display.Highlight), display.Merge(base, display.Shadow)
	case BorderDarkEdge:
		return display.Merge(base, display.Highlight), display.Merge(base, display.DarkShadow)
	default:
		return b.Color, b.Color
	}
}
