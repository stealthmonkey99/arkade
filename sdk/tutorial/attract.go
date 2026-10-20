// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Catcher's slot in the cabinet's idle screensaver.
//
// Attract runs in a DEDICATED cartridge instance that only ever sees
// cart_init, cart_attract and cart_teardown — no load phase, no Update,
// no HandleInput. So none of the game state built up by playing exists
// here; this file starts from nothing every time the screensaver comes
// around.
//
// Audio and the peripherals are fully live in that instance, which is
// why the convention is STRUCTURAL silence rather than a careful
// one: attract plays zero sounds and touches no LCDs or lights, and
// attract_test.go asserts the sound count is zero across a long run. A
// title sting, if a game wants one, is fired from a path attract never
// reaches (LoadFrame, not Init).

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// attractDemo is the screensaver's own tiny simulation — deliberately
// not the game's. It needs no lives, no score and no input, and reusing
// sim would mean every rule change had to be re-reasoned about here too.
type attractDemo struct {
	acc     uint64
	elapsed uint64
	x       int
	dir     int
	walk    walker
	drops   [3]fruit
	top     ui.AttractScore
	init    bool
	buf     [8 + cartlib.MaxNameLen]byte
}

var demo attractDemo

// Attract implements cartlib.Attractor. main.go's //export cart_attract
// shim is the other half — implementing this method alone does not opt
// in.
func (g *catcherGame) Attract(dtMicros uint64, fb *cartlib.Framebuffer) {
	demo.step(dtMicros)
	demo.draw(fb)
}

func (d *attractDemo) step(dtMicros uint64) {
	if !d.init {
		d.init = true
		d.x, d.dir = 8, 1
		// Spread the three drops down the screen so the first frame
		// already looks like a game rather than an empty sky.
		for i := range d.drops {
			d.drops[i] = fruit{live: true, x: 10 + i*18, y16: -i * 22 * 16}
		}
		d.top = topScore(d.buf[:])
	}

	d.elapsed += dtMicros
	d.walk.advance(dtMicros, true)

	d.acc += dtMicros
	for d.acc >= stepMicros {
		d.acc -= stepMicros
		d.x += d.dir * walkStep
		if d.x <= 0 {
			d.x, d.dir = 0, 1
		}
		if d.x >= catcherMaxX {
			d.x, d.dir = catcherMaxX, -1
		}
		for i := range d.drops {
			f := &d.drops[i]
			f.y16 += fallBase
			// No RandomU64 here: attract is a decoration, and drawing
			// from the engine's shared RNG stream from a screensaver
			// would move every other consumer's position in it.
			if f.y() >= fbH-fruitH {
				f.y16 = -8 * 16
				f.x = (f.x + 23) % (fbW - fruitW + 1)
			}
		}
	}
}

func (d *attractDemo) draw(fb *cartlib.Framebuffer) {
	// ClearTransparent, not Clear: the engine's starfield shows through
	// underneath, which is what makes a cartridge's attract slot look
	// like part of the cabinet's screensaver rather than a takeover.
	fb.ClearTransparent()

	led := ui.FontLED{}
	w := led.TextWidth("CATCHER", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 4, "CATCHER", 1, colAccent)

	for i := range d.drops {
		f := d.drops[i]
		if f.live {
			spriteFruit.Draw(fb, f.x, f.y())
		}
	}
	d.walk.cel().Draw(fb, d.x, catcherY)

	if d.top.Score > 0 {
		ui.AttractPanel{
			X: 6, Y: 40, W: 52, H: 12,
			Top:    d.top,
			Trophy: true,
			Color:  colDim,
			T:      d.elapsed,
		}.Draw(fb)
	}
}
