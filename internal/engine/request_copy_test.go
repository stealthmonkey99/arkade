// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func TestLabPreviewIgnoresTheCallersLaterEdits(t *testing.T) {
	rig := newPreviewRig(t, nil)
	rig.Tick(2)
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	glyph := hardware.GlyphRows{0x04, 0x0e, 0x1f, 0x0e, 0x04, 0, 0, 0}
	frames := []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}}
	lcd := &engine.LabPreviewLCD{
		Lines:  [2][2]string{{"◆", ""}, {"◆", ""}},
		Glyphs: map[rune]hardware.GlyphRows{'◆': glyph},
	}
	wav := []byte("RIFF....WAVEfmt ")
	sound := &engine.LabPreviewSound{WAV: wav, Gain: 0.5}
	rig.RT.RequestLabPreview(engine.LabPreview{Frames: frames, LCD: lcd, Sound: sound, Timeout: time.Second})

	frames[0].Data[0][0] = display.Green
	frames[0] = engine.LabPreviewFrame{Data: solidFrame(t, rig.RT, display.Green)}
	lcd.Glyphs['◆'] = hardware.GlyphRows{}
	lcd.Glyphs['◇'] = glyph
	sound.WAV = wav[:4]
	rig.Tick(1)

	if got := rig.PixelAt(0, 0); got != display.Red {
		t.Errorf("panel pixel = %+v, want the requested red", got)
	}
	if got := hw.PreviewGlyphs(); len(got) != 1 || got['◆'] != glyph {
		t.Errorf("preview glyphs = %v, want only ◆ = %v", got, glyph)
	}
	var played int
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "play" && ev.Owner == engine.LabPreviewOwner {
			played = ev.Bytes
		}
	}
	if played != len(wav) {
		t.Errorf("played %d bytes, want the requested %d", played, len(wav))
	}
}

func TestLightShowIgnoresTheCallersLaterEdits(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(2)
	hw := virtualHW(t, rig)

	show := seats(hardware.AllLights, 0)
	steps := []engine.LightStep{{Seats: show}}
	rig.RT.RequestLightShow(engine.LightShow{Steps: steps, Timeout: time.Second})
	steps[0].Seats = seats(0, 0)
	rig.Tick(1)

	if got := litMask(hw, 0); got != show[0] {
		t.Errorf("seat 0 = %v, want the requested %v", got, show[0])
	}
}
