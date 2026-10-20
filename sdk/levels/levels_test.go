// SPDX-License-Identifier: GPL-2.0-or-later

package levels_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels"
)

// sample is a 4x3 level with one layer and one entry of each metadata kind,
// written by hand so only the runtime accessors are under test.
func sample() *levels.Level {
	return &levels.Level{
		Name: "SAMPLE", W: 4, H: 3,
		Cells: "####" +
			"#@$#" +
			"####",
		Layers: []levels.Layer{
			{Name: "hazard", W: 4, H: 3, Cells: "...." + ".~.." + "...."},
		},
		Meta: []levels.Entry{
			{Key: "coop", Kind: levels.KindBool, Int: 1},
			{Key: "hint", Kind: levels.KindStr, Str: "push right"},
			{Key: "par", Int: 7},
		},
	}
}

func TestLevelAt(t *testing.T) {
	lv := sample()
	for _, tc := range []struct {
		x, y int
		want byte
	}{
		{0, 0, '#'},
		{1, 1, '@'},
		{2, 1, '$'},
		{3, 2, '#'},
		{-1, 0, 0},
		{0, -1, 0},
		{4, 0, 0},
		{0, 3, 0},
	} {
		if got := lv.At(tc.x, tc.y); got != tc.want {
			t.Errorf("At(%d, %d) = %q, want %q", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestLevelRow(t *testing.T) {
	lv := sample()
	if got := lv.Row(1); got != "#@$#" {
		t.Errorf("Row(1) = %q, want %q", got, "#@$#")
	}
	if got := lv.Row(3); got != "" {
		t.Errorf("Row(3) = %q, want \"\" outside the level", got)
	}
	if got := lv.Row(-1); got != "" {
		t.Errorf("Row(-1) = %q, want \"\" outside the level", got)
	}
}

func TestLevelCountAndFind(t *testing.T) {
	lv := sample()
	if got := lv.Count('#'); got != 10 {
		t.Errorf("Count('#') = %d, want 10", got)
	}
	if got := lv.Count('?'); got != 0 {
		t.Errorf("Count('?') = %d, want 0", got)
	}
	x, y, ok := lv.Find('$')
	if !ok || x != 2 || y != 1 {
		t.Errorf("Find('$') = (%d, %d, %v), want (2, 1, true)", x, y, ok)
	}
	if _, _, ok := lv.Find('?'); ok {
		t.Error("Find('?') found a character that is not in the grid")
	}
}

func TestLevelFindIsRowMajor(t *testing.T) {
	lv := &levels.Level{W: 3, H: 2, Cells: "..x" + "x.."}
	x, y, ok := lv.Find('x')
	if !ok || x != 2 || y != 0 {
		t.Errorf("Find('x') = (%d, %d, %v), want (2, 0, true)", x, y, ok)
	}
}

func TestLevelLayer(t *testing.T) {
	lv := sample()
	hz := lv.Layer("hazard")
	if hz == nil {
		t.Fatal("Layer(\"hazard\") = nil, want the layer")
	}
	if got := hz.At(1, 1); got != '~' {
		t.Errorf("hazard.At(1, 1) = %q, want '~'", got)
	}
	if got := hz.At(9, 9); got != 0 {
		t.Errorf("hazard.At(9, 9) = %q, want 0 outside the layer", got)
	}
	if lv.Layer("nope") != nil {
		t.Error("Layer(\"nope\") returned a layer that does not exist")
	}
	if (&levels.Level{}).Layer("hazard") != nil {
		t.Error("Layer on a level with no layers returned non-nil")
	}
}

func TestLevelMetadataAccessors(t *testing.T) {
	lv := sample()

	if n, ok := lv.Int("par"); !ok || n != 7 {
		t.Errorf(`Int("par") = (%d, %v), want (7, true)`, n, ok)
	}
	if s, ok := lv.Str("hint"); !ok || s != "push right" {
		t.Errorf(`Str("hint") = (%q, %v), want ("push right", true)`, s, ok)
	}
	if v, ok := lv.Bool("coop"); !ok || !v {
		t.Errorf(`Bool("coop") = (%v, %v), want (true, true)`, v, ok)
	}

	if _, ok := lv.Int("budget"); ok {
		t.Error(`Int("budget") reported a key the level does not have`)
	}

	// wrong kind reads as missing, not zero
	if n, ok := lv.Int("hint"); ok || n != 0 {
		t.Errorf(`Int("hint") = (%d, %v), want (0, false) for a string entry`, n, ok)
	}
	if v, ok := lv.Bool("par"); ok || v {
		t.Errorf(`Bool("par") = (%v, %v), want (false, false) for an int entry`, v, ok)
	}
	if s, ok := lv.Str("coop"); ok || s != "" {
		t.Errorf(`Str("coop") = (%q, %v), want ("", false) for a bool entry`, s, ok)
	}
}

func TestBoolFalseIsPresent(t *testing.T) {
	lv := &levels.Level{Meta: []levels.Entry{{Key: "coop", Kind: levels.KindBool, Int: 0}}}
	v, ok := lv.Bool("coop")
	if v || !ok {
		t.Errorf(`Bool("coop") = (%v, %v), want (false, true): the key is there and it is false`, v, ok)
	}
}

func TestLegendOrdinal(t *testing.T) {
	lg := &levels.Legend{Runes: "# .$", Names: []string{"wall", "floor", "target", "crate"}}
	for i := 0; i < len(lg.Runes); i++ {
		if got := lg.Ordinal(lg.Runes[i]); got != i {
			t.Errorf("Ordinal(%q) = %d, want %d", lg.Runes[i], got, i)
		}
	}
	if got := lg.Ordinal('?'); got != -1 {
		t.Errorf("Ordinal('?') = %d, want -1", got)
	}
}

// The generator emits integer entries without a Kind field.
func TestKindIntIsZeroValue(t *testing.T) {
	var e levels.Entry
	if e.Kind != levels.KindInt {
		t.Fatalf("zero Entry.Kind = %d, want KindInt (%d)", e.Kind, levels.KindInt)
	}
	lv := &levels.Level{Meta: []levels.Entry{{Key: "par", Int: 3}}}
	if n, ok := lv.Int("par"); !ok || n != 3 {
		t.Errorf(`Int("par") = (%d, %v), want (3, true)`, n, ok)
	}
}
