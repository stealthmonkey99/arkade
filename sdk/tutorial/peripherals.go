// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Sound, the seat's LCD and the lit buttons. The LCD is the HUD and never
// repeats the panel; a button is lit exactly when pressing it would do
// something.

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Sound handles, resolved once in Init. Paths are bundle-relative,
// exactly as packed into the .rcart. On the browser host these resolve
// asynchronously during the splash phase; playing one that has not
// landed yet (or a bare .wasm with no bundle at all) is a safe no-op, so
// a dev build stays quiet instead of crashing.
var (
	sndCatch uint32
	sndMiss  uint32
	sndDash  uint32
	sndOver  uint32
	sndTheme uint32
)

func loadSounds(s *cartlib.Setup) {
	sndCatch = s.LoadSound("assets/catch.wav")
	sndMiss = s.LoadSound("assets/miss.wav")
	sndDash = s.LoadSound("assets/dash.wav")
	sndOver = s.LoadSound("assets/gameover.wav")
	sndTheme = s.LoadSound("assets/theme.wav")
}

func playCatch() { cartlib.PlaySound(sndCatch, 0) }
func playMiss()  { cartlib.PlaySound(sndMiss, 0) }
func playDash()  { cartlib.PlaySound(sndDash, 0) }

// playOver stops the bed first, so the sting lands in silence rather
// than fighting the loop for the same speaker.
func playOver() {
	cartlib.StopSound(sndTheme)
	cartlib.PlaySound(sndOver, 0)
}

func startMusic() { cartlib.PlaySound(sndTheme, cartlib.PlaySoundForever) }
func stopMusic()  { cartlib.StopSound(sndTheme) }

// The basket. A 16x2 character LCD draws ASCII out of the chip's own mask
// ROM and nothing else, so anything pictorial has to be uploaded as a 5x8
// bitmap first. The engine ships a table of those for the runes games kept
// asking for — the hearts on the lives line below are one — and
// Setup.DefineLCDGlyph is how a game brings a rune that table has never
// heard of, without anybody editing the engine.
//
// Read the hex down the left and the picture is right there: an arch for
// the handle, a full-width rim, a woven body, a narrower base. Bit 4 is the
// leftmost of the five pixels, so 0x11 is "both edges lit, middle dark".
//
// The rune is ֍ (U+058D, ARMENIAN ETERNITY SIGN): a plain, single-width,
// monochrome character that already reads as woven. Never an emoji —
// terminals draw those in colour and two cells wide, and both hosts refuse
// them (Setup.DefineLCDGlyph returns false).
const basket = '֍'

var basketRows = cartlib.GlyphRows{
	0x00, // . . . . .
	0x0e, // . # # # .   handle
	0x11, // # . . . #   handle ends
	0x1f, // # # # # #   rim
	0x15, // # . # . #   weave
	0x1f, // # # # # #   weave
	0x0e, // . # # # .   base
	0x00, // . . . . .
}

// defineGlyphs runs once, from Init. Nothing is reserved by defining a
// glyph: what is scarce is the chip's eight CGRAM slots, and that budget is
// per SCREEN and per MOMENT. The playing screen spends two of the eight —
// this basket on line 1, the engine's heart on line 2 — and the eight are
// shared between the two sources, so the number worth watching is how many
// DIFFERENT custom runes can be on one panel together.
func defineGlyphs(s *cartlib.Setup) {
	s.DefineLCDGlyph(basket, basketRows)
}

// LCD text, built once: formatting a fresh string or []byte every frame
// would allocate under an allocator that never gives memory back.
var (
	lcdTitle1  = []byte("CATCHER ֍")
	lcdTitle2  = []byte("GREEN TO PLAY")
	lcdNewBest = []byte("NEW BEST!")
	lcdName1   = []byte("TOP 10!")
	lcdName2   = []byte("YOUR INITIALS")

	// The hearts come from the engine's built-in glyph table — no
	// definition needed, they have been there for years. A rune in NEITHER
	// table (not the engine's, not this game's) renders as "?".
	lcdLives = [4][]byte{
		[]byte("LIVES"),
		[]byte("LIVES ♥"),
		[]byte("LIVES ♥♥"),
		[]byte("LIVES ♥♥♥"),
	}

	// lcdScore is the one line that is formatted at runtime, into a
	// fixed buffer.
	lcdScore [16]byte
)

// The two prefixes lcdScore is built with. In play the basket names the
// number without spending four of sixteen columns on the word — the panel
// is already showing the basket, so the LCD reads as the same object's
// tally. At game over the panel's banner already says GAME OVER and shows
// the score, so the LCD carries what the panel does not: the best score on
// the cabinet's table, and whether this run just beat it.
const (
	scorePrefixPlaying = "֍ "
	scorePrefixBest    = "BEST "
)

// refreshReadout runs every playing frame and writes the LCD only when the
// numbers on it are out of date. Score and lives move on a catch or a miss,
// a few times a run, so nearly every call returns at the guard.
func (g *catcherGame) refreshReadout() {
	if g.sim.score == g.shownScore && g.sim.lives == g.shownLives {
		return
	}
	g.pushLCD()
}

// pushLCD writes the current screen's readout, unconditionally. Every
// transition calls it directly, so the display is never left showing the
// previous screen's text.
func (g *catcherGame) pushLCD() {
	seat := g.seat
	if seat == cartlib.SeatNone {
		// The browser's shared keyboard sits at no seat, and the page draws
		// both LCDs: the score goes on both rather than on neither.
		seat = cartlib.SeatBoth
	}
	switch g.machine.State() {
	case stTitle:
		cartlib.SetTextBytes(seat, lcdTitle1, lcdTitle2)
	case stPlaying:
		n := formatScore(scorePrefixPlaying, g.sim.score)
		cartlib.SetTextBytes(seat, lcdScore[:n], lcdLives[clampLives(g.sim.lives)])
		g.shownScore, g.shownLives = g.sim.score, g.sim.lives
	case stOver:
		best, line2 := g.top.Score, []byte(nil)
		if g.sim.score > best {
			best, line2 = g.sim.score, lcdNewBest
		}
		n := formatScore(scorePrefixBest, best)
		cartlib.SetTextBytes(seat, lcdScore[:n], line2)
	case stName:
		cartlib.SetTextBytes(seat, lcdName1, lcdName2)
	}
}

func clampLives(lives int) int {
	if lives < 0 {
		return 0
	}
	if lives > 3 {
		return 3
	}
	return lives
}

// formatScore writes "<prefix><n>" into lcdScore and returns its length in
// BYTES, which is what SetTextBytes wants. Hand-rolled because a cartridge
// avoids fmt: it costs binary size and drags reflection in behind it. Note
// that a prefix carrying a custom glyph is more than one byte per column —
// the basket is two, the engine's heart three — so size the buffer by
// bytes, not by the LCD's sixteen columns.
func formatScore(prefix string, v int) int {
	n := copy(lcdScore[:], prefix)
	if v < 0 {
		v = 0
	}
	var digits [10]byte
	i := len(digits)
	for {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
		if v == 0 {
			break
		}
	}
	return n + copy(lcdScore[n:], digits[i:])
}

// showPlayingLights is eligibility lighting in its purest form: the blue
// D button is lit exactly while a dash is available and goes dark the
// instant one is spent. only=true means the other five buttons are
// turned off in the same call, so the lamp table always tells the whole
// truth.
func (g *catcherGame) showPlayingLights() {
	g.shownDash = g.sim.dashReady()
	if g.seat == cartlib.SeatNone {
		return
	}
	mask := cartlib.ButtonMask(0)
	if g.shownDash {
		mask = cartlib.MaskJoyD
	}
	cartlib.SetButtonLights(g.seat, mask, true)
}

// refreshLights is refreshReadout's twin for the lamps: it runs every
// playing frame and relights only when a dash has been spent or has come
// back, twice per dash.
func (g *catcherGame) refreshLights() {
	if g.sim.dashReady() == g.shownDash {
		return
	}
	g.showPlayingLights()
}

// blankIdleSeat clears the seat nobody is playing at once a one-player run
// starts: the title's invitation left on it would read as a game still
// waiting for a player. A keyboard run has no idle seat — pushLCD writes
// both LCDs — so a seatless start leaves both alone.
func blankIdleSeat(seat cartlib.Seat) {
	if !seat.Single() {
		return
	}
	cartlib.SetText(seat.Other(), "", "")
	cartlib.SetButtonLights(seat.Other(), 0, true)
}

// showTitlePeripherals lights green (start) on BOTH seats: the title
// invites either controller, and the seat the run locks to is whichever
// one presses.
func showTitlePeripherals() {
	cartlib.SetText(cartlib.SeatBoth, "CATCHER ֍", "GREEN TO PLAY")
	cartlib.SetButtonLights(cartlib.SeatBoth, cartlib.MaskJoyA, true)
}

func showNameLights(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.SetButtonLights(seat, cartlib.MaskJoyA|cartlib.MaskJoyB, true)
}

// lightsGameOver is the bundle path of the run's closing lamp cue, listed
// under this cartridge's manifest assets.lights — which is also the
// namespace host_lights_play draws from, so a typo here is a build error
// from rcartpack, not a lamp that quietly never lights.
const lightsGameOver = "assets/lights/gameover.arkadelights.json"

// playGameOverLights is the lamp half of the game-over beat, and it is a
// FILE rather than code (sdk/docs/reference-lights.md). The old version of
// this function was one SetButtonLights(seat, 0, true) — an instant dark
// panel — and the flourish that replaces it, two flashes of the whole panel
// before the dark, would have cost a timer, a frame index and a switch in
// Update to write by hand.
//
// The sequence names BOTH seats in every frame; passing this seat filters
// it to the one panel in front of the player, which is what the seat
// argument is for. It cannot MOVE a p1 cue onto p2 — a mask is per-seat
// data — so a cue meant for either seat has to say both, and this one does.
//
// The last frame is dark, so where the run leaves the panel is exactly
// where the hand-written call left it. That is deliberate: the flourish is
// an addition to the moment, not a change to the state after it.
func playGameOverLights(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.PlayLights(lightsGameOver, seat, cartlib.LightsLoopFile)
}

// showConfirmPeripherals puts up the shared quit legend. The LCD is
// monochrome text, so it cannot draw a coloured disc the way the panel
// can — it names the colour instead and lets the lit lamp identify the
// button. Never a letter, on either surface.
func showConfirmPeripherals(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.SetText(seat, ui.HoldConfirmLCD1, ui.HoldConfirmLCD2)
	cartlib.SetButtonLights(seat, cartlib.MaskJoyB, true)
}
