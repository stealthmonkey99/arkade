// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import (
	btlib "github.com/stealthmonkey99/arkade/internal/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// setArt skips unchanged art so those rows are not redrawn.
func setArt(el *element.SpriteEl, art display.Sprite) {
	if sameSprite(el.Data(), art) {
		return
	}
	el.SetData(art)
}

func sameSprite(a, b display.Sprite) bool {
	if len(a) != len(b) {
		return false
	}
	for y := range a {
		if len(a[y]) != len(b[y]) {
			return false
		}
		for x := range a[y] {
			if a[y][x] != b[y][x] {
				return false
			}
		}
	}
	return true
}

func blank(w, h int) display.Sprite {
	out := make(display.Sprite, h)
	for y := range out {
		out[y] = make([]display.Pixel, w)
	}
	return out
}

// Seat colours: P1 red, P2 blue, matching the cabinet's own seats.
var (
	seatP1 = display.Pixel{R: 235, G: 80, B: 70, A: 255}
	seatP2 = display.Pixel{R: 70, G: 140, B: 255, A: 255}
	unseat = display.Pixel{R: 70, G: 70, B: 70, A: 255}
)

func seatColor(seat input.Seat) display.Pixel {
	switch seat {
	case input.SeatP1:
		return seatP1
	case input.SeatP2:
		return seatP2
	default:
		return unseat
	}
}

// Vertical layout shared by both screens: header 2..9, rows 11..55, hint 56..63.
const (
	panelH    = 64
	listTop   = 11
	rowH      = 9
	hintH     = 8
	hintTop   = panelH - hintH // 56
	listSlots = (hintTop - listTop) / rowH
)

const (
	confirmW = 52
	confirmH = 34
)

const rowW = 60

// rowArt: a filled pip means the engine can read the pad now; hollow means
// it is only remembered.
func rowArt(r row, selected bool) display.Sprite {
	art := blank(rowW, 9)
	bg := display.Pixel{}
	if selected {
		bg = display.Pixel{R: 40, G: 40, B: 48, A: 255}
		for y := range art {
			for x := range art[y] {
				art[y][x] = bg
			}
		}
	}

	if r.isAdd {
		paste(art, display.TextSprite("+ ADD NEW", display.Pixel{R: 200, G: 200, B: 200, A: 255}), 10, 1)
		return art
	}

	pip := seatColor(r.seat)
	if !r.live {
		pip = unseat
	}
	for y := 3; y < 6; y++ {
		for x := 2; x < 5; x++ {
			inner := y == 4 && x == 3
			if r.live || !inner {
				art[y][x] = pip
			}
		}
	}

	name := bluetoothName(r)
	paste(art, display.TextSprite(name, display.Pixel{R: 210, G: 210, B: 210, A: 255}), 8, 1)
	if r.seat.Valid() {
		paste(art, seatBadge(r.seat), rowW-13, 1)
	}
	return art
}

func seatBadge(seat input.Seat) display.Sprite {
	label := "P1"
	if seat == input.SeatP2 {
		label = "P2"
	}
	return display.TextSprite(label, seatColor(seat))
}

func bluetoothName(r row) string {
	name := btlib.DisplayName(r.dev)
	if len(name) > 8 {
		name = name[:8]
	}
	return name
}

func paste(dst, src display.Sprite, x, y int) {
	for sy := range src {
		dy := y + sy
		if dy < 0 || dy >= len(dst) {
			continue
		}
		for sx := range src[sy] {
			dx := x + sx
			if dx < 0 || dx >= len(dst[dy]) || src[sy][sx].A == 0 {
				continue
			}
			dst[dy][dx] = src[sy][sx]
		}
	}
}

func scanRowArt(d btlib.Device, selected bool) display.Sprite {
	art := blank(rowW, 9)
	if selected {
		for y := range art {
			for x := range art[y] {
				art[y][x] = display.Pixel{R: 40, G: 40, B: 48, A: 255}
			}
		}
	}

	pip := display.Pixel{R: 80, G: 80, B: 86, A: 255}
	switch {
	case d.Connected:
		pip = display.Pixel{R: 60, G: 220, B: 90, A: 255}
	case d.Paired:
		pip = display.Pixel{R: 240, G: 200, B: 40, A: 255}
	case d.Input:
		pip = display.Pixel{R: 70, G: 140, B: 255, A: 255}
	}
	for y := 3; y < 6; y++ {
		for x := 2; x < 5; x++ {
			art[y][x] = pip
		}
	}

	ink := display.Pixel{R: 200, G: 200, B: 200, A: 255}
	if !btlib.HasRealName(d) {
		// bare MACs, including the dashed alias BlueZ invents, are dimmed
		ink = display.Pixel{R: 110, G: 110, B: 110, A: 255}
	}
	paste(art, display.TextSprite(scanName(d), ink), 8, 1)
	return art
}

func scanName(d btlib.Device) string {
	name := btlib.DisplayName(d)
	if len(name) > 12 {
		name = name[:12]
	}
	return name
}

// dimSprite scales colour toward black and leaves alpha untouched, so
// transparent pixels stay transparent.
func dimSprite(art display.Sprite, pct int) display.Sprite {
	out := make(display.Sprite, len(art))
	for y := range art {
		out[y] = make([]display.Pixel, len(art[y]))
		for x, px := range art[y] {
			out[y][x] = display.Pixel{
				R: uint8(int(px.R) * pct / 100),
				G: uint8(int(px.G) * pct / 100),
				B: uint8(int(px.B) * pct / 100),
				A: px.A,
			}
		}
	}
	return out
}

const spinnerSize = 11

// spinnerRing is eight hand-placed dot positions, clockwise from the top;
// a computed circle at this radius is lumpy.
var spinnerRing = [8][2]int{{5, 0}, {8, 1}, {10, 4}, {8, 8}, {5, 10}, {2, 8}, {0, 4}, {2, 1}}

// spinnerArt is a bright head with a fading tail.
func spinnerArt(phase int, c display.Pixel) display.Sprite {
	art := blank(spinnerSize, spinnerSize)
	for i := 0; i < len(spinnerRing); i++ {
		back := (phase - i + len(spinnerRing)) % len(spinnerRing)
		level := 100 - back*22
		if level <= 8 {
			continue
		}
		p := spinnerRing[i]
		px := display.Pixel{
			R: uint8(int(c.R) * level / 100),
			G: uint8(int(c.G) * level / 100),
			B: uint8(int(c.B) * level / 100),
			A: 255,
		}
		art[p[1]][p[0]] = px
		if back == 0 && p[0]+1 < spinnerSize {
			art[p[1]][p[0]+1] = px
		}
	}
	return art
}

// spinnerStepMs is how long one dot holds; a revolution is under a second.
const spinnerStepMs = 110

func confirmArt(name string, w, h int) display.Sprite {
	art := blank(w, h)
	body := display.Pixel{R: 24, G: 24, B: 30, A: 255}
	edge := display.Pixel{R: 120, G: 120, B: 130, A: 255}
	for y := range art {
		for x := range art[y] {
			switch {
			case y == 0 || y == h-1 || x == 0 || x == w-1:
				art[y][x] = edge
			default:
				art[y][x] = body
			}
		}
	}
	paste(art, display.TextSprite("DROP", display.Pixel{R: 235, G: 80, B: 70, A: 255}), 4, 4)
	if len(name) > 9 {
		name = name[:9]
	}
	paste(art, display.TextSprite(name, display.Pixel{R: 220, G: 220, B: 220, A: 255}), 4, 13)
	ui.HintRow{X: 4, Y: 24, LabelColor: ui.Color{R: 180, G: 180, B: 180, A: 255}, Hints: []ui.Hint{
		{Text: "YES", Color: ui.ButtonGreen},
		{Text: "NO", Color: ui.ButtonRed},
	}}.Draw(&display.SpriteCanvas{Sprite: art})
	return art
}

// lampSet is which buttons are lit; a lit button must do something in the
// current state.
type lampSet struct{ a, b, c, d bool }

func (l lampSet) lights() hardware.ButtonLights {
	return hardware.ButtonLights{"joya": l.a, "joyb": l.b, "joyc": l.c, "joyd": l.d}
}
