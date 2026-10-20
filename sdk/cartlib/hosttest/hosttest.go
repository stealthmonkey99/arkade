// SPDX-License-Identifier: GPL-2.0-or-later

// Package hosttest is an in-process fake of the cabinet for a cartridge's
// own `go test`. [New] installs a [Host] as the [cartlib.HostABI] every
// cartlib call reaches under the plain Go toolchain; the Host then drives
// the Game through the same cart_* bodies the wasm exports use, so Init
// gets a live [cartlib.Setup], the load phase runs, and the quit-key default
// applies, exactly as on the cabinet:
//
//	func TestAppleNeverLandsOnTheSnake(t *testing.T) {
//	    h := hosttest.New(t)
//	    h.Rand = []uint64{spawnIdx(32, 32), foodIdx(32, 31)}
//	    h.Init(game)
//	    h.AdvancePastSplash()
//	    h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
//	    h.Tick(33_333)
//	    if h.Pixel(32, 31) != appleColor { ... }
//	}
//
// The fake answers as the cabinet would where a cartridge can observe the
// difference: RandomU64 and NowMicros follow scripts, storage and the score
// table persist across the test, the LCD and lamp state are the last values
// written, sounds and lamp sequences are bookkept, and every call is also
// logged on the Host's exported fields. Where the cabinet's answer depends
// on hardware or on the mounted bundle, the fake takes the bare-.wasm
// answer (no sheet, no listed lamp sequence refused) unless the test
// scripts it with [Host.MountAssets], [Host.AddCel] or [Host.Channels].
//
// The cabinet runs a cartridge under TinyGo, whose escape analysis differs
// from the host toolchain's, so [Host.AllocsPerRun] is a fast first check
// on "nothing allocates after Init" and carttest.RequireNoAllocsAfterInit,
// which reads TinyGo's own -print-allocs report, stays the oracle.
package hosttest

import (
	"testing"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
)

// SplashMicros is one load-phase update long enough to complete any custom
// cartlib.Loader and cartlib.MinSplashMicros, the value carttest uses.
const SplashMicros = 3_000_000

// MaxSounds is how many sounds one Host hands out handles for.
const MaxSounds = 64

// lcdBytes is how much of an LCD line the fake keeps; the cabinet shows 16
// characters.
const lcdBytes = 64

// LogEntry is one host_log call.
type LogEntry struct {
	Level cartlib.LogLevel // severity the cartridge logged at
	Msg   string           // message text
}

// SoundCall is one host_play_sound or host_stop_sound call. Loops is
// meaningful for a play.
type SoundCall struct {
	Op    string            // "play", "stop", "stopall"
	ID    uint32            // handle from Setup.LoadSound; 0 for "stopall"
	Loops cartlib.LoopCount // repeat count passed to a play
}

// StrategyCall is one player-strategy call. Kind is set for "push", Mode
// for "switch", Seat for "lockseat".
type StrategyCall struct {
	Op   string               // "push", "pop", "lock", "unlock", "switch", "lockseat"
	Kind cartlib.StrategyKind // the strategy pushed
	Mode cartlib.SwitchMode   // the mode switched to
	Seat cartlib.Seat         // the seat locked
}

// ScoreEntry is one row of the fake's top-ten table.
type ScoreEntry struct {
	Score int    // points; always positive
	Name  string // name the player entered
}

// Channel is one scripted non-panel input channel. Value answers a button
// (non-zero is held) or an axis; a delta reports Value once on the next
// read, then zero. Blob is a blob channel's bytes, whose length is the
// declared length a bind is checked against.
type Channel struct {
	Kind  ChannelKind // how the channel reads
	Value int32       // button state, axis position or pending delta
	Blob  []byte      // payload of a [ChannelBlob]
}

// ChannelKind is host_input_bind's kind argument, in wire numbering.
type ChannelKind uint32

const (
	// ChannelButton is a held-or-not button; Value non-zero is held.
	ChannelButton ChannelKind = 0
	// ChannelAxis is a signed position read as Value.
	ChannelAxis ChannelKind = 1
	// ChannelDelta is a motion count that reads once, then zero.
	ChannelDelta ChannelKind = 2
	// ChannelBlob is a byte payload read from Blob.
	ChannelBlob ChannelKind = 3
)

// Cel is one sprite-sheet cel the fake serves to Setup.LoadSheet and
// Sheet.Cel: straight-alpha RGBA, row-major, len W*H*4.
type Cel struct {
	W, H             int    // size, in pixels; each at most 64
	AnchorX, AnchorY int    // anchor offset from the top-left, in pixels
	RGBA             []byte // straight-alpha pixels, row-major, W*H*4 bytes
}

type lcdLine struct {
	b [lcdBytes]byte
	n int
}

// Host is the fake. Script fields (Rand, Now, Channels, the cels) are set
// before the calls that read them; state fields hold the last value the
// cartridge wrote; call-log fields grow with every call unless
// [Host.AllocsPerRun] has paused them.
type Host struct {
	tb testing.TB

	// Rand and Now are the RandomU64 and NowMicros scripts: consumed in
	// order, repeating the last value once exhausted, 0 when empty.
	Rand []uint64
	Now  []uint64 // microseconds, as NowMicros reports them

	// Channels are the non-panel channels this cabinet offers, by full
	// name ("p1.spinner"). Set them before Init.
	Channels map[string]Channel

	// Storage is the cartridge's private key-value store.
	Storage map[string][]byte
	// Scores is the cartridge's top-ten table, highest first, with the
	// cabinet's policy: a score places when positive and either the table
	// has room or it strictly beats the lowest entry; ties rank after
	// existing equal scores.
	Scores []ScoreEntry
	// RecentNames is the per-seat name the player last confirmed and
	// RecentMRU the per-seat newest-first history behind RecentNameAt.
	RecentNames [2]string   // indexed by seat
	RecentMRU   [2][]string // indexed by seat, newest first
	// Lamps is each seat's lamp levels as last written; a fade lands at
	// once. LightsPlaying is the lamp-sequence path on each seat, "" for
	// none; the fake plays every path it is given.
	Lamps         [2]cartlib.LampLevels
	LightsPlaying [2]string // indexed by seat
	// Glyphs is the cartridge's LCD glyph table.
	Glyphs map[rune]cartlib.GlyphRows
	// RawDirections is the last host_set_raw_directions value.
	RawDirections bool

	// Call logs.
	Logs         []LogEntry     // every host_log call
	Sounds       []SoundCall    // play, stop and stop-all calls
	Strategy     []StrategyCall // player-strategy calls
	Exits        int            // Exit calls
	Rejects      int            // RejectKey calls
	StorageReads []string       // key of every StorageRead

	frame      [cartlib.FBWidth * cartlib.FBHeight * 4]byte
	blits      int
	lcd        [2][2]lcdLine
	lcdWritten [2]bool
	sounds     []string
	playing    [MaxSounds + 1]bool
	bound      []boundChannel
	sheets     []string
	cels       map[string]map[string]Cel
	celHandles []celRef
	randIdx    int
	nowIdx     int
	quiet      bool
}

type boundChannel struct {
	name string
	kind ChannelKind
}

type celRef struct{ sheet, name string }

// New returns a Host installed as cartlib's host for the rest of the test;
// the previous host comes back when the test ends.
func New(tb testing.TB) *Host {
	tb.Helper()
	h := &Host{
		tb:      tb,
		Storage: map[string][]byte{},
		Glyphs:  map[rune]cartlib.GlyphRows{},
		cels:    map[string]map[string]Cel{},
	}
	prev := cartlib.UseHost(h)
	tb.Cleanup(func() { cartlib.UseHost(prev) })
	return h
}

var _ cartlib.HostABI = (*Host)(nil)

// Init registers g with cartlib and runs cart_init: g.Init receives the live
// Setup, and the load phase begins. Call [Host.AdvancePastSplash] before
// expecting gameplay frames.
func (h *Host) Init(g cartlib.Game) {
	cartlib.Run(g)
	cartlib.CartInit()
}

// Input delivers records as one cart_handle_input batch, through the same
// decode and quit-key default as the cabinet.
func (h *Host) Input(records ...cartlib.InputRecord) {
	cartlib.CartHandleInput(Encode(records))
}

// Tick runs cart_update with dtMicros, then cart_render, so [Host.Frame]
// holds the frame the cabinet would show after this tick.
func (h *Host) Tick(dtMicros uint64) {
	cartlib.CartUpdate(dtMicros)
	cartlib.CartRender()
}

// Render runs cart_render alone.
func (h *Host) Render() { cartlib.CartRender() }

// AdvancePastSplash consumes the load phase with one [SplashMicros] update
// and a render, the way carttest does for wasm. The Game never sees that dt.
func (h *Host) AdvancePastSplash() { h.Tick(SplashMicros) }

// Teardown runs cart_teardown.
func (h *Host) Teardown() { cartlib.CartTeardown() }

// Frame is the last frame blitted, row-major RGBA, [cartlib.FBWidth] by
// [cartlib.FBHeight]. It is a view: the next blit overwrites it.
func (h *Host) Frame() []byte { return h.frame[:] }

// Pixel is the last frame's pixel at (x, y); out of range is transparent
// black.
func (h *Host) Pixel(x, y int) cartlib.Color {
	if x < 0 || y < 0 || x >= cartlib.FBWidth || y >= cartlib.FBHeight {
		return cartlib.Color{}
	}
	o := (y*cartlib.FBWidth + x) * 4
	return cartlib.Color{R: h.frame[o], G: h.frame[o+1], B: h.frame[o+2], A: h.frame[o+3]}
}

// Blits is how many frames have been blitted.
func (h *Host) Blits() int { return h.blits }

// LCD is the text last written to seat's LCD, and whether anything has
// been. SeatBoth and SeatNone read as seat one.
func (h *Host) LCD(seat cartlib.Seat) (line1, line2 string, written bool) {
	i := seat.Index()
	if i < 0 {
		i = 0
	}
	l := &h.lcd[i]
	return string(l[0].b[:l[0].n]), string(l[1].b[:l[1].n]), h.lcdWritten[i]
}

// SoundID is the handle Setup.LoadSound returned for path, 0 if it was
// never loaded.
func (h *Host) SoundID(path string) uint32 {
	for i, p := range h.sounds {
		if p == path {
			return uint32(i + 1)
		}
	}
	return 0
}

// Playing reports whether the sound loaded as id has been played and not
// since stopped. The fake has no clock for a sound's end, so a one-shot
// stays playing until stopped.
func (h *Host) Playing(id uint32) bool {
	return id > 0 && int(id) <= len(h.sounds) && h.playing[id]
}

// Bound reports whether a channel by name was bound, and as which kind.
func (h *Host) Bound(name string) (ChannelKind, bool) {
	for _, b := range h.bound {
		if b.name == name {
			return b.kind, true
		}
	}
	return 0, false
}

// AddCel makes the sheet at a bundle-relative path serve cel name, so
// Setup.LoadSheet(path) succeeds and Sheet.Cel(name) lifts it. w and h are
// at most 64 and rgba holds w*h*4 bytes.
func (h *Host) AddCel(path, name string, w, hgt int, rgba []byte) {
	h.tb.Helper()
	if w < 1 || hgt < 1 || w > 64 || hgt > 64 || len(rgba) != w*hgt*4 {
		h.tb.Fatalf("hosttest.AddCel(%q, %q): %dx%d with %d bytes is not a cel", path, name, w, hgt, len(rgba))
	}
	if h.cels[path] == nil {
		h.cels[path] = map[string]Cel{}
	}
	h.cels[path][name] = Cel{W: w, H: hgt, RGBA: append([]byte(nil), rgba...)}
}

// AllocsPerRun is testing.AllocsPerRun with the Host's call logs paused, so
// the count is the cartridge's own heap allocations plus the fake's fixed
// bookkeeping, which allocates nothing: use it to pin "no allocation after
// Init" on host Go.
//
//	h.AdvancePastSplash()
//	if n := h.AllocsPerRun(100, func() { h.Tick(33_333) }); n != 0 { ... }
//
// A storage write under a new key still allocates in the fake's map, and a
// first-time sound or channel bind cannot happen after Init anyway.
func (h *Host) AllocsPerRun(runs int, fn func()) float64 {
	h.quiet = true
	defer func() { h.quiet = false }()
	return testing.AllocsPerRun(runs, fn)
}

func (h *Host) scripted(idx *int, script []uint64) uint64 {
	if len(script) == 0 {
		return 0
	}
	i := *idx
	if i >= len(script) {
		return script[len(script)-1]
	}
	*idx++
	return script[i]
}

// seatRange maps a wire seat to the Lamps/lcd indexes it addresses.
func seatRange(player int32) (lo, hi int) {
	switch player {
	case -1:
		return 0, 2
	case 0, 1:
		return int(player), int(player) + 1
	}
	return 0, 0
}

func seatOfWire(player int32) cartlib.Seat {
	if player == -1 {
		return cartlib.SeatBoth
	}
	return cartlib.SeatOf(int(player))
}

// Blit copies pix into the frame [Host.Frame] reads and counts it in
// [Host.Blits]. A cartridge reaches it through Render, not directly.
func (h *Host) Blit(pix []byte) {
	copy(h.frame[:], pix)
	h.blits++
}

// LoadSound hands out handles 1 to [MaxSounds] in load order and returns 0
// once they run out. It accepts any path, existing or not; call it in Init.
func (h *Host) LoadSound(path string) uint32 {
	if len(h.sounds) >= MaxSounds {
		return 0
	}
	h.sounds = append(h.sounds, path)
	return uint32(len(h.sounds))
}

// PlaySound marks id playing and logs a "play" call on [Host.Sounds]. An id
// that was never loaded is logged but changes no state.
func (h *Host) PlaySound(id, loopCount uint32) {
	if id > 0 && int(id) <= len(h.sounds) {
		h.playing[id] = true
	}
	if !h.quiet {
		h.Sounds = append(h.Sounds, SoundCall{Op: "play", ID: id, Loops: cartlib.LoopCount(loopCount)})
	}
}

// StopSound marks id not playing and logs a "stop" call on [Host.Sounds].
// An id that was never loaded is logged but changes no state.
func (h *Host) StopSound(id uint32) {
	if id > 0 && int(id) <= len(h.sounds) {
		h.playing[id] = false
	}
	if !h.quiet {
		h.Sounds = append(h.Sounds, SoundCall{Op: "stop", ID: id})
	}
}

// StopAllSounds marks every sound not playing and logs a "stopall" call on
// [Host.Sounds].
func (h *Host) StopAllSounds() {
	h.playing = [MaxSounds + 1]bool{}
	if !h.quiet {
		h.Sounds = append(h.Sounds, SoundCall{Op: "stopall"})
	}
}

// StorageRead copies the value stored under key into out, truncated to
// len(out), and returns the bytes copied; a missing key returns 0. Every
// call is logged on [Host.StorageReads].
func (h *Host) StorageRead(key string, out []byte) uint32 {
	if !h.quiet {
		h.StorageReads = append(h.StorageReads, key)
	}
	return uint32(copy(out, h.Storage[key]))
}

// StorageWrite stores a copy of val under key in [Host.Storage]. It has no
// return value, and the fake accepts any key, so the cabinet's key rules
// are not enforced here.
func (h *Host) StorageWrite(key string, val []byte) {
	if cur, ok := h.Storage[key]; ok && cap(cur) >= len(val) {
		h.Storage[key] = append(cur[:0], val...)
		return
	}
	h.Storage[key] = append([]byte(nil), val...)
}

// NowMicros answers the next value of the [Host.Now] script, in
// microseconds, repeating the last once exhausted and 0 when empty.
// Cartridges time from dtMicros, not this.
func (h *Host) NowMicros() uint64 { return h.scripted(&h.nowIdx, h.Now) }

// RandomU64 answers the next value of the [Host.Rand] script, repeating the
// last once exhausted and 0 when empty.
func (h *Host) RandomU64() uint64 { return h.scripted(&h.randIdx, h.Rand) }

// Log appends the message to [Host.Logs] with its level.
func (h *Host) Log(level uint32, msg string) {
	if !h.quiet {
		h.Logs = append(h.Logs, LogEntry{cartlib.LogLevel(level), msg})
	}
}

// SetText stores line1 and line2, each cut to 64 bytes, as the LCD text of
// the addressed seat: 0 or 1 for one seat, -1 for both. Read it back with
// [Host.LCD].
func (h *Host) SetText(player int32, line1, line2 []byte) {
	lo, hi := seatRange(player)
	for i := lo; i < hi; i++ {
		h.lcd[i][0].n = copy(h.lcd[i][0].b[:], line1)
		h.lcd[i][1].n = copy(h.lcd[i][1].b[:], line2)
		h.lcdWritten[i] = true
	}
}

// SetButtonLights lights the lamps in mask (bit n is lamp n) at full level
// on the addressed seat (0, 1, or -1 for both) and stops any lamp sequence
// there. A non-zero only turns every lamp outside mask off; zero leaves
// them as they were.
func (h *Host) SetButtonLights(player int32, mask uint32, only int32) {
	lo, hi := seatRange(player)
	for i := lo; i < hi; i++ {
		h.LightsPlaying[i] = ""
		for lamp := range h.Lamps[i] {
			switch {
			case mask&(1<<uint(lamp)) != 0:
				h.Lamps[i][lamp] = cartlib.LampLevelMax
			case only != 0:
				h.Lamps[i][lamp] = 0
			}
		}
	}
}

// SetLights sets every lamp on the addressed seat (0, 1, or -1 for both) to
// full for a non-zero on and off for zero, and stops any lamp sequence
// there.
func (h *Host) SetLights(player, on int32) {
	lo, hi := seatRange(player)
	for i := lo; i < hi; i++ {
		h.LightsPlaying[i] = ""
		for lamp := range h.Lamps[i] {
			if on != 0 {
				h.Lamps[i][lamp] = cartlib.LampLevelMax
			} else {
				h.Lamps[i][lamp] = 0
			}
		}
	}
}

// DefineLcdGlyph stores rows (five bits per row, one row per byte) as the
// glyph for codepoint and returns 1, or returns 0 for a code point below
// 0x80, an invalid one, or a new glyph past [cartlib.MaxCartGlyphs]. The emoji and wide-rune refusals need the
// engine's tables and are carttest's to check.
func (h *Host) DefineLcdGlyph(codepoint int32, rows uint64) uint32 {
	r := rune(codepoint)
	if r < 0x80 || !utf8.ValidRune(r) {
		return 0
	}
	if _, known := h.Glyphs[r]; !known && len(h.Glyphs) >= cartlib.MaxCartGlyphs {
		return 0
	}
	var g cartlib.GlyphRows
	for i := range g {
		g[i] = byte(rows>>(8*uint(i))) & 0x1f
	}
	h.Glyphs[r] = g
	return 1
}

// Exit counts the request in [Host.Exits]; the test may keep driving the
// Game.
func (h *Host) Exit() { h.Exits++ }

// RejectKey counts the request in [Host.Rejects].
func (h *Host) RejectKey() { h.Rejects++ }

func (h *Host) qualifies(score int) bool {
	if score <= 0 {
		return false
	}
	if len(h.Scores) < 10 {
		return true
	}
	return score > h.Scores[len(h.Scores)-1].Score
}

func (h *Host) rank(score int) int {
	rank := 1
	for _, e := range h.Scores {
		if e.Score >= score {
			rank++
		}
	}
	return rank
}

// ScoreSubmit inserts the score into [Host.Scores] under the cabinet's
// policy and returns its 1-based rank, or 0 if it does not qualify. The
// table keeps ten entries.
func (h *Host) ScoreSubmit(score int64, name string) uint32 {
	s := int(score)
	if !h.qualifies(s) {
		return 0
	}
	idx := h.rank(s) - 1
	h.Scores = append(h.Scores, ScoreEntry{})
	copy(h.Scores[idx+1:], h.Scores[idx:])
	h.Scores[idx] = ScoreEntry{Score: s, Name: name}
	if len(h.Scores) > 10 {
		h.Scores = h.Scores[:10]
	}
	return uint32(idx + 1)
}

// ScoreRead writes the entry at the 1-based rank into out as the score in
// eight little-endian bytes followed by the name, and returns the bytes
// written. It returns 0 for a rank out of range or an out shorter than 8.
func (h *Host) ScoreRead(rank uint32, out []byte) uint32 {
	if rank < 1 || int(rank) > len(h.Scores) || len(out) < 8 {
		return 0
	}
	e := h.Scores[rank-1]
	s := uint64(int64(e.Score))
	for i := 0; i < 8; i++ {
		out[i] = byte(s >> (8 * uint(i)))
	}
	return uint32(8 + copy(out[8:], e.Name))
}

// ScoreCount is the number of entries in [Host.Scores].
func (h *Host) ScoreCount() uint32 { return uint32(len(h.Scores)) }

// ScoreQualifies returns 1 if score would place in the table and 0 if not;
// the table is not changed.
func (h *Host) ScoreQualifies(score int64) uint32 {
	if h.qualifies(int(score)) {
		return 1
	}
	return 0
}

// ScoreRank returns the 1-based rank score would take, or 0 if it would not
// place; the table is not changed.
func (h *Host) ScoreRank(score int64) uint32 {
	if !h.qualifies(int(score)) {
		return 0
	}
	return uint32(h.rank(int(score)))
}

// RecentNameGet copies the name last noted for seat player (0 or 1) into
// out and returns the bytes copied; 0 for an unset name or another player
// value.
func (h *Host) RecentNameGet(player int32, out []byte) uint32 {
	if player < 0 || int(player) >= len(h.RecentNames) {
		return 0
	}
	return uint32(copy(out, h.RecentNames[player]))
}

// RecentNameAt copies the index-th most recent name for seat player (0 or
// 1), newest first, into out and returns the bytes copied; 0 past the end
// of the history or for another player value.
func (h *Host) RecentNameAt(player int32, index uint32, out []byte) uint32 {
	if player < 0 || int(player) >= len(h.RecentMRU) || int(index) >= len(h.RecentMRU[player]) {
		return 0
	}
	return uint32(copy(out, h.RecentMRU[player][index]))
}

// RecentNameNote records name as the seat's latest, moving it to the front
// of a history of five. An empty name or a player other than 0 or 1 is
// ignored.
func (h *Host) RecentNameNote(player int32, name string) {
	if name == "" || player < 0 || int(player) >= len(h.RecentNames) {
		return
	}
	h.RecentNames[player] = name
	mru := []string{name}
	for _, prev := range h.RecentMRU[player] {
		if prev != name && len(mru) < 5 {
			mru = append(mru, prev)
		}
	}
	h.RecentMRU[player] = mru
}

func (h *Host) strategy(c StrategyCall) {
	if !h.quiet {
		h.Strategy = append(h.Strategy, c)
	}
}

// PushPlayerStrategy logs a "push" call with the strategy kind on
// [Host.Strategy].
func (h *Host) PushPlayerStrategy(kind uint32) {
	h.strategy(StrategyCall{Op: "push", Kind: cartlib.StrategyKind(kind)})
}

// PopPlayerStrategy logs a "pop" call on [Host.Strategy].
func (h *Host) PopPlayerStrategy() { h.strategy(StrategyCall{Op: "pop"}) }

// LockPlayerStrategy logs a "lock" call on [Host.Strategy].
func (h *Host) LockPlayerStrategy() { h.strategy(StrategyCall{Op: "lock"}) }

// UnlockPlayerStrategy logs an "unlock" call on [Host.Strategy].
func (h *Host) UnlockPlayerStrategy() { h.strategy(StrategyCall{Op: "unlock"}) }

// SwitchPlayer logs a "switch" call with the mode on [Host.Strategy].
func (h *Host) SwitchPlayer(mode uint32) {
	h.strategy(StrategyCall{Op: "switch", Mode: cartlib.SwitchMode(mode)})
}

// LockPlayerSeat logs a "lockseat" call on [Host.Strategy] with the
// physical seat; -1 reads as [cartlib.SeatBoth].
func (h *Host) LockPlayerSeat(physical int32) {
	h.strategy(StrategyCall{Op: "lockseat", Seat: seatOfWire(physical)})
}

// SetRawDirections sets [Host.RawDirections] to whether enabled is
// non-zero.
func (h *Host) SetRawDirections(enabled uint32) { h.RawDirections = enabled != 0 }

// LoadSheet returns a 1-based sheet handle if a cel was added at path with
// [Host.AddCel], and 0 otherwise. Call it in Init.
func (h *Host) LoadSheet(path string) uint32 {
	if _, ok := h.cels[path]; !ok {
		return 0
	}
	h.sheets = append(h.sheets, path)
	return uint32(len(h.sheets))
}

// SheetCel returns a 1-based cel handle for the named cel of a loaded
// sheet, or 0 for an unknown sheet handle or cel name.
func (h *Host) SheetCel(sheet uint32, name string) uint32 {
	if sheet < 1 || int(sheet) > len(h.sheets) {
		return 0
	}
	path := h.sheets[sheet-1]
	if _, ok := h.cels[path][name]; !ok {
		return 0
	}
	h.celHandles = append(h.celHandles, celRef{path, name})
	return uint32(len(h.celHandles))
}

func (h *Host) cel(handle uint32) (Cel, bool) {
	if handle < 1 || int(handle) > len(h.celHandles) {
		return Cel{}, false
	}
	ref := h.celHandles[handle-1]
	c, ok := h.cels[ref.sheet][ref.name]
	return c, ok
}

// CelInfo writes the cel's width, height, anchor X and anchor Y as four
// little-endian 16-bit values into out and returns 1. It returns 0 for an
// unknown handle or an out shorter than 8.
func (h *Host) CelInfo(cel uint32, out []byte) uint32 {
	c, ok := h.cel(cel)
	if !ok || len(out) < 8 {
		return 0
	}
	for i, v := range [4]int{c.W, c.H, c.AnchorX, c.AnchorY} {
		out[2*i], out[2*i+1] = byte(v), byte(v>>8)
	}
	return 1
}

// CelRead copies the cel's straight-alpha RGBA pixels into rgba and returns
// the bytes copied. It returns 0 for an unknown handle or an rgba shorter
// than the cel.
func (h *Host) CelRead(cel uint32, rgba []byte) uint32 {
	c, ok := h.cel(cel)
	if !ok || len(rgba) < len(c.RGBA) {
		return 0
	}
	return uint32(copy(rgba, c.RGBA))
}

// InputBind returns a handle for the channel name in [Host.Channels] if it
// exists with the same kind, and -1 otherwise or once
// [cartlib.MaxBoundChannels] are bound. Call it in Init.
func (h *Host) InputBind(name string, kind uint32) int32 {
	c, ok := h.Channels[name]
	if !ok || c.Kind != ChannelKind(kind) || len(h.bound) >= cartlib.MaxBoundChannels {
		return -1
	}
	h.bound = append(h.bound, boundChannel{name, c.Kind})
	return int32(len(h.bound) - 1)
}

func (h *Host) channel(handle int32, kind ChannelKind) (Channel, string, bool) {
	if handle < 0 || int(handle) >= len(h.bound) || h.bound[handle].kind != kind {
		return Channel{}, "", false
	}
	name := h.bound[handle].name
	c, ok := h.Channels[name]
	return c, name, ok
}

// InputButton returns 1 if the bound button channel's Value is non-zero and
// 0 otherwise, including for an invalid handle or one bound as another
// kind.
func (h *Host) InputButton(handle int32) uint32 {
	if c, _, ok := h.channel(handle, ChannelButton); ok && c.Value != 0 {
		return 1
	}
	return 0
}

// InputAxis returns the bound axis channel's Value, or 0 for an invalid
// handle or one bound as another kind.
func (h *Host) InputAxis(handle int32) int32 {
	c, _, ok := h.channel(handle, ChannelAxis)
	if !ok {
		return 0
	}
	return c.Value
}

// InputDelta returns the bound delta channel's Value and zeroes it, so the
// next read is 0 until the test sets it again. An invalid handle or one
// bound as another kind returns 0.
func (h *Host) InputDelta(handle int32) int32 {
	c, name, ok := h.channel(handle, ChannelDelta)
	if !ok {
		return 0
	}
	v := c.Value
	c.Value = 0
	h.Channels[name] = c
	return v
}

// InputBlob copies the bound blob channel's bytes into dst and returns the
// count. It returns 0 for an invalid handle, one bound as another kind, or
// a dst shorter than the blob.
func (h *Host) InputBlob(handle int32, dst []byte) uint32 {
	c, _, ok := h.channel(handle, ChannelBlob)
	if !ok || len(dst) < len(c.Blob) {
		return 0
	}
	return uint32(copy(dst, c.Blob))
}

// OutputBind returns -1, as every current host does: no output channel exists.
func (h *Host) OutputBind(name string) int32 { return -1 }

// OutputSet does nothing, because [Host.OutputBind] never returns a handle.
func (h *Host) OutputSet(handle, value int32) {}

// LightsPlay records path in [Host.LightsPlaying] for the addressed seat
// (0, 1, or -1 for both) and returns 1. It returns 0 for a loop mode past
// [cartlib.LightsLoopCycle] or a seat other than 0, 1 or -1; the path
// itself is not checked.
func (h *Host) LightsPlay(path string, seat int32, loop uint32) uint32 {
	if loop > uint32(cartlib.LightsLoopCycle) {
		return 0
	}
	lo, hi := seatRange(seat)
	if lo == hi {
		return 0
	}
	for i := lo; i < hi; i++ {
		h.LightsPlaying[i] = path
	}
	return 1
}

// LightsStop clears [Host.LightsPlaying] for the addressed seat (0, 1, or
// -1 for both).
func (h *Host) LightsStop(seat int32) {
	lo, hi := seatRange(seat)
	for i := lo; i < hi; i++ {
		h.LightsPlaying[i] = ""
	}
}

// SetLampLevels writes the four-bit level of each lamp whose bit is set in
// touch (bit n for lamp n, levels packed four bits per lamp) on the
// addressed seat, and stops any lamp sequence there. The fade is ignored:
// levels land at once, and a touch with none of the six lamp bits does
// nothing.
func (h *Host) SetLampLevels(seat int32, levels, touch, fadeMs uint32) {
	if touch&0x3f == 0 {
		return
	}
	lo, hi := seatRange(seat)
	for i := lo; i < hi; i++ {
		h.LightsPlaying[i] = ""
		for lamp := range h.Lamps[i] {
			if touch&(1<<uint(lamp)) != 0 {
				h.Lamps[i][lamp] = uint8(levels>>(4*uint(lamp))) & 0xf
			}
		}
	}
}
