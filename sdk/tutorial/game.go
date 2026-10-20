// SPDX-License-Identifier: GPL-2.0-or-later

package main

// The cartlib.Game implementation: the screen machine and the lifecycle
// transitions between its five screens. Everything here is allowed to
// call host functions; nothing here decides a game rule (catcher.go does
// that).

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/fsm"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// The screens. sdk/fsm holds which one is up; a cartridge declares its own
// dense enum exactly as it would for a plain uint8, and dispatches on it
// with the same switch. What the package adds is that the variable can
// only move through Go/Push/Pop, and every arrival runs enterScreen — so a
// screen cannot be entered without asserting what it owns.
const (
	stTitle fsm.State = iota
	stPlaying
	stOver
	stName
	// stConfirm is the hold-B quit overlay. It is a screen, not a flag:
	// input goes to it, Render draws it, and Update freezes everything
	// under it, which is what a screen means.
	stConfirm
)

// legalEdges is the map of this cartridge, one bit per destination. It is
// validation and editor data — it never decides anything. Pop has no entry
// because its destination is the stack, not an edge.
var legalEdges = []uint16{
	stTitle:   1 << stPlaying,
	stPlaying: 1<<stOver | 1<<stConfirm,
	stOver:    1<<stName | 1<<stTitle,
	stName:    1<<stTitle | 1<<stConfirm,
	stConfirm: 0,
}

const (
	// overHoldMicros is how long the game-over banner holds before the
	// game moves on by itself. Everything except quitting is ignored for
	// all of it — Authoring patterns' death-screen lockout, because
	// players are still mashing when they die.
	overHoldMicros = 2_500_000

	// nameMaxLen is the initials budget. Three is the arcade convention
	// and what every other cartridge uses.
	nameMaxLen = 3
)

type catcherGame struct {
	// machine is which screen is up. There is deliberately no `st` field
	// beside it: one truth, and no way to change it that skips the hook.
	machine fsm.Machine
	sim     sim

	held    ui.HeldDirs
	confirm ui.HoldConfirm
	name    *ui.NameEntry

	// seat is the physical seat this run is locked to, or
	// cartlib.SeatNone when the player is on a keyboard with no seat.
	// It addresses the LCD and the button lights: the input record's
	// Physical byte, never its Player byte (see the cartlib reference,
	// "Input").
	seat cartlib.Seat

	stepAcc uint64 // dtMicros accumulated toward the next sim step
	lcdAcc  uint64 // LCD refresh throttle

	// Scratch owned by the game, filled once per use. Building these in
	// Render or Update would allocate every frame, which -gc=leaking
	// never gives back (GOLDEN RULE 7).
	top     ui.AttractScore
	nameBuf [8 + cartlib.MaxNameLen]byte
}

func (g *catcherGame) Init(s *cartlib.Setup) {
	// Art first: loadSprites reads the cartridge's sheet out of its own
	// bundle (sprites.go). Everything that loads or binds goes through s,
	// the Setup that is live only until Init returns — it allocates, and
	// -gc=leaking never gives that back.
	loadSprites(s)
	loadSounds(s)
	defineGlyphs(s)
	g.name = ui.NewNameEntry(17, 30, nameMaxLen)
	g.confirm.W, g.confirm.H = cartlib.FBWidth, cartlib.FBHeight
	g.seat = cartlib.SeatNone
	g.machine.OnEnter(g.enterScreen)
	g.machine.Edges(legalEdges)

	// A push alone is inert — input stays permissive until something
	// locks it — so the title can be started from either controller.
	// startPlay locks to whichever seat actually pressed.
	cartlib.PushPlayerStrategy(cartlib.StrategyOnePlayerSolo)

	// Reset places the machine and fires the title's entry hook, which is
	// the whole of "show the title screen".
	g.machine.Reset(stTitle)
}

// enterScreen is the ONE place a screen asserts what it owns: its LCD
// line, its button lamps, and whatever it starts fresh. Nothing on the
// cabinet restores peripherals for you, and this hook runs on EVERY
// arrival — including the Pop back out from under a dismissed overlay,
// which is what puts the covered screen's LCD and lamps back.
//
// "Every arrival" is also the constraint on what belongs here. Work that
// must not be redone on a resume — reading the seat's last initials into
// the name widget, resetting the sim — stays at the call site below. The
// hook is for what is true whenever this screen is on top, not for how the
// player got to it.
func (g *catcherGame) enterScreen(prev, next fsm.State) {
	switch next {
	case stTitle:
		// The title belongs to no seat. Releasing the run's lock here is
		// what lets the next player start from either controller.
		g.sim.reset()
		g.held.Reset()
		stopMusic()
		cartlib.UnlockPlayerStrategy()
		g.seat = cartlib.SeatNone
		g.top = topScore(g.nameBuf[:])
		showTitlePeripherals()
	case stPlaying:
		g.pushLCD()
		g.showPlayingLights()
	case stOver:
		g.held.Reset()
		g.pushLCD()
		playGameOverLights(g.seat)
	case stName:
		g.pushLCD()
		showNameLights(g.seat)
	case stConfirm:
		showConfirmPeripherals(g.seat)
	}
}

// startPlay locks the panel and the input gate to the seat we have
// actually seen a press from. Never lock to a seat you haven't observed:
// an unobserved lock gates the real player out of their own game.
func (g *catcherGame) startPlay(seat cartlib.Seat) {
	g.sim.reset()
	playerWalk.reset()
	g.stepAcc = 0
	g.seat = seat
	cartlib.LockPlayerToSeat(seat)
	startMusic()
	g.machine.Go(stPlaying)
}

func (g *catcherGame) enterOver() {
	stopMusic()
	playOver()
	g.machine.Go(stOver)
}

// enterName opens the initials entry, prefilled with this seat's last
// name so a returning player accepts it with one press. The prefill is
// here rather than in the entry hook on purpose: the hook also runs when
// the quit overlay is dismissed, and re-prefilling then would wipe
// initials the player had already typed.
func (g *catcherGame) enterName() {
	g.name.Reset()
	if prev, ok := cartlib.RecentName(g.seat, g.nameBuf[:]); ok {
		g.name.SetName(string(prev))
	}
	g.name.SetRecents(cartlib.RecentNames(g.seat))
	g.machine.Go(stName)
}

func (g *catcherGame) submitName() {
	name := g.name.Name()
	cartlib.ScoreSubmit(g.sim.score, name)
	cartlib.NoteRecentName(g.seat, name)
	g.machine.Go(stTitle)
}

func (g *catcherGame) Update(dtMicros uint64) {
	g.held.Update(dtMicros)
	g.lcdAcc += dtMicros

	// Tick feeds the clock of the screen on TOP of the stack, and only
	// that one. That is what freezes a covered screen for free: while the
	// overlay is up, the banner timer below it does not advance.
	g.machine.Tick(dtMicros)

	switch g.machine.State() {
	case stPlaying:
		g.updatePlaying(dtMicros)
	case stOver:
		// Elapsed is time since this screen was entered — the per-screen
		// clock a cartridge would otherwise hand-roll and forget to zero.
		if g.machine.Elapsed() >= overHoldMicros {
			if cartlib.ScoreQualifies(g.sim.score) {
				g.enterName()
			} else {
				g.machine.Go(stTitle)
			}
		}
	case stName:
		// Move is level-triggered: the currently HELD direction, every
		// frame. Do is edge-triggered and lives in HandleInput.
		g.name.Move(g.held.Current())
		g.name.Update(dtMicros)
		if g.name.Done() {
			g.submitName()
		}
	case stConfirm:
		if g.confirm.Update(dtMicros) {
			cartlib.Exit()
		}
	}
	// stTitle has no per-frame work: Tick above IS its animation clock,
	// which render.go reads back as machine.Elapsed().
}

func (g *catcherGame) updatePlaying(dtMicros uint64) {
	dir := 0
	if g.held.Held(ui.DirLeft) {
		dir = -1
	} else if g.held.Held(ui.DirRight) {
		dir = 1
	}
	playerWalk.advance(dtMicros, dir != 0)

	// Fixed-step integration: accumulate the frame's delta and run whole
	// 25ms steps. dtMicros is the per-tick delta, never a running total
	// (see the cartlib reference, "The Game interface").
	g.stepAcc += dtMicros
	for g.stepAcc >= stepMicros {
		g.stepAcc -= stepMicros
		res := g.sim.step(dir, cartlib.RandomU64)
		if res.caught > 0 {
			playCatch()
			g.lcdAcc = lcdIntervalMicros // force the readout up now
		}
		if res.missed > 0 {
			playMiss()
			g.lcdAcc = lcdIntervalMicros
		}
		if g.sim.over() {
			g.enterOver()
			return
		}
	}
	g.showPlayingLights()
	g.tickLCD()
}

// topScore reads rank 1 out of the cabinet's shared table for the title
// screen. buf is caller-owned scratch; the name string is the one copy,
// made once per visit to the title.
func topScore(buf []byte) ui.AttractScore {
	score, name, ok := cartlib.ScoreRead(1, buf)
	if !ok {
		return ui.AttractScore{}
	}
	return ui.AttractScore{Name: string(name), Score: score}
}
