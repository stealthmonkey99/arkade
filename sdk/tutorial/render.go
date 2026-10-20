// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Every screen, drawn through the shared sdk/ui toolkit onto the
// cartridge's framebuffer.
//
// A *cartlib.Framebuffer IS a ui.Canvas, so widgets composite straight
// into the frame — and because the draw code targets ui.Canvas rather
// than the framebuffer type, a plain host `go test` can render any state
// into a ui.Layer and assert pixels without TinyGo or wasm.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The palette. LED panels bloom, so these are deliberately dimmer than a
// monitor would suggest — review the result at 1x from across a table,
// not zoomed in.
var (
	colSky    = ui.Color{R: 8, G: 12, B: 26, A: 255}
	colGround = ui.Color{R: 34, G: 46, B: 38, A: 255}
	colScore  = ui.Color{R: 255, G: 214, B: 92, A: 255}
	colLives  = ui.Color{R: 232, G: 66, B: 74, A: 255}
	colDim    = ui.Color{R: 150, G: 156, B: 170, A: 255}
	colAccent = ui.Color{R: 57, G: 215, B: 160, A: 255}
	colDash   = ui.Color{R: 92, G: 150, B: 255, A: 255}
)

const groundY = catcherY + catcherH // 61: the line the catcher stands on

// footer is built ONCE, at package level. Hints is caller-owned, exactly
// like Menu.Items, and building a slice inside a render function would
// allocate a new one every frame.
//
// Note what the hints draw: the buttons themselves, as coloured discs in
// the cabinet's own button colours. Never a letter — the machine's
// buttons are round, coloured and completely unlettered, so "PRESS A"
// names a control the player cannot find.
var footer = ui.HintRow{
	Y:          56,
	LabelColor: ui.Color{R: 170, G: 176, B: 190, A: 255},
	Hints: []ui.Hint{
		{Text: "PLAY", Color: ui.ButtonGreen},
		{Text: "MOVE", Color: ui.White, Control: ui.HintStick},
	},
}

func (g *catcherGame) Render(fb *cartlib.Framebuffer) {
	// Render may run BEFORE the first Update — the engine's init render —
	// so every branch here has to draw a valid frame from whatever Init
	// alone set up.
	//
	// Under is the screen BENEATH the top of the stack, or the top itself
	// when nothing is layered over it. Switching on it means the overlay
	// composes for free: the frozen screen draws exactly as it always
	// does, and the overlay goes on top of it.
	switch g.machine.Under() {
	case stTitle:
		g.drawTitle(fb)
	case stPlaying:
		g.drawPlaying(fb)
	case stOver:
		g.drawPlaying(fb)
		g.drawOverBanner(fb)
	case stName:
		g.drawNameEntry(fb)
	}

	// The overlay draws LAST, over the frozen screen underneath it.
	if g.machine.State() == stConfirm {
		g.confirm.Draw(fb)
	}
}

func (g *catcherGame) drawPlaying(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)
	ui.FillRect(fb, 0, groundY, cartlib.FBWidth, cartlib.FBHeight-groundY, colGround)

	g.drawWaveTitle(fb)

	for i := range g.sim.fruit {
		f := g.sim.fruit[i]
		if f.live {
			spriteFruit.Draw(fb, f.x, f.y())
		}
	}
	playerWalk.cel().Draw(fb, g.sim.x, catcherY)
	g.drawHUD(fb)
}

// drawWaveTitle flashes the wave's name across the panel as it starts.
// That name is the level's own — the token a human typed on the `level`
// line of assets/waves.arkadelevels, or into the level editor's rename box — so
// the authored data reaches the player as words and not only as falling
// fruit. It draws BEFORE the fruit, not after: a wave always opens with
// something falling through this band, and a fruit that passes in front of
// the lettering reads as a backdrop while one that the lettering punches
// through reads as a glitch.
func (g *catcherGame) drawWaveTitle(fb *cartlib.Framebuffer) {
	name := g.sim.waveTitle()
	if name == "" {
		return
	}
	small := ui.Font3x5{}
	w := small.TextWidth(len(name), 1)
	small.DrawText(fb, (cartlib.FBWidth-w)/2, 14, name, 1, colAccent)
}

// drawHUD is the panel's own readout: score top left, lives top right.
// It does not repeat the LCD, and the LCD does not repeat it — the same
// numbers, but the panel is what a player watching the fruit can see
// without looking down.
func (g *catcherGame) drawHUD(fb *cartlib.Framebuffer) {
	small := ui.Font3x5{}
	// DrawUint draws a number straight from an integer, with no string
	// built and thrown away on the way.
	small.DrawUint(fb, 1, 1, uint32(g.sim.score), 1, colScore)

	for i := 0; i < g.sim.lives; i++ {
		ui.FillRect(fb, cartlib.FBWidth-3-i*4, 1, 2, 2, colLives)
	}

	// The dash meter: a short bar under the score that fills back up as
	// the cooldown runs out, so "why is blue dark" has a visible answer.
	if g.sim.dashReady() {
		ui.FillRect(fb, 1, 7, 8, 1, colDash)
		return
	}
	w := 8 - 8*g.sim.dashCool/dashCooldownSteps
	if w > 0 {
		ui.FillRect(fb, 1, 7, w, 1, colDim)
	}
}

func (g *catcherGame) drawTitle(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)
	ui.FillRect(fb, 0, groundY, cartlib.FBWidth, cartlib.FBHeight-groundY, colGround)

	led := ui.FontLED{}
	w := led.TextWidth("CATCHER", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 6, "CATCHER", 1, colAccent)

	// Three pieces of fruit sitting still above the catcher, so the
	// title screen says what the game is without a word of explanation.
	spriteFruit.Draw(fb, 14, 22)
	spriteFruit.Draw(fb, 30, 18)
	spriteFruit.Draw(fb, 45, 24)
	walkCels[0].Draw(fb, (cartlib.FBWidth-catcherW)/2, catcherY)

	// A one-row AttractPanel: the same widget the cabinet's screensaver
	// uses for a full leaderboard, sized down to a single high-score
	// line. Rows that would fall outside [Y, Y+H) are skipped whole.
	// T is the accumulated dtMicros — never a wall clock — which is what
	// lets a long name scroll without breaking golden reproducibility.
	// Elapsed is the machine's per-screen clock: time since the title was
	// entered, zeroed on the way in by the machine rather than by hand.
	if g.top.Score > 0 {
		ui.AttractPanel{
			X: 6, Y: 34, W: 52, H: 12,
			Top:    g.top,
			Trophy: true,
			Color:  colDim,
			T:      g.machine.Elapsed(),
		}.Draw(fb)
	}

	footer.X = (cartlib.FBWidth - footer.Width()) / 2
	footer.Draw(fb)
}

func (g *catcherGame) drawOverBanner(fb *cartlib.Framebuffer) {
	ui.Rect{
		X: 4, Y: 22, W: 56, H: 20,
		Fill:    ui.Color{R: 10, G: 10, B: 14, A: 235},
		Stroke:  colLives,
		StrokeW: 1,
	}.Draw(fb)

	led := ui.FontLED{}
	w := led.TextWidth("GAME OVER", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 26, "GAME OVER", 1, ui.White)

	small := ui.Font3x5{}
	sw := small.TextWidth(small.NumDigits(uint32(g.sim.score)), 1)
	small.DrawUint(fb, (cartlib.FBWidth-sw)/2, 35, uint32(g.sim.score), 1, colScore)
}

func (g *catcherGame) drawNameEntry(fb *cartlib.Framebuffer) {
	fb.Clear(colSky.R, colSky.G, colSky.B)

	led := ui.FontLED{}
	w := led.TextWidth("TOP 10!", 1)
	led.DrawText(fb, (cartlib.FBWidth-w)/2, 8, "TOP 10!", 1, colScore)

	small := ui.Font3x5{}
	sw := small.TextWidth(small.NumDigits(uint32(g.sim.score)), 1)
	small.DrawUint(fb, (cartlib.FBWidth-sw)/2, 18, uint32(g.sim.score), 1, colDim)

	g.name.Draw(fb)
}

// playerWalk is the gameplay catcher's cycle. The walk lives with the
// drawing rather than in catcherGame because that is what it is — how
// long a cel is held is a property of the art, and the game only ever
// has to say whether the catcher is moving.
var playerWalk walker

// walker plays the catcher's walk cycle over walkCels — the four steps
// sprites.go lifted out of the bundle's sheet, with walkHoldMicros saying
// how long each is held. It has no clock of its own, deliberately: what a
// tick is worth is a per-game answer, so the cartridge advances it.
type walker struct {
	acc  uint64 // time spent on the current step
	step int    // index into walkCels / walkHoldMicros
}

func (w *walker) reset() { w.acc, w.step = 0, 0 }

// advance moves the cycle on while the catcher is moving and parks it on
// the standing cel when they stop.
func (w *walker) advance(dtMicros uint64, moving bool) {
	if !moving {
		w.reset()
		return
	}
	w.acc += dtMicros
	for {
		hold := walkHoldMicros[w.step]
		if hold == 0 || w.acc < hold {
			return
		}
		w.acc -= hold
		// The cycle loops, which is the only mode this game needs; the
		// envelope also spells "once" (clamp at the last step) and
		// "pingpong" (walk back down the order).
		w.step++
		if w.step >= len(walkCels) {
			w.step = 0
		}
	}
}

func (w *walker) cel() *ui.Sprite { return walkCels[w.step] }
