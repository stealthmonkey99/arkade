// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"
	"time"
)

// These exercise the standby struct without a Runtime or app registry, so they cannot collide with the process-global registry (TestEngineQuitKeyStopsLoop needs no shell). cmd/arkade/standby_test.go covers the full integration.

func TestStandbyIdleEntersAttract(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 0, epoch)

	if got := s.step(epoch.Add(4*time.Minute), true); got != actionNone {
		t.Fatalf("expected no action before attractAfter, got %v", got)
	}
	if s.state != StateActive {
		t.Fatalf("expected StateActive, got %v", s.state)
	}

	now := epoch.Add(5 * time.Minute)
	if got := s.step(now, true); got != actionEnterAttract {
		t.Fatalf("expected actionEnterAttract, got %v", got)
	}
	if s.state != StateAttract {
		t.Fatalf("expected StateAttract, got %v", s.state)
	}
}

func TestStandbyForceDarkIsHardAndMotionDoesNotWake(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 10*time.Minute, epoch)

	now := epoch.Add(time.Minute)
	s.forceDark(now)
	if s.state != StateDark {
		t.Fatalf("expected StateDark after forceDark, got %v", s.state)
	}
	if !s.hard {
		t.Fatal("forceDark must be hard: sleep is as deliberate as the BACK-in-attract path, so the person who just pressed it must not be woken back up by their own motion")
	}
	if s.lastInput != now || s.lastPresence != now {
		t.Fatal("forceDark must reset both clocks to now (fresh-clock contract)")
	}

	// Motion alone must not undo a deliberate sleep.
	s.noteMotion(now.Add(time.Minute))
	if s.state != StateDark {
		t.Fatalf("expected motion to leave a hard dark alone, got %v", s.state)
	}

	// A real input still wakes normally (noteInput never checks hard).
	if wake := s.noteInput(now.Add(2 * time.Minute)); !wake {
		t.Fatal("expected a real input to wake a hard dark")
	}
	if s.state != StateActive {
		t.Fatalf("expected StateActive after input wake, got %v", s.state)
	}
}

func TestStandbyForceDarkWithZeroDecayIsSoftAndMotionWakes(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 0, epoch)

	now := epoch.Add(time.Minute)
	s.forceDark(now)
	if s.hard {
		t.Fatal("expected hard flag to stay clear when hardDecayAfter is 0")
	}

	// Motion wakes a soft dark back to ATTRACT, same as a timeout dark.
	s.noteMotion(now.Add(time.Minute))
	if s.state != StateAttract {
		t.Fatalf("expected motion to wake soft dark to StateAttract, got %v", s.state)
	}
}

func TestStandbyMotionKeepsAttractAliveIndefinitely(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 0, epoch)
	now := epoch.Add(5 * time.Minute)
	if got := s.step(now, true); got != actionEnterAttract {
		t.Fatalf("expected actionEnterAttract, got %v", got)
	}

	// Motion just before the standby deadline resets it; repeat well past
	// what a single standbyAfter window would have allowed.
	for i := 0; i < 5; i++ {
		now = now.Add(9 * time.Minute)
		if wake := s.noteMotion(now); wake {
			t.Fatalf("motion while already ATTRACT should not report a wake")
		}
		if got := s.step(now, true); got != actionNone {
			t.Fatalf("iteration %d: motion should have kept attract alive, got %v", i, got)
		}
		if s.state != StateAttract {
			t.Fatalf("iteration %d: expected StateAttract, got %v", i, s.state)
		}
	}
}

func TestStandbyNoPresenceGoesDark(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 0, epoch)
	now := epoch.Add(5 * time.Minute)
	s.step(now, true) // -> attract

	now = now.Add(10 * time.Minute)
	if got := s.step(now, true); got != actionEnterDark {
		t.Fatalf("expected actionEnterDark, got %v", got)
	}
	if s.state != StateDark {
		t.Fatalf("expected StateDark, got %v", s.state)
	}
}

func TestStandbyMotionWakesDarkToAttract(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &standby{attractAfter: 5 * time.Minute, standbyAfter: 10 * time.Minute, state: StateDark, lastInput: epoch, lastPresence: epoch}

	if wake := s.noteMotion(epoch.Add(time.Second)); !wake {
		t.Fatal("expected motion to wake DARK back to ATTRACT")
	}
	if s.state != StateAttract {
		t.Fatalf("expected StateAttract, got %v", s.state)
	}
}

func TestStandbyInputAlwaysWakesToActive(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, start := range []StandbyState{StateAttract, StateDark} {
		s := &standby{attractAfter: 5 * time.Minute, standbyAfter: 10 * time.Minute, state: start, lastInput: epoch, lastPresence: epoch}
		if wake := s.noteInput(epoch.Add(time.Second)); !wake {
			t.Fatalf("from %v: expected input to report a wake", start)
		}
		if s.state != StateActive {
			t.Fatalf("from %v: expected StateActive, got %v", start, s.state)
		}
	}

	// Input while already ACTIVE is not a wake — just refreshes the timer.
	s := newStandby(5*time.Minute, 10*time.Minute, 0, epoch)
	if wake := s.noteInput(epoch.Add(time.Minute)); wake {
		t.Fatal("input while already ACTIVE should not report a wake")
	}
}

func TestStandbyAttractGateHoldsActive(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 0, epoch)

	// allowAttract=false (boot app running) holds ACTIVE no matter how
	// stale the input clock gets...
	if got := s.step(epoch.Add(time.Hour), false); got != actionNone {
		t.Fatalf("gated step must not act, got %v", got)
	}
	if s.state != StateActive {
		t.Fatalf("expected StateActive while gated, got %v", s.state)
	}

	// ...but the clock kept aging: the next ungated step fires immediately.
	if got := s.step(epoch.Add(time.Hour+time.Second), true); got != actionEnterAttract {
		t.Fatalf("expected actionEnterAttract once ungated, got %v", got)
	}

	// The gate only guards the ACTIVE→ATTRACT edge: attract→dark still
	// happens even if a boot app were somehow current.
	if got := s.step(epoch.Add(2*time.Hour), false); got != actionEnterDark {
		t.Fatalf("expected actionEnterDark regardless of gate, got %v", got)
	}
}

func TestStandbyZeroAttractAfterDisablesFeature(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(0, 10*time.Minute, 0, epoch)
	if got := s.step(epoch.Add(24*time.Hour), true); got != actionNone {
		t.Fatalf("expected disabled standby to never act, got %v", got)
	}
	if s.state != StateActive {
		t.Fatalf("expected StateActive forever, got %v", s.state)
	}
}

func TestStandbyZeroStandbyAfterDisablesDarkOnly(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 0, 0, epoch)
	now := epoch.Add(5 * time.Minute)
	if got := s.step(now, true); got != actionEnterAttract {
		t.Fatalf("expected actionEnterAttract, got %v", got)
	}
	now = now.Add(24 * time.Hour)
	if got := s.step(now, true); got != actionNone {
		t.Fatalf("expected attract to run forever with standbyAfter=0, got %v", got)
	}
	if s.state != StateAttract {
		t.Fatalf("expected StateAttract, got %v", s.state)
	}
}

func TestStandbyForceAttractResetsClocksAndState(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 10*time.Minute, epoch)

	// Age the clocks first, like a shell that's been sitting idle a while
	// before someone finally presses BACK.
	stale := epoch.Add(4 * time.Minute)
	s.lastInput, s.lastPresence = stale, stale

	now := epoch.Add(10 * time.Minute)
	s.forceAttract(now)

	if s.state != StateAttract {
		t.Fatalf("expected StateAttract, got %v", s.state)
	}
	if !s.lastInput.Equal(now) || !s.lastPresence.Equal(now) {
		t.Fatalf("expected both clocks reset to now, got lastInput=%v lastPresence=%v", s.lastInput, s.lastPresence)
	}
	// The attract->dark countdown must start fresh from now, not from the
	// stale clocks: standbyAfter later should NOT yet have elapsed.
	if got := s.step(now.Add(9*time.Minute), true); got != actionNone {
		t.Fatalf("expected countdown to restart from forceAttract's now, got %v", got)
	}
	if got := s.step(now.Add(10*time.Minute+time.Second), true); got != actionEnterDark {
		t.Fatalf("expected actionEnterDark once the fresh countdown elapses, got %v", got)
	}
}

func TestStandbyEnterHardDarkSetsHardFlag(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 10*time.Minute, epoch)
	s.state = StateAttract

	now := epoch.Add(time.Minute)
	s.enterHardDark(now)

	if s.state != StateDark {
		t.Fatalf("expected StateDark, got %v", s.state)
	}
	if !s.hard {
		t.Fatal("expected hard flag set")
	}
	if !s.lastPresence.Equal(now) {
		t.Fatalf("expected lastPresence reset to now (the press proves presence), got %v", s.lastPresence)
	}
}

func TestStandbyEnterHardDarkWithZeroDecayIsPlainDark(t *testing.T) {
	// hardDecayAfter == 0 disables hard mode entirely: the BACK-in-attract
	// press still goes dark, it's just never "hard".
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := newStandby(5*time.Minute, 10*time.Minute, 0, epoch)
	s.state = StateAttract

	s.enterHardDark(epoch.Add(time.Minute))

	if s.state != StateDark {
		t.Fatalf("expected StateDark, got %v", s.state)
	}
	if s.hard {
		t.Fatal("expected hard flag to stay clear when hardDecayAfter is 0")
	}
}

func TestStandbyHardDarkSuppressesMotionWakeButRefreshesPresence(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &standby{
		attractAfter: 5 * time.Minute, standbyAfter: 10 * time.Minute, hardDecayAfter: 10 * time.Minute,
		state: StateDark, hard: true, lastInput: epoch, lastPresence: epoch,
	}

	now := epoch.Add(time.Minute)
	if wake := s.noteMotion(now); wake {
		t.Fatal("expected motion to NOT wake a hard dark")
	}
	if s.state != StateDark {
		t.Fatalf("expected to remain StateDark, got %v", s.state)
	}
	if !s.hard {
		t.Fatal("expected hard flag to remain set")
	}
	if !s.lastPresence.Equal(now) {
		t.Fatalf("expected lastPresence to still refresh (person still in room), got %v", s.lastPresence)
	}
}

func TestStandbyHardDarkDecaysToSoftAfterNoPresence(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &standby{
		attractAfter: 5 * time.Minute, standbyAfter: 10 * time.Minute, hardDecayAfter: 10 * time.Minute,
		state: StateDark, hard: true, lastInput: epoch, lastPresence: epoch,
	}

	// Just before the decay window: still hard, motion still doesn't wake.
	before := epoch.Add(10*time.Minute - time.Second)
	if got := s.step(before, true); got != actionNone {
		t.Fatalf("expected no action before hardDecayAfter elapses, got %v", got)
	}
	if !s.hard {
		t.Fatal("expected hard flag still set just before the decay window")
	}

	// Past the decay window with no intervening motion (lastPresence never
	// refreshed): hard clears.
	after := epoch.Add(10 * time.Minute)
	if got := s.step(after, true); got != actionHardDecay {
		t.Fatalf("expected actionHardDecay, got %v", got)
	}
	if s.hard {
		t.Fatal("expected hard flag cleared after decay")
	}
	if s.state != StateDark {
		t.Fatalf("expected to remain StateDark (decay doesn't wake), got %v", s.state)
	}

	// Now that it's soft again, motion wakes it like an ordinary dark.
	if wake := s.noteMotion(after.Add(time.Second)); !wake {
		t.Fatal("expected motion to wake the now-soft dark")
	}
	if s.state != StateAttract {
		t.Fatalf("expected StateAttract, got %v", s.state)
	}
}

func TestStandbyHardDarkMotionKeepsItHardIndefinitely(t *testing.T) {
	// Motion refreshing lastPresence must keep resetting the decay
	// countdown — a person still in the room never lets hard dark decay.
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &standby{
		attractAfter: 5 * time.Minute, standbyAfter: 10 * time.Minute, hardDecayAfter: 10 * time.Minute,
		state: StateDark, hard: true, lastInput: epoch, lastPresence: epoch,
	}

	now := epoch
	for i := 0; i < 5; i++ {
		now = now.Add(9 * time.Minute)
		s.noteMotion(now)
		if got := s.step(now, true); got != actionNone {
			t.Fatalf("iteration %d: motion should have kept it hard, got %v", i, got)
		}
		if !s.hard {
			t.Fatalf("iteration %d: expected hard to remain set", i)
		}
	}
}

func TestStandbyInputWakeClearsHardFlag(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &standby{
		attractAfter: 5 * time.Minute, standbyAfter: 10 * time.Minute, hardDecayAfter: 10 * time.Minute,
		state: StateDark, hard: true, lastInput: epoch, lastPresence: epoch,
	}

	if wake := s.noteInput(epoch.Add(time.Second)); !wake {
		t.Fatal("expected a real button press to wake a hard dark")
	}
	if s.state != StateActive {
		t.Fatalf("expected StateActive, got %v", s.state)
	}
	if s.hard {
		t.Fatal("expected hard flag cleared on wake")
	}
}
