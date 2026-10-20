// SPDX-License-Identifier: GPL-2.0-or-later

// Package levels holds the cartridge-side shape of a cartridge's authored levels:
// the Go literals the level generator emits from a text pack file. The pack
// format and the generator live in the host-side subpackage
// [github.com/stealthmonkey99/arkade/sdk/levels/packfile]; this package parses
// nothing: the host-only tooling is packfile, and this package is the half that
// runs in the cartridge.
//
// A level is a grid of single-byte cells, a pack-wide [Legend] naming each cell
// character, and open metadata read through typed accessors. Further grids over
// the same board are named [Layer]s. What a byte cannot carry has its own type:
// a [Record] is a typed thing at a position, an [Exit] is an edge to another
// level in the same pack, and a named [Legend] can attach a parameter to each
// cell character. Their arguments, locks and parameters are all [Args].
//
//	for y := 0; y < lv.H; y++ {
//	    for x := 0; x < lv.W; x++ {
//	        if lv.At(x, y) == '#' {
//	            drawWall(x, y)
//	        }
//	    }
//	}
//	par, _ := lv.Int("par")
//
// Everything here runs inside the cartridge under TinyGo, so it uses no maps,
// no imports and no allocation, at load or afterwards. Lookups scan short
// slices linearly. The generated pack must be composite literals of constants
// only, with no function calls and no init: a table built by a function is
// silently absent in the wasm build while host-side tests still pass.
package levels

// Kind says which of an [Entry]'s or [Value]'s value fields is meaningful. KindInt is the
// zero value, so a validation hook can return a derived integer as a bare
// literal such as levels.Entry{Key: "par", Int: n}.
type Kind uint8

// The Kind values.
const (
	KindInt   Kind = iota // Entry.Int / Value.Int
	KindStr               // Entry.Str / Value.Str
	KindBool              // Entry.Int / Value.Int, 0 or 1
	KindColor             // Entry.Int / Value.Int, 0xRRGGBB
	KindNone              // no value, as for an unvalued legend entry
)

// Entry is one metadata key and its value. The generator writes a level's
// entries sorted by Key.
type Entry struct {
	Key  string // metadata key, unique within a level
	Kind Kind   // which of Int and Str is meaningful
	Int  int    // KindInt's value; KindBool's, as 0 or 1
	Str  string // KindStr's value
}

// Value is one typed value in a positional slot: a [Record] argument, an
// [Exit] lock or a [Legend] entry's parameter. It has the fields of an [Entry]
// without the key.
type Value struct {
	Kind Kind   // which of Int and Str is meaningful
	Int  int    // KindInt's value; KindBool's as 0 or 1; KindColor's as 0xRRGGBB
	Str  string // KindStr's value
}

// Args is a positional list of [Value]s. Every accessor takes an index and
// reports ok=false when it is out of range or holds a value of another kind,
// so a string where a number was expected reads as absent, not as zero.
type Args []Value

// Int returns argument i as an integer.
func (a Args) Int(i int) (value int, ok bool) {
	if v := a.at(i, KindInt); v != nil {
		return v.Int, true
	}
	return 0, false
}

// Str returns argument i as a string.
func (a Args) Str(i int) (value string, ok bool) {
	if v := a.at(i, KindStr); v != nil {
		return v.Str, true
	}
	return "", false
}

// Bool returns argument i as a boolean. The first result is the value, the
// second says whether there was one.
func (a Args) Bool(i int) (value, ok bool) {
	if v := a.at(i, KindBool); v != nil {
		return v.Int != 0, true
	}
	return false, false
}

// Color returns argument i as a packed 0xRRGGBB colour.
func (a Args) Color(i int) (value uint32, ok bool) {
	if v := a.at(i, KindColor); v != nil {
		return uint32(v.Int), true
	}
	return 0, false
}

// Point returns arguments i and i+1 as an x, y pair; ok is false unless both
// are integers. A route is a [Record] whose arguments alternate x and y:
//
//	for i := 0; ; i += 2 {
//	    x, y, ok := r.Args.Point(i)
//	    if !ok {
//	        break
//	    }
//	    ...
//	}
func (a Args) Point(i int) (x, y int, ok bool) {
	px, okx := a.Int(i)
	py, oky := a.Int(i + 1)
	if !okx || !oky {
		return 0, 0, false
	}
	return px, py, true
}

func (a Args) at(i int, kind Kind) *Value {
	if i < 0 || i >= len(a) || a[i].Kind != kind {
		return nil
	}
	return &a[i]
}

// Legend names the grid characters a pack uses. Runes[i] is cell ordinal i's
// character and Names[i] is the name the pack file declared for it, in
// declaration order, so parallel tables (tile colors, sprites) can be indexed
// by ordinal.
//
// Name is "" for a pack's main legend and the declared name for an extra one
// ([Pack.Legends]). Values, when non-empty, holds one [Value] per ordinal, the
// parameter that entry carries:
//
//	rgb, ok := lg.Values.Color(lg.Ordinal(c))
//
// An entry written without a parameter has [KindNone], so it reads as missing
// at every type.
type Legend struct {
	Name   string   // "" for the main legend
	Runes  string   // cell characters; Runes[i] is ordinal i
	Names  []string // declared name per ordinal, parallel to Runes
	Values Args     // per-ordinal parameter; empty when the legend has none
}

// Ordinal returns c's cell ordinal, or -1 if c is not in the legend. It scans
// Runes linearly.
func (lg *Legend) Ordinal(c byte) int {
	for i := 0; i < len(lg.Runes); i++ {
		if lg.Runes[i] == c {
			return i
		}
	}
	return -1
}

// Layer is an additional named grid over the same level, with its own
// dimensions (an edge grid is one column wider than the cells it separates).
//
// A layer is written in the pack's main legend unless Legend names another,
// which is "" for the main legend. [Pack.LayerLegend] resolves it.
type Layer struct {
	Name   string // layer name, unique within a level
	Legend string // name of the legend the cells use; "" for the main legend
	W, H   int    // dimensions, in cells
	Cells  string // row-major, W*H bytes
}

// At returns the character at (x, y), or 0 outside the layer.
func (ly *Layer) At(x, y int) byte {
	if x < 0 || y < 0 || x >= ly.W || y >= ly.H {
		return 0
	}
	return ly.Cells[y*ly.W+x]
}

// Record is one typed thing at one position in a level: a spawn point, an
// enemy, a pickup, a trigger, a patrol route. Kind is the cartridge's own word
// for it; this package assigns it no meaning.
//
// X and Y carry no unit: the cartridge decides whether they are cells or
// pixels, and a validation hook is where range checks belong.
//
// Args are the per-kind properties; a route's waypoints go there too (see
// [Args.Point]).
type Record struct {
	Kind string // the cartridge's own word for the thing
	X, Y int    // position, in the cartridge's chosen unit
	Args Args   // per-kind properties, in authored order
}

// Exit is one edge out of a level into another level of the same pack.
//
// Target is an index into [Pack.Levels]. The parser resolves the level name
// written in the pack file, so a dangling reference fails the build.
//
// Dir is a token the cartridge gives meaning to, such as "left" or "next", and
// is unique within a level. Locks are what the doorway asks of the player, such
// as a quota tier, an ability name or an item.
type Exit struct {
	Dir    string // direction token, unique within a level
	Target int    // index into Pack.Levels
	Locks  Args   // what the doorway asks of the player
}

// Level is one authored level: its main grid, any extra layers, its records and
// exits, and its metadata.
type Level struct {
	Name    string   // level name, unique within a pack
	W, H    int      // main grid dimensions, in cells
	Cells   string   // the main grid, row-major, W*H bytes
	Layers  []Layer  // extra named grids
	Records []Record // typed things, in authored order
	Exits   []Exit   // edges to other levels of the pack
	Meta    []Entry  // metadata, sorted by Key
}

// At returns the main grid's character at (x, y), or 0 outside the level. No
// cell character is treated as solid; a game that wants outside to be wall
// wraps this.
func (l *Level) At(x, y int) byte {
	if x < 0 || y < 0 || x >= l.W || y >= l.H {
		return 0
	}
	return l.Cells[y*l.W+x]
}

// Row returns grid row y as a W-byte string, or "" outside the level. It
// slices Cells, so it never copies or allocates.
func (l *Level) Row(y int) string {
	if y < 0 || y >= l.H {
		return ""
	}
	return l.Cells[y*l.W : (y+1)*l.W]
}

// Count returns how many main-grid cells hold c.
func (l *Level) Count(c byte) int {
	n := 0
	for i := 0; i < len(l.Cells); i++ {
		if l.Cells[i] == c {
			n++
		}
	}
	return n
}

// Find returns the position of the first c in the main grid, scanning
// row-major, and whether there was one.
func (l *Level) Find(c byte) (x, y int, ok bool) {
	for i := 0; i < len(l.Cells); i++ {
		if l.Cells[i] == c {
			return i % l.W, i / l.W, true
		}
	}
	return 0, 0, false
}

// Layer returns the named extra grid, or nil if the level has none.
func (l *Level) Layer(name string) *Layer {
	for i := range l.Layers {
		if l.Layers[i].Name == name {
			return &l.Layers[i]
		}
	}
	return nil
}

// Record returns the level's first record of kind, or nil. Records keep the
// order the pack file authored them in, so first means first on the page.
func (l *Level) Record(kind string) *Record {
	for i := range l.Records {
		if l.Records[i].Kind == kind {
			return &l.Records[i]
		}
	}
	return nil
}

// CountRecords returns how many of the level's records have the given kind.
func (l *Level) CountRecords(kind string) int {
	n := 0
	for i := range l.Records {
		if l.Records[i].Kind == kind {
			n++
		}
	}
	return n
}

// Exit returns the level's exit in direction dir, or nil if there is none.
func (l *Level) Exit(dir string) *Exit {
	for i := range l.Exits {
		if l.Exits[i].Dir == dir {
			return &l.Exits[i]
		}
	}
	return nil
}

// Int returns the integer metadata value for key. ok is false when the key is
// absent or holds a value of another kind, so a typo in the pack file reads as
// missing rather than as zero.
func (l *Level) Int(key string) (value int, ok bool) {
	if e := l.entry(key, KindInt); e != nil {
		return e.Int, true
	}
	return 0, false
}

// Str returns the string metadata value for key, with the same
// wrong-kind-is-missing rule as [Level.Int].
func (l *Level) Str(key string) (value string, ok bool) {
	if e := l.entry(key, KindStr); e != nil {
		return e.Str, true
	}
	return "", false
}

// Bool returns the boolean metadata value for key. The first result is the
// value, the second says whether the key was there, with the same
// wrong-kind-is-missing rule as [Level.Int].
func (l *Level) Bool(key string) (value, ok bool) {
	if e := l.entry(key, KindBool); e != nil {
		return e.Int != 0, true
	}
	return false, false
}

// Color returns the packed 0xRRGGBB metadata value for key, with the same
// wrong-kind-is-missing rule as [Level.Int].
func (l *Level) Color(key string) (value uint32, ok bool) {
	if e := l.entry(key, KindColor); e != nil {
		return uint32(e.Int), true
	}
	return 0, false
}

func (l *Level) entry(key string, kind Kind) *Entry {
	for i := range l.Meta {
		if l.Meta[i].Key == key && l.Meta[i].Kind == kind {
			return &l.Meta[i]
		}
	}
	return nil
}

// Pack is one authored level pack, the contents of one .arkadelevels file. Legend is
// the main alphabet every level's grid is written in; Legends holds the extra
// named ones a [Layer] may use.
type Pack struct {
	Name    string   // pack name
	Legend  Legend   // the main legend
	Legends []Legend // extra named legends
	Levels  []Level  // levels in authored order; Exit.Target indexes this
}

// LegendNamed returns the pack's legend called name: the main legend for "",
// one of [Pack.Legends] otherwise, or nil when there is no such legend.
func (p *Pack) LegendNamed(name string) *Legend {
	if name == "" {
		return &p.Legend
	}
	for i := range p.Legends {
		if p.Legends[i].Name == name {
			return &p.Legends[i]
		}
	}
	return nil
}

// LayerLegend returns the legend ly's cells are written in. It is never nil for
// a layer from a parsed pack, since the parser rejects unknown legend names.
func (p *Pack) LayerLegend(ly *Layer) *Legend { return p.LegendNamed(ly.Legend) }
