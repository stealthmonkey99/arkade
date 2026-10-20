// SPDX-License-Identifier: GPL-2.0-or-later

package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// paletteChars maps well-known palette colors to stable glyphs for ASCII
// frame dumps and golden files.
var paletteChars = []struct {
	p display.Pixel
	c rune
}{
	{display.Black, '.'},
	{display.White, '#'},
	{display.Red, 'R'},
	{display.Green, 'G'},
	{display.DarkGreen, 'g'},
	{display.Blue, 'B'},
	{display.Yellow, 'Y'},
	{display.Cyan, 'C'},
	{display.Magenta, 'M'},
	{display.Orange, 'O'},
	{display.LightGray, 'L'},
	{display.Gray, 'm'},
	{display.DarkGray, 'd'},
	{display.Focus, 'F'},
	{display.Transparent, ' '},
}

// ASCII renders a frame as ASCII art plus a legend for non-palette colors.
// Deterministic output — the format golden tests and --dump-frame share.
func ASCII(frame display.Matrix) string {
	known := map[display.Pixel]rune{}
	for _, pc := range paletteChars {
		known[pc.p] = pc.c
	}
	extras := map[display.Pixel]rune{}
	next := 0
	extraGlyphs := []rune("abcefhijknopqrstuvwxz0123456789*+=@$%&?!<>~^")

	var sb strings.Builder
	for _, row := range frame {
		for _, p := range row {
			ch, ok := known[p]
			if !ok {
				ch, ok = extras[p]
				if !ok {
					if next < len(extraGlyphs) {
						ch = extraGlyphs[next]
					} else {
						ch = '?'
					}
					extras[p] = ch
					next++
				}
			}
			sb.WriteRune(ch)
		}
		sb.WriteString("\n")
	}
	if len(extras) > 0 {
		type entry struct {
			ch rune
			p  display.Pixel
		}
		list := make([]entry, 0, len(extras))
		for p, ch := range extras {
			list = append(list, entry{ch, p})
		}
		// Total order: several colors can share the overflow glyph '?',
		// so ties break on the color itself or the output isn't stable.
		sort.Slice(list, func(i, j int) bool {
			if list[i].ch != list[j].ch {
				return list[i].ch < list[j].ch
			}
			a, b := list[i].p, list[j].p
			ak := uint32(a.R)<<24 | uint32(a.G)<<16 | uint32(a.B)<<8 | uint32(a.A)
			bk := uint32(b.R)<<24 | uint32(b.G)<<16 | uint32(b.B)<<8 | uint32(b.A)
			return ak < bk
		})
		sb.WriteString("--- legend ---\n")
		for _, e := range list {
			fmt.Fprintf(&sb, "%c = #%02x%02x%02x/%02x\n", e.ch, e.p.R, e.p.G, e.p.B, e.p.A)
		}
	}
	return sb.String()
}
