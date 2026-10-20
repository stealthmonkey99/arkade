// SPDX-License-Identifier: GPL-2.0-or-later

//go:build tinygo

package cartlib

// wasmHost is the host as the wrappers see it: Go values in, one wasm import
// out. It is a zero-size struct so every call is static and inlinable.
type wasmHost struct{}

var host wasmHost

func (wasmHost) Blit(pix []byte) { hostBlit(ptrOf(pix), uint32(len(pix))) }

func (wasmHost) LoadSound(path string) uint32 {
	return hostLoadSound(ptrOfString(path), uint32(len(path)))
}

func (wasmHost) PlaySound(id, loopCount uint32) { hostPlaySound(id, loopCount) }
func (wasmHost) StopSound(id uint32)            { hostStopSound(id) }
func (wasmHost) StopAllSounds()                 { hostStopAllSounds() }

func (wasmHost) StorageRead(key string, out []byte) uint32 {
	return hostStorageRead(ptrOfString(key), uint32(len(key)), ptrOf(out), uint32(len(out)))
}

func (wasmHost) StorageWrite(key string, val []byte) {
	hostStorageWrite(ptrOfString(key), uint32(len(key)), ptrOf(val), uint32(len(val)))
}

func (wasmHost) NowMicros() uint64 { return hostNowMicros() }
func (wasmHost) RandomU64() uint64 { return hostRandomU64() }

func (wasmHost) Log(level uint32, msg string) {
	hostLog(level, ptrOfString(msg), uint32(len(msg)))
}

func (wasmHost) SetText(player int32, line1, line2 []byte) {
	hostSetText(player, ptrOf(line1), uint32(len(line1)), ptrOf(line2), uint32(len(line2)))
}

func (wasmHost) SetButtonLights(player int32, mask uint32, only int32) {
	hostSetButtonLights(player, mask, only)
}

func (wasmHost) SetLights(player, on int32) { hostSetLights(player, on) }

func (wasmHost) DefineLcdGlyph(codepoint int32, rows uint64) uint32 {
	return hostDefineLcdGlyph(codepoint, rows)
}

func (wasmHost) Exit()      { hostExit() }
func (wasmHost) RejectKey() { hostRejectKey() }

func (wasmHost) ScoreSubmit(score int64, name string) uint32 {
	return hostScoreSubmit(score, ptrOfString(name), uint32(len(name)))
}

func (wasmHost) ScoreRead(rank uint32, out []byte) uint32 {
	return hostScoreRead(rank, ptrOf(out), uint32(len(out)))
}

func (wasmHost) ScoreCount() uint32                { return hostScoreCount() }
func (wasmHost) ScoreQualifies(score int64) uint32 { return hostScoreQualifies(score) }
func (wasmHost) ScoreRank(score int64) uint32      { return hostScoreRank(score) }

func (wasmHost) RecentNameGet(player int32, out []byte) uint32 {
	return hostRecentNameGet(player, ptrOf(out), uint32(len(out)))
}

func (wasmHost) RecentNameAt(player int32, index uint32, out []byte) uint32 {
	return hostRecentNameAt(player, index, ptrOf(out), uint32(len(out)))
}

func (wasmHost) RecentNameNote(player int32, name string) {
	hostRecentNameNote(player, ptrOfString(name), uint32(len(name)))
}

func (wasmHost) PushPlayerStrategy(kind uint32)  { hostPushPlayerStrategy(kind) }
func (wasmHost) PopPlayerStrategy()              { hostPopPlayerStrategy() }
func (wasmHost) LockPlayerStrategy()             { hostLockPlayerStrategy() }
func (wasmHost) UnlockPlayerStrategy()           { hostUnlockPlayerStrategy() }
func (wasmHost) SwitchPlayer(mode uint32)        { hostSwitchPlayer(mode) }
func (wasmHost) LockPlayerSeat(physical int32)   { hostLockPlayerSeat(physical) }
func (wasmHost) SetRawDirections(enabled uint32) { hostSetRawDirections(enabled) }

func (wasmHost) LoadSheet(path string) uint32 {
	return hostLoadSheet(ptrOfString(path), uint32(len(path)))
}

func (wasmHost) SheetCel(sheet uint32, name string) uint32 {
	return hostSheetCel(sheet, ptrOfString(name), uint32(len(name)))
}

func (wasmHost) CelInfo(cel uint32, out []byte) uint32 { return hostCelInfo(cel, ptrOf(out)) }

func (wasmHost) CelRead(cel uint32, rgba []byte) uint32 {
	return hostCelRead(cel, ptrOf(rgba), uint32(len(rgba)))
}

func (wasmHost) InputBind(name string, kind uint32) int32 {
	return hostInputBind(ptrOfString(name), uint32(len(name)), kind)
}

func (wasmHost) InputButton(handle int32) uint32 { return hostInputButton(handle) }
func (wasmHost) InputAxis(handle int32) int32    { return hostInputAxis(handle) }
func (wasmHost) InputDelta(handle int32) int32   { return hostInputDelta(handle) }

func (wasmHost) InputBlob(handle int32, dst []byte) uint32 {
	return hostInputBlob(handle, ptrOf(dst), uint32(len(dst)))
}

func (wasmHost) OutputBind(name string) int32 {
	return hostOutputBind(ptrOfString(name), uint32(len(name)))
}

func (wasmHost) OutputSet(handle, value int32) { hostOutputSet(handle, value) }

func (wasmHost) LightsPlay(path string, seat int32, loop uint32) uint32 {
	return hostLightsPlay(ptrOfString(path), uint32(len(path)), seat, loop)
}

func (wasmHost) LightsStop(seat int32) { hostLightsStop(seat) }

func (wasmHost) SetLampLevels(seat int32, levels, touch, fadeMs uint32) {
	hostSetLampLevels(seat, levels, touch, fadeMs)
}

// staleSetup is what a Setup call after Init does on the cabinet: the call
// fails with its documented failure value and msg, a constant so nothing
// allocates, reaches the host log. A trap would unload the cartridge for a
// bug the player cannot see.
func staleSetup(msg string) { Log(LogError, msg) }
