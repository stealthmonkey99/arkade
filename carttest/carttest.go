// SPDX-License-Identifier: GPL-2.0-or-later

// Package carttest is the host-side test harness for wasm cartridges. It
// re-exports the Cartridge runtime, the wire-record vocabulary and the display
// types, and provides a scripted FakeHost, so a cartridge module's golden
// tests can drive its checked-in .wasm the way the cabinet would.
//
// It is the public test harness for cartridge authors: a cartridge module
// imports it from its tests by requiring github.com/stealthmonkey99/arkade,
// which brings the engine's own dependencies (wazero, beep) into the
// cartridge's test graph but never into its TinyGo build.
package carttest

import (
	"context"

	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// Re-exported runtime types.
type (
	Cartridge = wasm.Cartridge
	Host      = wasm.Host
	Options   = wasm.Options

	InputRecord = wasm.InputRecord

	LogLevel     = wasm.LogLevel
	StrategyKind = wasm.StrategyKind
	SwitchMode   = wasm.SwitchMode

	// ChannelKind is host_input_bind's kind argument; ChannelSource is the
	// optional Host interface for input beyond the cabinet's own panel.
	ChannelKind   = wasm.ChannelKind
	ChannelSource = wasm.ChannelSource
)

// The channel kinds, in wire numbering.
const (
	ChannelButton = wasm.ChannelButton
	ChannelAxis   = wasm.ChannelAxis
	ChannelDelta  = wasm.ChannelDelta
	ChannelBlob   = wasm.ChannelBlob
)

// SupportedABIVersion is the newest cart_abi_version the host implements and
// MinSupportedABIVersion the oldest it still mounts.
const (
	SupportedABIVersion    = wasm.SupportedABIVersion
	MinSupportedABIVersion = wasm.MinSupportedABIVersion
)

// ErrABIMismatch is returned when a cartridge's ABI version is unsupported.
var ErrABIMismatch = wasm.ErrABIMismatch

// NewCartridge instantiates wasmBytes against host.
func NewCartridge(ctx context.Context, wasmBytes []byte, host Host, opts Options) (*Cartridge, error) {
	return wasm.NewCartridge(ctx, wasmBytes, host, opts)
}

// ExportsAttract reports whether the module bytes export cart_attract.
func ExportsAttract(ctx context.Context, wasmBytes []byte) bool {
	return wasm.ExportsAttract(ctx, wasmBytes)
}

// StorageKeyOK reports whether the cabinet host accepts key for storage; the
// host silently drops writes with an invalid key.
func StorageKeyOK(key string) bool { return wasm.StorageKeyOK(key) }

// Wire-record vocabulary.
const (
	RecordKindKeydown        = wasm.RecordKindKeydown
	RecordKindKeyup          = wasm.RecordKindKeyup
	RecordKindKeypress       = wasm.RecordKindKeypress
	RecordKindJoystickChange = wasm.RecordKindJoystickChange

	RecordSourceKeyboard   = wasm.RecordSourceKeyboard
	RecordSourceController = wasm.RecordSourceController

	RecordFlagVirtual = wasm.RecordFlagVirtual
	RecordFlagShift   = wasm.RecordFlagShift

	SeatNone = wasm.SeatNone
)

// PackRecords encodes input records into their wire form.
func PackRecords(records []InputRecord) []byte { return wasm.PackRecords(records) }

// UnpackRecords decodes the wire form produced by PackRecords.
func UnpackRecords(buf []byte) []InputRecord { return wasm.UnpackRecords(buf) }

// Host argument enums.
const (
	LogDebug = wasm.LogDebug
	LogInfo  = wasm.LogInfo
	LogWarn  = wasm.LogWarn
	LogError = wasm.LogError

	StrategyAnyPlayer          = wasm.StrategyAnyPlayer
	StrategyOnePlayerSolo      = wasm.StrategyOnePlayerSolo
	StrategyOnePlayerAgainstAI = wasm.StrategyOnePlayerAgainstAI
	StrategyTwoPlayerTurnBased = wasm.StrategyTwoPlayerTurnBased

	SwitchToggle      = wasm.SwitchToggle
	SwitchToPrimary   = wasm.SwitchToPrimary
	SwitchToSecondary = wasm.SwitchToSecondary
)

// Display types golden tests assert against.
type (
	Pixel  = display.Pixel
	Matrix = display.Matrix
)

// DefaultWidth and DefaultHeight are the cabinet LED matrix dimensions.
const (
	DefaultWidth  = display.DefaultWidth
	DefaultHeight = display.DefaultHeight
)

// Named colors golden tests reference.
var (
	Black    = display.Black
	White    = display.White
	Red      = display.Red
	Green    = display.Green
	DarkGray = display.DarkGray
)
