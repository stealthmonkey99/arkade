// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import (
	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// The BLUETOOTH tile shows one pip per seat: filled when a wireless pad plays it, outlined when not.
const (
	padPipSize = 5
	padPipGap  = 0
	// padsRefreshMs is slow because the controller list is behind a mutex the HID read loops use.
	padsRefreshMs = 700
)

// refreshPadSeats records which seats have a wireless pad and how many awake pads have no seat yet.
// Wired sticks are excluded because they are always present. A newly woken pad
// usually attaches beyond seat 1, so the loose count is the normal first state.
func (a *app) refreshPadSeats(stats clock.FrameTime) {
	if a.padsPrimed && stats.DurationMs-a.padsAt < padsRefreshMs {
		return
	}
	a.padsPrimed, a.padsAt = true, stats.DurationMs
	var seats [input.SeatCount]bool
	loose := 0
	if m := a.ctx.RT.Controllers; m != nil {
		for _, at := range m.List() {
			if at.Bus != assetfile.BusBluetooth {
				continue
			}
			if at.Seat.Valid() {
				seats[at.Seat.Index()] = true
				continue
			}
			loose++
		}
	}
	a.st.padSeats, a.st.padsLoose = seats, loose
}

// padPipColors match the cabinet sides and the Bluetooth app's badges.
var padPipColors = [2]display.Pixel{
	{R: 235, G: 80, B: 70, A: 255},  // P1, the red side
	{R: 70, G: 140, B: 255, A: 255}, // P2, the blue side
}

// padLooseColor marks an awake pad with no seat: solid but grey.
var padLooseColor = display.Pixel{R: 150, G: 150, B: 158, A: 255}

var padEmptyColor = display.Pixel{R: 70, G: 70, B: 78, A: 255}

// drawPadPips paints the two seat pips at x, y. An unseated pad borrows the
// first free seat's square in grey; with both seats taken it is not shown.
func drawPadPips(out display.Sprite, x, y int, seats [2]bool, loose int) {
	borrow := -1
	if loose > 0 {
		switch {
		case !seats[0]:
			borrow = 0
		case !seats[1]:
			borrow = 1
		}
	}
	for seat := 0; seat < 2; seat++ {
		ox := x + seat*(padPipSize+padPipGap)
		switch {
		case seats[seat]:
			drawPadPip(out, ox, y, padPipColors[seat], true)
		case seat == borrow:
			drawPadPip(out, ox, y, padLooseColor, true)
		default:
			drawPadPip(out, ox, y, padEmptyColor, false)
		}
	}
}

func drawPadPip(out display.Sprite, x, y int, c display.Pixel, filled bool) {
	for dy := 0; dy < padPipSize; dy++ {
		for dx := 0; dx < padPipSize; dx++ {
			edge := dy == 0 || dy == padPipSize-1 || dx == 0 || dx == padPipSize-1
			if !filled && !edge {
				continue
			}
			px, py := x+dx, y+dy
			if py < 0 || py >= len(out) || px < 0 || px >= len(out[py]) {
				continue
			}
			out[py][px] = c
		}
	}
}

// padPipsWidth is the width the tile label must stop short of.
const padPipsWidth = padPipSize*2 + padPipGap
