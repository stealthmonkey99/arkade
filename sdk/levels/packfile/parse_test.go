// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels"
	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

func mustParse(t *testing.T, src string) *levels.Pack {
	t.Helper()
	p, err := packfile.Parse("t.levels", []byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return p
}

const minimal = `pack tiny

legend
  #  wall
  \s floor

level ONE
  |###|
  |# #|
  |###|
`

func TestParseMinimal(t *testing.T) {
	p := mustParse(t, minimal)
	if p.Name != "tiny" {
		t.Errorf("pack name = %q, want %q", p.Name, "tiny")
	}
	if p.Legend.Runes != "# " {
		t.Errorf("legend runes = %q, want %q", p.Legend.Runes, "# ")
	}
	if len(p.Legend.Names) != 2 || p.Legend.Names[0] != "wall" || p.Legend.Names[1] != "floor" {
		t.Errorf("legend names = %q, want [wall floor]", p.Legend.Names)
	}
	if len(p.Levels) != 1 {
		t.Fatalf("%d levels, want 1", len(p.Levels))
	}
	lv := p.Levels[0]
	if lv.Name != "ONE" || lv.W != 3 || lv.H != 3 || lv.Cells != "###"+"# #"+"###" {
		t.Errorf("level = %q %dx%d %q", lv.Name, lv.W, lv.H, lv.Cells)
	}
}

func TestParseIgnoresBlanksAndComments(t *testing.T) {
	p := mustParse(t, `
// a comment first

pack tiny
legend
  // and one inside the legend
  #  wall
  \s floor

level ONE
  |###|

  // between rows
  |# #|
  |###|
`)
	if got := p.Levels[0].Cells; got != "###"+"# #"+"###" {
		t.Errorf("cells = %q", got)
	}
}

func TestParseMetadataKinds(t *testing.T) {
	p := mustParse(t, `pack tiny
legend
  #  wall
level ONE
  par 12
  neg -3
  coop true
  solo false
  hint keep left
  quoted "12"
  spaced " pad "
  |#|
`)
	lv := &p.Levels[0]
	if n, ok := lv.Int("par"); !ok || n != 12 {
		t.Errorf(`Int("par") = (%d, %v)`, n, ok)
	}
	if n, ok := lv.Int("neg"); !ok || n != -3 {
		t.Errorf(`Int("neg") = (%d, %v)`, n, ok)
	}
	if v, ok := lv.Bool("coop"); !ok || !v {
		t.Errorf(`Bool("coop") = (%v, %v)`, v, ok)
	}
	if v, ok := lv.Bool("solo"); !ok || v {
		t.Errorf(`Bool("solo") = (%v, %v)`, v, ok)
	}
	if s, ok := lv.Str("hint"); !ok || s != "keep left" {
		t.Errorf(`Str("hint") = (%q, %v)`, s, ok)
	}
	if s, ok := lv.Str("quoted"); !ok || s != "12" {
		t.Errorf(`Str("quoted") = (%q, %v)`, s, ok)
	}
	if _, ok := lv.Int("quoted"); ok {
		t.Error(`Int("quoted") read a quoted value as a number`)
	}
	if s, ok := lv.Str("spaced"); !ok || s != " pad " {
		t.Errorf(`Str("spaced") = (%q, %v), want " pad "`, s, ok)
	}
}

func TestParseSortsMetadata(t *testing.T) {
	p := mustParse(t, `pack tiny
legend
  #  wall
level ONE
  zeta 1
  alpha 2
  mid 3
  |#|
`)
	var keys []string
	for _, e := range p.Levels[0].Meta {
		keys = append(keys, e.Key)
	}
	if strings.Join(keys, ",") != "alpha,mid,zeta" {
		t.Errorf("meta keys = %v, want sorted", keys)
	}
}

func TestParseLayers(t *testing.T) {
	p := mustParse(t, `pack tiny
legend
  #  wall
  .  none
  X  divider
level ONE
  |##|
  |##|
  layer edges
  |...|
  |...|
`)
	lv := &p.Levels[0]
	if lv.W != 2 || lv.H != 2 {
		t.Errorf("main grid %dx%d, want 2x2", lv.W, lv.H)
	}
	edges := lv.Layer("edges")
	if edges == nil {
		t.Fatal("layer edges missing")
	}
	if edges.W != 3 || edges.H != 2 {
		t.Errorf("layer %dx%d, want 3x2", edges.W, edges.H)
	}
}

func TestParseErrors(t *testing.T) {
	const head = "pack tiny\nlegend\n  #  wall\n  \\s floor\n"
	for _, tc := range []struct {
		name string
		src  string
		line int
		want string
	}{
		{"stray line", "hello there\n", 1, "expected `pack`"},
		{"no pack directive", "// nothing but a comment\n", 1, "no `pack` directive"},
		{"pack twice", "pack a\npack b\n", 2, "exactly one pack"},
		{"bad pack name", "pack 9lives\n", 1, "is not [A-Za-z]"},
		{"legend before pack", "legend\n", 1, "`legend` before `pack`"},
		{"legend twice", "pack a\nlegend\n  # w\nlegend\n", 4, "exactly one main legend"},
		{"bad legend char", "pack a\nlegend\n  ## wall\n", 3, "want one printable character"},
		{"pipe in legend", "pack a\nlegend\n  | wall\n", 3, "delimits grid rows"},
		{"bad legend name", "pack a\nlegend\n  # -wall\n", 3, "legend name"},
		{"duplicate legend char", "pack a\nlegend\n  # wall\n  # stone\n", 4, "duplicate legend character"},
		{"duplicate legend name", "pack a\nlegend\n  # wall\n  X wall\n", 4, "duplicate legend name"},
		{"level before legend", "pack a\nlevel ONE\n", 2, "`level` before `legend`"},
		{"unquoted spaced name", head + "level FIRST BOX\n  |##|\n", 5, "bare token or a quoted string"},
		{"duplicate level name", head + "level ONE\n  |##|\nlevel ONE\n  |##|\n", 7, `duplicate level name "ONE"`},
		{"level with no grid", head + "level ONE\n  par 1\n", 5, `level "ONE" has no grid`},
		{"ragged grid", head + "level ONE\n  |###|\n  |##|\n", 7, "row is 2 cells wide, the grid's first row is 3"},
		{"unknown rune", head + "level ONE\n  |#Q#|\n", 6, "'Q' at column 1 is not in the legend"},
		{"unterminated row", head + "level ONE\n  |###\n", 6, "must end with '|'"},
		{"empty row", head + "level ONE\n  ||\n", 6, "empty grid row"},
		{"rows not contiguous", head + "level ONE\n  |###|\n  par 1\n  |###|\n", 8, "must be contiguous"},
		{"row outside a level", "pack a\n|###|\n", 2, "grid row outside a level"},
		{"layer before grid", head + "level ONE\n  layer edges\n  |###|\n", 6, "`layer` before the level's own grid"},
		{"layer outside a level", head + "layer edges\n", 5, "`layer` outside a level"},
		{"duplicate layer", head + "level ONE\n  |#|\n  layer a\n  |#|\n  layer a\n  |#|\n", 9, "duplicate layer name"},
		{"metadata without value", head + "level ONE\n  par\n  |#|\n", 6, `metadata key "par" has no value`},
		{"duplicate metadata", head + "level ONE\n  par 1\n  par 2\n  |#|\n", 7, "duplicate metadata key"},
		{"reserved name key", head + "level ONE\n  name X\n  |#|\n", 6, "`name` is not a metadata key"},
		{"bad metadata key", head + "level ONE\n  9par 1\n  |#|\n", 6, "not a metadata key"},
		{"bad quoted value", head + "level ONE\n  hint \"oops\n  |#|\n", 6, "not a valid quoted string"},
		{"no levels", "pack a\nlegend\n  # wall\n", 1, "no levels"},
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

// errLine returns the line of the first joined error whose message contains
// want.
func errLine(t *testing.T, err error, want string) int {
	t.Helper()
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		var pe *packfile.Error
		if errors.As(err, &pe) {
			return pe.Line
		}
		t.Fatalf("error %T is neither joined nor a *packfile.Error", err)
	}
	for _, e := range joined.Unwrap() {
		var pe *packfile.Error
		if errors.As(e, &pe) && strings.Contains(pe.Msg, want) {
			return pe.Line
		}
	}
	t.Fatalf("no *packfile.Error matching %q in %v", want, err)
	return 0
}

func TestParseReportsEveryError(t *testing.T) {
	_, err := packfile.Parse("t.levels", []byte(`pack tiny
legend
  #  wall
level ONE
  |#Q#|
  |##|
`))
	if err == nil {
		t.Fatal("Parse succeeded on a doubly-broken file")
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("error %T is not joined", err)
	}
	if n := len(joined.Unwrap()); n != 2 {
		t.Errorf("%d errors, want 2 (unknown rune, ragged row): %v", n, err)
	}
}

func TestErrorFormat(t *testing.T) {
	e := &packfile.Error{File: "solo.levels", Line: 14, Msg: "boom"}
	if got := e.Error(); got != "solo.levels:14: boom" {
		t.Errorf("Error() = %q, want %q", got, "solo.levels:14: boom")
	}
}

func TestParseAcceptsCRLF(t *testing.T) {
	p := mustParse(t, strings.ReplaceAll(minimal, "\n", "\r\n"))
	if got := p.Levels[0].Cells; got != "###"+"# #"+"###" {
		t.Errorf("cells = %q", got)
	}
}
