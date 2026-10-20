// SPDX-License-Identifier: GPL-2.0-or-later

package example

import "testing"

func TestDemoPackDecodes(t *testing.T) {
	if got := len(DemoPack.Levels); got != 2 {
		t.Fatalf("DemoPack has %d levels, want 2", got)
	}
	if got := DemoPack.Name; got != "demo" {
		t.Errorf("DemoPack.Name = %q, want %q", got, "demo")
	}

	first := &DemoPack.Levels[0]
	if first.Name != "FIRST STEPS" || first.W != 7 || first.H != 5 {
		t.Errorf("first level = %q %dx%d, want \"FIRST STEPS\" 7x5", first.Name, first.W, first.H)
	}
	if got := first.Row(1); got != "#@   *#" {
		t.Errorf("first.Row(1) = %q, want %q", got, "#@   *#")
	}

	x, y, ok := Start(first)
	if !ok || x != 1 || y != 1 {
		t.Errorf("Start(first) = (%d, %d, %v), want (1, 1, true)", x, y, ok)
	}
	if got := Coins(first); got != 2 {
		t.Errorf("Coins(first) = %d, want 2", got)
	}

	par, ok := first.Int("par")
	if !ok || par != 6 {
		t.Errorf(`first.Int("par") = (%d, %v), want (6, true)`, par, ok)
	}
	if hint, ok := first.Str("hint"); !ok || hint != "keep left" {
		t.Errorf(`first.Str("hint") = (%q, %v), want ("keep left", true)`, hint, ok)
	}
	if dark, ok := first.Bool("dark"); !ok || dark {
		t.Errorf(`first.Bool("dark") = (%v, %v), want (false, true)`, dark, ok)
	}
}

// The second level's water is impassable except where its "lilies" layer has
// a pad.
func TestLayerChangesWhatIsWalkable(t *testing.T) {
	deep := &DemoPack.Levels[1]
	if deep.Name != "DEEP END" {
		t.Fatalf("second level is %q, want \"DEEP END\"", deep.Name)
	}
	if got := deep.At(3, 1); got != '~' {
		t.Fatalf("deep.At(3, 1) = %q, want water", got)
	}
	if Blocked(deep, 3, 1) {
		t.Error("water at (3, 1) has a lily pad, so it must be walkable")
	}
	if !Blocked(deep, 4, 1) {
		t.Error("water at (4, 1) has no lily pad, so it must be blocked")
	}
	if !Blocked(deep, 0, 0) {
		t.Error("a wall must be blocked")
	}
	if !Blocked(deep, -1, 0) {
		t.Error("outside the level must be blocked")
	}
	if Blocked(deep, 1, 1) {
		t.Error("the start cell must be walkable")
	}
}

func TestExitsRecordsAndValuedLegend(t *testing.T) {
	first, deep := &DemoPack.Levels[0], &DemoPack.Levels[1]

	next, ok := Next(first)
	if !ok || next != deep {
		t.Errorf("Next(first) = (%v, %v), want the DEEP END level", next, ok)
	}
	if _, ok := Next(deep); ok {
		t.Error("DEEP END has an `exit next` it was never given")
	}
	if back := deep.Exit("back"); back == nil || back.Target != 0 {
		t.Errorf("DEEP END's back exit = %+v, want level 0", back)
	}

	if s, ok := Sign(first); !ok || s != "mind the gap" {
		t.Errorf(`Sign(first) = (%q, %v), want ("mind the gap", true)`, s, ok)
	}
	if _, ok := Sign(deep); ok {
		t.Error("DEEP END has a sign it was never given")
	}

	want := [][2]int{{3, 2}, {5, 3}, {3, 1}}
	got := Route(deep, nil)
	if len(got) != len(want) {
		t.Fatalf("Route(deep) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("waypoint %d = %v, want %v", i, got[i], want[i])
		}
	}

	// (3, 2) is a 'd' on the tint layer: the palette legend's deep blue.
	if rgb, ok := Tint(deep, 3, 2); !ok || rgb != 0x103048 {
		t.Errorf("Tint(deep, 3, 2) = (%#06x, %v), want (0x103048, true)", rgb, ok)
	}
	// (2, 1) is 's', shallow.
	if rgb, ok := Tint(deep, 2, 1); !ok || rgb != 0x2a6d8f {
		t.Errorf("Tint(deep, 2, 1) = (%#06x, %v), want (0x2a6d8f, true)", rgb, ok)
	}
	// '.' is declared with no colour, so it reads as missing, not black
	if rgb, ok := Tint(deep, 0, 0); ok {
		t.Errorf("Tint(deep, 0, 0) = (%#06x, %v), want no colour at all", rgb, ok)
	}
	if _, ok := Tint(first, 1, 1); ok {
		t.Error("FIRST STEPS has a tint layer it was never given")
	}
}
