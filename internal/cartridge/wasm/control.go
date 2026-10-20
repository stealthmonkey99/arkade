// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// StrategyKind is host_push_player_strategy's kind argument. Display rotation
// is implied by the kind, so a cartridge cannot build a strategy the engine
// never offers.
type StrategyKind uint32

const (
	StrategyAnyPlayer          StrategyKind = 0 // everyone plays; base of the stack
	StrategyOnePlayerSolo      StrategyKind = 1 // adopt one controller, rotate toward them
	StrategyOnePlayerAgainstAI StrategyKind = 2 // adopt one controller, stay upright
	StrategyTwoPlayerTurnBased StrategyKind = 3 // alternate; drive with SwitchPlayer
)

func (k StrategyKind) String() string {
	switch k {
	case StrategyAnyPlayer:
		return "any"
	case StrategyOnePlayerSolo:
		return "one-player-solo"
	case StrategyOnePlayerAgainstAI:
		return "one-player-against-ai"
	case StrategyTwoPlayerTurnBased:
		return "two-player-turn-based"
	default:
		return "unknown"
	}
}

// SwitchMode is host_switch_player's argument, spelling out the engine's
// variadic SwitchPlayer.
type SwitchMode uint32

const (
	SwitchToggle      SwitchMode = 0
	SwitchToPrimary   SwitchMode = 1
	SwitchToSecondary SwitchMode = 2
)

// maxStrategyDepth caps pushes; each one runs an Init that rotates the display.
const maxStrategyDepth = 8

type controlImports struct {
	host interface {
		Control
		Logger
	}
	strategyDepth int
}

func (c *controlImports) pushStrategy(_ context.Context, _ api.Module, kind uint32) {
	switch k := StrategyKind(kind); k {
	case StrategyAnyPlayer, StrategyOnePlayerSolo, StrategyOnePlayerAgainstAI, StrategyTwoPlayerTurnBased:
		if c.strategyDepth >= maxStrategyDepth {
			hostWarnf(c.host, "push_player_strategy: stack depth %d reached, ignoring", maxStrategyDepth)
			return
		}
		c.strategyDepth++
		c.host.PushPlayerStrategy(k)
	default:
		hostWarnf(c.host, "push_player_strategy: unknown kind %d", kind)
	}
}

func (c *controlImports) popStrategy(_ context.Context, _ api.Module) {
	if c.strategyDepth > 0 {
		c.strategyDepth--
	}
	c.host.PopPlayerStrategy()
}

func (c *controlImports) lockStrategy(_ context.Context, _ api.Module) {
	c.host.LockPlayerStrategy()
}

func (c *controlImports) unlockStrategy(_ context.Context, _ api.Module) {
	c.host.UnlockPlayerStrategy()
}

func (c *controlImports) switchPlayer(_ context.Context, _ api.Module, mode uint32) {
	switch m := SwitchMode(mode); m {
	case SwitchToggle, SwitchToPrimary, SwitchToSecondary:
		c.host.SwitchPlayer(m)
	default:
		hostWarnf(c.host, "switch_player: unknown mode %d", mode)
	}
}

func (c *controlImports) lockSeat(_ context.Context, _ api.Module, physical int32) {
	if seat, ok := physicalSeatArg(c.host, "host_lock_player_seat: physical", physical); ok {
		c.host.LockPlayerToSeat(seat)
	}
}

func (c *controlImports) setRawDirections(_ context.Context, _ api.Module, enabled uint32) {
	c.host.SetRawDirections(enabled != 0)
}

func (c *controlImports) rejectKey(_ context.Context, _ api.Module) {
	c.host.RejectKey()
}

func (c *controlImports) exit(_ context.Context, _ api.Module) {
	c.host.Exit()
}
