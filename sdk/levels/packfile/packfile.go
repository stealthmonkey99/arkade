// SPDX-License-Identifier: GPL-2.0-or-later

// Package packfile reads the Årkade level pack format and writes the
// generated Go a cartridge compiles. It runs in a host-side generator, never
// inside a cartridge, so unlike [github.com/stealthmonkey99/arkade/sdk/levels]
// it may use maps, fmt and unbounded allocation.
//
// # The pack file format
//
// One file holds one pack, named `<name>` plus [ArkadeExt] ([Ext] is also read;
// [HasExt] accepts either). It is line-oriented ASCII text. A complete file:
//
//	// Comments start with // and take the whole line.
//	pack solo
//
//	legend
//	  #  wall
//	  \s floor
//	  .  target
//	  $  crate
//	  *  crate-on-target
//
//	level L01
//	  par 1
//	  p1x 3
//	  p1y 1
//	  |#####|
//	  |#.$ #|
//	  |#   #|
//	  |#####|
//
//	level "FIRST BOX"
//	  budget 3
//	  |#####|
//	  |#$ .#|
//	  |#####|
//	  layer hazard
//	  |.....|
//	  |..~..|
//	  |.....|
//
// Leading and trailing whitespace on a line is insignificant. Blank lines and
// comment lines are dropped before parsing and may appear anywhere, including
// between grid rows.
//
// Six directives:
//
//   - `pack NAME`: exactly once, first. NAME is `[A-Za-z][A-Za-z0-9_-]*` and
//     names the generated variable (see [Emit]).
//   - `legend`: exactly once, after `pack` and before any level. It declares
//     the main legend, the alphabet every level's grid is written in. Each
//     following line is `CHARACTER NAME [VALUE]`: one printable-ASCII
//     character (or `\s` for space, `\\` for backslash), the name for that
//     cell, `[A-Za-z0-9][A-Za-z0-9_-]*`, and an optional parameter (see
//     "Valued legends"). Characters and names must each be unique within a
//     legend, and `|` cannot be a legend character because it delimits grid
//     rows.
//   - `legend NAME`: an extra legend, also before the first level, for a layer
//     whose cells do not mean terrain. Same entry syntax, with its own
//     characters and names.
//   - `level NAME`: starts a level. NAME is a bare token, or a quoted Go
//     string literal when it contains spaces (`level "FIRST BOX"`), and is
//     unique within the pack.
//   - `layer NAME [LEGEND]`: inside a level, after the main grid, starts an
//     additional named grid ([levels.Layer]). NAME is a bare token, unique
//     within the level. LEGEND names one of the pack's extra legends; without
//     it the layer is written in the main legend.
//   - `ent KIND X Y [ARG ...]`: one [levels.Record]. See "Records and exits".
//   - `exit DIR TARGET [LOCK ...]`: one [levels.Exit]. See "Records and exits".
//
// A line's first token is a directive if it is one of those words, so they
// cannot be metadata keys. Otherwise, inside a level, a line beginning with
// `|` is a grid row and anything else is a metadata entry.
//
// A grid row is `|`, exactly W cell characters, `|`. The delimiters keep an
// editor that strips trailing whitespace from narrowing a row of spaces. Every
// character must be in the grid's legend, every row of one grid must be the
// same width, and the rows of one grid must be contiguous.
//
// A metadata entry is `KEY VALUE`, where KEY is `[A-Za-z][A-Za-z0-9_-]*` and
// VALUE is the rest of the line, trimmed. The value's type comes from its
// spelling, and a cartridge reads it back through the matching typed accessor:
//
//	par 12            int      lv.Int("par")
//	coop true         bool     lv.Bool("coop")
//	tint #d04040      colour   lv.Color("tint")
//	hint "go left"    string   lv.Str("hint")
//	hint go left      string   (quotes are only needed for "12", "true", a
//	                            leading '#', or leading/trailing spaces)
//
// A token starting with `#` is a colour and must be exactly six hex digits;
// any other token beginning with `#` is an error, not a string. Keys must be
// unique within a level, and `name` is not a valid key because a level's name
// is the token on its `level` line.
//
// # Records and exits
//
// Records and exits carry data a one-byte cell cannot:
//
//	level west-floor
//	  wing west
//	  |#######..######|
//	  ... 14 more rows ...
//	  exit right west-pod
//	  exit up corridor-n2 mapwing
//	  ent terminal 12 48 file1
//	  ent npc 40 48 guard
//	  ent patrol 8 52 24 52 40 52
//
// `ent KIND X Y [ARG ...]` is one record. KIND is a bare token with a meaning
// only the cartridge knows. X and Y are non-negative integers up to 65535 with
// no unit (see [levels.Record]). ARGs are positional values typed by spelling,
// as metadata is, and are read back through [levels.Args]. A route is a record
// whose arguments alternate x and y ([levels.Args.Point]).
//
// `exit DIR TARGET [LOCK ...]` is one edge out of the level. DIR is a bare
// token the cartridge gives meaning to, unique within the level. TARGET is
// another level's name, spelled as its `level` line spells it. The parser
// resolves it, so an exit to a level the pack does not contain is an error with
// the line number, and [levels.Exit].Target is an index into
// [levels.Pack].Levels. LOCKs are values typed by spelling.
//
// Records and exits keep the order they were written in; metadata is sorted by
// key.
//
// # Valued legends
//
// A legend entry may carry one value after its name:
//
//	legend palette
//	  r  red    #d04040
//	  g  grass  #40a040
//	  .  none
//
//	level P01
//	  |###|
//	  layer reveal palette
//	  |rg.|
//
// The parameter is typed by spelling like any other value, and a cartridge
// reads it by ordinal: `lg.Values.Color(lg.Ordinal(c))`.
//
// Every valued entry of one legend must be the same kind, else the parse
// fails. An entry written without a parameter is [levels.KindNone] and reads as
// missing at every type.
//
// # Errors
//
// [Parse] reports every problem it finds (up to 20), each as an [Error]
// carrying the file and line:
//
//	solo.arkadelevels:14: row is 6 cells wide, the grid's first row is 5
//	solo.arkadelevels:21: 'Q' at column 3 is not in the legend
//	solo.arkadelevels:30: duplicate level name "L01"
//	rooms.arkadelevels:88: level "west-floor": `exit right` points at unknown level "west-podd"
//
// # In a cartridge
//
// A cartridge's packs live in [AssetsDir] and compile to [GenFile], the
// levels.Pack literals TinyGo builds in: a cartridge has no filesystem to read
// a pack from at run time, and static data folds into the wasm at compile time
// where a parser would cost code size and allocation. The pack file is the
// source; GenFile is build output, committed like manifest_gen.go, regenerated
// by every cartridge build and refused by preflight when stale. A cartridge
// with generation-time checks runs them from its own [GeneratorDir], whose
// main is [Main].
//
// # Size
//
// A grid ships as one byte per cell, folded to a single Go string constant by
// [Emit]. A packed bitmask would be smaller but is not readable in a diff.
//
// # Envelope front matter
//
// A pack file may begin with one asset envelope, the common name, description,
// tags and pinned generator that every asset kind carries, as a fenced block of
// comment lines before `pack`:
//
//	// --- arkade ---
//	// {
//	//   "arkade": 1,
//	//   "kind": "levels",
//	//   "name": "walls",
//	//   "generator": { "tool": "levellab", "version": "1.0.0" }
//	// }
//	// --- arkade ---
//
//	pack walls
//	...
//
// [SplitEnvelope] and [FormatEnvelope] find the block's JSON and render one
// back. [Parse] drops the block as ordinary comment lines, so a pack decodes to
// the same [levels.Pack] with or without it.
package packfile

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/stealthmonkey99/arkade/sdk/levels"
)

// Ext is the alternate pack file extension. Files named with it are read
// exactly as [ArkadeExt] files are; name a new pack with ArkadeExt.
const Ext = ".levels"

// ArkadeExt is a pack file's extension, named like the other asset kinds
// (walls.arkadelevels beside pop.arkadesound.wav). [Render] and [HasPacks]
// read every file in their directory that ends in it or in [Ext].
const ArkadeExt = ".arkadelevels"

// HasExt reports whether name ends in [Ext] or [ArkadeExt].
func HasExt(name string) bool {
	return strings.HasSuffix(name, Ext) || strings.HasSuffix(name, ArkadeExt)
}

// maxDim stops a malformed file from turning into a megabyte of generated Go.
const maxDim = 256

const maxErrors = 20

// maxCoord is a sanity rail only: the unit of a record's X and Y is the
// cartridge's, so its real bound belongs in a Hook.
const maxCoord = 65535

// Error is one problem at one line of one pack file.
type Error struct {
	File string // name as passed to [Parse]
	Line int    // 1-based line number
	Msg  string // what is wrong
}

// Error formats as file:line: message.
func (e *Error) Error() string { return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg) }

// File is one parsed pack file: the pack it declares and the path it was read
// from. The generated file cites only the path's base name.
type File struct {
	Path string       // file the pack was read from
	Pack *levels.Pack // the parsed pack
}

// ParseFile reads and parses one pack file.
func ParseFile(path string) (*File, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pack, err := Parse(path, src)
	if err != nil {
		return nil, err
	}
	return &File{Path: path, Pack: pack}, nil
}

// Parse parses one pack file's bytes. name appears in error messages and is
// otherwise unused, so callers holding source in memory can pass any label.
//
// On failure the returned error joins every [Error] found, in line order.
func Parse(name string, src []byte) (*levels.Pack, error) {
	p := &parser{file: name, pack: &levels.Pack{}}
	for i, raw := range strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		p.do(i+1, line)
	}
	p.finishLevel()
	p.finishPack()
	if len(p.errs) > 0 {
		return nil, errors.Join(p.errs...)
	}
	return p.pack, nil
}

type section uint8

const (
	sectionNone section = iota
	sectionLegend
	sectionLevel
)

type parser struct {
	file string
	errs []error
	pack *levels.Pack

	sec  section
	line int // the line currently being consumed, for errorf

	// the main legend or the newest of pack.Legends, while sec is sectionLegend
	legend *levels.Legend

	lvName  string
	lvLine  int
	lvMeta  []levels.Entry
	lvRec   []levels.Record
	lvExit  []levels.Exit
	lvLayer []levels.Layer
	lvGrid  levels.Level // only W/H/Cells; set once the main grid closes
	lvHas   bool

	// the grid under construction: the main grid when layerName is ""
	layerName   string
	layerLegend string
	layerLine   int
	rows        []string
	rowsDone    bool // a non-row line has closed this grid

	// an exit may target a level declared later, so resolution waits for finishPack
	pending []pendingExit
}

type pendingExit struct {
	level, exit int    // indices into pack.Levels and that level's Exits
	target      string // the level name as the file spelled it
	line        int
}

func (p *parser) errorf(line int, format string, args ...any) {
	if len(p.errs) >= maxErrors {
		return
	}
	p.errs = append(p.errs, &Error{File: p.file, Line: line, Msg: fmt.Sprintf(format, args...)})
	if len(p.errs) == maxErrors {
		p.errs = append(p.errs, &Error{File: p.file, Line: line, Msg: "too many errors"})
	}
}

func (p *parser) do(line int, s string) {
	p.line = line
	keyword, rest := split2(s)
	switch keyword {
	case "pack":
		p.doPack(rest)
		return
	case "legend":
		p.doLegend(rest)
		return
	case "level":
		p.doLevel(rest)
		return
	case "layer":
		p.doLayer(rest)
		return
	case "ent":
		p.doEnt(rest)
		return
	case "exit":
		p.doExit(rest)
		return
	}
	// checked before the row prefix so a `|` here is reported as an illegal legend character
	if p.sec == sectionLegend {
		p.doLegendEntry(s)
		return
	}
	if strings.HasPrefix(s, "|") {
		p.doRow(s)
		return
	}
	if p.sec == sectionLevel {
		p.doMeta(keyword, rest)
		return
	}
	p.errorf(line, "expected `pack`, got %q", keyword)
}

func (p *parser) doPack(rest string) {
	if p.pack.Name != "" {
		p.errorf(p.line, "a file declares exactly one pack (already %q)", p.pack.Name)
		return
	}
	if p.sec != sectionNone {
		p.errorf(p.line, "`pack` must come first")
		return
	}
	if !isIdent(rest) {
		p.errorf(p.line, "pack name %q is not [A-Za-z][A-Za-z0-9_-]*", rest)
		return
	}
	p.pack.Name = rest
}

func (p *parser) doLegend(rest string) {
	if p.pack.Name == "" {
		p.errorf(p.line, "`legend` before `pack`")
		return
	}
	if p.sec == sectionLevel {
		p.errorf(p.line, "`legend` must come before the first level")
		return
	}
	if rest == "" {
		if len(p.pack.Legend.Runes) > 0 {
			p.errorf(p.line, "a file declares exactly one main legend (extra ones are named: `legend palette`)")
			return
		}
		p.sec, p.legend = sectionLegend, &p.pack.Legend
		return
	}
	if !isIdent(rest) {
		p.errorf(p.line, "legend name %q is not [A-Za-z][A-Za-z0-9_-]*", rest)
		return
	}
	if len(p.pack.Legend.Runes) == 0 {
		p.errorf(p.line, "named legend %q before the main `legend`", rest)
		return
	}
	for i := range p.pack.Legends {
		if p.pack.Legends[i].Name == rest {
			p.errorf(p.line, "duplicate legend name %q", rest)
			return
		}
	}
	p.pack.Legends = append(p.pack.Legends, levels.Legend{Name: rest})
	p.sec, p.legend = sectionLegend, &p.pack.Legends[len(p.pack.Legends)-1]
}

func (p *parser) doLegendEntry(s string) {
	lg := p.legend
	if lg == nil { // `legend` itself was rejected; its entries have no home
		return
	}
	char, rest := split2(s)
	name, valTok := split2(rest)
	c, ok := unescapeRune(char)
	if !ok {
		p.errorf(p.line, "legend entry starts with %q, want one printable character (or \\s for space)", char)
		return
	}
	if c == '|' {
		p.errorf(p.line, "'|' cannot be a legend character; it delimits grid rows")
		return
	}
	if !isLegendName(name) {
		p.errorf(p.line, "legend name %q is not [A-Za-z0-9][A-Za-z0-9_-]*", name)
		return
	}
	if lg.Ordinal(c) >= 0 {
		p.errorf(p.line, "duplicate legend character %s", quoteRune(c))
		return
	}
	for _, existing := range lg.Names {
		if existing == name {
			p.errorf(p.line, "duplicate legend name %q", name)
			return
		}
	}
	value := levels.Value{Kind: levels.KindNone}
	if valTok != "" {
		v, err := parseScalar(valTok)
		if err != nil {
			p.errorf(p.line, "legend %s %s: %v", quoteRune(c), name, err)
			return
		}
		// one legend is one table indexed by ordinal, so its values share a kind
		for i := range lg.Values {
			if lg.Values[i].Kind != levels.KindNone && lg.Values[i].Kind != v.Kind {
				p.errorf(p.line, "legend %s %s has %s, but %s in this legend has %s — one legend's values are one kind",
					quoteRune(c), name, kindName(v.Kind), quoteRune(lg.Runes[i]), kindName(lg.Values[i].Kind))
				return
			}
		}
		value = v
	}
	lg.Runes += string(c)
	lg.Names = append(lg.Names, name)
	lg.Values = append(lg.Values, value)
}

func (p *parser) doLevel(rest string) {
	if len(p.pack.Legend.Runes) == 0 {
		p.errorf(p.line, "`level` before `legend`")
		return
	}
	p.finishLevel()
	name, ok := levelName(rest)
	if !ok {
		p.errorf(p.line, "level name %q must be a bare token or a quoted string", rest)
		name = rest
	}
	for i := range p.pack.Levels {
		if p.pack.Levels[i].Name == name {
			p.errorf(p.line, "duplicate level name %q", name)
			break
		}
	}
	p.sec, p.legend = sectionLevel, nil
	p.lvName, p.lvLine = name, p.line
}

func (p *parser) doLayer(rest string) {
	if p.sec != sectionLevel {
		p.errorf(p.line, "`layer` outside a level")
		return
	}
	if !p.lvHas && len(p.rows) == 0 {
		p.errorf(p.line, "level %q: `layer` before the level's own grid", p.lvName)
		return
	}
	name, legend := split2(rest)
	if !isIdent(name) {
		p.errorf(p.line, "layer name %q is not [A-Za-z][A-Za-z0-9_-]*", name)
		return
	}
	if legend != "" && p.pack.LegendNamed(legend) == nil {
		p.errorf(p.line, "layer %q is written in legend %q, which the pack does not declare", name, legend)
		legend = "" // fall back to the main legend so the rows still get checked
	}
	p.finishGrid()
	for i := range p.lvLayer {
		if p.lvLayer[i].Name == name {
			p.errorf(p.line, "level %q: duplicate layer name %q", p.lvName, name)
			break
		}
	}
	p.layerName, p.layerLegend, p.layerLine = name, legend, p.line
}

func (p *parser) doEnt(rest string) {
	if p.sec != sectionLevel {
		p.errorf(p.line, "`ent` outside a level")
		return
	}
	p.closeRows()
	toks, err := splitTokens(rest)
	if err != nil {
		p.errorf(p.line, "`ent`: %v", err)
		return
	}
	if len(toks) < 3 {
		p.errorf(p.line, "`ent` wants <kind> <x> <y> [arg ...], got %q", rest)
		return
	}
	if !isIdent(toks[0]) {
		p.errorf(p.line, "entity kind %q is not [A-Za-z][A-Za-z0-9_-]*", toks[0])
		return
	}
	x, okx := coord(toks[1])
	y, oky := coord(toks[2])
	if !okx || !oky {
		p.errorf(p.line, "`ent %s` position (%s, %s) is not two integers in 0..%d",
			toks[0], toks[1], toks[2], maxCoord)
		return
	}
	args, err := parseArgs(toks[3:])
	if err != nil {
		p.errorf(p.line, "`ent %s`: %v", toks[0], err)
		return
	}
	p.lvRec = append(p.lvRec, levels.Record{Kind: toks[0], X: x, Y: y, Args: args})
}

func (p *parser) doExit(rest string) {
	if p.sec != sectionLevel {
		p.errorf(p.line, "`exit` outside a level")
		return
	}
	p.closeRows()
	toks, err := splitTokens(rest)
	if err != nil {
		p.errorf(p.line, "`exit`: %v", err)
		return
	}
	if len(toks) < 2 {
		p.errorf(p.line, "`exit` wants <dir> <target-level> [lock ...], got %q", rest)
		return
	}
	dir := toks[0]
	if !isIdent(dir) {
		p.errorf(p.line, "exit direction %q is not [A-Za-z][A-Za-z0-9_-]*", dir)
		return
	}
	for i := range p.lvExit {
		if p.lvExit[i].Dir == dir {
			p.errorf(p.line, "level %q: duplicate `exit %s`", p.lvName, dir)
			return
		}
	}
	target, ok := levelName(toks[1])
	if !ok {
		p.errorf(p.line, "exit target %q must be a bare token or a quoted string", toks[1])
		return
	}
	locks, err := parseArgs(toks[2:])
	if err != nil {
		p.errorf(p.line, "`exit %s`: %v", dir, err)
		return
	}
	p.pending = append(p.pending, pendingExit{
		level:  len(p.pack.Levels), // this level's index once finishLevel flushes it
		exit:   len(p.lvExit),
		target: target,
		line:   p.line,
	})
	p.lvExit = append(p.lvExit, levels.Exit{Dir: dir, Target: -1, Locks: locks})
}

func (p *parser) doRow(s string) {
	if p.sec != sectionLevel {
		p.errorf(p.line, "grid row outside a level")
		return
	}
	if !strings.HasSuffix(s, "|") || len(s) < 2 {
		p.errorf(p.line, "grid row must end with '|'")
		return
	}
	if p.rowsDone {
		p.errorf(p.line, "grid rows must be contiguous")
		return
	}
	row := s[1 : len(s)-1]
	if row == "" {
		p.errorf(p.line, "empty grid row")
		return
	}
	if len(p.rows) > 0 && len(row) != len(p.rows[0]) {
		p.errorf(p.line, "row is %d cells wide, the grid's first row is %d", len(row), len(p.rows[0]))
		return
	}
	if len(p.rows) == 0 && len(row) > maxDim {
		p.errorf(p.line, "row is %d cells wide, the limit is %d", len(row), maxDim)
		return
	}
	if len(p.rows) == maxDim {
		p.errorf(p.line, "grid is taller than %d rows", maxDim)
		return
	}
	lg := p.activeLegend()
	for x := 0; x < len(row); x++ {
		if lg.Ordinal(row[x]) < 0 {
			if lg.Name == "" {
				p.errorf(p.line, "%s at column %d is not in the legend", quoteRune(row[x]), x)
			} else {
				p.errorf(p.line, "%s at column %d is not in legend %q", quoteRune(row[x]), x, lg.Name)
			}
			break
		}
	}
	p.rows = append(p.rows, row)
}

func (p *parser) activeLegend() *levels.Legend {
	if p.layerName != "" && p.layerLegend != "" {
		if lg := p.pack.LegendNamed(p.layerLegend); lg != nil {
			return lg
		}
	}
	return &p.pack.Legend
}

// closeRows makes any later row an error instead of a second half of the grid.
func (p *parser) closeRows() {
	if len(p.rows) > 0 {
		p.rowsDone = true
	}
}

func (p *parser) doMeta(key, value string) {
	p.closeRows()
	if !isIdent(key) {
		p.errorf(p.line, "%q is not a directive, and not a metadata key [A-Za-z][A-Za-z0-9_-]*", key)
		return
	}
	if key == "name" {
		p.errorf(p.line, "`name` is not a metadata key; a level is named on its `level` line")
		return
	}
	if value == "" {
		p.errorf(p.line, "metadata key %q has no value", key)
		return
	}
	for i := range p.lvMeta {
		if p.lvMeta[i].Key == key {
			p.errorf(p.line, "level %q: duplicate metadata key %q", p.lvName, key)
			return
		}
	}
	entry, err := parseValue(key, value)
	if err != nil {
		p.errorf(p.line, "metadata %s: %v", key, err)
		return
	}
	p.lvMeta = append(p.lvMeta, entry)
}

func (p *parser) finishGrid() {
	rows, name, legend, line := p.rows, p.layerName, p.layerLegend, p.layerLine
	p.rows, p.layerName, p.layerLegend, p.rowsDone = nil, "", "", false
	if len(rows) == 0 {
		if name != "" {
			p.errorf(line, "level %q: layer %q has no grid rows", p.lvName, name)
		}
		return
	}
	w, h := len(rows[0]), len(rows)
	cells := strings.Join(rows, "")
	if name == "" {
		p.lvGrid, p.lvHas = levels.Level{W: w, H: h, Cells: cells}, true
		return
	}
	p.lvLayer = append(p.lvLayer, levels.Layer{Name: name, Legend: legend, W: w, H: h, Cells: cells})
}

func (p *parser) finishLevel() {
	if p.sec != sectionLevel {
		return
	}
	p.finishGrid()
	if !p.lvHas {
		p.errorf(p.lvLine, "level %q has no grid", p.lvName)
	} else {
		lv := p.lvGrid
		lv.Name, lv.Layers, lv.Meta = p.lvName, p.lvLayer, p.lvMeta
		lv.Records, lv.Exits = p.lvRec, p.lvExit
		sortEntries(lv.Meta)
		p.pack.Levels = append(p.pack.Levels, lv)
	}
	p.lvName, p.lvMeta, p.lvLayer, p.lvHas = "", nil, nil, false
	p.lvRec, p.lvExit = nil, nil
}

func (p *parser) finishPack() {
	if p.pack.Name == "" {
		p.errorf(1, "no `pack` directive")
	}
	if len(p.pack.Legend.Runes) == 0 {
		p.errorf(1, "no `legend` block")
	}
	p.resolveExits()
	trimValues(&p.pack.Legend)
	for i := range p.pack.Legends {
		trimValues(&p.pack.Legends[i])
	}
	if len(p.pack.Levels) == 0 && len(p.errs) == 0 {
		p.errorf(1, "no levels")
	}
}

func (p *parser) resolveExits() {
	for _, pe := range p.pending {
		if pe.level >= len(p.pack.Levels) {
			continue // its level failed to flush; that error is already reported
		}
		lv := &p.pack.Levels[pe.level]
		target := -1
		for i := range p.pack.Levels {
			if p.pack.Levels[i].Name == pe.target {
				target = i
				break
			}
		}
		if target < 0 {
			p.errorf(pe.line, "level %q: `exit %s` points at unknown level %q",
				lv.Name, lv.Exits[pe.exit].Dir, pe.target)
			continue
		}
		lv.Exits[pe.exit].Target = target
	}
	p.pending = nil
}

// trimValues drops a legend's value column when no entry carried one.
func trimValues(lg *levels.Legend) {
	for _, v := range lg.Values {
		if v.Kind != levels.KindNone {
			return
		}
	}
	lg.Values = nil
}

// split2 splits s into its first whitespace-delimited token and the trimmed
// remainder.
func split2(s string) (first, rest string) {
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}

func unescapeRune(tok string) (byte, bool) {
	switch tok {
	case `\s`:
		return ' ', true
	case `\\`:
		return '\\', true
	}
	if len(tok) != 1 || tok[0] < 0x21 || tok[0] > 0x7E {
		return 0, false
	}
	return tok[0], true
}

// quoteRune spells space as the escape an author would type.
func quoteRune(c byte) string {
	if c == ' ' {
		return `'\s' (space)`
	}
	return strconv.QuoteRune(rune(c))
}

func levelName(rest string) (string, bool) {
	if strings.HasPrefix(rest, `"`) {
		s, err := strconv.Unquote(rest)
		if err != nil || s == "" {
			return "", false
		}
		return s, true
	}
	if rest == "" || strings.ContainsAny(rest, " \t") {
		return "", false
	}
	return rest, true
}

func parseValue(key, value string) (levels.Entry, error) {
	v, err := parseScalar(value)
	if err != nil {
		return levels.Entry{}, err
	}
	return levels.Entry{Key: key, Kind: v.Kind, Int: v.Int, Str: v.Str}, nil
}

// parseScalar types one value from its spelling: true/false is a bool, an
// integer is an int, `#rrggbb` is a colour, a quoted Go string literal is a
// string, and anything else is a string verbatim.
func parseScalar(value string) (levels.Value, error) {
	switch value {
	case "true":
		return levels.Value{Kind: levels.KindBool, Int: 1}, nil
	case "false":
		return levels.Value{Kind: levels.KindBool, Int: 0}, nil
	}
	if n, err := strconv.Atoi(value); err == nil {
		return levels.Value{Kind: levels.KindInt, Int: n}, nil
	}
	if strings.HasPrefix(value, "#") {
		rgb, ok := parseColor(value)
		if !ok {
			// a mistyped colour must not read back as a string
			return levels.Value{}, fmt.Errorf("%s is not a #rrggbb colour (quote it to mean the string)", value)
		}
		return levels.Value{Kind: levels.KindColor, Int: int(rgb)}, nil
	}
	if strings.HasPrefix(value, `"`) {
		s, err := strconv.Unquote(value)
		if err != nil {
			return levels.Value{}, fmt.Errorf("%s is not a valid quoted string", value)
		}
		return levels.Value{Kind: levels.KindStr, Str: s}, nil
	}
	return levels.Value{Kind: levels.KindStr, Str: value}, nil
}

func parseArgs(toks []string) (levels.Args, error) {
	if len(toks) == 0 {
		return nil, nil
	}
	out := make(levels.Args, 0, len(toks))
	for i, tok := range toks {
		v, err := parseScalar(tok)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %v", i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// parseColor decodes `#rrggbb` into 0xRRGGBB; exactly six hex digits.
func parseColor(tok string) (uint32, bool) {
	if len(tok) != 7 || tok[0] != '#' {
		return 0, false
	}
	var rgb uint32
	for i := 1; i < 7; i++ {
		c := tok[i]
		switch {
		case c >= '0' && c <= '9':
			rgb = rgb<<4 | uint32(c-'0')
		case c >= 'a' && c <= 'f':
			rgb = rgb<<4 | uint32(c-'a'+10)
		case c >= 'A' && c <= 'F':
			rgb = rgb<<4 | uint32(c-'A'+10)
		default:
			return 0, false
		}
	}
	return rgb, true
}

func coord(tok string) (int, bool) {
	n, err := strconv.Atoi(tok)
	if err != nil || n < 0 || n > maxCoord {
		return 0, false
	}
	return n, true
}

// splitTokens splits on whitespace, keeping a quoted string literal as one
// token.
func splitTokens(s string) ([]string, error) {
	var out []string
	for i := 0; i < len(s); {
		if s[i] == ' ' || s[i] == '\t' {
			i++
			continue
		}
		if s[i] != '"' {
			j := i
			for j < len(s) && s[j] != ' ' && s[j] != '\t' {
				j++
			}
			out = append(out, s[i:j])
			i = j
			continue
		}
		j := i + 1
		for j < len(s) {
			if s[j] == '\\' {
				j += 2
				continue
			}
			if s[j] == '"' {
				j++
				break
			}
			j++
		}
		if j > len(s) || j == i+1 || s[j-1] != '"' {
			return nil, fmt.Errorf("unterminated quoted argument in %q", s[i:])
		}
		out = append(out, s[i:j])
		i = j
	}
	return out, nil
}

func kindName(k levels.Kind) string {
	switch k {
	case levels.KindStr:
		return "a string"
	case levels.KindBool:
		return "a bool"
	case levels.KindColor:
		return "a colour"
	case levels.KindNone:
		return "no value"
	}
	return "an int"
}

func isIdent(s string) bool {
	if s == "" || !isAlpha(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isAlpha(s[i]) && !isDigit(s[i]) && s[i] != '_' && s[i] != '-' {
			return false
		}
	}
	return true
}

// isLegendName is isIdent that also allows a leading digit.
func isLegendName(s string) bool {
	if s == "" || (!isAlpha(s[0]) && !isDigit(s[0])) {
		return false
	}
	return isIdent("a" + s[1:])
}

func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func sortEntries(e []levels.Entry) {
	for i := 1; i < len(e); i++ {
		for j := i; j > 0 && e[j].Key < e[j-1].Key; j-- {
			e[j], e[j-1] = e[j-1], e[j]
		}
	}
}
