// SPDX-License-Identifier: GPL-2.0-or-later

package screensaver

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
)

func newEditor() (*Editor, *int) {
	n := 0
	return &Editor{
		Schedule: &engine.Schedule{From: "06:00", To: "09:00"},
		Fields: []Field{{
			Label:  "N",
			Value:  func() string { return "v" },
			Adjust: func(d int) { n += d },
		}},
	}, &n
}

func TestShiftHHMMRoundTripAndWrap(t *testing.T) {
	cases := []struct {
		in   string
		mins int
		want string
	}{
		{"06:00", -TimeStepMinutes, "05:45"},
		{"00:00", -TimeStepMinutes, "23:45"},
		{"23:45", TimeStepMinutes, "00:00"},
		{"nonsense", TimeStepMinutes, "00:15"},
	}
	for _, c := range cases {
		if got := ShiftHHMM(c.in, c.mins); got != c.want {
			t.Errorf("ShiftHHMM(%q, %d) = %q, want %q", c.in, c.mins, got, c.want)
		}
	}
}

func TestRowCursorWraps(t *testing.T) {
	e, _ := newEditor()
	if e.Rows() != RowFields+1 {
		t.Fatalf("Rows = %d", e.Rows())
	}
	if e.HandleKey(input.KeyUp) != Changed || e.Row != e.Rows()-1 {
		t.Errorf("up from the top should wrap to the last row, got %d", e.Row)
	}
	if e.HandleKey(input.KeyDown) != Changed || e.Row != 0 {
		t.Errorf("down from the last row should wrap to the top, got %d", e.Row)
	}
}

func TestDayCursorWraps(t *testing.T) {
	e, _ := newEditor()
	e.Row = RowDays
	e.HandleKey(input.KeyLeft)
	if e.Day != 6 {
		t.Errorf("left from Sunday = %d, want 6", e.Day)
	}
	e.HandleKey(input.KeyRight)
	if e.Day != 0 {
		t.Errorf("right from Saturday = %d, want 0", e.Day)
	}
}

func TestJoyAOnlyActsOnDays(t *testing.T) {
	e, _ := newEditor()
	if e.HandleKey(input.KeyJoyA) != Unhandled {
		t.Error("green off the DAYS row should be unhandled")
	}
	e.Row, e.Day = RowDays, 3
	if e.HandleKey(input.KeyJoyA) != Changed || !e.Schedule.Days[3] {
		t.Error("green on DAYS should toggle the cursor day on")
	}
	e.HandleKey(input.KeyJoyA)
	if e.Schedule.Days[3] {
		t.Error("green again should toggle it off")
	}
}

func TestSaveAndUnhandled(t *testing.T) {
	e, _ := newEditor()
	if e.HandleKey(input.KeyJoyB) != Save {
		t.Error("red should save")
	}
	if e.HandleKey(input.KeyName("other")) != Unhandled {
		t.Error("unknown keys should be unhandled")
	}
}

func TestLights(t *testing.T) {
	e, _ := newEditor()
	if l := e.Lights(); !l["joyb"] || l["joya"] {
		t.Errorf("off DAYS: %v", l)
	}
	e.Row = RowDays
	if l := e.Lights(); !l["joyb"] || !l["joya"] {
		t.Errorf("on DAYS: %v", l)
	}
}

func TestRowsLabelsValuesAndFields(t *testing.T) {
	e, n := newEditor()
	want := []string{"ENABLED", "FROM", "TO", "DAYS", "N"}
	for i, w := range want {
		if got := e.Label(i); got != w {
			t.Errorf("Label(%d) = %q, want %q", i, got, w)
		}
	}
	if e.Value(RowEnabled) != "OFF" {
		t.Errorf("default off reading = %q", e.Value(RowEnabled))
	}
	e.HandleKey(input.KeyRight)
	if e.Value(RowEnabled) != "ON" {
		t.Errorf("default on reading = %q", e.Value(RowEnabled))
	}
	e.On, e.Off = "YES", "NO"
	if e.Value(RowEnabled) != "YES" {
		t.Errorf("custom on reading = %q", e.Value(RowEnabled))
	}
	e.HandleKey(input.KeyLeft)
	if e.Value(RowEnabled) != "NO" {
		t.Errorf("custom off reading = %q", e.Value(RowEnabled))
	}
	e.Row = RowFields
	e.HandleKey(input.KeyRight)
	if *n != 1 || e.Value(RowFields) != "v" {
		t.Errorf("field adjust n=%d value=%q", *n, e.Value(RowFields))
	}
}
