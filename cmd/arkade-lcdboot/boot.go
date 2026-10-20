// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	lcdCols   = 16
	barCells  = 8
	barCols   = 5 // columns per cell, so the bar has 40 steps
	stageCols = 7 // "network", the longest stage name

	spinTick  = 250 * time.Millisecond
	creepTau  = 700 * time.Millisecond
	creepSpan = 0.85 // of a stage's share: the bar approaches the next milestone, never arrives
)

// stage is a boot milestone: the name shown while in flight and the unit whose activation ends it.
type stage struct{ name, unit string }

// parseStages reads the -stages flag's name=unit pairs in expected order.
func parseStages(spec string) ([]stage, error) {
	var out []stage
	for _, field := range strings.Split(spec, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		name, unit, ok := strings.Cut(field, "=")
		if !ok || name == "" || unit == "" {
			return nil, fmt.Errorf("stage %q: want name=unit", field)
		}
		out = append(out, stage{name, unit})
	}
	if len(out) == 0 {
		return nil, errors.New("no stages")
	}
	return out, nil
}

func unitNames(stages []stage) []string {
	out := make([]string, len(stages))
	for i, s := range stages {
		out[i] = s.unit
	}
	return out
}

// parseActive reads `systemctl is-active <unit>...`: one word per unit, in
// order. Unknown units still get a line, so only a short answer is padded.
func parseActive(out string, n int) []bool {
	got := make([]bool, n)
	for i, word := range strings.Fields(out) {
		if i >= n {
			break
		}
		got[i] = word == "active"
	}
	return got
}

// tracker latches milestones so the bar never walks backwards.
type tracker struct {
	stages  []stage
	done    []bool
	reached int
	stage   string
	since   time.Time
}

func newTracker(stages []stage, now time.Time) *tracker {
	t := &tracker{stages: stages, done: make([]bool, len(stages)), since: now}
	t.stage = stages[0].name
	return t
}

// update folds one poll in. The bar counts every landed milestone; the name
// is the first outstanding one.
func (t *tracker) update(active []bool, now time.Time) {
	for i := range t.done {
		if i < len(active) && active[i] {
			t.done[i] = true
		}
	}
	reached := 0
	for _, done := range t.done {
		if done {
			reached++
		}
	}
	stage := t.stages[len(t.stages)-1].name
	for i, done := range t.done {
		if !done {
			stage = t.stages[i].name
			break
		}
	}
	if reached != t.reached || stage != t.stage {
		t.reached, t.stage, t.since = reached, stage, now
	}
}

// progressFraction is milestones reached out of total plus an asymptotic
// creep through the one in flight.
func progressFraction(reached, total int, elapsed time.Duration) float64 {
	if total <= 0 {
		return 0
	}
	if reached >= total {
		return 1
	}
	share := 1 / float64(total)
	creep := share * creepSpan * (1 - math.Exp(-float64(elapsed)/float64(creepTau)))
	return float64(reached)*share + creep
}

func progressBar(frac float64) string {
	cols := int(math.Round(clamp01(frac) * barCells * barCols))
	var b strings.Builder
	for cell := 0; cell < barCells; cell++ {
		lit := cols - barCols*cell
		if lit < 0 {
			lit = 0
		}
		if lit > barCols {
			lit = barCols
		}
		b.WriteRune(barGlyphs[lit])
	}
	return b.String()
}

var joystickFrames = [4]rune{joyLeft, joyUp, joyRight, joyUp}

func joystickFrame(elapsed time.Duration) rune {
	return joystickFrames[int(elapsed/spinTick)%len(joystickFrames)]
}

// bootLines is P1: name and stick on top, bar and stage below.
func bootLines(frac float64, stage string, elapsed time.Duration) [2]string {
	return [2]string{
		padRight("Årkade", lcdCols-1) + string(joystickFrame(elapsed)),
		progressBar(frac) + " " + padRight(stage, stageCols),
	}
}

// finalLines is the last frame: bar full, stick upright.
func finalLines(stage string) [2]string {
	return [2]string{
		padRight("Årkade", lcdCols-1) + string(joyUp),
		progressBar(1) + " " + padRight(stage, stageCols),
	}
}

// crashLines is the crash card: a warning sign where the stick was, the reason scrolling below.
func crashLines(msg string, elapsed time.Duration) [2]string {
	return [2]string{
		padRight("Årkade", lcdCols-1) + string(warnGlyph),
		marqueeWindow(msg, int(elapsed/marqueeTick)),
	}
}

// statusLines is P2: IP address, CPU temperature and wifi signal.
func statusLines(s systemInfo) [2]string {
	top := "waiting for wifi"
	if s.ip != "" {
		top = "IP " + s.ip
	}
	temp := "cpu --°C"
	if s.tempOK {
		temp = fmt.Sprintf("cpu %d°C", s.tempC)
	}
	bottom := padRight(temp, 10)
	if s.rssiOK {
		bottom += fmt.Sprintf("%ddBm", s.rssi)
	}
	return [2]string{padRight(top, lcdCols), padRight(bottom, lcdCols)}
}

const marqueeGap = "   "

// marqueeWindow is the 16-column view of msg at step, wrapping through a gap.
func marqueeWindow(msg string, step int) string {
	runes := []rune(msg + marqueeGap)
	if len(runes) <= lcdCols {
		return padRight(msg, lcdCols)
	}
	off := ((step % len(runes)) + len(runes)) % len(runes)
	out := make([]rune, lcdCols)
	for i := range out {
		out[i] = runes[(off+i)%len(runes)]
	}
	return string(out)
}

func padRight(s string, n int) string {
	if pad := n - utf8.RuneCountInString(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func clamp01(f float64) float64 {
	switch {
	case f < 0 || math.IsNaN(f):
		return 0
	case f > 1:
		return 1
	}
	return f
}
