// SPDX-License-Identifier: GPL-2.0-or-later

package render

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// Console renders frames to the terminal using half-block characters (▀)
// with 24-bit color: each cell shows two stacked pixels (fg top, bg bottom).
type Console struct {
	*Base
	mu          sync.Mutex
	out         *os.File
	framesPaint uint64
	logs        []logLine
	maxLogs     int
	// lastHeader is the hardware header from the last full repaint; a header
	// change forces a repaint even when the frame is clean.
	lastHeader   []string
	HardwareLine func() []string // optional virtual hardware status lines
	InputBadge   func() string   // optional input-mode badge for stats line
	// PerfStats supplies heap/system MB for the stats line.
	PerfStats func() (heapMB, sysMB float64)
	Debug     bool
}

type logLine struct {
	level string
	text  string
}

// gutter opens every row. Terminals fill left padding with the first cell's
// background, which in half-block encoding is the lower pixel, leaving a
// sliver when column 0 is lit; a blank default-colour cell avoids it.
const gutter = " "

// NewConsole builds a console renderer writing to stdout.
func NewConsole() *Console {
	c := &Console{out: os.Stdout, maxLogs: 8, Debug: true}
	c.Base = NewBase(c)
	return c
}

// Init switches to the alternate screen and hides the cursor.
func (c *Console) Init() error {
	fmt.Fprint(c.out, "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	return nil
}

// Close restores the terminal.
func (c *Console) Close() error {
	fmt.Fprint(c.out, "\x1b[?25h\x1b[?1049l")
	return nil
}

// Log adds a line to the on-screen log tail.
func (c *Console) Log(level, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, line := range strings.Split(text, "\n") {
		c.logs = append(c.logs, logLine{level, line})
	}
	if len(c.logs) > c.maxLogs {
		c.logs = c.logs[len(c.logs)-c.maxLogs:]
	}
}

// PaintImpl draws the frame. Raw terminal mode is assumed, so lines end
// with \r\n.
func (c *Console) PaintImpl(data display.Matrix, stats clock.FrameTime, dirty bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var header []string
	if c.HardwareLine != nil {
		header = c.HardwareLine()
	}
	if !dirty && c.framesPaint > 0 && slices.Equal(header, c.lastHeader) {
		// Frame and hardware header are both unchanged; refresh only the stats
		// line in place.
		c.writeStats(stats, dirty, true, len(data))
		return
	}
	c.framesPaint++
	c.lastHeader = append(c.lastHeader[:0], header...)

	var b strings.Builder
	b.Grow(len(data) * len(data[0]) * 24)
	b.WriteString("\x1b[H")

	// Reset before \x1b[K: with background-color erase it smears the live background.
	for _, line := range header {
		b.WriteString(gutter)
		b.WriteString(line)
		b.WriteString("\x1b[0m\x1b[K\r\n")
	}

	for y := 0; y < len(data); y += 2 {
		top := data[y]
		var bot []display.Pixel
		if y+1 < len(data) {
			bot = data[y+1]
		}
		b.WriteString(gutter)
		var lastFg, lastBg display.Pixel
		first := true
		for x := 0; x < len(top); x++ {
			fg := top[x]
			bg := display.Black
			if bot != nil && x < len(bot) {
				bg = bot[x]
			}
			if first || fg != lastFg {
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm", fg.R, fg.G, fg.B)
				lastFg = fg
			}
			if first || bg != lastBg {
				fmt.Fprintf(&b, "\x1b[48;2;%d;%d;%dm", bg.R, bg.G, bg.B)
				lastBg = bg
			}
			first = false
			b.WriteString("▀")
		}
		b.WriteString("\x1b[0m\x1b[K\r\n")
	}

	// Log tail.
	for _, line := range c.logs {
		color := ""
		switch line.level {
		case "error":
			color = "\x1b[31m"
		case "warn":
			color = "\x1b[33m"
		default:
			color = "\x1b[90m"
		}
		b.WriteString(gutter)
		b.WriteString(color)
		b.WriteString(truncate(line.text, 120))
		b.WriteString("\x1b[0m\x1b[K\r\n")
	}

	fmt.Fprint(c.out, b.String())
	c.writeStats(stats, dirty, false, len(data))
}

func (c *Console) writeStats(stats clock.FrameTime, dirty, reposition bool, frameRows int) {
	if !c.Debug {
		return
	}
	light := "○"
	if dirty {
		light = "●"
	}
	badge := ""
	if c.InputBadge != nil {
		badge = c.InputBadge()
	}
	mem := ""
	if c.PerfStats != nil {
		heap, sys := c.PerfStats()
		mem = fmt.Sprintf(" | h: %.1f MB | s: %.1f MB", heap, sys)
	}
	// The badge brings its own SGR, so it sits outside the dim-italic run, which
	// is reopened for the counters.
	line := fmt.Sprintf(gutter+"\x1b[3;90m%s\x1b[0m %s \x1b[3;90mf: %d | t: %d | e: %.0fms | d: %.1fs | x: %d%s\x1b[0m\x1b[K",
		light, badge, c.framesPaint, stats.Ticks, stats.ElapsedMs, stats.DurationMs/1000, stats.DroppedFrames, mem)
	if reposition {
		// Stats live on the last written row; jump there absolutely.
		hw := 0
		if c.HardwareLine != nil {
			hw = len(c.HardwareLine())
		}
		row := hw + (frameRows+1)/2 + len(c.logs) + 1
		fmt.Fprintf(c.out, "\x1b[%d;1H%s", row, line)
		return
	}
	fmt.Fprintf(c.out, "%s\r\n", line)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
