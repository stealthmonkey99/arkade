// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/render"
)

// A wireless pad keeps its link alive while the cabinet is dark, flattening an unused battery. Every route into DARK goes through enterDark, which is where the host is told to drop the links.
func TestPowerDownFiresOnceUntilTheCabinetWakes(t *testing.T) {
	var reasons []string
	n := powerDownNotice{fn: func(reason string) { reasons = append(reasons, reason) }}

	n.notify("standby")
	n.notify("standby") // re-entering DARK must not ask twice
	if len(reasons) != 1 {
		t.Fatalf("OnPowerDown fired %d times for one transition, want 1", len(reasons))
	}

	n.wake()
	n.notify("standby")
	if len(reasons) != 2 {
		t.Errorf("OnPowerDown fired %d times across a wake and a second sleep, want 2", len(reasons))
	}
}

func TestPowerDownWithNoHookIsHarmless(t *testing.T) {
	rt, err := New(Options{Renderer: render.NewCapture(false), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	rt.powerDown.notify("standby")
	rt.RingBell()
}

// A pad connecting arrives on the hot-plug goroutine, so its wake is folded in on the next tick instead of reaching the standby timers from under the engine.
func TestHardwareWakeIsTakenOnce(t *testing.T) {
	rt := &Runtime{}
	if rt.hardwareWake.Load() {
		t.Fatal("a fresh runtime already has a wake pending")
	}
	rt.WakeFromHardware()
	if !rt.hardwareWake.Load() {
		t.Fatal("WakeFromHardware did not raise the signal")
	}
	// Taking it clears it: one connect is one wake.
	if !rt.hardwareWake.Swap(false) {
		t.Error("the signal was not pending when the tick came to take it")
	}
	if rt.hardwareWake.Load() {
		t.Error("the signal survived being taken")
	}
}

// A deliberate sleep enters HARD dark, which ignores motion on purpose. A connecting controller is as deliberate as a press, so it must wake through the input path.
func TestHardwareWakeLeavesHardDark(t *testing.T) {
	s := &standby{}
	s.state = StateDark
	s.hard = true

	// The motion path refuses.
	if s.noteMotion(time.Now()) {
		t.Fatal("motion woke HARD dark; that is the state's whole purpose")
	}
	if s.state != StateDark || !s.hard {
		t.Fatal("motion changed the state it was supposed to ignore")
	}

	// The input path wakes it.
	if !s.noteInput(time.Now()) {
		t.Error("a controller connecting did not wake HARD dark")
	}
	if s.state != StateActive {
		t.Errorf("state %v after a controller connected, want active", s.state)
	}
	if s.hard {
		t.Error("the hard flag survived a deliberate wake")
	}
}

func TestHardwareWakeLeavesSoftDark(t *testing.T) {
	s := &standby{}
	s.state = StateDark
	if !s.noteInput(time.Now()) {
		t.Error("a controller connecting did not wake soft dark")
	}
	if s.state != StateActive {
		t.Errorf("state %v, want active", s.state)
	}
}
