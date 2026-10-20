// SPDX-License-Identifier: GPL-2.0-or-later

//go:build tinygo

package cartlib

// The whole "arkade" import set; TinyGo drops any a cartridge never reaches.
// wasm linking ignores signedness, so an int32/uint32 mismatch with the
// host links and silently reinterprets the bits. The host_draw_* and sprite
// imports are left out: the framebuffer blit after Render overwrites them.

//go:wasmimport arkade host_blit
func hostBlit(ptr, length uint32)

//go:wasmimport arkade host_load_sound
func hostLoadSound(ptr, length uint32) uint32

//go:wasmimport arkade host_play_sound
func hostPlaySound(id, loopCount uint32)

//go:wasmimport arkade host_stop_sound
func hostStopSound(id uint32)

//go:wasmimport arkade host_stop_all_sounds
func hostStopAllSounds()

//go:wasmimport arkade host_storage_read
func hostStorageRead(keyPtr, keyLen, outPtr, outCap uint32) uint32

//go:wasmimport arkade host_storage_write
func hostStorageWrite(keyPtr, keyLen, valPtr, valLen uint32)

//go:wasmimport arkade host_now_micros
func hostNowMicros() uint64

//go:wasmimport arkade host_random_u64
func hostRandomU64() uint64

//go:wasmimport arkade host_log
func hostLog(level, ptr, length uint32)

//go:wasmimport arkade host_set_text
func hostSetText(player int32, l1Ptr, l1Len, l2Ptr, l2Len uint32)

//go:wasmimport arkade host_set_button_lights
func hostSetButtonLights(player int32, mask uint32, only int32)

//go:wasmimport arkade host_set_lights
func hostSetLights(player, on int32)

//go:wasmimport arkade host_define_lcd_glyph
func hostDefineLcdGlyph(codepoint int32, rows uint64) uint32

// Importing host_exit hands the cartridge its quit keys on the cabinet,
// which is why cartlib answers quit presses for a Game that is not a
// [QuitOwner].
//
//go:wasmimport arkade host_exit
func hostExit()

//go:wasmimport arkade host_reject_key
func hostRejectKey()

//go:wasmimport arkade host_score_submit
func hostScoreSubmit(score int64, namePtr, nameLen uint32) uint32

//go:wasmimport arkade host_score_read
func hostScoreRead(rank, outPtr, outCap uint32) uint32

//go:wasmimport arkade host_score_count
func hostScoreCount() uint32

//go:wasmimport arkade host_score_qualifies
func hostScoreQualifies(score int64) uint32

//go:wasmimport arkade host_score_rank
func hostScoreRank(score int64) uint32

//go:wasmimport arkade host_recent_name_get
func hostRecentNameGet(player int32, outPtr, outCap uint32) uint32

//go:wasmimport arkade host_recent_name_at
func hostRecentNameAt(player int32, index, outPtr, outCap uint32) uint32

//go:wasmimport arkade host_recent_name_note
func hostRecentNameNote(player int32, namePtr, nameLen uint32)

//go:wasmimport arkade host_push_player_strategy
func hostPushPlayerStrategy(kind uint32)

//go:wasmimport arkade host_pop_player_strategy
func hostPopPlayerStrategy()

//go:wasmimport arkade host_lock_player_strategy
func hostLockPlayerStrategy()

//go:wasmimport arkade host_unlock_player_strategy
func hostUnlockPlayerStrategy()

//go:wasmimport arkade host_switch_player
func hostSwitchPlayer(mode uint32)

//go:wasmimport arkade host_lock_player_seat
func hostLockPlayerSeat(physical int32)

//go:wasmimport arkade host_set_raw_directions
func hostSetRawDirections(enabled uint32)

//go:wasmimport arkade host_load_sheet
func hostLoadSheet(pathPtr, pathLen uint32) uint32

//go:wasmimport arkade host_sheet_cel
func hostSheetCel(sheet, namePtr, nameLen uint32) uint32

//go:wasmimport arkade host_cel_info
func hostCelInfo(cel, outPtr uint32) uint32

//go:wasmimport arkade host_cel_read
func hostCelRead(cel, rgbaPtr, rgbaCap uint32) uint32

//go:wasmimport arkade host_input_bind
func hostInputBind(namePtr, nameLen, kind uint32) int32

//go:wasmimport arkade host_input_button
func hostInputButton(handle int32) uint32

//go:wasmimport arkade host_input_axis
func hostInputAxis(handle int32) int32

//go:wasmimport arkade host_input_delta
func hostInputDelta(handle int32) int32

//go:wasmimport arkade host_input_blob
func hostInputBlob(handle int32, dstPtr, dstCap uint32) uint32

//go:wasmimport arkade host_output_bind
func hostOutputBind(namePtr, nameLen uint32) int32

//go:wasmimport arkade host_output_set
func hostOutputSet(handle, value int32)

//go:wasmimport arkade host_lights_play
func hostLightsPlay(pathPtr, pathLen uint32, seat int32, loop uint32) uint32

//go:wasmimport arkade host_lights_stop
func hostLightsStop(seat int32)

//go:wasmimport arkade host_set_lamp_levels
func hostSetLampLevels(seat int32, levels, touch, fadeMs uint32)
