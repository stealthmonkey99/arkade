// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scores"
)

// Mode selects what an EngineHost lets the cartridge change beyond its own
// storage and display.
type Mode uint8

const (
	// ModePlay is a launched game: every host import is live.
	ModePlay Mode = iota
	// ModeAttract is a screensaver slot with no player: scores and recent
	// names are read-only, and the strategy, raw-direction, exit and
	// reject-key calls are inert.
	ModeAttract
)

// engineControl is a launched game's Control.
type engineControl struct {
	ctx *engine.AppContext
	// strategies are this cartridge's pushes, innermost last; AppContext does
	// not retain the values Lock and Unlock are called on.
	strategies []engine.PlayerStrategy
}

func (c *engineControl) PushPlayerStrategy(kind StrategyKind) {
	var s engine.PlayerStrategy
	switch kind {
	case StrategyAnyPlayer:
		s = engine.NewAnyPlayer(c.ctx.RT)
	case StrategyOnePlayerSolo:
		s = engine.OnePlayerSolo(c.ctx.RT)
	case StrategyOnePlayerAgainstAI:
		s = engine.OnePlayerAgainstAI(c.ctx.RT)
	case StrategyTwoPlayerTurnBased:
		s = engine.NewTwoPlayerTurnBased(c.ctx.RT)
	default:
		return
	}
	c.strategies = append(c.strategies, s)
	c.ctx.PushPlayerStrategy(s)
}

// PopPlayerStrategy does nothing when this cartridge pushed no strategy.
func (c *engineControl) PopPlayerStrategy() {
	if len(c.strategies) == 0 {
		return
	}
	c.strategies = c.strategies[:len(c.strategies)-1]
	c.ctx.PopPlayerStrategy()
}

// LockPlayerStrategy commits the top strategy to the controller that adopted it.
func (c *engineControl) LockPlayerStrategy() {
	if s := c.topStrategy(); s != nil {
		s.Lock()
	}
}

func (c *engineControl) UnlockPlayerStrategy() {
	if s := c.topStrategy(); s != nil {
		s.Unlock()
	}
}

func (c *engineControl) topStrategy() engine.PlayerStrategy {
	if len(c.strategies) == 0 {
		return nil
	}
	return c.strategies[len(c.strategies)-1]
}

// SwitchPlayer flips or sets the turn through the Runtime, which ignores it
// under any strategy but TwoPlayerTurnBased.
func (c *engineControl) SwitchPlayer(mode SwitchMode) {
	switch mode {
	case SwitchToggle:
		c.ctx.SwitchPlayer()
	case SwitchToPrimary:
		c.ctx.SwitchPlayer(true)
	case SwitchToSecondary:
		c.ctx.SwitchPlayer(false)
	}
}

func (c *engineControl) LockPlayerToSeat(physical int) {
	c.ctx.RT.LockPlayerToSeat(input.SeatOf(physical))
}

// SetRawDirections sets the Runtime's current-app override, which resets on
// the next app load.
func (c *engineControl) SetRawDirections(enabled bool) {
	c.ctx.RT.SetRawDirectionsOverride(enabled)
}

func (c *engineControl) RejectKey() { c.ctx.RejectKey() }

// Exit forwards to AppContext.Exit, which only queues the unload for the
// next Tick.
func (c *engineControl) Exit() { c.ctx.Exit() }

// noControl is an attract slot's Control: it has no player to gate and no
// session to leave.
type noControl struct{}

func (noControl) PushPlayerStrategy(StrategyKind) {}
func (noControl) PopPlayerStrategy()              {}
func (noControl) LockPlayerStrategy()             {}
func (noControl) UnlockPlayerStrategy()           {}
func (noControl) SwitchPlayer(SwitchMode)         {}
func (noControl) LockPlayerToSeat(int)            {}
func (noControl) SetRawDirections(bool)           {}
func (noControl) RejectKey()                      {}
func (noControl) Exit()                           {}

// engineScoreboard is the table scoped to slug. No import takes a slug, so a
// cartridge cannot name another game's table.
type engineScoreboard struct {
	ctx  *engine.AppContext
	slug string
	log  Logger
}

// table is re-read on every call rather than cached: the calls come at human
// speed, and the shell and attract read the same table independently.
func (b engineScoreboard) table() scores.Table {
	return scores.Load(b.ctx.RT.Store, b.slug)
}

// ScoreSubmit inserts a qualifying score and returns its 1-based rank, or 0
// when it did not place.
func (b engineScoreboard) ScoreSubmit(score int, name string) int {
	t := b.table()
	if !t.Qualifies(score) {
		return 0
	}
	rank := t.Insert(score, name)
	if err := scores.Save(b.ctx.RT.Store, b.slug, t); err != nil {
		hostWarnf(b.log, "score submit: %v", err)
		return 0
	}
	return rank
}

// ScoreRead returns the entry at a 1-based rank, ok=false past the end.
func (b engineScoreboard) ScoreRead(rank int) (int, string, bool) {
	t := b.table()
	if rank < 1 || rank > len(t.Entries) {
		return 0, "", false
	}
	e := t.Entries[rank-1]
	return e.Score, e.Name, true
}

func (b engineScoreboard) ScoreCount() int { return len(b.table().Entries) }

func (b engineScoreboard) ScoreQualifies(score int) bool { return b.table().Qualifies(score) }

func (b engineScoreboard) ScoreRank(score int) int { return b.table().Rank(score) }

// RecentName returns the name physical seat player submitted within the
// engine's TTL.
func (b engineScoreboard) RecentName(player int) (string, bool) {
	return b.ctx.RT.RecentName(player)
}

// RecentNameAt returns the index-th name, newest first, in the physical
// seat's persistent MRU, or ok=false past the end.
func (b engineScoreboard) RecentNameAt(player, index int) (string, bool) {
	return b.ctx.RT.RecentNameAt(player, index)
}

func (b engineScoreboard) NoteRecentName(player int, name string) {
	b.ctx.RT.NoteRecentName(player, name)
}

// readOnlyScoreboard is an attract slot's Scoreboard: it reads the game's
// table and names but never writes either.
type readOnlyScoreboard struct{ engineScoreboard }

func (readOnlyScoreboard) ScoreSubmit(int, string) int { return 0 }
func (readOnlyScoreboard) NoteRecentName(int, string)  {}
