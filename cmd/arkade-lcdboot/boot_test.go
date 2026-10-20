// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"
	"time"
	"unicode/utf8"
)

func TestParseStages(t *testing.T) {
	got, err := parseStages("network=network-online.target, engine=arkade.service")
	if err != nil {
		t.Fatalf("parseStages: %v", err)
	}
	want := []stage{{"network", "network-online.target"}, {"engine", "arkade.service"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("parseStages = %v, want %v", got, want)
	}
	for _, bad := range []string{"", "network", "=arkade.service", "network="} {
		if _, err := parseStages(bad); err == nil {
			t.Errorf("parseStages(%q) should fail", bad)
		}
	}
}

func TestParseActiveTakesOneWordPerUnit(t *testing.T) {
	got := parseActive("active\ninactive\nactivating\nunknown\n", 4)
	want := []bool{true, false, false, false}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unit %d = %v, want %v", i, got[i], want[i])
		}
	}
	if got := parseActive("active\n", 3); len(got) != 3 || !got[0] || got[1] || got[2] {
		t.Errorf("a short answer = %v, want only the first unit up", got)
	}
	if got := parseActive("", 2); got[0] || got[1] {
		t.Errorf("no answer = %v, want nothing up", got)
	}
}

func TestTrackerLatchesMilestones(t *testing.T) {
	stages := []stage{{"network", "a"}, {"sound", "b"}, {"engine", "c"}}
	now := time.Now()
	tr := newTracker(stages, now)
	if tr.stage != "network" || tr.reached != 0 {
		t.Fatalf("a fresh tracker = %d/%q, want 0/network", tr.reached, tr.stage)
	}

	tr.update([]bool{true, false, false}, now.Add(time.Second))
	if tr.reached != 1 || tr.stage != "sound" || !tr.since.Equal(now.Add(time.Second)) {
		t.Fatalf("after the first milestone = %d/%q at %v", tr.reached, tr.stage, tr.since)
	}

	tr.update([]bool{false, false, false}, now.Add(2*time.Second))
	if tr.reached != 1 || tr.stage != "sound" {
		t.Errorf("a unit that dropped out = %d/%q, want the milestone latched", tr.reached, tr.stage)
	}
	if !tr.since.Equal(now.Add(time.Second)) {
		t.Errorf("an unchanged poll should not restart the creep, since = %v", tr.since)
	}

	tr.update([]bool{false, false, true}, now.Add(3*time.Second))
	if tr.reached != 2 || tr.stage != "sound" {
		t.Errorf("out-of-order milestone = %d/%q, want 2/sound", tr.reached, tr.stage)
	}

	tr.update([]bool{true, true, true}, now.Add(4*time.Second))
	if tr.reached != 3 || tr.stage != "engine" {
		t.Errorf("everything up = %d/%q, want 3/engine", tr.reached, tr.stage)
	}
}

func TestProgressFractionCreepsWithoutOvershooting(t *testing.T) {
	if got := progressFraction(0, 4, 0); got != 0 {
		t.Errorf("nothing reached, no time = %v, want 0", got)
	}
	if got := progressFraction(4, 4, 0); got != 1 {
		t.Errorf("everything reached = %v, want 1", got)
	}
	prev := 0.0
	for _, d := range []time.Duration{100, 400, 700, 2000, 10000} {
		got := progressFraction(1, 4, d*time.Millisecond)
		if got <= prev {
			t.Errorf("at %v the bar stalled: %v then %v", d, prev, got)
		}
		if got >= 0.5 {
			t.Errorf("at %v the bar reached the next milestone early: %v", d, got)
		}
		prev = got
	}
	if prev <= 0.25+0.8*0.25 {
		t.Errorf("a long stage should creep most of its share, got %v", prev)
	}
}

func TestProgressBarFillsFiveColumnsPerCell(t *testing.T) {
	cases := []struct {
		frac float64
		want string
	}{
		{0, "        "},
		{-1, "        "},
		{1.0 / 40, "▏       "},
		{3.0 / 40, "▍       "},
		{5.0 / 40, "█       "},
		{0.5, "████    "},
		{1, "████████"},
		{2, "████████"},
	}
	for _, c := range cases {
		if got := progressBar(c.frac); got != c.want {
			t.Errorf("progressBar(%v) = %q, want %q", c.frac, got, c.want)
		}
	}
}

// The driver centres shorter lines and scrolls longer ones.
func TestCardsAreExactlySixteenColumns(t *testing.T) {
	cards := map[string][2]string{
		"boot":   bootLines(0.3, "network", 0),
		"done":   bootLines(1, "engine", 1234*time.Millisecond),
		"final":  finalLines("engine"),
		"crash":  crashLines("engine exit 2 - retry in 5s", 900*time.Millisecond),
		"status": statusLines(systemInfo{ip: "192.168.1.42", tempC: 41, tempOK: true, rssi: -62, rssiOK: true}),
		"cold":   statusLines(systemInfo{}),
	}
	for name, lines := range cards {
		for row, line := range lines {
			if n := utf8.RuneCountInString(line); n != lcdCols {
				t.Errorf("%s row %d = %q (%d columns), want %d", name, row, line, n, lcdCols)
			}
		}
	}
}

func TestFinalCardFillsTheBar(t *testing.T) {
	got := finalLines("engine")
	if want := progressBar(1) + " " + padRight("engine", stageCols); got[1] != want {
		t.Errorf("final row 1 = %q, want %q", got[1], want)
	}
	if mid := bootLines(0.4, "sound", 0); got[1] == mid[1] {
		t.Error("the final card should not look like a stage still in flight")
	}
}

// A stage whose unit arkade.service does not wait on would stick the bar.
func TestDefaultStagesLandBeforeTheEngine(t *testing.T) {
	stages, err := parseStages(defaultStages)
	if err != nil {
		t.Fatalf("parseStages(defaultStages): %v", err)
	}
	want := []string{"network.target", "sound.target", "bluetooth.service", "arkade.service"}
	if len(stages) != len(want) {
		t.Fatalf("default stages = %v, want %d of them", stages, len(want))
	}
	for i, unit := range want {
		if stages[i].unit != unit {
			t.Errorf("stage %d = %q, want %q", i, stages[i].unit, unit)
		}
		if n := utf8.RuneCountInString(stages[i].name); n > stageCols {
			t.Errorf("stage name %q is %d columns, the row has room for %d", stages[i].name, n, stageCols)
		}
	}
}

func TestStatusLinesSaysWhatItKnows(t *testing.T) {
	cold := statusLines(systemInfo{})
	if cold[0] != "waiting for wifi" {
		t.Errorf("no address = %q", cold[0])
	}
	if cold[1] != padRight("cpu --°C", lcdCols) {
		t.Errorf("no thermal zone = %q", cold[1])
	}
	warm := statusLines(systemInfo{ip: "192.168.1.42", tempC: 41, tempOK: true, rssi: -62, rssiOK: true})
	if warm[0] != "IP 192.168.1.42 " {
		t.Errorf("address row = %q", warm[0])
	}
	if warm[1] != "cpu 41°C  -62dBm" {
		t.Errorf("temperature row = %q", warm[1])
	}
}

func TestJoystickRocks(t *testing.T) {
	want := []rune{joyLeft, joyUp, joyRight, joyUp, joyLeft}
	for i, w := range want {
		at := time.Duration(i)*spinTick + 10*time.Millisecond
		if got := joystickFrame(at); got != w {
			t.Errorf("frame at %v = %q, want %q", at, got, w)
		}
	}
}

func TestMarqueeWindowWraps(t *testing.T) {
	msg := "engine exit 2 - retry in 5s"
	first := marqueeWindow(msg, 0)
	if first != msg[:lcdCols] {
		t.Errorf("step 0 = %q, want the head of the message", first)
	}
	if got := marqueeWindow(msg, 1); got != msg[1:lcdCols+1] {
		t.Errorf("step 1 = %q, want one column along", got)
	}
	if got := marqueeWindow(msg, len(msg)+len(marqueeGap)); got != first {
		t.Errorf("a full wrap = %q, want %q", got, first)
	}
	for _, step := range []int{0, 5, 40, 1000} {
		if n := utf8.RuneCountInString(marqueeWindow(msg, step)); n != lcdCols {
			t.Errorf("step %d is %d columns", step, n)
		}
	}
	short := marqueeWindow("done", 7)
	if short != padRight("done", lcdCols) {
		t.Errorf("text that fits should not scroll, got %q", short)
	}
}

func TestEveryFiresOncePerPeriod(t *testing.T) {
	fired := 0
	for elapsed := frameTick; elapsed <= 2*time.Second; elapsed += frameTick {
		if every(elapsed, pollTick) {
			fired++
		}
	}
	if fired != 4 {
		t.Errorf("polls in two seconds = %d, want 4", fired)
	}
}

// bootGlyphs plus the engine's table hold more bitmaps than the HD44780's 8
// CGRAM slots, which works only while no card needs more than 8 at once.
func TestNoCardNeedsMoreThanEightCustomGlyphs(t *testing.T) {
	const cgramSlots = 8
	var cards [][2]string
	for cols := 0; cols <= barCells*barCols; cols++ {
		frac := float64(cols) / (barCells * barCols)
		for tick := range len(joystickFrames) {
			cards = append(cards, bootLines(frac, "network", time.Duration(tick)*spinTick))
		}
	}
	cards = append(cards, finalLines("engine"))
	for step := range 40 {
		cards = append(cards, crashLines("engine exit 2 - retry in 5s", time.Duration(step)*marqueeTick))
	}
	cards = append(cards,
		statusLines(systemInfo{ip: "192.168.1.42", tempC: 41, tempOK: true, rssi: -62, rssiOK: true}),
		statusLines(systemInfo{}),
	)
	for _, lines := range cards {
		custom := map[rune]bool{}
		for _, line := range lines {
			for _, r := range line {
				if r >= utf8.RuneSelf {
					custom[r] = true
				}
			}
		}
		if len(custom) > cgramSlots {
			t.Errorf("card %q needs %d custom glyphs, the LCD holds %d", lines, len(custom), cgramSlots)
		}
	}
}
