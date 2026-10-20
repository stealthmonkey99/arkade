// SPDX-License-Identifier: GPL-2.0-or-later

package ui

import "github.com/stealthmonkey99/arkade/sdk/ui/input"

// HoldConfirm is the standard exit-confirmation overlay: press the quit key
// (B) to open it, then hold B to quit.
//
//   - Pressing B during play opens the overlay, and the cartridge stops
//     ticking its sim while Active.
//   - The opening press must be released before the hold can begin, so a
//     single long press cannot quit.
//   - Holding B fills a progress bar. At HoldMicros the widget confirms,
//     blinks for 800 ms, and Update reports true once the blink has played
//     and B is released. Releasing early drains the bar three times faster
//     than it filled and keeps the overlay open.
//   - Any other press closes the overlay and resumes play.
//
// Title, menu and game-over screens exit at once and do not use this widget.
// A qualifying name entry does use it, because a fumbled B would discard a
// score the player just earned. Opened over a [NameEntry], call
// [HoldConfirm.Avoid] with [NameEntry.LetterBand] right after
// [HoldConfirm.Open] so the panel does not cover the initials. A cartridge
// whose B returns to its title rather than exiting keeps that destination and
// retitles the prompt through Title, for example "END RUN?".
//
// The overlay is a modal over the screen that raised it: model it as a screen
// pushed on top (`fsm.Machine.Push` and `Pop`). Popping re-enters the covered
// screen, which must re-assert its own LCD text and lamps; peripherals are
// never restored for you.
//
// HoldConfirm keeps state across frames, so hold one as a field. A zero value
// is ready to use, and none of its methods allocate.
//
// # Feeding input
//
// While Active, pass every input record to [HoldConfirm.Feed], keydown and
// keyup included. Route it before any keypress-only filter, or the hold never
// arms on a real pad:
//
//	if confirm.Active() {
//	    if !confirm.Feed(r) && r.IsPress() {
//	        confirm.Close() // any other press cancels
//	    }
//	    return
//	}
//
// Open the overlay from the playing screen's quit press with
// [HoldConfirm.Open] followed by Feed of that same record. Each frame while
// Active, call [HoldConfirm.Update] with dtMicros and `cartlib.Exit` when it
// reports true. Call [HoldConfirm.Draw] last so the overlay composites over
// the frozen game.
//
// Real buttons send keydown and keyup, and the first keyup after Open arms
// the hold. The dev console keyboard sends only keypresses, so each virtual
// keypress grants PulseMicros of held credit and the credit expiring stands
// in for the release.
//
// HoldConfirmLCD1 and HoldConfirmLCD2 are the matching per-seat LCD legend.
type HoldConfirm struct {
	// W, H are the overlay's canvas size in pixels. Zero or less defaults
	// to 64.
	W, H int
	// Title is the panel's headline. Empty defaults to "QUIT GAME?".
	Title string
	// HoldMicros is how long B must be held to confirm, in microseconds. Zero
	// defaults to
	// 1,000,000 (1 s), the standard hold.
	HoldMicros uint64
	// PulseMicros is the held credit, in microseconds, granted per
	// [HoldConfirm.Pulse]. Zero defaults to 600,000, long enough to bridge a terminal's key auto-repeat delay.
	PulseMicros uint64
	// QuarterTurns rotates the whole overlay by that many clockwise quarter
	// turns (0-3) on a square canvas, so a prompt on a seat-locked tabletop
	// board stands upright for the players. Leave 0 elsewhere. Avoid does
	// not apply to a rotated overlay.
	QuarterTurns int

	active      bool
	armed       bool // opening press released
	held        bool
	virtualLeft uint64
	progress    uint64
	confirmed   bool
	blinkAccum  uint64
	done        bool
	avoidY      int
	avoidRows   int
}

// HoldConfirmLCD1 and HoldConfirmLCD2 are the standard 16x2 LCD legend for
// the overlay. Set them on the opening seat when it opens and re-assert the
// resumed screen's own text when it closes. Line 2 names the button by color
// because the cabinet's buttons are unlettered. The strings are compiled into
// each cartridge.
const (
	HoldConfirmLCD1 = "QUIT GAME?"
	HoldConfirmLCD2 = "HOLD RED TO QUIT"
)

// holdDrainFactor is how many times faster progress drains than it fills.
const holdDrainFactor = 3

// holdBlinkMicros is the confirmed-state blink: two on/off cycles.
const holdBlinkMicros = 800_000

// holdBlinkPeriod is one blink half-cycle.
const holdBlinkPeriod = 200_000

// Open shows the overlay with zeroed progress, unarmed: the press that opened
// it must be released before the hold fills. It does nothing while already
// active.
func (h *HoldConfirm) Open() {
	if h.active {
		return
	}
	h.active = true
	h.armed = false
	h.held = false
	h.virtualLeft = 0
	h.progress = 0
	h.confirmed = false
	h.blinkAccum = 0
	h.done = false
	h.avoidY, h.avoidRows = 0, 0
}

// Avoid keeps the panel off canvas rows y through y+rows-1 for the rest of
// this opening, typically a [NameEntry]'s [NameEntry.LetterBand]. Draw then
// centres the panel in the free space above the band, else below it, and
// stays centred when neither fits or the centred panel already clears it.
// Open forgets the band, so call Avoid after Open; on a closed overlay it
// does nothing. rows <= 0 clears the band. It has no effect on a rotated
// overlay (QuarterTurns != 0).
func (h *HoldConfirm) Avoid(y, rows int) {
	if h.active {
		h.avoidY, h.avoidRows = y, rows
	}
}

// Close hides the overlay and resets all hold state. Call it when the player
// cancels with any other press, then re-assert the resumed screen's LCD text
// and lamps.
func (h *HoldConfirm) Close() {
	h.active = false
	h.armed = false
	h.held = false
	h.virtualLeft = 0
	h.progress = 0
	h.confirmed = false
	h.blinkAccum = 0
	h.done = false
	h.avoidY, h.avoidRows = 0, 0
}

// Active reports whether the overlay is open. While true the cartridge freezes
// its sim and routes input here.
func (h *HoldConfirm) Active() bool { return h.active }

// KeyDown marks B physically pressed. It does nothing while closed, and does
// not fill the bar until a [HoldConfirm.KeyUp] has armed the widget.
func (h *HoldConfirm) KeyDown() {
	if h.active {
		h.held = true
	}
}

// KeyUp marks B physically released. The first release after Open arms the
// hold. It also drops outstanding virtual credit, since a source that reports
// release is authoritative, so a browser key that sends keydown, a virtual
// keypress and keyup stops filling at release.
func (h *HoldConfirm) KeyUp() {
	h.held = false
	h.virtualLeft = 0
	if h.active {
		h.armed = true
	}
}

// Pulse grants PulseMicros of synthetic hold credit, standing in for a device
// that cannot report key release. Arming then happens when the opening
// press's credit expires. It does nothing while closed.
func (h *HoldConfirm) Pulse() {
	if !h.active {
		return
	}
	p := h.PulseMicros
	if p == 0 {
		p = 600_000
	}
	h.virtualLeft = p
}

// Feed routes one input record to the open overlay. A quit-key record
// ([input.Record.IsQuit]: KeyJoyB, KeyQuit or KeyEscape) drives the hold, with
// keydown calling KeyDown, keyup KeyUp and a virtual keypress Pulse, and Feed
// reports true. Any other record is ignored and Feed reports false; a press
// among those is the player cancelling, which the caller answers with Close.
// Feed on a closed overlay does nothing and reports false.
func (h *HoldConfirm) Feed(r input.Record) bool {
	if !h.active || !r.IsQuit() {
		return false
	}
	switch r.Kind {
	case input.KindKeydown:
		h.KeyDown()
	case input.KindKeyup:
		h.KeyUp()
	case input.KindKeypress:
		if r.Virtual() {
			h.Pulse()
		}
	}
	return true
}

// Holding reports whether B counts as held: a real hold or unexpired virtual
// credit.
func (h *HoldConfirm) Holding() bool { return h.held || h.virtualLeft > 0 }

// Armed reports whether the opening press has been released.
func (h *HoldConfirm) Armed() bool { return h.armed }

// Confirmed reports whether the hold has completed and the blink is playing or
// has played.
func (h *HoldConfirm) Confirmed() bool { return h.confirmed }

// Progress returns the current fill fraction in [0, 1].
func (h *HoldConfirm) Progress() float64 {
	limit := h.HoldMicros
	if limit == 0 {
		limit = 1_000_000
	}
	if h.confirmed || h.progress >= limit {
		return 1
	}
	return float64(h.progress) / float64(limit)
}

// Update advances the widget by dtMicros microseconds and returns true exactly once: on the
// first frame where the hold has confirmed, the blink has played and B is
// released, which is when the cartridge can exit without leaking a held B to
// the shell. Call it once per frame from Update while Active.
func (h *HoldConfirm) Update(dtMicros uint64) bool {
	if !h.active || h.done {
		return false
	}

	// Virtual credit decays in every phase; its expiry is the release signal.
	if h.virtualLeft > 0 {
		if h.virtualLeft > dtMicros {
			h.virtualLeft -= dtMicros
		} else {
			h.virtualLeft = 0
			if !h.armed {
				h.armed = true
			}
		}
	}

	if h.confirmed {
		h.blinkAccum += dtMicros
		if h.blinkAccum >= holdBlinkMicros && !h.Holding() {
			h.done = true
			return true
		}
		return false
	}

	if !h.armed {
		return false
	}

	limit := h.HoldMicros
	if limit == 0 {
		limit = 1_000_000
	}
	if h.Holding() {
		h.progress += dtMicros
	} else {
		drain := dtMicros * holdDrainFactor
		if h.progress > drain {
			h.progress -= drain
		} else {
			h.progress = 0
		}
	}
	if h.progress >= limit {
		h.progress = limit
		h.confirmed = true
		h.blinkAccum = 0
	}
	return false
}

// Draw composites the overlay onto cv from the render phase: a dimming backdrop, a beveled panel,
// the title, and the hold indicator (a red button dot that brightens with
// progress beside a filling bar, captioned "HOLD TO QUIT"). Once confirmed the
// indicator row blinks white and the caption reads "BYE!". The panel keeps
// clear of the band given to Avoid. Draw does nothing while closed.
func (h *HoldConfirm) Draw(cv Canvas) {
	if !h.active {
		return
	}
	w, hh := h.W, h.H
	if w <= 0 {
		w = 64
	}
	if hh <= 0 {
		hh = 64
	}
	q := ((h.QuarterTurns % 4) + 4) % 4
	if q != 0 {
		cv = rotQuarter{cv: cv, w: w, h: hh, q: q}
	}
	FillRect(cv, 0, 0, w, hh, Color{0, 0, 0, 140})

	const pw, ph = 52, 28
	px, py := (w-pw)/2, (hh-ph)/2
	if q == 0 {
		// A rotated overlay's up is not the entry's up, so dodging along Y
		// would move it sideways on the panel.
		py = h.panelY(hh, ph, py)
	}
	DarkEdgeBorder(px, py, pw, ph, Color{26, 26, 32, 255}).Draw(cv)

	title := h.Title
	if title == "" {
		title = "QUIT GAME?"
	}
	font := Font3x5{}
	tx := px + (pw-font.TextWidth(len(title), 1))/2
	font.DrawText(cv, tx, py+4, title, 1, White)

	frac := h.Progress()
	blinkOn := h.confirmed && (h.blinkAccum/holdBlinkPeriod)%2 == 0
	dot := Color{R: 140 + uint8(115*frac), G: 30, B: 30, A: 255}
	fill := Color{220, 50, 50, 255}
	if blinkOn {
		dot = Color{255, 255, 255, 255}
		fill = Color{255, 255, 255, 255}
	} else if h.confirmed {
		dot = Color{255, 230, 230, 255}
	}
	cx, cy := px+7, py+15
	Circle{CX: cx, CY: cy, R: 3, Fill: dot, Stroke: Color{70, 10, 10, 255}}.Draw(cv)

	barX, barY := px+13, py+12
	barW, barH := pw-13-4, 7
	FillRect(cv, barX+1, barY+1, barW-2, barH-2, Black)
	LoadingBar{
		X: barX, Y: barY, W: barW, H: barH,
		Progress: frac,
		Fill:     fill,
		Border:   Color{120, 120, 120, 255},
	}.Draw(cv)

	caption := "HOLD TO QUIT"
	if h.confirmed {
		caption = "BYE!"
	}
	cxp := px + (pw-font.TextWidth(len(caption), 1))/2
	font.DrawText(cv, cxp, py+ph-7, caption, 1, Color{180, 180, 180, 255})
}

func (h *HoldConfirm) panelY(canvasH, ph, centered int) int {
	y, rows := h.avoidY, h.avoidRows
	if rows <= 0 {
		return centered
	}
	if y >= centered+ph || y+rows <= centered {
		return centered // the centered panel already clears it
	}
	if y >= ph {
		return (y - ph) / 2
	}
	if below := canvasH - (y + rows); below >= ph {
		return y + rows + (below-ph)/2
	}
	return centered
}

// rotQuarter is a Canvas adapter that rotates every draw by q clockwise
// quarter turns (q in 1..3) on a square canvas.
type rotQuarter struct {
	cv   Canvas
	w, h int
	q    int
}

func (r rotQuarter) Bounds() (w, h int) { return r.w, r.h }

func (r rotQuarter) Blend(x, y int, src Color) {
	switch r.q {
	case 1: // 90° CW
		r.cv.Blend(r.h-1-y, x, src)
	case 2: // 180°
		r.cv.Blend(r.w-1-x, r.h-1-y, src)
	case 3: // 90° CCW
		r.cv.Blend(y, r.w-1-x, src)
	default:
		r.cv.Blend(x, y, src)
	}
}
