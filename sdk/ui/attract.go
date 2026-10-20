// SPDX-License-Identifier: GPL-2.0-or-later

package ui

// Attract drives the attract-mode phase cycle: an ordered list of timed
// beats (a score panel, then a silent demo, back to the panel, and so on)
// with the phase index, per-phase clock and per-transition setup handled for
// the cartridge. It does not decide what a phase draws or when a demo ends
// early, and it knows nothing about sound, LCDs, lamps or scores.
//
// Phases is built once, in Init or as a package-level variable, not inside a
// render function. A phase with DurationMicros > 0 ends itself:
// [Attract.Update] advances to the next phase, wrapping past the last to
// Phases[0], once the phase clock reaches that duration. A phase with
// DurationMicros == 0 never times out; the cartridge calls [Attract.Next] or
// [Attract.Goto] when it decides the phase is over, or to pick the next phase
// from data. Call Update from the update phase and [Attract.Draw] from the
// render phase. Attract itself allocates nothing.
//
// A phase's OnEnter runs once, synchronously, when the phase becomes current,
// including Phases[0] before the first transition (fired lazily by Update or
// Draw). It is the place to arm a demo's silent flag and swap in a local
// deterministic RNG before anything can play a sound or draw from RandomU64.
//
// Attract is a plain value, not safe for concurrent use. Its zero value is
// ready to use once Phases is set. With no Phases, every method is a no-op.
type Attract struct {
	// Phases is the ordered beat list. Build it once as a package-level
	// var, like [Menu].Items.
	Phases []AttractPhase

	phase   int
	elapsed uint64
	started bool
}

// AttractPhase is one beat of an Attract loop: how long it runs before
// auto-advancing, what to draw while it runs, and what to do once when it
// begins.
type AttractPhase struct {
	// DurationMicros is how long this phase runs before Update advances to
	// the next phase. Zero means the phase never times out.
	DurationMicros uint64

	// OnEnter runs once, synchronously, when this phase becomes current. Nil
	// is a no-op.
	OnEnter func()

	// Draw renders the phase onto cv; elapsed is the phase's own clock, the
	// microseconds accumulated since entry. Nil is a no-op.
	Draw func(elapsed uint64, cv Canvas)
}

// ensureStarted fires Phases[0]'s OnEnter once, before the first transition.
func (a *Attract) ensureStarted() {
	if a.started {
		return
	}
	a.started = true
	a.fireEnter()
}

func (a *Attract) fireEnter() {
	if a.phase < 0 || a.phase >= len(a.Phases) {
		return
	}
	if f := a.Phases[a.phase].OnEnter; f != nil {
		f()
	}
}

// Update advances the current phase's clock by dtMicros microseconds. If the phase has a
// nonzero DurationMicros that the clock has reached, it advances to the next
// phase (wrapping), fires its OnEnter and reports true. Call it once per
// frame with that frame's dtMicros.
//
// A phase with DurationMicros == 0 still accumulates its clock but never
// advances here.
func (a *Attract) Update(dtMicros uint64) bool {
	a.ensureStarted()
	if len(a.Phases) == 0 {
		return false
	}
	a.elapsed += dtMicros
	d := a.Phases[a.phase].DurationMicros
	if d == 0 || a.elapsed < d {
		return false
	}
	a.Next()
	return true
}

// Next ends the current phase and moves to the next one in Phases, wrapping
// past the last to index 0. It zeroes the clock and fires the new phase's
// OnEnter. Use it for a cartridge-decided early exit that should behave like a
// timeout.
func (a *Attract) Next() {
	if len(a.Phases) == 0 {
		return
	}
	a.Goto(a.phase + 1)
}

// Goto jumps to phase i, zeroing the clock and firing that phase's OnEnter.
// i is wrapped into range, so a negative i or one past the end is valid.
// Going to the current phase restarts it.
func (a *Attract) Goto(i int) {
	n := len(a.Phases)
	if n == 0 {
		return
	}
	i %= n
	if i < 0 {
		i += n
	}
	a.phase = i
	a.elapsed = 0
	a.started = true
	a.fireEnter()
}

// Phase reports the current phase's index into Phases.
func (a *Attract) Phase() int { return a.phase }

// Elapsed reports the microseconds accumulated since the current phase began. A
// phase's Draw receives the same value; Elapsed serves drawing outside that
// callback, such as LCD text or button lights.
func (a *Attract) Elapsed() uint64 { return a.elapsed }

// Progress reports the current phase's completion in [0, 1]: Elapsed divided
// by DurationMicros, clamped. It is 0 for a phase with DurationMicros == 0.
func (a *Attract) Progress() float64 {
	if a.phase < 0 || a.phase >= len(a.Phases) {
		return 0
	}
	d := a.Phases[a.phase].DurationMicros
	if d == 0 {
		return 0
	}
	if a.elapsed >= d {
		return 1
	}
	return float64(a.elapsed) / float64(d)
}

// Draw fires Phases[0]'s OnEnter if it has not run yet, then calls the current
// phase's Draw with its elapsed clock. It does nothing with no Phases or a nil
// Draw.
func (a *Attract) Draw(cv Canvas) {
	a.ensureStarted()
	if a.phase < 0 || a.phase >= len(a.Phases) {
		return
	}
	if f := a.Phases[a.phase].Draw; f != nil {
		f(a.elapsed, cv)
	}
}
