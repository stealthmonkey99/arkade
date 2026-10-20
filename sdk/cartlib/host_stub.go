// SPDX-License-Identifier: GPL-2.0-or-later

//go:build !tinygo

package cartlib

// HostABI is the cabinet's half of the ABI as Go values: one method per
// "arkade" import, with the import's wire types (a seat is the int32 the
// host receives, -1 for both) and Go slices and strings where the import
// takes a pointer and length. It exists under the plain Go toolchain only,
// where no wasm host is present: cartlib's wrappers call whatever [UseHost]
// installed, so a cartridge's own `go test` can drive the Game against an
// in-process fake. sdk/cartlib/hosttest provides that fake; a cartridge
// test uses it rather than implementing HostABI itself.
//
// Each method's contract is its import's, in docs/CARTRIDGE.md; the fake's
// job is to answer as the cabinet would.
type HostABI interface {
	// Blit is host_blit: it presents pix, FBWidth*FBHeight*4 bytes of row-major RGBA, as the frame.
	Blit(pix []byte)

	// LoadSound is host_load_sound: it returns the sound's handle, or 0 on failure.
	LoadSound(path string) uint32

	// PlaySound is host_play_sound: it plays handle id loopCount times in total; an unknown id is a silent no-op.
	PlaySound(id, loopCount uint32)

	// StopSound is host_stop_sound: it stops handle id; an unknown id is a no-op.
	StopSound(id uint32)

	// StopAllSounds is host_stop_all_sounds: it stops every sound the cartridge is playing.
	StopAllSounds()

	// StorageRead is host_storage_read: it copies the value under key into out and returns the byte count, 0 for an absent or illegal key, truncated to len(out).
	StorageRead(key string, out []byte) uint32

	// StorageWrite is host_storage_write: it stores val under key; the host drops a write whose key is illegal or val oversized, silently.
	StorageWrite(key string, val []byte)

	// NowMicros is host_now_micros: it returns the engine clock in microseconds, not wall time.
	NowMicros() uint64

	// RandomU64 is host_random_u64: it returns the next value of the engine-seeded RNG.
	RandomU64() uint64

	// Log is host_log: it records msg at the given LogLevel value.
	Log(level uint32, msg string)

	// SetText is host_set_text: it sets both LCD lines for player, -1 meaning both seats; the host copies the bytes before returning.
	SetText(player int32, line1, line2 []byte)

	// SetButtonLights is host_set_button_lights: it turns on the lamps in mask for player (-1 for both); a non-zero only turns every other lamp off, and the call stops any lamp sequence on those seats.
	SetButtonLights(player int32, mask uint32, only int32)

	// SetLights is host_set_lights: it turns all six lamps of player (-1 for both) on for a non-zero on, off for zero, and stops any lamp sequence on those seats.
	SetLights(player, on int32)

	// DefineLcdGlyph is host_define_lcd_glyph: it defines codepoint with the packed rows and returns non-zero if accepted, 0 if refused.
	DefineLcdGlyph(codepoint int32, rows uint64) uint32

	// Exit is host_exit: it requests the cartridge's unload, applied between ticks.
	Exit()

	// RejectKey is host_reject_key: it rings the "that key does nothing" bell, a no-op where there is none.
	RejectKey()

	// ScoreSubmit is host_score_submit: it records score and name in the top-10 table and returns the 1-based rank, or 0 if it did not place.
	ScoreSubmit(score int64, name string) uint32

	// ScoreRead is host_score_read: it writes the entry at 1-based rank into out (i64 little-endian score, then the name bytes) and returns the byte count, 0 past the end of the table.
	ScoreRead(rank uint32, out []byte) uint32

	// ScoreCount is host_score_count: it returns how many entries the table holds, 0 to 10.
	ScoreCount() uint32

	// ScoreQualifies is host_score_qualifies: it returns non-zero if score would place, by ScoreSubmit's rule.
	ScoreQualifies(score int64) uint32

	// ScoreRank is host_score_rank: it returns the 1-based rank score would take, or 0 if it would not place, writing nothing.
	ScoreRank(score int64) uint32

	// RecentNameGet is host_recent_name_get: it copies the name last noted for player (0 or 1) within the past hour into out and returns its length, 0 if none.
	RecentNameGet(player int32, out []byte) uint32

	// RecentNameAt is host_recent_name_at: it copies the index-th newest name of player's (0 or 1) history into out and returns its length, 0 past the end.
	RecentNameAt(player int32, index uint32, out []byte) uint32

	// RecentNameNote is host_recent_name_note: it records name as the one player (0 or 1) just confirmed; the host trims blanks and ignores an empty name.
	RecentNameNote(player int32, name string)

	// PushPlayerStrategy is host_push_player_strategy: it pushes the strategy of the given StrategyKind value.
	PushPlayerStrategy(kind uint32)

	// PopPlayerStrategy is host_pop_player_strategy: it removes the strategy this cartridge most recently pushed.
	PopPlayerStrategy()

	// LockPlayerStrategy is host_lock_player_strategy: it enforces the current strategy.
	LockPlayerStrategy()

	// UnlockPlayerStrategy is host_unlock_player_strategy: it relaxes the current strategy.
	UnlockPlayerStrategy()

	// SwitchPlayer is host_switch_player: it toggles or sets the turn by SwitchMode value; a no-op outside the two-player turn-based strategy.
	SwitchPlayer(mode uint32)

	// LockPlayerSeat is host_lock_player_seat: it locks the current one-player strategy to the physical seat 0 or 1; a no-op under any other strategy.
	LockPlayerSeat(physical int32)

	// SetRawDirections is host_set_raw_directions: a non-zero enabled turns the direction remap off for this instance, zero turns it on.
	SetRawDirections(enabled uint32)

	// LoadSheet is host_load_sheet: it returns the sheet's handle, or 0 on failure.
	LoadSheet(path string) uint32

	// SheetCel is host_sheet_cel: it returns the handle of the named cel of sheet, or 0 if there is none.
	SheetCel(sheet uint32, name string) uint32

	// CelInfo is host_cel_info: it writes the cel's four little-endian u16s (w, h, anchorX, anchorY) into out and returns non-zero, or 0 for a bad handle.
	CelInfo(cel uint32, out []byte) uint32

	// CelRead is host_cel_read: it copies the cel's straight-alpha RGBA into rgba and returns the byte count, which is w*h*4 on success and 0 on failure.
	CelRead(cel uint32, rgba []byte) uint32

	// InputBind is host_input_bind: it binds the named channel as the given channel kind (0 button, 1 axis, 2 delta, 3 blob) and returns its handle, or a negative value on failure.
	InputBind(name string, kind uint32) int32

	// InputButton is host_input_button: it returns non-zero while the bound button channel is held.
	InputButton(handle int32) uint32

	// InputAxis is host_input_axis: it returns the bound axis channel's position.
	InputAxis(handle int32) int32

	// InputDelta is host_input_delta: it returns the bound delta channel's motion since the previous update, the same value for every read within a tick.
	InputDelta(handle int32) int32

	// InputBlob is host_input_blob: it copies the bound blob channel into dst and returns the byte count, 0 if dst is shorter than the channel's declared length.
	InputBlob(handle int32, dst []byte) uint32

	// OutputBind is host_output_bind: it returns an output channel's handle, or a negative value on failure, which is every current host's answer.
	OutputBind(name string) int32

	// OutputSet is host_output_set: it drives the bound output channel.
	OutputSet(handle, value int32)

	// LightsPlay is host_lights_play: it starts the lamp sequence at path on seat (-1 for both) with the given LightsLoop value and returns non-zero if anything is now playing.
	LightsPlay(path string, seat int32, loop uint32) uint32

	// LightsStop is host_lights_stop: it stops the lamp sequence on seat (-1 for both), leaving the lamps at their levels.
	LightsStop(seat int32)

	// SetLampLevels is host_set_lamp_levels: it sets the lamps in touch on seat (-1 for both) to the packed levels (PackLampLevels), fading over fadeMs milliseconds.
	SetLampLevels(seat int32, levels, touch, fadeMs uint32)
}

var installedHost HostABI

// UseHost installs h as the host behind every cartlib call for the rest of
// the process, or until the next UseHost, and returns the host it replaced
// (nil at first). With none installed the first host call panics, so a test
// that reaches the host by accident fails loudly instead of reading zero.
// hosttest.New calls this and restores the previous host when the test
// ends.
func UseHost(h HostABI) (previous HostABI) {
	previous, installedHost = installedHost, h
	return previous
}

// seamHost forwards each wrapper call to the installed HostABI. Like
// wasmHost it is a zero-size struct, so wrappers.go reads the same under
// both toolchains.
type seamHost struct{}

var host seamHost

func abi() HostABI {
	if installedHost == nil {
		panic("cartlib: host call with no host installed (build with tinygo for a real cartridge; under `go test`, install one with hosttest.New or exercise only pure Game logic)")
	}
	return installedHost
}

func (seamHost) Blit(pix []byte)                           { abi().Blit(pix) }
func (seamHost) LoadSound(path string) uint32              { return abi().LoadSound(path) }
func (seamHost) PlaySound(id, loopCount uint32)            { abi().PlaySound(id, loopCount) }
func (seamHost) StopSound(id uint32)                       { abi().StopSound(id) }
func (seamHost) StopAllSounds()                            { abi().StopAllSounds() }
func (seamHost) StorageRead(key string, out []byte) uint32 { return abi().StorageRead(key, out) }
func (seamHost) StorageWrite(key string, val []byte)       { abi().StorageWrite(key, val) }
func (seamHost) NowMicros() uint64                         { return abi().NowMicros() }
func (seamHost) RandomU64() uint64                         { return abi().RandomU64() }
func (seamHost) Log(level uint32, msg string)              { abi().Log(level, msg) }
func (seamHost) SetText(player int32, line1, line2 []byte) { abi().SetText(player, line1, line2) }
func (seamHost) SetButtonLights(player int32, mask uint32, only int32) {
	abi().SetButtonLights(player, mask, only)
}
func (seamHost) SetLights(player, on int32) { abi().SetLights(player, on) }
func (seamHost) DefineLcdGlyph(codepoint int32, rows uint64) uint32 {
	return abi().DefineLcdGlyph(codepoint, rows)
}
func (seamHost) Exit()                                       { abi().Exit() }
func (seamHost) RejectKey()                                  { abi().RejectKey() }
func (seamHost) ScoreSubmit(score int64, name string) uint32 { return abi().ScoreSubmit(score, name) }
func (seamHost) ScoreRead(rank uint32, out []byte) uint32    { return abi().ScoreRead(rank, out) }
func (seamHost) ScoreCount() uint32                          { return abi().ScoreCount() }
func (seamHost) ScoreQualifies(score int64) uint32           { return abi().ScoreQualifies(score) }
func (seamHost) ScoreRank(score int64) uint32                { return abi().ScoreRank(score) }
func (seamHost) RecentNameGet(player int32, out []byte) uint32 {
	return abi().RecentNameGet(player, out)
}
func (seamHost) RecentNameAt(player int32, index uint32, out []byte) uint32 {
	return abi().RecentNameAt(player, index, out)
}
func (seamHost) RecentNameNote(player int32, name string)  { abi().RecentNameNote(player, name) }
func (seamHost) PushPlayerStrategy(kind uint32)            { abi().PushPlayerStrategy(kind) }
func (seamHost) PopPlayerStrategy()                        { abi().PopPlayerStrategy() }
func (seamHost) LockPlayerStrategy()                       { abi().LockPlayerStrategy() }
func (seamHost) UnlockPlayerStrategy()                     { abi().UnlockPlayerStrategy() }
func (seamHost) SwitchPlayer(mode uint32)                  { abi().SwitchPlayer(mode) }
func (seamHost) LockPlayerSeat(physical int32)             { abi().LockPlayerSeat(physical) }
func (seamHost) SetRawDirections(enabled uint32)           { abi().SetRawDirections(enabled) }
func (seamHost) LoadSheet(path string) uint32              { return abi().LoadSheet(path) }
func (seamHost) SheetCel(sheet uint32, name string) uint32 { return abi().SheetCel(sheet, name) }
func (seamHost) CelInfo(cel uint32, out []byte) uint32     { return abi().CelInfo(cel, out) }
func (seamHost) CelRead(cel uint32, rgba []byte) uint32    { return abi().CelRead(cel, rgba) }
func (seamHost) InputBind(name string, kind uint32) int32  { return abi().InputBind(name, kind) }
func (seamHost) InputButton(handle int32) uint32           { return abi().InputButton(handle) }
func (seamHost) InputAxis(handle int32) int32              { return abi().InputAxis(handle) }
func (seamHost) InputDelta(handle int32) int32             { return abi().InputDelta(handle) }
func (seamHost) InputBlob(handle int32, dst []byte) uint32 { return abi().InputBlob(handle, dst) }
func (seamHost) OutputBind(name string) int32              { return abi().OutputBind(name) }
func (seamHost) OutputSet(handle, value int32)             { abi().OutputSet(handle, value) }
func (seamHost) LightsPlay(path string, seat int32, loop uint32) uint32 {
	return abi().LightsPlay(path, seat, loop)
}
func (seamHost) LightsStop(seat int32) { abi().LightsStop(seat) }
func (seamHost) SetLampLevels(seat int32, levels, touch, fadeMs uint32) {
	abi().SetLampLevels(seat, levels, touch, fadeMs)
}

// staleSetup is what a Setup call after Init does under the plain Go
// toolchain: a panic, so a test catches the bug the cabinet would only log.
func staleSetup(msg string) { panic(msg) }

// CartInit is the cart_init body for a host-Go harness: it runs [Game.Init]
// with the live [Setup] and starts the load phase. It also resets the
// instance state a fresh cartridge starts with (the load phase, the cheat
// taint), so one test binary can boot the Game more than once.
// sdk/cartlib/hosttest drives the Cart* functions; a cartridge test should
// not need to call them itself.
func CartInit() { dispatchInit() }

// CartHandleInput is cart_handle_input for one batch of 8-byte records, as
// the host would write them into cart_alloc's buffer: it decodes them and
// applies the quit-key default before [Game.HandleInput]. A batch longer
// than the buffer is truncated, as on the cabinet.
func CartHandleInput(wire []byte) {
	n := copy(allocBuf[:], wire)
	dispatchHandleInput(allocBuf[:n])
}

// CartUpdate is cart_update: it advances the load phase, and calls
// [Game.Update] only once the load phase is over.
func CartUpdate(dtMicros uint64) { dispatchUpdate(dtMicros) }

// CartRender is cart_render: it draws the splash or [Loader] frame during
// the load phase and [Game.Render] after it, then blits the frame.
func CartRender() { dispatchRender() }

// CartTeardown is cart_teardown: it runs [Teardowner.Teardown] when the
// Game implements it.
func CartTeardown() { dispatchTeardown() }
