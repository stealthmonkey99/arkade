// SPDX-License-Identifier: GPL-2.0-or-later

// Package hardware models the cabinet peripherals. The pi subpackage drives
// the real ones; Virtual records state and renders it as text everywhere
// else.
package hardware

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// LcdCols matches the physical 16×2 character LCDs.
const LcdCols = 16

// PowerOffLine1 and PowerOffLine2 are the goodbye the LCDs hold once the
// software is gone, written by the power-off sequence and by suite teardown.
const (
	PowerOffLine1 = "Power OFF"
	PowerOffLine2 = "under table"
)

// LightButtons lists the per-player lit buttons in pin order; the index is the
// MCP23017 pin offset within the player's port.
var LightButtons = []string{"joya", "joyb", "joyc", "joyd", "joyaux1", "joyaux2"}

// ButtonLights is a partial per-button update, keyed by LightButtons names.
type ButtonLights map[string]bool

// Suite is the cabinet peripheral contract: dual 16x2 LCDs and per-player
// button lights. Index -1 addresses both players.
type Suite interface {
	SetText(index int, line1, line2 string)
	SetLights(index int, on bool)
	// SetButtonLights updates individual buttons; only=true turns every
	// unmentioned button off.
	SetButtonLights(index int, lights ButtonLights, only bool)
	// SetLampLevels sets a player's lamps to brightness levels, 0 to
	// LampLevelMax.
	SetLampLevels(index int, levels LampLevels)
	StatusLines() []string
	// SetFanConfig updates the cooling-fan threshold (°F) and mode; a no-op
	// on suites with no fan.
	SetFanConfig(thresholdF float64, mode FanMode)
	Close() error
}

// GlyphRows is one custom LCD glyph: eight 5-bit pixel rows, top to bottom,
// bit 4 the leftmost column, as in an HD44780 CGRAM upload.
type GlyphRows = [8]byte

// GlyphPager is optionally implemented by a Suite whose LCDs can page extra
// custom glyphs in at runtime over the built-in table.
//
// Two overlays stack over the built-in table, preview winning:
//
//	preview  (operator, one at a time, expires)
//	cart     (the loaded cartridge, dies with it)
//	built-in (compiled into the binary)
//
// Neither overlay changes the budget of 8 CGRAM slots per screen.
type GlyphPager interface {
	// SetPreviewGlyphs replaces the whole preview overlay. Runes not in the
	// map fall back to the layer below; a nil or empty map clears it.
	//
	// The change must show on the next text write even if the text is
	// unchanged, so an already-showing rune is re-uploaded.
	SetPreviewGlyphs(glyphs map[rune]GlyphRows)

	// SetCartGlyphs replaces the whole cartridge overlay under the same rules
	// as SetPreviewGlyphs. Callers pass the entire table each time, and nil
	// at teardown.
	SetCartGlyphs(glyphs map[rune]GlyphRows)
}

func buttonIndex(name string) int {
	for i, b := range LightButtons {
		if b == name {
			return i
		}
	}
	return -1
}

const motionTimeout = 5 * time.Second

// Virtual is the in-memory hardware suite. Its own methods are nil-safe;
// the microphone methods promoted from MicSlot need a non-nil suite.
type Virtual struct {
	mu        sync.Mutex
	lcds      [2][2]string
	lights    [2]LampLevels
	lastInput time.Time
	animFrame int
	// the two GlyphPager overlays, recorded for tests
	glyphs     map[rune]GlyphRows
	cartGlyphs map[rune]GlyphRows
	MicSlot

	backlight uint8
}

// NewVirtual builds a virtual hardware suite with the synthetic microphone
// attached; AttachMic(nil) removes it.
func NewVirtual() *Virtual {
	v := &Virtual{backlight: LampLevelMax} // the cabinet's backpacks power up lit
	v.AttachMic(NewVirtualMic())
	return v
}

func (v *Virtual) SetText(index int, line1, line2 string) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.lcds {
		if index >= 0 && index != i {
			continue
		}
		v.lcds[i][0] = clip(line1)
		v.lcds[i][1] = clip(line2)
	}
}

func (v *Virtual) SetLights(index int, on bool) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.lights {
		if index >= 0 && index != i {
			continue
		}
		for b := range v.lights[i] {
			v.lights[i][b] = lampLevel(on)
		}
	}
}

// SetButtonLights updates named buttons; only=true clears the rest first.
func (v *Virtual) SetButtonLights(index int, lights ButtonLights, only bool) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.lights {
		if index >= 0 && index != i {
			continue
		}
		if only {
			v.lights[i] = LampLevels{}
		}
		for name, on := range lights {
			if b := buttonIndex(name); b >= 0 {
				v.lights[i][b] = lampLevel(on)
			}
		}
	}
}

func (v *Virtual) SetLampLevels(index int, levels LampLevels) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.lights {
		if index >= 0 && index != i {
			continue
		}
		v.lights[i] = levels.Clamp()
	}
}

func lampLevel(on bool) uint8 {
	if on {
		return LampLevelMax
	}
	return 0
}

// Lines reports one player's two LCD lines as last written, or empty strings
// for an index outside 0..1.
func (v *Virtual) Lines(index int) (line1, line2 string) {
	if v == nil || index < 0 || index > 1 {
		return "", ""
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.lcds[index][0], v.lcds[index][1]
}

// SetPreviewGlyphs records the preview overlay; nothing renders it here.
func (v *Virtual) SetPreviewGlyphs(glyphs map[rune]GlyphRows) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.glyphs = copyGlyphs(glyphs)
}

func (v *Virtual) SetCartGlyphs(glyphs map[rune]GlyphRows) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cartGlyphs = copyGlyphs(glyphs)
}

func (v *Virtual) PreviewGlyphs() map[rune]GlyphRows {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return copyGlyphs(v.glyphs)
}

// CartGlyphs returns a copy of the cartridge overlay, or nil.
func (v *Virtual) CartGlyphs() map[rune]GlyphRows {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return copyGlyphs(v.cartGlyphs)
}

// copyGlyphs copies an overlay map, normalizing empty to nil.
func copyGlyphs(in map[rune]GlyphRows) map[rune]GlyphRows {
	if len(in) == 0 {
		return nil
	}
	out := make(map[rune]GlyphRows, len(in))
	for r, rows := range in {
		out[r] = rows
	}
	return out
}

func (v *Virtual) ButtonLit(index int, name string) bool {
	if v == nil {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	b := buttonIndex(name)
	if b < 0 || index < 0 || index > 1 {
		return false
	}
	return v.lights[index][b] > 0
}

func (v *Virtual) LampLevels(index int) LampLevels {
	if v == nil {
		return LampLevels{}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if index < 0 || index > 1 {
		return LampLevels{}
	}
	return v.lights[index]
}

// SetBacklightLevel records the backlight level, clamped to LampLevelMax.
func (v *Virtual) SetBacklightLevel(level uint8) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.backlight = min(level, LampLevelMax)
}

func (v *Virtual) BacklightLevel() uint8 {
	if v == nil {
		return 0
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.backlight
}

// InputSignal marks user presence and animates the motion glyph.
func (v *Virtual) InputSignal() {
	if v == nil {
		return
	}
	v.mu.Lock()
	v.lastInput = time.Now()
	v.mu.Unlock()
}

// SetFanConfig is a no-op; there is no fan relay.
func (v *Virtual) SetFanConfig(thresholdF float64, mode FanMode) {}

// LastMotion reports the last InputSignal time, this suite's only presence
// proxy, and whether one has been recorded. The timestamp is wall-clock, not
// the engine clock.
func (v *Virtual) LastMotion() (time.Time, bool) {
	if v == nil {
		return time.Time{}, false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.lastInput.IsZero() {
		return time.Time{}, false
	}
	return v.lastInput, true
}

var motionGlyphs = []string{"←", "↖", "↑", "↗", "→", "↘", "↓", "↙"}

func ansi(color, s string) string { return "\x1b[" + color + "m" + s + "\x1b[0m" }

// lightGlyph renders one button in its chalk color: hollow when dark,
// filled at full, half-filled for anything dimmed between.
func lightGlyph(level uint8, color string) string {
	glyph := "○"
	switch {
	case level >= LampLevelMax:
		glyph = "●"
	case level > 0:
		glyph = "◐"
	}
	return ansi(color, glyph)
}

// StatusLines renders the header: P2 lights, P2 LCD, P1 LCD, P1 lights, fan,
// motion, as four rows of blocks.
func (v *Virtual) StatusLines() []string {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	lightsBlock := func(p int) []string {
		l := v.lights[p]
		return []string{
			fmt.Sprintf("  P%d   ", p+1),
			fmt.Sprintf("%s %s %s  ", lightGlyph(l[0], "32"), lightGlyph(l[1], "31"), lightGlyph(l[4], "37")),
			fmt.Sprintf(" %s %s %s ", lightGlyph(l[2], "33"), lightGlyph(l[3], "34"), lightGlyph(l[5], "37")),
			"       ",
		}
	}
	lcdBlock := func(p int) []string {
		border := ansi("33", strings.Repeat("▄", LcdCols+2))
		top := ansi("43;30", " "+pad(v.lcds[p][0])+" ")
		bot := ansi("43;30", " "+pad(v.lcds[p][1])+" ")
		return []string{border, top, bot, ansi("33", strings.Repeat("▀", LcdCols+2))}
	}
	fanBlock := []string{ansi("31", "✻  "), "~°F", "   ", "   "}
	motionActive := time.Since(v.lastInput) < motionTimeout
	move := "⏾"
	color := "31"
	if motionActive {
		v.animFrame = (v.animFrame + 1) % len(motionGlyphs)
		move = motionGlyphs[v.animFrame]
		color = "32"
	}
	motionBlock := []string{ansi(color, move), " ", " ", " "}

	blocks := [][]string{
		lightsBlock(1), lcdBlock(1), {"   ", "   ", "   ", "   "},
		lcdBlock(0), lightsBlock(0), fanBlock, motionBlock,
	}
	rows := make([]string, 4)
	for r := range rows {
		parts := make([]string, len(blocks))
		for b, block := range blocks {
			parts[b] = block[r]
		}
		rows[r] = strings.Join(parts, " ")
	}
	return rows
}

func (v *Virtual) Close() error { return nil }

var (
	_ Suite           = (*Virtual)(nil)
	_ GlyphPager      = (*Virtual)(nil)
	_ BacklightDimmer = (*Virtual)(nil)
	_ BacklightReader = (*Virtual)(nil)
	_ MotionHistory   = (*Virtual)(nil)
	_ InputSpy        = (*Virtual)(nil)
	_ MicSource       = (*Virtual)(nil)
	_ MicBander       = (*Virtual)(nil)
)

func pad(s string) string {
	return clip(s) + strings.Repeat(" ", max(0, LcdCols-len([]rune(clip(s)))))
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > LcdCols {
		return string(r[:LcdCols])
	}
	return s
}
