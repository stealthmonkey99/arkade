// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// PlayerStrategy filters controller input and orients the display for the
// active player(s). The Runtime keeps a stack; the top strategy sees every
// controller-sourced event before dispatch.
type PlayerStrategy interface {
	// Init runs when the strategy becomes active (pushed, or re-exposed by
	// a pop). Implementations track their own first-time state.
	Init()
	// HandleInputSource observes the source of an allowed event (used to
	// adopt the active controller).
	HandleInputSource(src input.PlayerSource)
	// AllowInput reports whether an event from src may dispatch.
	AllowInput(src input.PlayerSource) bool
	// Lock/Unlock toggle enforcement (games lock once play starts).
	Lock()
	Unlock()
}

// strategyStack is the player strategies in force, AnyPlayer at the bottom;
// the top one gates every controller event. Whichever strategy becomes the
// top, by a push or by the pop above it, is re-initialised.
type strategyStack []PlayerStrategy

func (s *strategyStack) push(st PlayerStrategy) {
	*s = append(*s, st)
	st.Init()
}

func (s *strategyStack) pop() {
	if len(*s) == 0 {
		return
	}
	*s = (*s)[:len(*s)-1]
	if top := s.top(); top != nil {
		top.Init()
	}
}

func (s strategyStack) top() PlayerStrategy {
	if len(s) == 0 {
		return nil
	}
	return s[len(s)-1]
}

// PushPlayerStrategy activates a strategy on top of the stack.
func (rt *Runtime) PushPlayerStrategy(s PlayerStrategy) { rt.strategies.push(s) }

// PopPlayerStrategy removes the top strategy, re-initializing the one beneath.
func (rt *Runtime) PopPlayerStrategy() { rt.strategies.pop() }

// SwitchPlayer forwards to a TwoPlayerTurnBased strategy on top of the stack.
func (rt *Runtime) SwitchPlayer(toPrimary ...bool) {
	if s, ok := rt.strategies.top().(*TwoPlayerTurnBasedStrategy); ok {
		s.SwitchPlayer(toPrimary...)
	}
}

// LockPlayerToSeat forwards to a OnePlayerStrategy on top of the stack (see
// LockToSeat). A no-op for any other strategy.
func (rt *Runtime) LockPlayerToSeat(seat input.Seat) {
	if s, ok := rt.strategies.top().(*OnePlayerStrategy); ok {
		s.LockToSeat(seat)
	}
}

// isQuitKey reports whether a key is the leave-this-app gesture. B is never a
// gameplay action (D = interact), so gateInput can exempt it for every app.
func isQuitKey(k input.KeyName) bool {
	return k == input.KeyJoyB || k == input.KeyQuit || k == input.KeyEscape
}

// gateInput applies the top strategy to a controller-sourced event before
// dispatch: returns false when the event must be dropped. Keyboard sources
// always pass.
//
// Quit keys are never gated, so either seat can leave a locked 1P game. The
// event still reaches the cartridge, which decides what to do with it.
// HandleInputSource is skipped for a gated-out seat's quit so that seat is
// not adopted as primary.
func (rt *Runtime) gateInput(ev input.Event) bool {
	s := rt.strategies.top()
	if s == nil || ev.Source.Type == input.SourceKeyboard {
		return true
	}
	if !s.AllowInput(ev.Source) {
		// All three key kinds: a hold-B confirm arms on keydown and disarms on
		// keyup, so passing only keypress would let it open but never complete.
		switch ev.Kind {
		case input.KindKeypress, input.KindKeydown, input.KindKeyup:
			if isQuitKey(ev.Name) {
				return true
			}
		}
		rt.Log("debug", fmt.Sprintf("input from %s prevented by player strategy", ev.Source.ID))
		return false
	}
	s.HandleInputSource(ev.Source)
	return true
}

// sameSeat reports whether two sources are the same player, meaning the same
// seat rather than the same device: the bolted-in stick and a wireless pad
// assigned to a seat are both that player, whatever their slots.
func sameSeat(a, b input.PlayerSource) bool { return a.Seat == b.Seat }

// AnyPlayerStrategy accepts everything and resets the display upright; it
// is the base of the stack.
type AnyPlayerStrategy struct {
	rt *Runtime
}

// NewAnyPlayer builds the base strategy.
func NewAnyPlayer(rt *Runtime) *AnyPlayerStrategy { return &AnyPlayerStrategy{rt: rt} }

func (s *AnyPlayerStrategy) Init()                                { s.rt.Renderer.VirtualRotate(display.Upright) }
func (s *AnyPlayerStrategy) HandleInputSource(input.PlayerSource) {}
func (s *AnyPlayerStrategy) AllowInput(input.PlayerSource) bool   { return true }
func (s *AnyPlayerStrategy) Lock()                                {}
func (s *AnyPlayerStrategy) Unlock()                              {}

// OnePlayerStrategy adopts whichever controller plays and, in Solo mode,
// rotates the display toward that player's side of the table once locked.
type OnePlayerStrategy struct {
	rt                  *Runtime
	rotateForPlayer     bool
	created             bool
	locked              bool
	controllerConfirmed bool
	primary             *input.PlayerSource
}

// OnePlayerSolo rotates the display toward the confirmed player.
func OnePlayerSolo(rt *Runtime) *OnePlayerStrategy {
	return &OnePlayerStrategy{rt: rt, rotateForPlayer: true}
}

// OnePlayerAgainstAI keeps the display upright.
func OnePlayerAgainstAI(rt *Runtime) *OnePlayerStrategy {
	return &OnePlayerStrategy{rt: rt}
}

func (s *OnePlayerStrategy) Init() {
	firstTime := !s.created
	s.created = true
	if firstTime {
		return
	}
	s.rotateForPrimary()
}

func (s *OnePlayerStrategy) rotateForPrimary() {
	if !s.rotateForPlayer || !s.controllerConfirmed {
		return
	}
	orient := display.Upright
	if s.locked && s.primary != nil {
		switch s.primary.Seat {
		case input.SeatP1:
			orient = display.CounterClockwise
		case input.SeatP2:
			orient = display.Clockwise
		}
	}
	s.rt.Renderer.VirtualRotate(orient)
}

func (s *OnePlayerStrategy) HandleInputSource(src input.PlayerSource) {
	if !s.locked && src.Type == input.SourceController && (s.primary == nil || !sameSeat(*s.primary, src)) {
		s.controllerConfirmed = true
		s.primary = &src
		s.rotateForPrimary()
	}
}

func (s *OnePlayerStrategy) AllowInput(src input.PlayerSource) bool {
	return !s.locked || (s.primary != nil && sameSeat(*s.primary, src))
}

func (s *OnePlayerStrategy) Lock() {
	s.locked = true
	s.rotateForPrimary()
}

// LockToSeat locks display rotation and input to a cabinet seat, whichever
// controller pressed first. The seat is the whole lock; no controller is
// looked up.
func (s *OnePlayerStrategy) LockToSeat(seat input.Seat) {
	s.controllerConfirmed = true
	s.primary = &input.PlayerSource{Type: input.SourceController, Seat: seat}
	s.locked = true
	s.rotateForPrimary()
}

func (s *OnePlayerStrategy) Unlock() {
	s.locked = false
	s.rotateForPrimary()
}

// TwoPlayerTurnBasedStrategy alternates input between the primary
// controller and everyone else; games call SwitchPlayer on turn change and
// Lock once play starts.
type TwoPlayerTurnBasedStrategy struct {
	rt          *Runtime
	created     bool
	locked      bool
	primary     *input.PlayerSource
	primaryTurn bool
}

// NewTwoPlayerTurnBased builds the turn-based strategy.
func NewTwoPlayerTurnBased(rt *Runtime) *TwoPlayerTurnBasedStrategy {
	return &TwoPlayerTurnBasedStrategy{rt: rt}
}

func (s *TwoPlayerTurnBasedStrategy) Init() {
	firstTime := !s.created
	s.created = true
	if !firstTime {
		return
	}
	// P1 takes the first turn; the seat is the player, not a device.
	s.adopt(input.PlayerSource{Type: input.SourceController, Seat: input.SeatP1})
}

func (s *TwoPlayerTurnBasedStrategy) adopt(src input.PlayerSource) {
	s.primary = &src
	s.primaryTurn = true
}

func (s *TwoPlayerTurnBasedStrategy) HandleInputSource(src input.PlayerSource) {
	if !s.locked && src.Type == input.SourceController && (s.primary == nil || !sameSeat(*s.primary, src)) {
		s.adopt(src)
	}
}

func (s *TwoPlayerTurnBasedStrategy) AllowInput(src input.PlayerSource) bool {
	if !s.locked || s.primary == nil {
		return true
	}
	if s.primaryTurn {
		return sameSeat(*s.primary, src)
	}
	return !sameSeat(*s.primary, src)
}

// SwitchPlayer flips (or sets) whose turn it is.
func (s *TwoPlayerTurnBasedStrategy) SwitchPlayer(toPrimary ...bool) {
	if len(toPrimary) > 0 {
		s.primaryTurn = toPrimary[0]
		return
	}
	s.primaryTurn = !s.primaryTurn
}

func (s *TwoPlayerTurnBasedStrategy) Lock()   { s.locked = true }
func (s *TwoPlayerTurnBasedStrategy) Unlock() { s.locked = false }
