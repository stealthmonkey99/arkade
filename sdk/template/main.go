// SPDX-License-Identifier: GPL-2.0-or-later

// Command template is the Årkade cartridge SDK's minimal starter project:
// copy this directory, rename the module and manifest slug, and replace
// demoGame's methods. See sdk/README.md for the quickstart. Read this file as
// a tour of the [cartlib.Game] interface.
package main

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// storageKey is namespaced per cartridge slug by the host.
const storageKey = "counter"

// demoGame draws a moving shape, drives the button lights, and round-trips a
// counter through host storage.
type demoGame struct {
	x, y, dx, dy int    // the bouncing block's position/velocity
	lightIdx     int    // which of the 6 button lights is lit
	stepAcc      uint64 // dtMicros accumulator, see Update
	counter      uint32 // persisted across cartridge (re)loads
}

func (g *demoGame) Manifest() []byte { return manifestBytes }

// Init receives the Setup every asset load and channel bind goes through;
// this demo has none to make.
func (g *demoGame) Init(s *cartlib.Setup) {
	g.x, g.y, g.dx, g.dy = cartlib.FBWidth/2, cartlib.FBHeight/2, 1, 1
	var buf [4]byte
	if cartlib.StorageRead(storageKey, buf[:]) == 4 {
		g.counter = leUint32(buf[:])
	}
}

// HandleInput bumps and persists the counter on JoyA, Enter or Space. Quit
// keys need no case: cartlib calls cartlib.Exit for any Game that does not
// implement cartlib.QuitOwner.
func (g *demoGame) HandleInput(records []cartlib.InputRecord) {
	for _, r := range records {
		if !r.IsPress() {
			continue
		}
		switch r.Key {
		case cartlib.KeyJoyA, cartlib.KeyEnter, cartlib.KeySpace:
			g.counter++
			var buf [4]byte
			putLEUint32(buf[:], g.counter)
			cartlib.StorageWrite(storageKey, buf[:])
		}
	}
}

// Update steps the block and lights once per 100ms of accumulated dtMicros,
// the only time source a cartridge may use.
func (g *demoGame) Update(dtMicros uint64) {
	const stepMicros = 100_000
	g.stepAcc += dtMicros
	for g.stepAcc >= stepMicros {
		g.stepAcc -= stepMicros
		g.x, g.y = g.x+g.dx, g.y+g.dy
		if g.x <= 0 || g.x >= cartlib.FBWidth-8 {
			g.dx = -g.dx
		}
		if g.y <= 0 || g.y >= cartlib.FBHeight-8 {
			g.dy = -g.dy
		}
		g.lightIdx = (g.lightIdx + 1) % 6
		cartlib.SetButtonLights(cartlib.SeatBoth, cartlib.MaskJoyA<<uint(g.lightIdx), true)
	}
}

// Render must draw a valid frame from Init's state alone, since it may run
// before the first Update. A *cartlib.Framebuffer is a ui.Canvas, so the
// sdk/ui widgets draw straight into it.
func (g *demoGame) Render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)

	white := ui.Color{R: 255, G: 255, B: 255, A: 255}

	ui.Label{X: 1, Y: 1, Text: "TEMPLATE", Color: white, Scale: 1}.Draw(fb)

	ui.Rect{X: g.x, Y: g.y, W: 8, H: 8, Fill: ui.Color{R: 255, G: 120, B: 0, A: 255}}.Draw(fb)

	ui.Font3x5{}.DrawUint(fb, 1, 56, g.counter, 1, white)
}

func leUint32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func putLEUint32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

// cartlib.Run registers the Game at package level, not in main, which the
// host never calls; see the cartlib reference, "Registration and the
// reactor model".
var game = &demoGame{}
var _ = cartlib.Run(game)

func main() {}
