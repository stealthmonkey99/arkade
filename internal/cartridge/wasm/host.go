// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"fmt"

	"github.com/tetratelabs/wazero/api"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// Sandbox caps on every length that crosses the ABI, whatever the guest's
// linear memory allows.
const (
	maxLogBytes      = 4096            // host_log message
	maxKeyBytes      = 256             // host_storage_read/write key
	maxStorageBytes  = 1 << 16         // host_storage_read/write value
	maxSpriteBytes   = 1 << 20         // host_load_sprite PNG
	maxSoundPathLen  = 512             // host_load_sound path
	maxBlitBytes     = 64 * 64 * 4 * 4 // generous, whatever Options sets
	maxWASIRandBytes = 4096            // wasi random_get buffer
	maxNameBytes     = 64              // leaderboard and recent names, room for multi-byte runes
	// maxScore keeps a hostile score from overflowing anything that sums it.
	maxScore = int64(1) << 53
	// maxLcdLineBytes leaves room for a padded 16-rune line with multi-byte
	// glyphs; the LCD clips to 16 columns downstream.
	maxLcdLineBytes = 64
)

// LogLevel is host_log's level argument.
type LogLevel uint32

const (
	LogDebug LogLevel = 0
	LogInfo  LogLevel = 1
	LogWarn  LogLevel = 2
	LogError LogLevel = 3
)

func (l LogLevel) String() string {
	switch l {
	case LogDebug:
		return "debug"
	case LogInfo:
		return "info"
	case LogWarn:
		return "warn"
	case LogError:
		return "error"
	default:
		return "unknown"
	}
}

// Logger receives the cartridge's log lines and the bridge's sandbox warnings.
type Logger interface {
	Log(level LogLevel, msg string)
}

// Surface is the framebuffer the drawing imports write. Colors arrive already
// unpacked from the wire's 0xRRGGBBAA.
type Surface interface {
	Blit(pixels []display.Pixel) // len == cols*rows, row-major
	DrawPixel(x, y int, c display.Pixel)
	FillRect(x, y, w, h int, c display.Pixel)
	LoadSprite(png []byte) (id uint32, err error)
	DrawSprite(id uint32, x, y int)
	// DrawCel blends straight-alpha RGBA pix, len w*h*4 and already mirrored,
	// at (x, y) with exactly DrawSprite's blend.
	DrawCel(pix []byte, w, h, x, y int)
}

type Bundle interface {
	// BundleAsset reads one bundle-relative path verbatim, as LoadSound
	// resolves it. ok is false when the file or the whole bundle is absent.
	BundleAsset(path string) ([]byte, bool)
}

type Speaker interface {
	LoadSound(path string) (id uint32, err error)
	PlaySound(id uint32, loopCount uint32)
	StopSound(id uint32)
	StopAllSounds()
}

// Storage is the cartridge's key-value persistence; the Host namespaces keys
// per cartridge.
type Storage interface {
	StorageRead(key string) (data []byte, ok bool)
	StorageWrite(key string, val []byte)
}

// Scoreboard is the cabinet's top-10 table for the calling cartridge only,
// and the per-seat recent names that prefill name entry. The host keeps
// internal/scores' policy because the shell and attract read the table from
// outside the game.
type Scoreboard interface {
	ScoreSubmit(score int, name string) (rank int)
	ScoreRead(rank int) (score int, name string, ok bool)
	ScoreCount() int
	ScoreQualifies(score int) bool
	ScoreRank(score int) int

	// RecentName is a physical seat's prefill, one name with a 1h TTL on the
	// engine clock. RecentNameAt walks the seat's MRU, up to five names
	// newest first, persisted without expiry.
	RecentName(player int) (string, bool)
	RecentNameAt(player, index int) (string, bool)
	NoteRecentName(player int, name string)
}

// Control is the cartridge's say over the engine around it: the player
// strategy that gates input and orients the display, raw directions, the
// reject bell and its own exit. A pushed strategy is permissive until locked.
type Control interface {
	PushPlayerStrategy(kind StrategyKind)
	PopPlayerStrategy()
	LockPlayerStrategy()
	UnlockPlayerStrategy()
	SwitchPlayer(mode SwitchMode)
	// LockPlayerToSeat locks the one-player strategy to physical seat 0 or 1,
	// whichever controller pressed first.
	LockPlayerToSeat(physical int)

	// SetRawDirections overrides the manifest's RawDirections until the app
	// unloads, so egocentric gameplay can keep screen-spatial menus.
	SetRawDirections(enabled bool)

	// RejectKey rings the engine's "that key does nothing here" bell.
	RejectKey()

	// Exit requests the cartridge's unload. It arrives mid guest call, so an
	// implementation only flags the request and never tears down here.
	Exit()
}

type LCD interface {
	SetText(seats input.Seats, line1, line2 string)
}

type Lamps interface {
	SetButtonLights(seats input.Seats, lights map[string]bool, only bool)
	SetLights(seats input.Seats, on bool)

	// SetLampLevels sets per-lamp brightness 0..15 for the lamps in touch,
	// cross-fading over fadeMs. touch arrives masked and non-zero, fadeMs
	// clamped; the fade is the implementation's.
	SetLampLevels(seats input.Seats, levels [6]uint8, touch uint8, fadeMs int)
}

// Session is the only time and randomness a cartridge may observe, and its log.
type Session interface {
	NowMicros() uint64
	RandomU64() uint64
	Logger
}

// Host is the engine services a cartridge's host_* imports bridge onto, one
// role per embedded interface. EngineHost is the production implementation.
//
// Methods are called synchronously from one guest call at a time, so need not
// be goroutine-safe. The bridge validates every argument first; where the ABI
// has no failure signal an implementation clips or no-ops. A LoadSprite or
// LoadSound error becomes a zero handle and a warning.
type Host interface {
	Surface
	Bundle
	Speaker
	Storage
	Scoreboard
	Control
	LCD
	Lamps
	Session
}

// GlyphDefiner is optionally implemented by a Host with character LCDs to
// page a cartridge's 5x8 glyphs into. host_define_lcd_glyph returns the same
// value whether or not the Host implements it.
type GlyphDefiner interface {
	// SetCartGlyphs receives the whole glyph table on every change and nil
	// at teardown, the hardware.GlyphPager contract.
	SetCartGlyphs(glyphs map[rune][8]byte)
}

// readBytes copies rather than aliases guest memory: the host never retains
// guest pointers past the call.
func readBytes(mod api.Module, ptr, length, max uint32) ([]byte, bool) {
	if length > max {
		return nil, false
	}
	buf, ok := mod.Memory().Read(ptr, length)
	if !ok {
		return nil, false
	}
	out := make([]byte, length)
	copy(out, buf)
	return out, true
}

// guestBytes is readBytes plus the sandbox warning, prefixed by what.
func guestBytes(log Logger, mod api.Module, what string, ptr, length, max uint32) ([]byte, bool) {
	b, ok := readBytes(mod, ptr, length, max)
	if !ok {
		hostWarnf(log, "%s ptr %#x/%d out of bounds or over %d bytes", what, ptr, length, max)
	}
	return b, ok
}

func guestString(log Logger, mod api.Module, what string, ptr, length, max uint32) (string, bool) {
	b, ok := guestBytes(log, mod, what, ptr, length, max)
	return string(b), ok
}

// seatArg reads the ABI's seat address: -1 is both seats, 0 and 1 one each.
func seatArg(log Logger, what string, v int32) (input.Seats, bool) {
	switch v {
	case -1:
		return input.BothSeats, true
	case 0, 1:
		return input.SeatsOf(input.SeatOf(int(v))), true
	}
	hostWarnf(log, "%s %d out of range [-1,1]", what, v)
	return input.NoSeats, false
}

func physicalSeatArg(log Logger, what string, v int32) (int, bool) {
	if v < 0 || v > 1 {
		hostWarnf(log, "%s %d out of range [0,1]", what, v)
		return 0, false
	}
	return int(v), true
}

// hostWarnf tags a rejected call apart from the cartridge's own log lines.
func hostWarnf(log Logger, format string, args ...any) {
	log.Log(LogWarn, "[cartridge sandbox] "+fmt.Sprintf(format, args...))
}

// bridge is one cartridge instance's import state and registries, each
// holding only the Host roles it serves.
type bridge struct {
	dims frameDims

	surface  surfaceImports
	speaker  speakerImports
	storage  storageImports
	scores   scoreImports
	control  *controlImports
	lcd      lcdImports
	session  sessionImports
	glyphs   *glyphRegistry
	sheets   *sheetRegistry
	channels *channelRegistry
	lights   *lightsRegistry
	mic      *micRegistry
}

func newBridge(host Host, dims frameDims, opts Options) *bridge {
	glyphs := newGlyphRegistry(host)
	return &bridge{
		dims:     dims,
		surface:  surfaceImports{host: host, dims: dims},
		speaker:  speakerImports{host: host},
		storage:  storageImports{host: host},
		scores:   scoreImports{host: host},
		control:  &controlImports{host: host},
		lcd:      lcdImports{host: host, glyphs: glyphs},
		session:  sessionImports{host: host},
		glyphs:   glyphs,
		sheets:   newSheetRegistry(host),
		channels: newChannelRegistry(host, opts.Recorder, opts.Replay),
		lights:   newLightsRegistry(host),
		mic:      newMicRegistry(host),
	}
}
