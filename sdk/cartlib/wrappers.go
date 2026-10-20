// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "unsafe"

// LogLevel is [Log]'s severity, host_log's level argument.
type LogLevel uint32

// The LogLevel values, lowest to highest severity.
const (
	LogDebug LogLevel = 0
	LogInfo  LogLevel = 1
	LogWarn  LogLevel = 2
	LogError LogLevel = 3
)

// seatArg is s as the host's seat argument: -1 for both, 0 or 1 for one. ok
// is false for SeatNone and any other value, and the caller then skips the
// host call.
func seatArg(s Seat) (arg int32, ok bool) {
	switch s {
	case SeatP1, SeatP2:
		return int32(s), true
	case SeatBoth:
		return -1, true
	}
	return 0, false
}

// LoopCount is [PlaySound]'s play count: n plays the sound n times in total,
// 0 and 1 both play it once, and [PlaySoundForever] loops until stopped.
type LoopCount uint32

// PlaySoundForever is the [LoopCount] that loops until [StopSound] or
// [StopAllSounds].
const PlaySoundForever LoopCount = 0xFFFFFFFF

// PlaySound plays the sound loaded by [Setup.LoadSound] as id, loops times
// in total. An unknown handle, including 0, is a silent no-op.
func PlaySound(id uint32, loops LoopCount) { host.PlaySound(id, uint32(loops)) }

// StopSound stops the sound loaded as id. An unknown handle is a no-op.
func StopSound(id uint32) { host.StopSound(id) }

// StopAllSounds stops every sound this cartridge is playing.
func StopAllSounds() { host.StopAllSounds() }

// MaxStorageKeyLen and MaxStorageValueLen are the hosts' caps on a storage
// key and value, in bytes. A write over either is dropped whole; see
// [StorageWrite].
const (
	MaxStorageKeyLen   = 256
	MaxStorageValueLen = 1 << 16
)

// StorageKeyOK reports whether key is a legal storage key: 1 to
// [MaxStorageKeyLen] bytes of [A-Za-z0-9_.-], and not "." or "..". The
// hosts refuse anything else.
func StorageKeyOK(key string) bool {
	if key == "" || key == "." || key == ".." || len(key) > MaxStorageKeyLen {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// StorageRead reads the value stored under key into out and returns how
// many bytes it wrote: 0 if the key is absent or illegal ([StorageKeyOK]),
// and at most len(out), so a value longer than out arrives truncated. Keys
// are private to the calling cartridge. out is caller-owned, so a
// package-level array sliced per call keeps this allocation-free.
func StorageRead(key string, out []byte) int {
	return int(host.StorageRead(key, out))
}

// StorageWrite stores val under key, replacing any earlier value, and
// reports whether the write was handed to the host. It returns false, and
// writes nothing rather than a truncated value, when key fails
// [StorageKeyOK] or val is longer than [MaxStorageValueLen]; split a larger
// value across keys. The ABI call has no result, so a host-side failure
// after a true return, such as a full disk, is silent. Valid from Init and
// Update.
func StorageWrite(key string, val []byte) bool {
	if !StorageKeyOK(key) || len(val) > MaxStorageValueLen {
		return false
	}
	host.StorageWrite(key, val)
	return true
}

// NowMicros returns the engine clock in microseconds, never wall time, so a
// replay reads the same values. Prefer accumulating [Game.Update]'s
// dtMicros; use NowMicros only where an absolute timestamp is needed, such
// as comparing against a stored one.
func NowMicros() uint64 { return host.NowMicros() }

// RandomU64 returns the next value from the engine-seeded RNG. It is a
// cartridge's only permitted source of randomness: any other source breaks
// replay determinism and golden tests.
func RandomU64() uint64 { return host.RandomU64() }

// Log sends msg to the host's log at the given [LogLevel]. Valid in any
// phase.
func Log(level LogLevel, msg string) {
	host.Log(uint32(level), msg)
}

// SetText sets the two 16-character LCD lines in front of seat, or in front
// of both seats with [SeatBoth]. [SeatNone] does nothing, so a record's
// Physical can be passed straight through.
//
// The host reads the strings in place, so passing a constant or an
// existing string allocates nothing; building one to pass (a
// `string(buf[:n])` conversion) allocates. For formatted text use
// [SetTextBytes]. LCD updates are meant for at most about 4 Hz
// (docs/CARTRIDGE.md rule 7), so throttle a readout driven by a per-tick
// value.
func SetText(seat Seat, line1, line2 string) {
	p, ok := seatArg(seat)
	if !ok {
		return
	}
	host.SetText(p, stringBytes(line1), stringBytes(line2))
}

// SetTextBytes is [SetText] taking the lines as bytes, so text formatted
// into a package-level buffer reaches the LCD without a string conversion:
//
//	var line [16]byte
//
//	n := copy(line[:], "SCORE ")
//	n += formatUint(line[n:], score) // the cartridge's own formatter
//	cartlib.SetTextBytes(seat, line[:n], nil)
//
// The host copies the bytes before returning, so the buffers may be reused
// as soon as the call returns. Allocation-free.
func SetTextBytes(seat Seat, line1, line2 []byte) {
	p, ok := seatArg(seat)
	if !ok {
		return
	}
	host.SetText(p, line1, line2)
}

// SetButtonLights turns on the lamps in mask on seat, or on both seats with
// [SeatBoth]. With only true every lamp outside mask turns off; with only
// false they keep their state. [SeatNone] does nothing. The call stops any
// lamp sequence playing on the seats it addresses (see [PlayLights]).
func SetButtonLights(seat Seat, mask ButtonMask, only bool) {
	p, ok := seatArg(seat)
	if !ok {
		return
	}
	o := int32(0)
	if only {
		o = 1
	}
	host.SetButtonLights(p, uint32(mask), o)
}

// SetLights turns all six of seat's lamps on or off, or both seats' with
// [SeatBoth]. [SeatNone] does nothing. Like [SetButtonLights], it stops any
// lamp sequence playing on the seats it addresses.
func SetLights(seat Seat, on bool) {
	p, ok := seatArg(seat)
	if !ok {
		return
	}
	o := int32(0)
	if on {
		o = 1
	}
	host.SetLights(p, o)
}

// LightsLoop is [PlayLights]' loop argument.
type LightsLoop uint32

const (
	// LightsLoopFile plays the sequence the way its own file says.
	LightsLoopFile LightsLoop = 0
	// LightsLoopOnce forces a single pass whatever the file says; the lamps
	// rest on the last frame.
	LightsLoopOnce LightsLoop = 1
	// LightsLoopCycle forces endless repetition whatever the file says.
	LightsLoopCycle LightsLoop = 2
)

// PlayLights starts the lamp sequence at a bundle-relative path, which must
// be one the manifest lists under assets.lights (docs/ASSETS.md, "Lamp
// sequences"). The host plays it on the engine clock. PlayLights reports
// whether anything is now playing: false for an unlisted path, an invalid
// loop, [SeatNone], or a sequence with no frames for the seat asked for,
// and the host logs why.
//
// seat is one seat, or [SeatBoth] for both seats as the file says. A frame
// writes only the seats it names, so a one-seat cue played at SeatBoth
// leaves the other seat alone. Valid in any phase and allocation-free for a
// constant path. [SetButtonLights], [SetLights] or a [SetLampLevels] that
// touches a lamp on the seat stops the sequence.
func PlayLights(path string, seat Seat, loop LightsLoop) bool {
	p, ok := seatArg(seat)
	if !ok {
		return false
	}
	return host.LightsPlay(path, p, uint32(loop)) != 0
}

// StopLights stops the sequence on one seat, or on both with [SeatBoth].
// The lamps keep the levels the sequence left them at; call
// [SetButtonLights] or [SetLights] to set them instead. [SeatNone] does
// nothing.
func StopLights(seat Seat) {
	if p, ok := seatArg(seat); ok {
		host.LightsStop(p)
	}
}

// LampLevelMax is the brightest lamp level. Levels run 0 (dark) to
// LampLevelMax; [SetButtonLights] and [SetLights] write LampLevelMax and 0.
// The cabinet's brightness setting scales every level afterwards.
const LampLevelMax = 15

// LampLevels is one seat's six lamp brightnesses, each 0..[LampLevelMax],
// indexed in the bit order of the Mask* constants: 0 JoyA, 1 JoyB, 2 JoyC,
// 3 JoyD, 4 JoyAux1, 5 JoyAux2.
type LampLevels [6]uint8

// PackLampLevels packs six levels into host_set_lamp_levels' wire form: lamp
// i in bits 4i..4i+3, bits 24..31 zero. Only the low four bits of each level
// survive, so a level above [LampLevelMax] wraps rather than clamps.
// [SetLampLevels] and [FadeLamps] call it for you; it is exported for
// cartridge tests.
func PackLampLevels(levels LampLevels) uint32 {
	var packed uint32
	for i, v := range levels {
		packed |= uint32(v&0xf) << (4 * uint(i))
	}
	return packed
}

// SetLampLevels sets per-lamp brightness on seat, or both seats with
// [SeatBoth], optionally as a cross-fade. [SeatNone] does nothing.
//
// touch names the lamps this call addresses: a lamp outside touch keeps its
// level and any fade running on it. touch == 0 is a complete no-op, with no
// write and no sequence stop. Otherwise the call stops any lamp sequence
// playing on the seats it addresses.
//
// fadeMs is milliseconds: 0 or negative writes instantly, and the host
// clamps it to 10000. A fade ramps each touched lamp linearly from its
// current level to the target on the engine clock, so a replay writes the
// same lamps on the same ticks. A later [SetButtonLights], [SetLights],
// SetLampLevels or sequence frame touching a lamp cancels its fade.
//
// Allocation-free. Call it from Init, HandleInput or Update.
func SetLampLevels(seat Seat, levels LampLevels, touch ButtonMask, fadeMs int) {
	p, ok := seatArg(seat)
	if !ok {
		return
	}
	if fadeMs < 0 {
		fadeMs = 0
	}
	host.SetLampLevels(p, PackLampLevels(levels), uint32(touch), uint32(fadeMs))
}

// FadeLamps ramps the lamps in mask to level over fadeMs milliseconds,
// leaving every other lamp's level and fade alone. level is
// 0..[LampLevelMax] and is truncated to four bits. It is [SetLampLevels]
// with one level for every touched lamp, and shares its seat, fade and
// sequence-stop rules; an empty mask is a complete no-op.
func FadeLamps(seat Seat, mask ButtonMask, level uint8, fadeMs int) {
	var levels LampLevels
	for i := range levels {
		levels[i] = level
	}
	SetLampLevels(seat, levels, mask, fadeMs)
}

// Exit requests this cartridge's unload, back to the shell on the cabinet.
// The engine applies it between ticks, so it is safe from HandleInput,
// Update or Render. Unless the Game implements [QuitOwner], cartlib calls
// Exit for every quit-key press.
func Exit() { host.Exit() }

// RejectKey rings the cabinet's "that key does nothing here" bell. A no-op
// on hosts with no bell, such as the browser.
func RejectKey() { host.RejectKey() }

// MaxNameLen is the host's cap on a submitted name, in bytes.
const MaxNameLen = 64

// ScoreSubmit records a finished run in the cartridge's top-10 table and
// returns its 1-based rank, or 0 if the score did not place and nothing was
// written. A score places when it is positive and either the table has room
// or it strictly beats the lowest entry; a tie with 10th place does not.
// The table is private to the calling cartridge, and is what the shell and
// attract mode show, so prefer it to [StorageWrite] for high scores.
//
// Returns 0 without calling the host once [CheatTainted] is true.
func ScoreSubmit(score int, name string) int {
	if cheatTainted {
		return 0
	}
	return int(host.ScoreSubmit(int64(score), name))
}

// ScoreRead returns the entry at a 1-based rank. ok is false past the end
// of the table, so a caller walks it without a length check:
//
//	for r := 1; ; r++ {
//	    score, name, ok := cartlib.ScoreRead(r, buf[:])
//	    if !ok { break }
//	    ...
//	}
//
// buf is caller-owned scratch of at least 8+[MaxNameLen] bytes, and name
// is a view into it, valid until buf is next written; copy it to keep it.
// Allocation-free, and unaffected by [CheatTainted].
func ScoreRead(rank int, buf []byte) (score int, name []byte, ok bool) {
	n := int(host.ScoreRead(uint32(rank), buf))
	if n < 8 {
		return 0, nil, false
	}
	// i64 score little-endian, then the raw UTF-8 name bytes.
	var s uint64
	for i := 7; i >= 0; i-- {
		s = s<<8 | uint64(buf[i])
	}
	return int(s), buf[8:n], true
}

// ScoreCount returns how many entries the table holds, 0..10.
func ScoreCount() int { return int(host.ScoreCount()) }

// ScoreQualifies reports whether score would place, by [ScoreSubmit]'s
// rule. Check it before offering name entry. Always false once
// [CheatTainted] is true.
func ScoreQualifies(score int) bool {
	if cheatTainted {
		return false
	}
	return host.ScoreQualifies(int64(score)) != 0
}

// ScoreRank returns the 1-based rank score would take now, or 0 if it would
// not place, without writing anything. Always 0 once [CheatTainted] is
// true.
func ScoreRank(score int) int {
	if cheatTainted {
		return 0
	}
	return int(host.ScoreRank(int64(score)))
}

// RecentName returns the name last noted for seat ([NoteRecentName]) within
// the past hour, across all games, for prefilling a name-entry screen. ok
// is false when there is none.
//
// seat is the physical seat (a record's Physical), not the pairing slot;
// SeatBoth and SeatNone return ok=false. buf is caller-owned scratch of at
// least [MaxNameLen] bytes and name is a view into it, valid until buf is
// next written. For the seat's non-expiring history use [RecentNameAt] or
// [RecentNames].
func RecentName(seat Seat, buf []byte) (name []byte, ok bool) {
	if !seat.Single() {
		return nil, false
	}
	n := int(host.RecentNameGet(int32(seat), buf))
	if n == 0 {
		return nil, false
	}
	return buf[:n], true
}

// RecentNameAt returns the index-th name in seat's most-recently-used list,
// index 0 being the newest, or ok=false once index runs past the end. Unlike
// [RecentName] the list never expires and survives cabinet restarts. seat,
// buf and the returned view follow RecentName's rules.
func RecentNameAt(seat Seat, index int, buf []byte) (name []byte, ok bool) {
	if !seat.Single() {
		return nil, false
	}
	n := int(host.RecentNameAt(int32(seat), uint32(index), buf))
	if n == 0 {
		return nil, false
	}
	return buf[:n], true
}

var (
	recentNameBufs    [5][MaxNameLen]byte
	recentNameList    [5]string
	recentNameReadBuf [8 + MaxNameLen]byte
)

// RecentNames returns up to five distinct names to offer a player about to
// enter one, best guess first, in the shape ui.NameEntry.SetRecents takes.
// seat is the physical seat at the controls; SeatBoth and SeatNone read as
// SeatP1. The list draws, in order, on seat's MRU list ([RecentNameAt]),
// the other seat's MRU list, then this cartridge's score table from the
// top, dropping blanks and duplicates.
//
// Allocation-free: the slice and its strings alias package-level arrays and
// are valid only until the next call.
func RecentNames(seat Seat) []string {
	if !seat.Single() {
		seat = SeatP1
	}
	n := 0
	for _, s := range [2]Seat{seat, seat.Other()} {
		for i := 0; i < len(recentNameList) && n < len(recentNameList); i++ {
			name, ok := RecentNameAt(s, i, recentNameReadBuf[:])
			if !ok {
				break
			}
			n = addRecentName(n, name)
		}
	}
	for rank := 1; rank <= 10 && n < len(recentNameList); rank++ {
		_, name, ok := ScoreRead(rank, recentNameReadBuf[:])
		if !ok {
			break
		}
		n = addRecentName(n, name)
	}
	return recentNameList[:n]
}

// addRecentName appends name to recentNameList[:n] unless it is blank,
// already there, or the list is full, and returns the new length.
func addRecentName(n int, name []byte) int {
	if len(name) == 0 || n >= len(recentNameList) {
		return n
	}
	if len(name) > MaxNameLen {
		name = name[:MaxNameLen]
	}
	for i := 0; i < n; i++ {
		if bytesEqualString(name, recentNameList[i]) {
			return n
		}
	}
	buf := recentNameBufs[n][:len(name)]
	copy(buf, name)
	recentNameList[n] = unsafe.String(&buf[0], len(buf))
	return n + 1
}

func bytesEqualString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

// NoteRecentName records name as the one the player at seat just confirmed,
// feeding both [RecentName] and [RecentNameAt]. Call it alongside
// [ScoreSubmit], only once the player has confirmed the name. The host
// trims surrounding blanks and ignores an empty name; SeatBoth and SeatNone
// do nothing, and so does every call once [CheatTainted] is true.
func NoteRecentName(seat Seat, name string) {
	if cheatTainted || !seat.Single() {
		return
	}
	host.RecentNameNote(int32(seat), name)
}

// StrategyKind is [PushPlayerStrategy]'s argument. Whether the display
// rotates is part of the kind: only [StrategyOnePlayerSolo] rotates it.
type StrategyKind uint32

const (
	// StrategyAnyPlayer lets every controller play. The engine's default.
	StrategyAnyPlayer StrategyKind = 0
	// StrategyOnePlayerSolo adopts the first controller to play and rotates
	// the display toward them. The right choice for a single-player game.
	StrategyOnePlayerSolo StrategyKind = 1
	// StrategyOnePlayerAgainstAI adopts one controller but keeps the display
	// upright, for a human-vs-CPU game played across the cabinet.
	StrategyOnePlayerAgainstAI StrategyKind = 2
	// StrategyTwoPlayerTurnBased alternates input between the two seats;
	// drive the turn with [SwitchPlayer].
	StrategyTwoPlayerTurnBased StrategyKind = 3
)

// SwitchMode is [SwitchPlayer]'s argument: toggle the turn, or hand it to
// the primary or secondary seat.
type SwitchMode uint32

// The SwitchMode values: SwitchToggle flips the turn, SwitchToPrimary and
// SwitchToSecondary hand it to that seat.
const (
	SwitchToggle      SwitchMode = 0
	SwitchToPrimary   SwitchMode = 1
	SwitchToSecondary SwitchMode = 2
)

// PushPlayerStrategy activates a player strategy, which decides whose input
// reaches [Game.HandleInput] and, for [StrategyOnePlayerSolo], rotates the
// display toward that player. A pushed strategy stays permissive until
// [LockPlayerStrategy]: push on the title screen, lock when play begins.
//
// Both hosts gate and rotate the same way, but neither gates a keyboard
// record, so a game must not rely on a strategy for correctness: a
// two-player game still demuxes on each record's Player itself.
func PushPlayerStrategy(kind StrategyKind) { host.PushPlayerStrategy(uint32(kind)) }

// PopPlayerStrategy removes the strategy this cartridge most recently
// pushed. The engine pops whatever a cartridge leaves pushed when it
// unloads.
func PopPlayerStrategy() { host.PopPlayerStrategy() }

// LockPlayerStrategy enforces the current strategy, committing to the
// controller that adopted the game. Call it when play starts.
func LockPlayerStrategy() { host.LockPlayerStrategy() }

// UnlockPlayerStrategy relaxes the current strategy, letting a new
// controller adopt the game.
func UnlockPlayerStrategy() { host.UnlockPlayerStrategy() }

// SwitchPlayer flips or sets whose turn it is under
// [StrategyTwoPlayerTurnBased]. A no-op under any other strategy.
func SwitchPlayer(mode SwitchMode) { host.SwitchPlayer(uint32(mode)) }

// LockPlayerToSeat locks the current one-player strategy to a physical
// seat: the display rotates toward it and input gates to it, whichever
// controller pressed first. A no-op unless the active strategy is
// one-player, and for SeatBoth or SeatNone.
func LockPlayerToSeat(seat Seat) {
	if seat.Single() {
		host.LockPlayerSeat(int32(seat))
	}
}

// SetRawDirections overrides the manifest's rawDirections flag for the
// running instance. With true, directions arrive as pushed, without the
// cabinet's seat-mount and display-orientation remap; with false the remap
// applies. An egocentric game typically enables it for play and disables it
// on every path back to a menu. The override ends when the cartridge is
// switched away from or unloads, so a fresh instance starts from the
// manifest. The browser host remaps, and honors this override, the same way.
func SetRawDirections(enabled bool) {
	e := uint32(0)
	if enabled {
		e = 1
	}
	host.SetRawDirections(e)
}
