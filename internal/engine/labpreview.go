// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// LabPreviewOwner is the audio owner for every preview sound. It is not an app slug, so stopping it
// cannot stop a real app's sound.
const LabPreviewOwner = "preview/owner"

// labPreviewLabel is the audio label within LabPreviewOwner; one label, so a second sound replaces the first.
const labPreviewLabel = "preview"

const (
	// DefaultLabPreviewTimeout is how long a preview holds the peripherals when the request does not say.
	DefaultLabPreviewTimeout = 10 * time.Second
	// MinLabPreviewTimeout floors a request's timeout.
	MinLabPreviewTimeout = 500 * time.Millisecond
	// MaxLabPreviewTimeout caps it; longer should be a second request, so a forgotten preview expires.
	MaxLabPreviewTimeout = time.Minute
)

const (
	// DefaultLabPreviewFrameDelay is the per-frame hold when a frame carries none (10fps).
	DefaultLabPreviewFrameDelay = 100 * time.Millisecond
	// MinLabPreviewFrameDelay floors a frame's hold. Frames advance on engine ticks (33ms at 30fps), so
	// below one tick means a new frame every tick.
	MinLabPreviewFrameDelay = 10 * time.Millisecond
	// MaxLabPreviewFrameDelay caps a frame's hold at the longest preview.
	MaxLabPreviewFrameDelay = MaxLabPreviewTimeout
)

// DefaultLabPreviewGain is the attenuation applied when the request chooses none: below full scale
// because lab renders are normalized to 0dBFS and the speakers are close to the player. The master
// volume applies on top.
const DefaultLabPreviewGain = 0.7

// LabPreviewFrame is one cel of a matrix preview: a full panel-sized frame
// and how long to hold it before the next one.
type LabPreviewFrame struct {
	// Data is a display-sized matrix; extra rows and columns are ignored and missing ones left black.
	Data display.Matrix
	// Delay is how long this frame holds; zero means DefaultLabPreviewFrameDelay, others are clamped.
	// Ignored for a single-frame preview.
	Delay time.Duration
}

// LabPreviewLCD is the character-LCD half of a preview.
type LabPreviewLCD struct {
	// Lines is [seat][row]. Both seats are always written so the other panel never shows the game underneath.
	Lines [2][2]string
	// Glyphs are custom 5x8 bitmaps paged into CGRAM for the duration (hardware.GlyphPager); a listed rune
	// temporarily shadows its shipped bitmap. nil for a text-only preview.
	Glyphs map[rune]hardware.GlyphRows
}

// LabPreviewSound is the speaker half of a preview.
type LabPreviewSound struct {
	// WAV is a complete RIFF/WAVE file, handed untouched to audio.Service.PlayBytes, which rejects non-WAV.
	WAV []byte
	// Gain is a linear 0..1 attenuation; zero means DefaultLabPreviewGain, so a silent preview leaves Sound nil.
	Gain float64
}

// LabPreview is one preview request: any combination of panel frames, LCD text and a sound, held for one
// shared timeout; an empty request is logged and dropped. It is an overlay, never a takeover: it loads,
// suspends and injects nothing, leaves the standby machine alone (a dark cabinet is lit for the preview,
// then goes dark again), and always ends on an engine-enforced deadline. Frames bypass the compositor in
// paint, LCD text is parked in the hardware shadow, and the WAV mixes alongside the app's audio.
type LabPreview struct {
	// Frames is one frame for a still, several for an animation; empty leaves the panel alone.
	Frames []LabPreviewFrame
	// Loop repeats a multi-frame sequence until the timeout instead of holding the last frame.
	Loop bool
	// LCD is the character-LCD preview, nil to leave the panels alone.
	LCD *LabPreviewLCD
	// Sound is the speaker preview, nil for a silent one.
	Sound *LabPreviewSound
	// Timeout is how long the preview holds; zero means DefaultLabPreviewTimeout, others are clamped.
	Timeout time.Duration
}

// empty reports whether this request would preview nothing at all.
func (p LabPreview) empty() bool {
	return len(p.Frames) == 0 && p.LCD == nil && p.Sound == nil
}

// labPreviewState is the live preview, owned by the engine goroutine.
type labPreviewState struct {
	frames []LabPreviewFrame
	loop   bool
	// idx is the frame on the panel; advanceAt is when it is due to be replaced.
	idx       int
	advanceAt time.Time
	// lcdHeld/glyphsSet/soundPlaying record what this preview took, so ending it returns exactly those.
	lcdHeld      bool
	glyphsSet    bool
	soundPlaying bool
	deadline     time.Time
}

// hasFrames reports whether this preview owns the LED panel.
func (s *labPreviewState) hasFrames() bool { return s != nil && len(s.frames) > 0 }

// frame returns the matrix currently due on the panel.
func (s *labPreviewState) frame() display.Matrix { return s.frames[s.idx].Data }

// RequestLabPreview queues a preview for the top of the next tick. Safe from any goroutine. The latest
// request wins, in the queue and on the cabinet: applying one ends the preview already up. The engine
// keeps its own copy, so the caller may reuse the request's slices and maps at once.
func (rt *Runtime) RequestLabPreview(p LabPreview) {
	p = p.clone()
	rt.requests.labPreview.Set(&p)
}

func (p LabPreview) clone() LabPreview {
	if p.Frames != nil {
		frames := make([]LabPreviewFrame, len(p.Frames))
		for i, f := range p.Frames {
			frames[i] = LabPreviewFrame{Data: display.CloneMatrix(f.Data), Delay: f.Delay}
		}
		p.Frames = frames
	}
	if p.LCD != nil {
		lcd := *p.LCD
		lcd.Glyphs = maps.Clone(lcd.Glyphs)
		p.LCD = &lcd
	}
	if p.Sound != nil {
		sound := *p.Sound
		sound.WAV = slices.Clone(sound.WAV)
		p.Sound = &sound
	}
	return p
}

// CancelLabPreview queues an immediate end to any preview, applied at the top of the next tick. Safe
// from any goroutine; harmless when nothing is previewing.
func (rt *Runtime) CancelLabPreview() { rt.requests.labPreview.Set(nil) }

// applyLabPreview applies one drained request; nil is a cancel.
func (rt *Runtime) applyLabPreview(now time.Time, p *LabPreview) {
	if p == nil {
		rt.endLabPreview()
		rt.Log("info", "lab: preview cancelled, cabinet restored")
		return
	}
	rt.enterLabPreview(now, *p)
}

// LabPreviewActive reports whether a preview owns any peripheral. Engine-goroutine only; HTTP callers
// read Status().LabPreviewActive.
func (rt *Runtime) LabPreviewActive() bool { return rt.labPreview != nil }

// ClampLabPreviewTimeout applies the request's timeout bounds; zero means the default. Exported so
// internal/net can echo the timeout the engine will use.
func ClampLabPreviewTimeout(d time.Duration) time.Duration {
	if d == 0 {
		return DefaultLabPreviewTimeout
	}
	return max(MinLabPreviewTimeout, min(MaxLabPreviewTimeout, d))
}

// clampLabPreviewDelay applies a frame's delay bounds; zero means the
// default.
func clampLabPreviewDelay(d time.Duration) time.Duration {
	if d == 0 {
		return DefaultLabPreviewFrameDelay
	}
	return max(MinLabPreviewFrameDelay, min(MaxLabPreviewFrameDelay, d))
}

// enterLabPreview applies one drained request, on the engine goroutine.
func (rt *Runtime) enterLabPreview(now time.Time, p LabPreview) {
	// Replace rather than stack, so every take of a peripheral pairs with one give-back.
	rt.endLabPreview()

	if p.empty() {
		rt.Log("warn", "lab: preview request has nothing to show, ignored")
		return
	}

	st := &labPreviewState{
		loop:     p.Loop,
		deadline: now.Add(ClampLabPreviewTimeout(p.Timeout)),
	}
	var took []string

	if len(p.Frames) > 0 {
		st.frames = p.Frames
		st.advanceAt = now.Add(clampLabPreviewDelay(p.Frames[0].Delay))
		// A goodnight sequence still running is abandoned, not resumed, when the preview ends.
		rt.darkFade.cancel()
		took = append(took, fmt.Sprintf("%d frame(s)", len(p.Frames)))
	}

	if p.LCD != nil {
		if len(p.LCD.Glyphs) > 0 {
			rt.setPreviewGlyphs(p.LCD.Glyphs)
			st.glyphsSet = true
			took = append(took, fmt.Sprintf("%d glyph(s)", len(p.LCD.Glyphs)))
		}
		// Hold before writing so an app writing its LCDs later this tick cannot land on the preview text.
		rt.hwShadow.holdText()
		st.lcdHeld = true
		for seat, lines := range p.LCD.Lines {
			rt.hwShadow.overrideText(seat, lines[0], lines[1])
		}
		// A preview can land on a dark cabinet; endLabPreview restores the backlight to the standby state.
		rt.setBacklight(true)
		took = append(took, "lcd")
	}

	if p.Sound != nil {
		gain := p.Sound.Gain
		if gain == 0 {
			gain = DefaultLabPreviewGain
		}
		if err := rt.Audio.PlayBytes(LabPreviewOwner, labPreviewLabel, p.Sound.WAV, gain); err != nil {
			rt.Log("warn", fmt.Sprintf("lab: preview sound rejected: %v", err))
		} else {
			st.soundPlaying = true
			took = append(took, fmt.Sprintf("%d byte wav @%.2f gain", len(p.Sound.WAV), gain))
		}
	}

	if len(took) == 0 {
		// Everything requested was refused (a WAV that did not decode); install no state that owns nothing.
		return
	}
	rt.labPreview = st
	rt.Log("info", fmt.Sprintf("lab: preview for %s: %s",
		st.deadline.Sub(now).Round(time.Millisecond), strings.Join(took, ", ")))
}

// setPreviewGlyphs installs the CGRAM overlay on suites that have one.
func (rt *Runtime) setPreviewGlyphs(glyphs map[rune]hardware.GlyphRows) {
	if g, ok := rt.Hardware.(hardware.GlyphPager); ok {
		g.SetPreviewGlyphs(glyphs)
	}
}

// stepLabPreview advances the frame sequence and expires the preview; called once per tick before
// anything paints.
func (rt *Runtime) stepLabPreview(now time.Time) {
	st := rt.labPreview
	if st == nil {
		return
	}
	if !now.Before(st.deadline) {
		rt.endLabPreview()
		rt.Log("info", "lab: preview timed out, cabinet restored")
		return
	}
	if len(st.frames) < 2 || now.Before(st.advanceAt) {
		return
	}
	next := st.idx + 1
	if next >= len(st.frames) {
		if !st.loop {
			// Hold the last frame for the rest of the timeout; a non-looping sequence ends on its final cel.
			st.advanceAt = st.deadline
			return
		}
		next = 0
	}
	st.idx = next
	st.advanceAt = now.Add(clampLabPreviewDelay(st.frames[next].Delay))
}

// endLabPreview gives back every peripheral this preview took and clears the
// state. A no-op when nothing is previewing, so callers never need to check.
func (rt *Runtime) endLabPreview() {
	st := rt.labPreview
	if st == nil {
		return
	}
	rt.labPreview = nil

	if st.lcdHeld {
		// Replay the app's view of the panels and hand the backlight back to standby, so a preview that lit a
		// dark cabinet does not leave it lit.
		rt.hwShadow.releaseText()
		rt.setBacklight(rt.standby.state != StateDark)
	}
	if st.glyphsSet {
		rt.setPreviewGlyphs(nil)
	}
	if st.soundPlaying {
		rt.Audio.Stop(LabPreviewOwner, labPreviewLabel)
	}
	if st.hasFrames() && rt.cur == nil && rt.standby.state == StateDark {
		// DARK stops painting entirely (see enterDark), so blacken the panel by hand as the transition did.
		rt.Renderer.Paint(blackFrame(rt.Display.Size()), rt.Stats())
	}
}
