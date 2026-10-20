// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels"
	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

// roomsSrc is a pack of rooms exercising records, exits and a patrol route
// spelled as coordinate arguments.
const roomsSrc = `pack rooms

legend
  #  wall
  \s floor

level west-floor
  wing west
  |####|
  |#  #|
  |####|
  exit right west-pod
  exit up corridor-n2 mapwing quota50
  ent terminal 12 48 file1
  ent npc 40 48 guard
  ent patrol 8 52 24 52 40 52

level west-pod
  |####|
  |#  #|
  |####|
  exit left west-floor

level corridor-n2
  |####|
  |#  #|
  |####|
  exit down west-floor mapwing quota50
`

func TestParseRecords(t *testing.T) {
	p := mustParse(t, roomsSrc)
	lv := &p.Levels[0]
	if got := len(lv.Records); got != 3 {
		t.Fatalf("%d records, want 3", got)
	}

	term := lv.Record("terminal")
	if term == nil {
		t.Fatal("no terminal record")
	}
	if term.X != 12 || term.Y != 48 {
		t.Errorf("terminal at (%d, %d), want (12, 48)", term.X, term.Y)
	}
	if s, ok := term.Args.Str(0); !ok || s != "file1" {
		t.Errorf(`terminal arg 0 = (%q, %v), want ("file1", true)`, s, ok)
	}
	if _, ok := term.Args.Int(0); ok {
		t.Error("terminal arg 0 read as an int")
	}
	if _, ok := term.Args.Str(1); ok {
		t.Error("terminal has a second argument it was never given")
	}

	// records keep file order, unsorted
	var kinds []string
	for i := range lv.Records {
		kinds = append(kinds, lv.Records[i].Kind)
	}
	if strings.Join(kinds, ",") != "terminal,npc,patrol" {
		t.Errorf("record kinds = %v, want authored order", kinds)
	}
	if n := lv.CountRecords("npc"); n != 1 {
		t.Errorf(`CountRecords("npc") = %d, want 1`, n)
	}
	if n := lv.CountRecords("goat"); n != 0 {
		t.Errorf(`CountRecords("goat") = %d, want 0`, n)
	}
	if lv.Record("goat") != nil {
		t.Error(`Record("goat") found a record the level does not have`)
	}
}

func TestParseRouteRecordReadsAsWaypoints(t *testing.T) {
	p := mustParse(t, roomsSrc)
	r := p.Levels[0].Record("patrol")
	if r == nil {
		t.Fatal("no patrol record")
	}
	want := [][2]int{{8, 52}, {24, 52}, {40, 52}}
	var got [][2]int
	// the record's own position is the first waypoint
	got = append(got, [2]int{r.X, r.Y})
	for i := 0; ; i += 2 {
		x, y, ok := r.Args.Point(i)
		if !ok {
			break
		}
		got = append(got, [2]int{x, y})
	}
	if len(got) != len(want) {
		t.Fatalf("%d waypoints, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("waypoint %d = %v, want %v", i, got[i], want[i])
		}
	}
	if _, _, ok := r.Args.Point(3); ok {
		t.Error("Point read a waypoint past the end of the argument list")
	}
}

func TestParseExitsResolveToLevelIndexes(t *testing.T) {
	p := mustParse(t, roomsSrc)
	floor := &p.Levels[0]

	right := floor.Exit("right")
	if right == nil {
		t.Fatal("no right exit")
	}
	if right.Target != 1 || p.Levels[right.Target].Name != "west-pod" {
		t.Errorf("right exit targets %d (%q), want 1 (west-pod)", right.Target, p.Levels[right.Target].Name)
	}
	if len(right.Locks) != 0 {
		t.Errorf("right exit has %d locks, want none", len(right.Locks))
	}

	// corridor-n2 is declared after the level that exits into it
	up := floor.Exit("up")
	if up == nil {
		t.Fatal("no up exit")
	}
	if p.Levels[up.Target].Name != "corridor-n2" {
		t.Errorf("up exit targets %q, want corridor-n2", p.Levels[up.Target].Name)
	}
	if s, ok := up.Locks.Str(0); !ok || s != "mapwing" {
		t.Errorf(`up lock 0 = (%q, %v), want ("mapwing", true)`, s, ok)
	}
	if s, ok := up.Locks.Str(1); !ok || s != "quota50" {
		t.Errorf(`up lock 1 = (%q, %v), want ("quota50", true)`, s, ok)
	}

	if floor.Exit("down") != nil {
		t.Error(`Exit("down") found an edge that is a wall`)
	}
}

func TestParseArgKinds(t *testing.T) {
	p := mustParse(t, `pack tiny
legend
  #  wall
level ONE
  |#|
  ent sign 0 0 12 -3 true false #d04040 bare "two words" "12"
`)
	a := p.Levels[0].Record("sign").Args
	if len(a) != 8 {
		t.Fatalf("%d arguments, want 8", len(a))
	}
	if n, ok := a.Int(0); !ok || n != 12 {
		t.Errorf("Int(0) = (%d, %v)", n, ok)
	}
	if n, ok := a.Int(1); !ok || n != -3 {
		t.Errorf("Int(1) = (%d, %v), want (-3, true) — a negative ARG is fine, only a coordinate is bounded", n, ok)
	}
	if v, ok := a.Bool(2); !ok || !v {
		t.Errorf("Bool(2) = (%v, %v)", v, ok)
	}
	if v, ok := a.Bool(3); !ok || v {
		t.Errorf("Bool(3) = (%v, %v)", v, ok)
	}
	if c, ok := a.Color(4); !ok || c != 0xd04040 {
		t.Errorf("Color(4) = (%#06x, %v), want (0xd04040, true)", c, ok)
	}
	if s, ok := a.Str(5); !ok || s != "bare" {
		t.Errorf("Str(5) = (%q, %v)", s, ok)
	}
	if s, ok := a.Str(6); !ok || s != "two words" {
		t.Errorf("Str(6) = (%q, %v)", s, ok)
	}
	if s, ok := a.Str(7); !ok || s != "12" {
		t.Errorf("Str(7) = (%q, %v)", s, ok)
	}
	if _, ok := a.Int(7); ok {
		t.Error(`Int(7) read the quoted "12" as a number`)
	}
	if _, ok := a.Int(99); ok {
		t.Error("Int(99) found an argument past the end")
	}
}

func TestParseColorMetadata(t *testing.T) {
	p := mustParse(t, `pack tiny
legend
  #  wall
level ONE
  tint #0A0b0C
  |#|
`)
	if c, ok := p.Levels[0].Color("tint"); !ok || c != 0x0a0b0c {
		t.Errorf(`Color("tint") = (%#06x, %v), want (0x0a0b0c, true)`, c, ok)
	}
	if _, ok := p.Levels[0].Str("tint"); ok {
		t.Error("a colour also read back as a string — the kinds are not disjoint")
	}
}

const paletteSrc = `pack art

legend
  # wall
  . blank

legend palette
  r red   #d04040
  g grass #40a040
  . none

level P01
  |##|
  |..|
  layer reveal palette
  |rg|
  |..|
`

func TestParseValuedLayerLegend(t *testing.T) {
	p := mustParse(t, paletteSrc)
	if len(p.Legends) != 1 || p.Legends[0].Name != "palette" {
		t.Fatalf("extra legends = %+v, want one named palette", p.Legends)
	}

	lv := &p.Levels[0]
	ly := lv.Layer("reveal")
	if ly == nil {
		t.Fatal("no reveal layer")
	}
	if ly.Legend != "palette" {
		t.Errorf("layer legend = %q, want palette", ly.Legend)
	}
	lg := p.LayerLegend(ly)
	if lg == nil || lg.Name != "palette" {
		t.Fatalf("LayerLegend = %+v, want the palette legend", lg)
	}

	for _, tc := range []struct {
		c    byte
		want uint32
		ok   bool
	}{
		{'r', 0xd04040, true},
		{'g', 0x40a040, true},
		{'.', 0, false}, // declared, but with no parameter
	} {
		got, ok := lg.Values.Color(lg.Ordinal(tc.c))
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("palette %q = (%#06x, %v), want (%#06x, %v)", tc.c, got, ok, tc.want, tc.ok)
		}
	}

	// '.' is "blank" in the main legend and "none" in the palette
	if p.Legend.Names[p.Legend.Ordinal('.')] != "blank" {
		t.Error("the main legend's '.' was overwritten by the palette's")
	}
	// A layer that names no legend still uses the main one.
	if p.LegendNamed("") != &p.Legend {
		t.Error(`LegendNamed("") is not the main legend`)
	}
	if p.LegendNamed("nope") != nil {
		t.Error("LegendNamed found a legend that does not exist")
	}
}

func TestUnvaluedLegendEntryIsMissingAtEveryType(t *testing.T) {
	p := mustParse(t, paletteSrc)
	lg := p.LegendNamed("palette")
	i := lg.Ordinal('.')
	if _, ok := lg.Values.Color(i); ok {
		t.Error("an unvalued entry read as a colour")
	}
	if _, ok := lg.Values.Int(i); ok {
		t.Error("an unvalued entry read as an int")
	}
	if _, ok := lg.Values.Str(i); ok {
		t.Error("an unvalued entry read as a string")
	}
	if _, ok := lg.Values.Bool(i); ok {
		t.Error("an unvalued entry read as a bool")
	}
	// a legend with no parameters has no value column
	if p.Legend.Values != nil {
		t.Errorf("the main legend has a value column it never used: %+v", p.Legend.Values)
	}
}

func TestParseEntityAndLegendErrors(t *testing.T) {
	const head = "pack tiny\nlegend\n  #  wall\n  \\s floor\n"
	for _, tc := range []struct {
		name string
		src  string
		line int
		want string
	}{
		// records
		{"ent outside a level", head + "ent npc 1 1\n", 5, "`ent` outside a level"},
		{"ent too few fields", head + "level ONE\n  |#|\n  ent npc 1\n", 7, "wants <kind> <x> <y>"},
		{"ent bad kind", head + "level ONE\n  |#|\n  ent 9npc 1 1\n", 7, "entity kind"},
		{"ent non-numeric position", head + "level ONE\n  |#|\n  ent npc a 1\n", 7, "is not two integers"},
		{"ent negative position", head + "level ONE\n  |#|\n  ent npc -1 1\n", 7, "is not two integers"},
		{"ent huge position", head + "level ONE\n  |#|\n  ent npc 70000 1\n", 7, "is not two integers"},
		{"ent bad colour arg", head + "level ONE\n  |#|\n  ent npc 1 1 #ff\n", 7, "not a #rrggbb colour"},
		{"ent unterminated quote", head + "level ONE\n  |#|\n  ent npc 1 1 \"oops\n", 7, "unterminated quoted argument"},
		{"ent between grid rows", head + "level ONE\n  |#|\n  ent npc 1 1\n  |#|\n", 8, "must be contiguous"},
		// exits
		{"exit outside a level", head + "exit left ONE\n", 5, "`exit` outside a level"},
		{"exit too few fields", head + "level ONE\n  |#|\n  exit left\n", 7, "wants <dir> <target-level>"},
		{"exit bad direction", head + "level ONE\n  |#|\n  exit 9left ONE\n", 7, "exit direction"},
		{"exit duplicate direction", head + "level ONE\n  |#|\n  exit left ONE\n  exit left ONE\n", 8, "duplicate `exit left`"},
		{"exit unknown target", head + "level ONE\n  |#|\n  exit left TWO\n", 7, `points at unknown level "TWO"`},
		{"exit bad lock", head + "level ONE\n  |#|\n  exit left ONE #zz\n", 7, "not a #rrggbb colour"},
		// legends
		{"named legend before main", "pack a\nlegend palette\n", 2, "before the main `legend`"},
		{"bad legend name", "pack a\nlegend\n  # w\nlegend 9nope\n", 4, "is not [A-Za-z]"},
		{"duplicate named legend", "pack a\nlegend\n  # w\nlegend p\n  r red\nlegend p\n", 6, `duplicate legend name "p"`},
		{"named legend after a level", head + "level ONE\n  |#|\nlegend p\n", 7, "must come before the first level"},
		{"mixed value kinds", "pack a\nlegend\n  # wall #d04040\n  . none 12\n", 4, "one legend's values are one kind"},
		{"bad legend value", "pack a\nlegend\n  # wall #zz\n", 3, "not a #rrggbb colour"},
		// layers
		{"unknown layer legend", head + "level ONE\n  |#|\n  layer art nope\n  |#|\n", 7, "which the pack does not declare"},
		{"cell outside the layer legend",
			"pack a\nlegend\n  # w\nlegend p\n  r red\nlevel ONE\n  |#|\n  layer art p\n  |#|\n",
			9, `'#' at column 0 is not in legend "p"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := packfile.Parse("t.levels", []byte(tc.src))
			if err == nil {
				t.Fatalf("Parse succeeded, want error %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v\nwant it to contain %q", err, tc.want)
			}
			if line := errLine(t, err, tc.want); line != tc.line {
				t.Errorf("reported on line %d, want %d (%v)", line, tc.line, err)
			}
		})
	}
}

func TestFormatRoundTripsTheNewConstructs(t *testing.T) {
	for _, src := range []string{roomsSrc, paletteSrc} {
		out, err := packfile.Format("t.levels", []byte(src))
		if err != nil {
			t.Fatalf("Format: %v", err)
		}
		if string(out) != src {
			t.Errorf("canonical pack is not a fixed point\n--- got\n%s\n--- want\n%s", out, src)
		}
		got, err := packfile.Parse("t.levels", out)
		if err != nil {
			t.Fatalf("Format's output does not parse: %v\n%s", err, out)
		}
		want := mustParse(t, src)
		assertPacksEqual(t, got, want)
	}
}

func TestFormatCanonicalisesAndKeepsCommentsOnEntities(t *testing.T) {
	src := `pack rooms

legend
  #  wall

// about the palette
legend palette
  // about red
  r    red      #d04040
  G    grass-2  #40a040

level ONE
  |#|
  // about the exit
  exit    left     ONE    vent
  // about the terminal
  ent   terminal    12   48    file1
  layer art    palette
  |r|
`
	out, err := packfile.Format("t.levels", []byte(src))
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"// about the palette", "// about red", "// about the exit", "// about the terminal",
		"legend palette\n",
		"  r red     #d04040\n", // the name column is padded so the values line up
		"  G grass-2 #40a040\n",
		"  exit left ONE vent\n",
		"  ent terminal 12 48 file1\n",
		"  layer art palette\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Format did not produce %q\n%s", want, got)
		}
	}
	again, err := packfile.Format("t.levels", out)
	if err != nil {
		t.Fatalf("Format of Format's own output: %v", err)
	}
	if string(again) != got {
		t.Errorf("Format is not idempotent\n--- first\n%s\n--- second\n%s", got, again)
	}
}

func TestEmitCarriesTheNewConstructs(t *testing.T) {
	p := mustParse(t, roomsSrc)
	src, err := packfile.Emit("main", []*packfile.File{{Path: "rooms.levels", Pack: p}})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	got := spaced(string(src))
	for _, want := range []string{
		`Records: []levels.Record{`,
		`{Kind: "terminal", X: 12, Y: 48, Args: levels.Args{`,
		`{Kind: levels.KindStr, Str: "file1"},`,
		`Exits: []levels.Exit{`,
		`{Dir: "right", Target: 1},`,
		`{Dir: "up", Target: 2, Locks: levels.Args{`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Emit did not produce %q\n%s", want, got)
		}
	}

	art := mustParse(t, paletteSrc)
	src, err = packfile.Emit("main", []*packfile.File{{Path: "art.levels", Pack: art}})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	got = spaced(string(src))
	for _, want := range []string{
		`Legends: []levels.Legend{`,
		`Name: "palette",`,
		`Values: levels.Args{`,
		`{Kind: levels.KindColor, Int: 0xd04040}, // 'r' red`,
		`{Kind: levels.KindNone}, // '.' none`,
		`Legend: "palette",`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Emit did not produce %q\n%s", want, got)
		}
	}
}

func assertPacksEqual(t *testing.T, got, want *levels.Pack) {
	t.Helper()
	if got.Name != want.Name || len(got.Levels) != len(want.Levels) {
		t.Fatalf("pack %q/%d levels, want %q/%d", got.Name, len(got.Levels), want.Name, len(want.Levels))
	}
	if got.Legend.Runes != want.Legend.Runes || len(got.Legends) != len(want.Legends) {
		t.Fatalf("legends differ: %+v vs %+v", got, want)
	}
	for i := range want.Legends {
		g, w := &got.Legends[i], &want.Legends[i]
		if g.Name != w.Name || g.Runes != w.Runes || len(g.Values) != len(w.Values) {
			t.Fatalf("legend %d differs:\n got %+v\nwant %+v", i, g, w)
		}
		for j := range w.Values {
			if g.Values[j] != w.Values[j] {
				t.Errorf("legend %q value %d = %+v, want %+v", w.Name, j, g.Values[j], w.Values[j])
			}
		}
	}
	for i := range want.Levels {
		g, w := &got.Levels[i], &want.Levels[i]
		if g.Name != w.Name || g.Cells != w.Cells || g.W != w.W || g.H != w.H {
			t.Errorf("level %d grid differs:\n got %+v\nwant %+v", i, g, w)
		}
		if len(g.Records) != len(w.Records) || len(g.Exits) != len(w.Exits) || len(g.Layers) != len(w.Layers) {
			t.Fatalf("level %q counts differ:\n got %+v\nwant %+v", w.Name, g, w)
		}
		for j := range w.Records {
			gr, wr := g.Records[j], w.Records[j]
			if gr.Kind != wr.Kind || gr.X != wr.X || gr.Y != wr.Y || len(gr.Args) != len(wr.Args) {
				t.Errorf("level %q record %d = %+v, want %+v", w.Name, j, gr, wr)
				continue
			}
			for k := range wr.Args {
				if gr.Args[k] != wr.Args[k] {
					t.Errorf("level %q record %d arg %d = %+v, want %+v", w.Name, j, k, gr.Args[k], wr.Args[k])
				}
			}
		}
		for j := range w.Exits {
			ge, we := g.Exits[j], w.Exits[j]
			if ge.Dir != we.Dir || ge.Target != we.Target || len(ge.Locks) != len(we.Locks) {
				t.Errorf("level %q exit %d = %+v, want %+v", w.Name, j, ge, we)
				continue
			}
			for k := range we.Locks {
				if ge.Locks[k] != we.Locks[k] {
					t.Errorf("level %q exit %d lock %d = %+v, want %+v", w.Name, j, k, ge.Locks[k], we.Locks[k])
				}
			}
		}
		for j := range w.Layers {
			if g.Layers[j] != w.Layers[j] {
				t.Errorf("level %q layer %d = %+v, want %+v", w.Name, j, g.Layers[j], w.Layers[j])
			}
		}
	}
}

// spaced collapses gofmt's column alignment.
func spaced(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}
