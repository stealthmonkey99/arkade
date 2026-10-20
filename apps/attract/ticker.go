// SPDX-License-Identifier: GPL-2.0-or-later

package attract

import (
	"fmt"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/scores"
)

// Ticker pacing: one 10px row passes every 1.75s, a pixel step per 175ms,
// derived from stats.DurationMs so frames stay deterministic.
const (
	tickerRowMs   = 1750
	tickerPitch   = 10
	tickerWindowH = 12 // y52..63
	tickerMsPerPx = tickerRowMs / tickerPitch
)

// ticker is the leaderboard crawl: ranks 2-10 scrolling upward through a
// small window, each row like "#2 ANNA 4210". A cycle ends with a
// window-height blank gap so the list re-enters cleanly.
type ticker struct {
	element.Base

	rows []display.Sprite // one pre-rendered row per rank 2..10

	// phaseMs anchors the scroll to the slot start; lastOff exists to
	// MarkDirty on change.
	phaseMs float64
	lastOff int
}

// tickerDim is dimmer than the #1 line above it.
var tickerDim = display.Pixel{R: 104, G: 104, B: 104, A: 255}

func newTicker() *ticker {
	tk := &ticker{}
	tk.InitBase(element.Options{OffsetX: 2, OffsetY: 52, Hidden: true})
	tk.Impl = tk.renderImpl
	return tk
}

// setRows pre-renders rows "#N NAME SCORE" (name omitted when anonymous) for
// rest = entries[1:]. Long rows clip at the panel edge.
func (tk *ticker) setRows(rest []scores.Entry) {
	tk.rows = tk.rows[:0]
	for i, e := range rest {
		text := fmt.Sprintf("#%d %d", i+2, e.Score)
		if e.Name != "" {
			text = fmt.Sprintf("#%d %s %d", i+2, strings.ToUpper(e.Name), e.Score)
		}
		row := element.NewLabel(element.LabelOptions{Text: text, Color: &tickerDim})
		tk.rows = append(tk.rows, row.Render(clock.FrameTime{}))
	}
	tk.MarkDirty()
}

// cycleMs is one full scroll: every row in and out plus the blank gap.
func (tk *ticker) cycleMs() float64 {
	return float64((len(tk.rows)*tickerPitch + tickerWindowH) * tickerMsPerPx)
}

// start re-anchors the scroll to the showcase slot's start time.
func (tk *ticker) start(phaseMs float64) {
	tk.phaseMs = phaseMs
	tk.lastOff = -1
	tk.MarkDirty()
}

// Advance marks the ticker dirty whenever the scroll moves a whole pixel.
func (tk *ticker) Advance(stats clock.FrameTime) {
	if !tk.Visible() || len(tk.rows) == 0 {
		return
	}
	if off := tk.offset(stats.DurationMs); off != tk.lastOff {
		tk.lastOff = off
		tk.MarkDirty()
	}
}

// offset is the upward scroll in pixels, looping over the cycle.
func (tk *ticker) offset(ms float64) int {
	total := len(tk.rows)*tickerPitch + tickerWindowH
	if total <= 0 || ms < tk.phaseMs {
		return 0
	}
	return int((ms-tk.phaseMs)/tickerMsPerPx) % total
}

func (tk *ticker) renderImpl(stats clock.FrameTime) display.Sprite {
	const width = 60
	out := make(display.Sprite, tickerWindowH)
	for y := range out {
		out[y] = make([]display.Pixel, width)
	}
	off := tk.offset(stats.DurationMs)
	for i, row := range tk.rows {
		top := tickerWindowH + i*tickerPitch - off
		for y := range row {
			wy := top + y
			if wy < 0 || wy >= tickerWindowH {
				continue
			}
			for x := 0; x < len(row[y]) && x < width; x++ {
				out[wy][x] = row[y][x]
			}
		}
	}
	return out
}

// Trophy pixel art: an 8x8 gold cup drawn beside the #1 line.
var (
	trophyGold     = display.Pixel{R: 252, G: 192, B: 32, A: 255}
	trophyDarkGold = display.Pixel{R: 176, G: 128, B: 16, A: 255}

	// '#' bright gold, '+' dark gold, '.' transparent.
	trophyArt = []string{
		".######.",
		"#.####.#",
		"#.####.#",
		"..####..",
		"...##...",
		"...##...",
		"..+##+..",
		".++++++.",
	}
)

// trophySprite decodes trophyArt into a display sprite.
func trophySprite() display.Sprite {
	out := make(display.Sprite, len(trophyArt))
	for y, line := range trophyArt {
		row := make([]display.Pixel, len(line))
		for x, c := range line {
			switch c {
			case '#':
				row[x] = trophyGold
			case '+':
				row[x] = trophyDarkGold
			}
		}
		out[y] = row
	}
	return out
}
