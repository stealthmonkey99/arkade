// SPDX-License-Identifier: GPL-2.0-or-later

// Package example is a worked example of the level pipeline: a pack file
// (example.arkadelevels), the Go generated from it (levels_gen.go), and the few lines
// a cartridge writes to read them. Both files are checked in.
package example

import "github.com/stealthmonkey99/arkade/sdk/levels"

// Cell ordinals for DemoPack's legend, in declaration order, for indexing
// tables with [levels.Legend.Ordinal].
const (
	cellWall = iota
	cellFloor
	cellStart
	cellCoin
	cellWater
	cellLily
	cellNone
)

// cellSolid says, per cell ordinal, whether the cell blocks movement.
var cellSolid = [...]bool{
	cellWall:  true,
	cellFloor: false,
	cellStart: false,
	cellCoin:  false,
	cellWater: true,
	cellLily:  false,
	cellNone:  false,
}

// Start returns the level's start cell.
func Start(lv *levels.Level) (x, y int, ok bool) { return lv.Find('@') }

// Next returns the level lv's `exit next` leads to, and whether there is one.
func Next(lv *levels.Level) (*levels.Level, bool) {
	e := lv.Exit("next")
	if e == nil {
		return nil, false
	}
	return &DemoPack.Levels[e.Target], true
}

// Sign returns the text of the level's `ent sign`, if it has one.
func Sign(lv *levels.Level) (string, bool) {
	r := lv.Record("sign")
	if r == nil {
		return "", false
	}
	return r.Args.Str(0)
}

// Route returns the waypoints of the level's `ent duck`: its position, then
// each coordinate pair in its arguments. It reuses out's storage.
func Route(lv *levels.Level, out [][2]int) [][2]int {
	r := lv.Record("duck")
	if r == nil {
		return out[:0]
	}
	out = append(out[:0], [2]int{r.X, r.Y})
	for i := 0; ; i += 2 {
		x, y, ok := r.Args.Point(i)
		if !ok {
			return out
		}
		out = append(out, [2]int{x, y})
	}
}

// Tint returns the colour the level's "tint" layer paints (x, y) with, read
// from the parameter of the legend that layer is written in.
func Tint(lv *levels.Level, x, y int) (rgb uint32, ok bool) {
	ly := lv.Layer("tint")
	if ly == nil {
		return 0, false
	}
	lg := DemoPack.LayerLegend(ly)
	return lg.Values.Color(lg.Ordinal(ly.At(x, y)))
}

// Coins returns how many coins the level holds.
func Coins(lv *levels.Level) int { return lv.Count('*') }

// Blocked reports whether (x, y) cannot be walked into: outside the level, a
// solid cell, or water with no lily pad on the level's "lilies" layer.
func Blocked(lv *levels.Level, x, y int) bool {
	c := lv.At(x, y)
	if c == 0 {
		return true
	}
	ord := DemoPack.Legend.Ordinal(c)
	if ord < 0 || !cellSolid[ord] {
		return false
	}
	if c == '~' {
		if lilies := lv.Layer("lilies"); lilies != nil && lilies.At(x, y) == 'o' {
			return false
		}
	}
	return true
}
