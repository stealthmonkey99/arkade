// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"fmt"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// paint stands in for a layer: render, then clean, as display.Layer does.
func paint(m *Menu, elapsedMs float64) {
	m.Render(clock.FrameTime{ElapsedMs: elapsedMs})
	m.MarkClean()
}

// TestMenuIsCleanAfterPaintUntilFocusMoves: a painted, idle menu lets the
// layer reuse its cache; a focus move dirties it again.
func TestMenuIsCleanAfterPaintUntilFocusMoves(t *testing.T) {
	m := newTestMenu(3)
	if !m.IsDirty() {
		t.Fatal("a new menu must paint once")
	}
	paint(m, 33.3)
	tick(m, 33.3, 5)
	if m.IsDirty() {
		t.Fatal("an idle menu stayed dirty after a paint")
	}
	m.HandleInput(input.Event{Kind: input.KindKeypress, Name: input.KeyDown})
	if !m.IsDirty() {
		t.Fatal("a focus move must dirty the menu")
	}
	paint(m, 33.3)
	if m.IsDirty() {
		t.Fatal("painting the moved focus did not clean the buttons")
	}
}

// TestMenuScrollsInUpdateNotRender: with the focus far down the list, Render
// alone never moves the view; each Update steps it until it settles, and the
// menu is dirty for exactly those steps.
func TestMenuScrollsInUpdateNotRender(t *testing.T) {
	m := newTestMenu(10)
	paint(m, 33.3)
	for i := 0; i < 9; i++ {
		m.HandleInput(input.Event{Kind: input.KindKeypress, Name: input.KeyDown})
	}
	first := m.Render(clock.FrameTime{ElapsedMs: 33.3})
	second := m.Render(clock.FrameTime{ElapsedMs: 33.3})
	if !display.MatrixEquals(first, second) {
		t.Fatal("Render advanced the scroll; animation belongs in Update")
	}
	m.MarkClean()

	steps := 0
	for i := 0; i < 60; i++ {
		m.Update(clock.FrameTime{ElapsedMs: 33.3})
		if !m.IsDirty() {
			break
		}
		steps++
		paint(m, 33.3)
	}
	if steps == 0 {
		t.Fatal("Update never scrolled toward the focused button")
	}
	if steps >= 60 {
		t.Fatal("scroll never settled")
	}
	settled := m.Render(clock.FrameTime{})
	if display.MatrixEquals(first, settled) {
		t.Fatal("the view did not move toward the focused button")
	}
	m.Update(clock.FrameTime{ElapsedMs: 33.3})
	if m.IsDirty() {
		t.Fatal("a settled menu must not keep repainting")
	}
}

// newTestMenu builds a Menu with n buttons labeled "0".."n-1".
func newTestMenu(n int) *Menu {
	buttons := make([]*Button, n)
	for i := range buttons {
		buttons[i] = NewButton(ButtonOptions{Label: fmt.Sprintf("%d", i)})
	}
	return NewMenu(MenuOptions{Buttons: buttons})
}

// selIndex returns the currently focused button's position in m.buttons.
func selIndex(m *Menu) int {
	sel := m.CurrentSelection()
	for i, b := range m.buttons {
		if b == sel {
			return i
		}
	}
	return -1
}

// tick calls Update n times with a fixed ElapsedMs (~33.3ms is 30fps).
func tick(m *Menu, elapsedMs float64, n int) {
	for i := 0; i < n; i++ {
		m.Update(clock.FrameTime{ElapsedMs: elapsedMs})
	}
}

// TestMenuControllerHeldDirectionAutoRepeats: a joystickchange followed only by
// Update ticks keeps advancing the selection.
func TestMenuControllerHeldDirectionAutoRepeats(t *testing.T) {
	m := newTestMenu(10)
	m.HandleInput(input.Event{Kind: input.KindJoystickChange, Position: input.JoyDown})
	if got := selIndex(m); got != 0 {
		t.Fatalf("a joystickchange alone (no keypress) must not itself move focus — that's the paired synthetic keypress's job on real hardware; got index %d", got)
	}

	// Under the delay: no step yet (~200ms of a 220ms delay).
	tick(m, 33.3, 6)
	if got := selIndex(m); got != 0 {
		t.Fatalf("stepped before menuHoldRepeatDelayMs elapsed: index %d after ~200ms of hold", got)
	}

	// Past the delay: exactly one step so far.
	tick(m, 33.3, 3) // ~300ms total
	afterFirst := selIndex(m)
	if afterFirst < 1 {
		t.Fatalf("no auto-step after the hold delay: index %d after ~300ms of hold", afterFirst)
	}

	// continued repeat, not a single extra step
	tick(m, 33.3, 15) // ~800ms more, ~1.1s total
	afterMore := selIndex(m)
	if afterMore <= afterFirst {
		t.Fatalf("held direction stopped repeating: index %d then %d — want continued advance from ticking alone", afterFirst, afterMore)
	}
	if afterMore >= len(m.buttons)-1 {
		t.Fatalf("clamped at the bottom (index %d) too early for the cadence check above to be meaningful", afterMore)
	}
}

// TestMenuJoystickReleaseStopsRepeat: the centering joystickchange ends the hold.
func TestMenuJoystickReleaseStopsRepeat(t *testing.T) {
	m := newTestMenu(10)
	m.HandleInput(input.Event{Kind: input.KindJoystickChange, Position: input.JoyDown})
	tick(m, 33.3, 10) // past the delay: at least one step
	held := selIndex(m)
	if held < 1 {
		t.Fatalf("expected at least one auto-step before release, got index %d", held)
	}

	m.HandleInput(input.Event{Kind: input.KindJoystickChange, Position: input.JoyCenter})
	tick(m, 33.3, 30) // ~1s more with the stick centered
	if got := selIndex(m); got != held {
		t.Fatalf("selection kept advancing after release: %d -> %d", held, got)
	}
}

func TestMenuKeypressEdgeIsNotDoubleStepped(t *testing.T) {
	m := newTestMenu(10)
	m.HandleInput(input.Event{Kind: input.KindJoystickChange, Position: input.JoyDown})
	m.HandleInput(input.Event{Kind: input.KindKeypress, Name: input.KeyDown})
	if got := selIndex(m); got != 1 {
		t.Fatalf("press edge should move exactly one step; got index %d", got)
	}
	tick(m, 1, 1)
	if got := selIndex(m); got != 1 {
		t.Fatalf("arming the hold added an extra step: index %d", got)
	}
}

func TestMenuKeyboardNeverArmsHold(t *testing.T) {
	m := newTestMenu(10)
	kbd := input.Event{Kind: input.KindKeypress, Name: input.KeyDown, Source: input.PlayerSource{Type: input.SourceKeyboard}}

	m.HandleInput(kbd)
	if got := selIndex(m); got != 1 {
		t.Fatalf("first keypress: got index %d, want 1", got)
	}

	tick(m, 33.3, 60) // ~2s idle, no new keypress
	if got := selIndex(m); got != 1 {
		t.Fatalf("Update alone advanced the keyboard selection to %d with no new keypress", got)
	}

	m.HandleInput(kbd)
	if got := selIndex(m); got != 2 {
		t.Fatalf("second keypress: got index %d, want exactly 2 (one step per keypress, no double-step)", got)
	}
}

// tickFrom calls Update n times as consecutive engine ticks starting at from,
// and returns the tick after the last.
func tickFrom(m *Menu, from uint64, n int) uint64 {
	for i := 0; i < n; i++ {
		m.Update(clock.FrameTime{Ticks: from, ElapsedMs: 33.3})
		from++
	}
	return from
}

func TestMenuDropsAHoldItWasAwayFor(t *testing.T) {
	m := newTestMenu(10)
	m.HandleInput(input.Event{Kind: input.KindJoystickChange, Position: input.JoyDown})
	next := tickFrom(m, 1, 10)
	held := selIndex(m)
	if held < 1 {
		t.Fatalf("expected at least one auto-step, got index %d", held)
	}

	// The stick is released while another scene owns input, then the menu
	// comes back a hundred ticks later.
	tickFrom(m, next+100, 30)
	if got := selIndex(m); got != held {
		t.Fatalf("a hold from before the menu was away kept repeating: %d -> %d", held, got)
	}
}

func TestMenuKeepsAHoldArmedOnItsReturnTick(t *testing.T) {
	m := newTestMenu(10)
	next := tickFrom(m, 1, 3)
	m.HandleInput(input.Event{Kind: input.KindJoystickChange, Position: input.JoyDown})
	tickFrom(m, next+100, 30)
	if got := selIndex(m); got < 1 {
		t.Fatalf("a stick pushed on the tick the menu came back did not repeat: index %d", got)
	}
}

// TestMenuStepsOncePerPadPush feeds real pad reports through the controller
// decoder: the push's joystick change and its keypress together make one step,
// and holding repeats.
func TestMenuStepsOncePerPadPush(t *testing.T) {
	m := newTestMenu(10)
	d := controller.NewDecoder(controller.Speedlink.Spec)
	feed := func(x, y byte) {
		for _, ev := range d.Feed([]byte{0, x, y}) {
			m.HandleInput(ev)
		}
	}
	feed(128, 128)
	feed(128, 255)
	if got := selIndex(m); got != 1 {
		t.Fatalf("one stick push moved focus to %d, want 1", got)
	}
	feed(128, 128)
	next := tickFrom(m, 1, 30)
	if got := selIndex(m); got != 1 {
		t.Fatalf("a released push kept stepping: index %d", got)
	}

	feed(128, 255)
	tickFrom(m, next, 30)
	if got := selIndex(m); got < 3 {
		t.Fatalf("a held push did not repeat: index %d", got)
	}
}
