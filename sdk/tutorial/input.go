// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Turning what the player did into what the game does: the frame's input
// records decoded, dispatched to the screen that is up, and the
// platform-standard hold-B exit that has to work from every one of them.
//
// Three rules from Authoring patterns' "Input truths" shape this whole
// file, and each of them is a bug that shipped first:
//
//   - Act on KindKeypress (r.IsPress()), never also on KindKeydown. The
//     cabinet sends a keydown AND a keypress per physical press, so
//     handling both fires everything twice.
//   - Route keyup to ui.HoldConfirm BEFORE any keypress-only early
//     return, or the confirm can never arm on a real pad.
//   - Track held directions with ui.HeldDirs and nothing else. A
//     terminal keyboard cannot report releases, so hand-rolled hold
//     tracking ships broken.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// OwnsQuitKeys tells cartlib that this game runs its own quit flow (the
// hold-B overlay), so cartlib must not answer B with an immediate Exit.
func (g *catcherGame) OwnsQuitKeys() bool { return true }

func (g *catcherGame) HandleInput(records []cartlib.InputRecord) {
	for i := range records {
		g.handleRecord(records[i])
	}
}

func (g *catcherGame) handleRecord(r cartlib.InputRecord) {
	g.trackHeld(r)

	// ui.HoldConfirm needs keydown AND keyup, so it routes before the
	// keypress-only filter below. This is why the overlay is a screen and
	// not a widget flag: the machine says which screen owns the record,
	// and this one owns it before the filter rather than after.
	if g.machine.State() == stConfirm {
		if !g.confirm.Feed(r) && r.IsPress() {
			g.closeConfirm() // any other press cancels
		}
		return
	}

	if !r.IsPress() {
		return // keypress is the canonical "pressed"; keydown would double-fire
	}

	switch g.machine.State() {
	case stTitle:
		g.titleInput(r)
	case stPlaying:
		g.playingInput(r)
	case stOver:
		// Locked out except for quitting — see overHoldMicros.
		if r.IsQuit() {
			cartlib.Exit()
		}
	case stName:
		g.nameInput(r)
	}
}

// trackHeld feeds ui.HeldDirs, the one sanctioned way to know a
// direction is being held: Feed fuses real keydown/keyup pairs, the
// stick's position level and a terminal's keypress-only stream. The stick
// is then set again so its diagonals count as their horizontal half —
// this game only moves left and right, and a player pushing up-left means
// left.
func (g *catcherGame) trackHeld(r cartlib.InputRecord) {
	g.held.Feed(r)
	if r.Kind == cartlib.KindJoystick {
		g.held.SetJoy(joyDir(r.Joy))
	}
}

func (g *catcherGame) titleInput(r cartlib.InputRecord) {
	switch r.Key {
	case cartlib.KeyJoyA, cartlib.KeyEnter, cartlib.KeySpace:
		g.startPlay(r.Physical)
	case cartlib.KeyQuit, cartlib.KeyEscape, cartlib.KeyJoyB:
		cartlib.Exit() // B at the title is immediate; no confirm
	}
}

func (g *catcherGame) playingInput(r cartlib.InputRecord) {
	switch r.Key {
	case cartlib.KeyJoyD:
		// D is "interact" platform-wide, and it is lit exactly when it
		// would do something. Pressing it when it wouldn't rings the
		// cabinet's polite "that does nothing" bell rather than
		// silently ignoring the player.
		if g.sim.startDash() {
			playDash()
			g.showPlayingLights()
		} else {
			cartlib.RejectKey()
		}
	case cartlib.KeyQuit, cartlib.KeyEscape, cartlib.KeyJoyB:
		g.openConfirm(r)
	}
}

func (g *catcherGame) nameInput(r cartlib.InputRecord) {
	switch r.Key {
	case cartlib.KeyJoyA, cartlib.KeyEnter, cartlib.KeySpace:
		g.name.Do(ui.ActionActivate)
		if g.name.Done() {
			g.submitName()
		}
	case cartlib.KeyQuit, cartlib.KeyEscape, cartlib.KeyJoyB:
		// The placement is not in the table yet, so a fumbled B here
		// would throw the whole run away. The platform-wide rule is that a
		// qualifying name entry asks first, and the overlay keeps clear of
		// the initials so the player can see what they would lose.
		g.openConfirm(r)
		g.confirm.Avoid(g.name.LetterBand())
	}
}

// openConfirm PUSHES the overlay over whatever raised it, and feeds it the
// press that opened it. The covered screen stays on the stack, which is
// why there is no "return state" variable here: the answer to "back to
// where?" is the stack itself, and a stack cannot disagree with the screen
// it was pushed from.
func (g *catcherGame) openConfirm(r cartlib.InputRecord) {
	g.confirm.Open()
	g.confirm.Feed(r)
	g.machine.Push(stConfirm)
}

// closeConfirm dismisses it. Nothing restores peripheral state for you —
// but Pop re-enters the covered screen, and entering a screen runs
// enterScreen, so its LCD text and lamps come back without this function
// knowing which screen that was.
func (g *catcherGame) closeConfirm() {
	g.confirm.Close()
	g.machine.Pop()
}

// joyDir maps the stick's 8-way position onto the four directions the
// widgets speak, with the diagonals resolved to their horizontal component.
func joyDir(j uint8) ui.Direction {
	switch j {
	case cartlib.JoyN:
		return ui.DirUp
	case cartlib.JoyS:
		return ui.DirDown
	case cartlib.JoyW, cartlib.JoyNW, cartlib.JoySW:
		return ui.DirLeft
	case cartlib.JoyE, cartlib.JoyNE, cartlib.JoySE:
		return ui.DirRight
	}
	return ui.DirNone
}
