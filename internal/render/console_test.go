// SPDX-License-Identifier: GPL-2.0-or-later

package render

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// paintBytes runs one PaintImpl against a scratch file and returns
// everything the renderer wrote.
func paintBytes(t *testing.T, c *Console, frame display.Matrix, dirty bool) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c.out = f
	c.PaintImpl(frame, clock.FrameTime{}, dirty)
	raw, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// bandFrame builds a w×h black frame with a solid band across pixel rows
// y0..y1 inset at x0, like the shell's selection highlight.
func bandFrame(w, h, x0, y0, y1 int, band display.Pixel) display.Matrix {
	f := make(display.Matrix, h)
	for y := range f {
		f[y] = make([]display.Pixel, w)
		for x := range f[y] {
			f[y][x] = display.Black
		}
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x < w; x++ {
			f[y][x] = band
		}
	}
	return f
}

func sgrFg(p display.Pixel) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", p.R, p.G, p.B) }
func sgrBg(p display.Pixel) string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", p.R, p.G, p.B) }

// TestConsoleFrameBytes pins the whole frame body byte for byte on a tiny
// frame.
func TestConsoleFrameBytes(t *testing.T) {
	band := display.RGB(27, 21, 42)
	frame := bandFrame(4, 4, 2, 2, 3, band)

	c := NewConsole()
	c.Debug = false
	got := paintBytes(t, c, frame, true)

	want := "\x1b[H" +
		gutter + sgrFg(display.Black) + sgrBg(display.Black) + "▀▀▀▀" + "\x1b[0m\x1b[K\r\n" +
		gutter + sgrFg(display.Black) + sgrBg(display.Black) + "▀▀" +
		sgrFg(band) + sgrBg(band) + "▀▀" + "\x1b[0m\x1b[K\r\n"
	if got != want {
		t.Fatalf("frame bytes\n got %s\nwant %s", strconv.Quote(got), strconv.Quote(want))
	}
}

// TestConsoleRowsOpenWithBothColors: every cell row must declare both
// foreground and background before its first half-block, and that cell must
// be exactly the two pixels of frame column 0, so an inset highlight never
// ghosts onto the left edge.
func TestConsoleRowsOpenWithBothColors(t *testing.T) {
	const w, h = 64, 64
	band := display.RGB(27, 21, 42)
	// The shell's own geometry: the highlight spans pixel rows 13..22 and
	// starts at x=11, so column 0 is never the highlight color.
	frame := bandFrame(w, h, 11, 13, 22, band)
	// Key both edge columns per pixel row so a half-cell slip or a color
	// leaking from the previous row's last cell is visible.
	for y := 0; y < h; y++ {
		frame[y][0] = display.RGB(uint8(1+y), 2, 3)
		frame[y][w-1] = display.RGB(9, 9, uint8(1+y))
	}

	c := NewConsole()
	c.Debug = false
	got := paintBytes(t, c, frame, true)

	body := strings.TrimPrefix(got, "\x1b[H")
	if body == got {
		t.Fatalf("frame did not start with the cursor-home jump: %s", strconv.Quote(got[:8]))
	}
	rows := strings.SplitAfter(body, "\x1b[0m\x1b[K\r\n")
	if n := len(rows) - 1; n != h/2 {
		t.Fatalf("got %d cell rows, want %d", n, h/2)
	}
	if rows[len(rows)-1] != "" {
		t.Fatalf("trailing bytes after the last row: %s", strconv.Quote(rows[len(rows)-1]))
	}
	for r, row := range rows[:h/2] {
		top, bot := frame[2*r][0], frame[2*r+1][0]
		prefix := gutter + sgrFg(top) + sgrBg(bot)
		if !strings.HasPrefix(row, prefix) {
			t.Errorf("cell row %d: first cell opens %s, want %s (fg = pixel (0,%d), bg = pixel (0,%d))",
				r, strconv.Quote(head(row, len(prefix))), strconv.Quote(prefix), 2*r, 2*r+1)
			continue
		}
		if n := strings.Count(row, "▀"); n != w {
			t.Errorf("cell row %d: %d half-blocks, want %d (a short row lets \\x1b[K eat into the frame)", r, n, w)
		}
		// The highlight's color must never open a row.
		if strings.HasPrefix(row, gutter+sgrFg(band)) || strings.HasPrefix(row, gutter+sgrBg(band)) {
			t.Errorf("cell row %d opens in the highlight color; the band starts at x=11, not x=0", r)
		}
	}
}

// TestConsoleErasesWithDefaultBackground pins the erase order on every row
// written: \x1b[K fills with the live background, so the SGR reset comes first.
func TestConsoleErasesWithDefaultBackground(t *testing.T) {
	c := NewConsole()
	c.Debug = true
	// A header line that ends mid-color, like a lit LCD field.
	c.HardwareLine = func() []string { return []string{"\x1b[43;30m LCD 1/9 "} }
	c.InputBadge = func() string { return "\x1b[1;30;46m P1 \x1b[0m" }
	c.Log("info", "a log line")
	c.Log("error", "and a red one")

	got := paintBytes(t, c, bandFrame(8, 8, 3, 2, 5, display.RGB(27, 21, 42)), true)

	// Walk the stream tracking the live background; every erase must see
	// the default one.
	bg := "default"
	for i := 0; i < len(got); {
		if !strings.HasPrefix(got[i:], "\x1b[") {
			i++
			continue
		}
		end := strings.IndexAny(got[i+2:], "ABCDEFGHJKSTfmnsulh")
		if end < 0 {
			break
		}
		params, final := got[i+2:i+2+end], got[i+2+end]
		switch final {
		case 'K':
			if bg != "default" {
				t.Errorf("\\x1b[K at byte %d erases with background %s; reset must come first", i, bg)
			}
		case 'm':
			for _, f := range strings.Split(params, ";") {
				switch f {
				case "", "0", "49":
					bg = "default"
				}
			}
			if strings.Contains(params, "48;2;") || isBasicBg(params) {
				bg = params
			}
		}
		i += 2 + end + 1
	}
}

// isBasicBg reports whether an SGR parameter list sets a legacy 40–47
// background.
func isBasicBg(params string) bool {
	for _, f := range strings.Split(params, ";") {
		if n, err := strconv.Atoi(f); err == nil && n >= 40 && n <= 47 {
			return true
		}
	}
	return false
}

func head(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func TestConsoleRepaintsOnHeaderChange(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	c := NewConsole()
	c.out = f
	header := []string{"LCD: time 0:00"}
	c.HardwareLine = func() []string { return header }

	frame := make(display.Matrix, 4)
	for y := range frame {
		frame[y] = make([]display.Pixel, 4)
	}

	c.PaintImpl(frame, clock.FrameTime{}, true) // initial full paint
	if c.framesPaint != 1 {
		t.Fatalf("framesPaint after initial paint = %d, want 1", c.framesPaint)
	}

	c.PaintImpl(frame, clock.FrameTime{}, false) // static frame, same header
	if c.framesPaint != 1 {
		t.Fatalf("clean frame + unchanged header repainted (framesPaint=%d), want stats-only refresh", c.framesPaint)
	}

	header = []string{"LCD: time 0:01"} // the clock ticks
	c.PaintImpl(frame, clock.FrameTime{}, false)
	if c.framesPaint != 2 {
		t.Fatalf("header change under a static frame did not repaint (framesPaint=%d), want 2", c.framesPaint)
	}
}
