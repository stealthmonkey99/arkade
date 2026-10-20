// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scores"
)

// PixelCall records one DrawPixel call.
type PixelCall struct {
	X, Y int
	C    display.Pixel
}

// RectCall records one FillRect call.
type RectCall struct {
	X, Y, W, H int
	C          display.Pixel
}

// SpriteDrawCall records one DrawSprite call.
type SpriteDrawCall struct {
	ID   uint32
	X, Y int
}

// CelDrawCall records one host_draw_cel after the bridge resolved the handle
// and applied any flip. Pix is a copy of the straight-alpha RGBA the host was
// asked to composite.
type CelDrawCall struct {
	W, H int
	X, Y int
	Pix  []byte
}

// SoundCall records one sound operation; Action is "load", "play", "stop" or "stopall".
type SoundCall struct {
	Action string
	ID     uint32
	Path   string
	Loop   uint32
}

// LogCall records one Log call.
type LogCall struct {
	Level LogLevel
	Msg   string
}

// TextSetCall records one SetText call. Player here and in the other call
// records is the seat as the ABI addresses it: 0, 1, or -1 for both.
type TextSetCall struct {
	Player       int
	Line1, Line2 string
}

// ButtonLightsCall records one SetButtonLights call.
type ButtonLightsCall struct {
	Player int
	Lights map[string]bool
	Only   bool
}

// LightsCall records one SetLights call.
type LightsCall struct {
	Player int
	On     bool
}

// LampLevelsCall records one per-lamp brightness write as made: the seat as
// passed, the levels unpacked, touch masked to the six lamps and the fade
// clamped. It covers host_set_lamp_levels and lamp-sequence frames carrying a
// fadeMs (touch is all six lamps); a frame without one is a SetButtonLights
// call, and a call touching no lamp is not recorded. Per-tick fade
// interpolation happens in the engine and never reaches a Host.
type LampLevelsCall struct {
	Player int
	Levels [6]uint8
	Touch  uint8
	FadeMs int
}

// StrategyCall records one player-strategy or host_switch_player call. Kind
// is meaningful only for "push", Mode only for "switch", Seat only for
// "lockseat".
type StrategyCall struct {
	Op   string // "push", "pop", "lock", "unlock", "switch", "lockseat"
	Kind StrategyKind
	Mode SwitchMode
	Seat int
}

// RecentNameCall records one NoteRecentName call.
type RecentNameCall struct {
	Player int
	Name   string
}

// FakeHost is a scripted, call-recording Host double. NowScript and
// RandScript are consumed in order and repeat their last value once
// exhausted. Mu guards every field.
type FakeHost struct {
	Mu sync.Mutex

	Blits   [][]display.Pixel
	Pixels  []PixelCall
	Rects   []RectCall
	Sprites map[uint32][]byte
	Draws   []SpriteDrawCall
	Sounds  []SoundCall
	Logs    []LogCall

	// Assets stands in for the mounted .rcart's asset map, keyed by
	// bundle-relative path. Empty means a bare .wasm build, where every
	// host_load_sheet returns the zero handle.
	Assets map[string][]byte
	// CelDraws records host_draw_cel calls. Art a cartridge draws guest-side
	// arrives through Blits instead.
	CelDraws []CelDrawCall

	Storage      map[string][]byte
	StorageReads []string

	LCDTexts     []TextSetCall
	ButtonLights []ButtonLightsCall
	LightSets    []LightsCall
	LampLevels   []LampLevelsCall
	// CartGlyphs is the current host_define_lcd_glyph table, a whole-table
	// snapshot rather than a call log; nil once the cartridge tears down.
	CartGlyphs map[rune][8]byte
	Exits      int
	Rejects    int

	// Scores is a real scores.Table, so tests assert the cabinet's actual
	// insertion, tie-break and eviction policy.
	Scores      scores.Table
	RecentNames [2]string
	// RecentMRU is the per-seat, newest-first name list behind
	// host_recent_name_at. It has no TTL and no file; set it directly to
	// script a seat's list.
	RecentMRU     [2][]string
	NoteNameCalls []RecentNameCall
	StrategyCalls []StrategyCall

	// RawDirCalls records every host_set_raw_directions call.
	RawDirCalls []bool

	// Channels are the scripted non-panel input channels the host offers; nil
	// means only the panel is attached.
	Channels map[string]FakeChannel
	// SessionProfiles is the HID profile set the mounted bundle installed, or
	// nil once it has torn down.
	SessionProfiles     []*controller.Spec
	SessionProfileCalls int

	nextID uint32

	NowScript  []uint64
	nowIdx     int
	RandScript []uint64
	randIdx    int
}

// NewFakeHost returns a FakeHost with empty sprite, storage and asset maps and
// no scripted time or randomness (both read 0).
func NewFakeHost() *FakeHost {
	return &FakeHost{
		Sprites: map[uint32][]byte{},
		Storage: map[string][]byte{},
		Assets:  map[string][]byte{},
	}
}

var _ Host = (*FakeHost)(nil)

// Blit appends a copy of a full-frame blit to Blits.
func (h *FakeHost) Blit(pixels []display.Pixel) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	cp := append([]display.Pixel(nil), pixels...)
	h.Blits = append(h.Blits, cp)
}

// DrawPixel records the call in Pixels; nothing is rasterised.
func (h *FakeHost) DrawPixel(x, y int, c display.Pixel) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Pixels = append(h.Pixels, PixelCall{x, y, c})
}

// FillRect records the call in Rects; nothing is rasterised.
func (h *FakeHost) FillRect(x, y, w, hgt int, c display.Pixel) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Rects = append(h.Rects, RectCall{x, y, w, hgt, c})
}

// LoadSprite stores a copy of the PNG bytes in Sprites under a fresh non-zero
// id and never fails.
func (h *FakeHost) LoadSprite(png []byte) (uint32, error) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.nextID++
	h.Sprites[h.nextID] = append([]byte(nil), png...)
	return h.nextID, nil
}

// DrawSprite records the call in Draws; the id is not validated.
func (h *FakeHost) DrawSprite(id uint32, x, y int) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Draws = append(h.Draws, SpriteDrawCall{id, x, y})
}

// DrawCel records the call, with a copy of the pixels, in CelDraws.
func (h *FakeHost) DrawCel(pix []byte, w, hgt, x, y int) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	// The bridge reuses a scratch buffer for flipped draws.
	h.CelDraws = append(h.CelDraws, CelDrawCall{
		W: w, H: hgt, X: x, Y: y,
		Pix: append([]byte(nil), pix...),
	})
}

// BundleAsset looks path up in Assets.
func (h *FakeHost) BundleAsset(path string) ([]byte, bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	data, ok := h.Assets[path]
	return data, ok
}

// LoadSound records a "load" in Sounds and returns a fresh non-zero id; the
// path is not checked.
func (h *FakeHost) LoadSound(path string) (uint32, error) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.nextID++
	id := h.nextID
	h.Sounds = append(h.Sounds, SoundCall{Action: "load", ID: id, Path: path})
	return id, nil
}

// PlaySound records a "play" in Sounds.
func (h *FakeHost) PlaySound(id uint32, loopCount uint32) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Sounds = append(h.Sounds, SoundCall{Action: "play", ID: id, Loop: loopCount})
}

// StopSound records a "stop" in Sounds.
func (h *FakeHost) StopSound(id uint32) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Sounds = append(h.Sounds, SoundCall{Action: "stop", ID: id})
}

// StopAllSounds records a "stopall" in Sounds.
func (h *FakeHost) StopAllSounds() {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Sounds = append(h.Sounds, SoundCall{Action: "stopall"})
}

// StorageRead logs key in StorageReads and returns the value held in Storage.
func (h *FakeHost) StorageRead(key string) ([]byte, bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.StorageReads = append(h.StorageReads, key)
	v, ok := h.Storage[key]
	return v, ok
}

// StorageWrite stores a copy of val in Storage; keys are not validated.
func (h *FakeHost) StorageWrite(key string, val []byte) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Storage[key] = append([]byte(nil), val...)
}

// NowMicros returns the next NowScript value, repeating the last once
// exhausted, or 0 for an empty script.
func (h *FakeHost) NowMicros() uint64 {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	return h.nextScripted(&h.nowIdx, h.NowScript)
}

// RandomU64 returns the next RandScript value, repeating the last once
// exhausted, or 0 for an empty script.
func (h *FakeHost) RandomU64() uint64 {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	return h.nextScripted(&h.randIdx, h.RandScript)
}

// nextScripted returns script[*idx] and advances *idx, repeating the last
// element once exhausted and returning 0 for an empty script.
func (h *FakeHost) nextScripted(idx *int, script []uint64) uint64 {
	if len(script) == 0 {
		return 0
	}
	i := *idx
	if i >= len(script) {
		i = len(script) - 1
	} else {
		*idx++
	}
	return script[i]
}

// Log appends the message to Logs.
func (h *FakeHost) Log(level LogLevel, msg string) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Logs = append(h.Logs, LogCall{level, msg})
}

func playerOf(seats input.Seats) int {
	i, _ := seats.Index()
	return i
}

// SetText records the LCD lines in LCDTexts.
func (h *FakeHost) SetText(seats input.Seats, line1, line2 string) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.LCDTexts = append(h.LCDTexts, TextSetCall{playerOf(seats), line1, line2})
}

// SetButtonLights records a copy of the light map in ButtonLights.
func (h *FakeHost) SetButtonLights(seats input.Seats, lights map[string]bool, only bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	cp := make(map[string]bool, len(lights))
	for k, v := range lights {
		cp[k] = v
	}
	h.ButtonLights = append(h.ButtonLights, ButtonLightsCall{playerOf(seats), cp, only})
}

// SetLights records the call in LightSets.
func (h *FakeHost) SetLights(seats input.Seats, on bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.LightSets = append(h.LightSets, LightsCall{playerOf(seats), on})
}

// SetLampLevels records the call in LampLevels.
func (h *FakeHost) SetLampLevels(seats input.Seats, levels [6]uint8, touch uint8, fadeMs int) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.LampLevels = append(h.LampLevels, LampLevelsCall{playerOf(seats), levels, touch, fadeMs})
}

// SetCartGlyphs records the cartridge's LCD glyph table; nothing is drawn.
func (h *FakeHost) SetCartGlyphs(glyphs map[rune][8]byte) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if len(glyphs) == 0 {
		h.CartGlyphs = nil
		return
	}
	cp := make(map[rune][8]byte, len(glyphs))
	for r, rows := range glyphs {
		cp[r] = rows
	}
	h.CartGlyphs = cp
}

// Exit records the request only, as the engine does; a scripted journey may
// keep driving the cartridge afterwards.
func (h *FakeHost) Exit() {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Exits++
}

// RejectKey counts the call in Rejects.
func (h *FakeHost) RejectKey() {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.Rejects++
}

// ScoreSubmit inserts into Scores under the cabinet's policy and returns the
// 1-based rank, or 0 when the score does not qualify.
func (h *FakeHost) ScoreSubmit(score int, name string) int {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if !h.Scores.Qualifies(score) {
		return 0
	}
	return h.Scores.Insert(score, name)
}

// ScoreRead returns the entry at the 1-based rank, with ok=false outside the table.
func (h *FakeHost) ScoreRead(rank int) (int, string, bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if rank < 1 || rank > len(h.Scores.Entries) {
		return 0, "", false
	}
	e := h.Scores.Entries[rank-1]
	return e.Score, e.Name, true
}

// ScoreCount returns the number of entries in Scores.
func (h *FakeHost) ScoreCount() int {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	return len(h.Scores.Entries)
}

// ScoreQualifies reports whether score would enter Scores.
func (h *FakeHost) ScoreQualifies(score int) bool {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	return h.Scores.Qualifies(score)
}

// ScoreRank returns the 1-based rank score would take in Scores.
func (h *FakeHost) ScoreRank(score int) int {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	return h.Scores.Rank(score)
}

// RecentName returns the seat's entry in RecentNames, with ok=false for an
// empty name or a seat outside 0..1.
func (h *FakeHost) RecentName(player int) (string, bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if player < 0 || player >= len(h.RecentNames) {
		return "", false
	}
	n := h.RecentNames[player]
	return n, n != ""
}

// RecentNameAt returns the index-th entry of a seat's MRU, newest first, and
// ok=false past the end.
func (h *FakeHost) RecentNameAt(player, index int) (string, bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if player < 0 || player >= len(h.RecentMRU) || index < 0 || index >= len(h.RecentMRU[player]) {
		return "", false
	}
	n := h.RecentMRU[player][index]
	return n, n != ""
}

// NoteRecentName logs the call in NoteNameCalls and, for a non-empty name on a
// valid seat, sets RecentNames and promotes the name in RecentMRU.
func (h *FakeHost) NoteRecentName(player int, name string) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.NoteNameCalls = append(h.NoteNameCalls, RecentNameCall{player, name})
	if name == "" || player < 0 || player >= len(h.RecentNames) {
		return
	}
	h.RecentNames[player] = name
	h.RecentMRU[player] = promoteName(h.RecentMRU[player], name, RecentMRUMax)
}

// RecentMRUMax is the per-seat MRU depth.
const RecentMRUMax = 5

// promoteName moves name to the front of list without duplicating it and
// drops the oldest entries past max.
func promoteName(list []string, name string, max int) []string {
	out := append([]string{name}, list...)
	n := 1
	for _, prev := range out[1:] {
		if prev == name || prev == "" || n >= max {
			continue
		}
		out[n] = prev
		n++
	}
	return out[:n]
}

// PushPlayerStrategy records a "push" in StrategyCalls.
func (h *FakeHost) PushPlayerStrategy(kind StrategyKind) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.StrategyCalls = append(h.StrategyCalls, StrategyCall{Op: "push", Kind: kind})
}

// PopPlayerStrategy records a "pop" in StrategyCalls.
func (h *FakeHost) PopPlayerStrategy() {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.StrategyCalls = append(h.StrategyCalls, StrategyCall{Op: "pop"})
}

// LockPlayerStrategy records a "lock" in StrategyCalls.
func (h *FakeHost) LockPlayerStrategy() {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.StrategyCalls = append(h.StrategyCalls, StrategyCall{Op: "lock"})
}

// UnlockPlayerStrategy records an "unlock" in StrategyCalls.
func (h *FakeHost) UnlockPlayerStrategy() {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.StrategyCalls = append(h.StrategyCalls, StrategyCall{Op: "unlock"})
}

// SwitchPlayer records a "switch" in StrategyCalls.
func (h *FakeHost) SwitchPlayer(mode SwitchMode) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.StrategyCalls = append(h.StrategyCalls, StrategyCall{Op: "switch", Mode: mode})
}

// LockPlayerToSeat records a "lockseat" in StrategyCalls.
func (h *FakeHost) LockPlayerToSeat(physical int) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.StrategyCalls = append(h.StrategyCalls, StrategyCall{Op: "lockseat", Seat: physical})
}

// SetRawDirections records the flag in RawDirCalls.
func (h *FakeHost) SetRawDirections(enabled bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.RawDirCalls = append(h.RawDirCalls, enabled)
}

// LastFrame returns the most recent host_blit frame. It panics if no blit was
// recorded.
func (h *FakeHost) LastFrame() []display.Pixel {
	return h.Blits[len(h.Blits)-1]
}

// MountAssets fills Assets from dir's assets/ tree, keyed as rcartpack keys
// them inside a .rcart ("assets/sprites/hero.png"). A missing assets/
// directory is not an error.
func (h *FakeHost) MountAssets(dir string) error {
	root := filepath.Join(dir, "assets")
	if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		h.Mu.Lock()
		defer h.Mu.Unlock()
		if h.Assets == nil {
			h.Assets = map[string][]byte{}
		}
		h.Assets[filepath.ToSlash(rel)] = data
		return nil
	})
}

// FakeChannel is one scripted non-panel channel. The cabinet's own panel
// channels are derived from input records and are not scripted here. Set
// Channels before Init.
type FakeChannel struct {
	Kind ChannelKind
	// Value is what SampleChannel returns for a button or axis; a delta
	// reports it once on the next sample, then resets to zero.
	Value int32
	// Blob is a blob channel's bytes; its length is the declared length a
	// bind is checked against.
	Blob []byte
}

// SetChannel scripts one offered channel.
func (h *FakeHost) SetChannel(name string, ch FakeChannel) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	if h.Channels == nil {
		h.Channels = map[string]FakeChannel{}
	}
	h.Channels[name] = ch
}

// OfferedChannel answers host_input_bind for a non-panel name.
func (h *FakeHost) OfferedChannel(name string) (ChannelKind, int, bool) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	c, ok := h.Channels[name]
	if !ok {
		return 0, 0, false
	}
	return c.Kind, len(c.Blob), true
}

// SampleChannel reads one scalar channel, consuming a delta's motion.
func (h *FakeHost) SampleChannel(name string) int32 {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	c, ok := h.Channels[name]
	if !ok {
		return 0
	}
	if c.Kind == ChannelDelta {
		v := c.Value
		c.Value = 0
		h.Channels[name] = c
		return v
	}
	return c.Value
}

// SampleChannelBlob copies one blob channel's bytes.
func (h *FakeHost) SampleChannelBlob(name string, dst []byte) int {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	c, ok := h.Channels[name]
	if !ok {
		return 0
	}
	return copy(dst, c.Blob)
}

// SetSessionProfiles records the mounted cartridge's HID profiles, replacing
// the whole set.
func (h *FakeHost) SetSessionProfiles(profiles []*controller.Spec) {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	h.SessionProfiles = profiles
	h.SessionProfileCalls++
}
