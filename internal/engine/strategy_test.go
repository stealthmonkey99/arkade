// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func controllerSrc(index int) input.PlayerSource {
	return input.PlayerSource{
		Type: input.SourceController, ID: "usb-test",
		Slot: input.SlotOf(index), Seat: input.SeatOf(index),
	}
}

func TestOnePlayerSoloRotatesTowardLockedPlayer(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	s := engine.OnePlayerSolo(rt)
	rt.PushPlayerStrategy(s)

	// Player 2's controller (physical slot 1) plays, then the game locks:
	// display rotates clockwise to face the right-hand player.
	s.HandleInputSource(controllerSrc(1))
	s.Lock()
	if got := rt.Renderer.VirtualOrientation(); got != display.Clockwise {
		t.Errorf("locked solo play from slot 1 should rotate CW, got %v", got)
	}
	// Unlock returns upright (between rounds).
	s.Unlock()
	if got := rt.Renderer.VirtualOrientation(); got != display.Upright {
		t.Errorf("unlock should restore upright, got %v", got)
	}
}

func TestOnePlayerLockFiltersOtherControllers(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	s := engine.OnePlayerSolo(rt)
	rt.PushPlayerStrategy(s)
	s.HandleInputSource(controllerSrc(0))
	s.Lock()

	if !s.AllowInput(controllerSrc(0)) {
		t.Error("the locked player's controller must pass")
	}
	if s.AllowInput(controllerSrc(1)) {
		t.Error("other controllers must be blocked while locked")
	}
	s.Unlock()
	if !s.AllowInput(controllerSrc(1)) {
		t.Error("unlock should re-admit everyone")
	}
}

func TestTwoPlayerTurnBasedAlternation(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	s := engine.NewTwoPlayerTurnBased(rt)
	rt.PushPlayerStrategy(s)
	s.HandleInputSource(controllerSrc(0)) // slot 0 becomes primary
	s.Lock()

	// Primary's turn: only slot 0 passes.
	if !s.AllowInput(controllerSrc(0)) || s.AllowInput(controllerSrc(1)) {
		t.Error("primary turn should admit only the primary controller")
	}
	s.SwitchPlayer()
	// Opponent's turn: only NOT-slot-0 passes.
	if s.AllowInput(controllerSrc(0)) || !s.AllowInput(controllerSrc(1)) {
		t.Error("secondary turn should admit only the other controller")
	}
	s.SwitchPlayer(true)
	if !s.AllowInput(controllerSrc(0)) {
		t.Error("SwitchPlayer(true) should return the turn to primary")
	}
}

func TestEngineGateDropsBlockedEvents(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	s := engine.NewTwoPlayerTurnBased(rt)
	rt.PushPlayerStrategy(s)
	s.HandleInputSource(controllerSrc(0))
	s.Lock()

	// Rotation hotkey from the blocked controller must not fire: 'r' from
	// slot 1 during primary's turn.
	rig.RT.Input.Push(input.Event{
		Kind: input.KindKeypress, Name: input.KeyRotate,
		Source: controllerSrc(1),
	})
	rig.Tick(1)
	if got := rt.Renderer.Orientation(); got != display.Upright {
		t.Errorf("blocked controller's event should be dropped, got rotation %v", got)
	}
	// Same event from the primary controller works.
	rig.RT.Input.Push(input.Event{
		Kind: input.KindKeypress, Name: input.KeyRotate,
		Source: controllerSrc(0),
	})
	rig.Tick(1)
	if got := rt.Renderer.Orientation(); got != display.Clockwise {
		t.Errorf("primary controller's event should dispatch, got %v", got)
	}
}

func TestKeyboardBypassesStrategyGate(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	s := engine.OnePlayerSolo(rt)
	rt.PushPlayerStrategy(s)
	s.HandleInputSource(controllerSrc(0))
	s.Lock()

	// Keyboard-typed sources always pass.
	rig.PressKeyboard(input.KeyRotate, false)
	rig.Tick(1)
	if got := rt.Renderer.Orientation(); got != display.Clockwise {
		t.Errorf("keyboard events must bypass the strategy gate, got %v", got)
	}
}

func TestStrategyStackPopsOnAppUnload(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	// Simulate an app pushing a strategy through its context and being
	// torn down: the stack returns to base and everything is allowed.
	s := engine.NewTwoPlayerTurnBased(rt)
	rt.PushPlayerStrategy(s)
	s.Lock()
	rt.PopPlayerStrategy()

	rig.RT.Input.Push(input.Event{
		Kind: input.KindKeypress, Name: input.KeyRotate,
		Source: controllerSrc(1),
	})
	rig.Tick(1)
	if got := rt.Renderer.Orientation(); got != display.Clockwise {
		t.Errorf("after pop the base strategy should admit everyone, got %v", got)
	}
}

// seatedSrc is a controller whose attach SLOT and SEAT differ, which is
// the state every test above is missing: controllerSrc sets them equal, so
// gating on the wrong one of the two looks correct forever.
func seatedSrc(index, seat int) input.PlayerSource {
	return input.PlayerSource{
		Type: input.SourceController, ID: "bt-test",
		Slot: input.SlotOf(index), Seat: input.SeatOf(seat),
	}
}

// A seat can hold more than one controller. A wireless pad assigned to P1 attaches after the two sticks, keeping index 2 while playing seat 0, so the lock must gate on seat rather than the first-attached device's index. Otherwise only RED, the exempt quit key, works.
func TestEverySeatedControllerPlaysThatSeat(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	s := engine.OnePlayerSolo(rt)
	rt.PushPlayerStrategy(s)
	s.LockToSeat(input.SeatP1)

	stick := controllerSrc(0) // bolted in, attached first
	pad := seatedSrc(2, 0)    // wireless, same seat, third to attach
	other := seatedSrc(1, 1)  // the other seat, must stay out

	if !s.AllowInput(stick) {
		t.Error("the stick bolted to seat 0 was locked out of its own seat")
	}
	if !s.AllowInput(pad) {
		t.Error("a pad assigned to seat 0 cannot play seat 0 — this is the bug that killed Snake and Acorn Hills on a wireless pad")
	}
	if s.AllowInput(other) {
		t.Error("seat 1 got into a game locked to seat 0")
	}
}

// On the turn-based strategy, a pad sharing seat 0 with the stick must not count as the non-primary device and play P2's turns.
func TestASharedSeatPlaysItsOwnTurns(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "test/bounce"})
	rt := rig.RT

	s := engine.NewTwoPlayerTurnBased(rt)
	rt.PushPlayerStrategy(s)
	s.Lock()

	pad := seatedSrc(2, 0) // wireless, seat 0, attached third
	s.SwitchPlayer(true)   // seat 0's turn
	if !s.AllowInput(pad) {
		t.Error("a pad seated at 0 could not take seat 0's turn")
	}
	s.SwitchPlayer(false) // seat 1's turn
	if s.AllowInput(pad) {
		t.Error("a pad seated at 0 played seat 1's turn")
	}
}
