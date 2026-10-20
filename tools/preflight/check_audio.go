// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"os"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// Measurement bands in Hz over the 20-10000 reference range.
const (
	audioRefLo = 20.0
	audioRefHi = 10000.0

	// The sub-bass top is audioFloorModel.hz.
	subBassLo          = 20.0
	midLo, midHi       = 200.0, 2000.0
	trebleLo, trebleHi = 2000.0, 10000.0
)

// Defaults for the speaker model: the top of the sub-bass band the cabinet
// cannot reproduce, and the largest energy fraction allowed inside it. They
// are calibrated to the reference cabinet and overridable by flag or by a
// cartridge's preflight.json.
const (
	audioDefaultFloorHz   = 150.0
	audioDefaultFloorFrac = 0.5
)

// audioFloorModel is a resolved speaker model.
type audioFloorModel struct {
	hz   float64
	frac float64
}

var referenceAudioFloor = audioFloorModel{hz: audioDefaultFloorHz, frac: audioDefaultFloorFrac}

// audioFloor is the speaker model for this cartridge: the scan options (the
// reference cabinet when unset) under its preflight.json override.
func (c *Cart) audioFloor() audioFloorModel {
	m := c.Options.AudioFloor
	if m == (audioFloorModel{}) {
		m = referenceAudioFloor
	}
	cfg, err := c.Config()
	if err != nil || cfg == nil {
		return m
	}
	return cfg.WavFloor.over(m)
}

// A sound warns only when under this fraction in both the mid and treble
// bands. Requiring body in the mid band alone flagged bright sounds that are
// clear on the cabinet.
const (
	midFloor    = 0.3
	trebleFloor = 0.3
)

// spectrumChecks expresses the model as an sfxlib spectrum assertion.
func (m audioFloorModel) spectrumChecks() []sfxlib.SpectrumCheck {
	return []sfxlib.SpectrumCheck{
		{Name: fmt.Sprintf("sub-%.0fHz", m.hz), Lo: subBassLo, Hi: m.hz, Max: m.frac},
	}
}

// nearSilentPeak is the peak amplitude below which a sound is effectively
// silent; sfxlib generators normalize to about 0.9.
const nearSilentPeak = 0.05

// checkWavAudibility runs sfxlib's framed spectrum analysis over every shipped
// WAV. Framing is needed because whole-signal analysis is meaningless for
// multi-second loops.
//
// Spectral findings only warn, since the model is a guess at the speaker.
// Unreadable, undecodable or empty WAVs fail.
func checkWavAudibility(c *Cart, r *Result) {
	wavs, err := c.AssetFiles(".wav")
	if err != nil {
		r.fail("", Loc{}, "walking assets/: %v", err)
		return
	}
	if len(wavs) == 0 {
		r.skip("no WAVs under %s/assets", c.Rel(c.Dir))
		return
	}
	model := c.audioFloor()
	checks := model.spectrumChecks()
	band := fmt.Sprintf("sub-%.0fHz", model.hz)

	bad := 0
	for _, path := range wavs {
		where := c.At(path)
		raw, err := os.ReadFile(path)
		if err != nil {
			r.fail("", where, "unreadable: %v", err)
			bad++
			continue
		}
		wav, err := sfxlib.Decode(raw)
		if err != nil {
			r.fail("", where, "undecodable: %v — the host's beep mixer will not play this either", err)
			bad++
			continue
		}
		if len(wav.Samples) == 0 {
			r.fail(ruleAudible, where, "no audio samples at all")
			bad++
			continue
		}
		if peak := wav.Peak(); peak < nearSilentPeak {
			r.warn(ruleAudible, where, "peak amplitude %.3f — effectively silent (sfxlib's generators normalize to ~0.9)", peak)
			bad++
			continue
		}

		fs := sfxlib.FrameSignal(wav.Samples, wav.SampleRate, 0, 0)
		low := fs.BandFraction(subBassLo, model.hz, audioRefLo, audioRefHi)
		mid := fs.BandFraction(midLo, midHi, audioRefLo, audioRefHi)
		treble := fs.BandFraction(trebleLo, trebleHi, audioRefLo, audioRefHi)
		spectrum := fmt.Sprintf("%.0f%% %s / %.0f%% 200-2kHz / %.0f%% 2-10kHz, %.2fs over %d frames",
			100*low, band, 100*mid, 100*treble, wav.Duration(), fs.Count())

		if err := fs.AssertSpectrum(audioRefLo, audioRefHi, checks); err != nil {
			r.warn(ruleAudible, where, "most of its energy is under the speaker model's floor — %s; audition it on the cabinet before changing anything (the model is calibrated to the reference cabinet, and speakers vary)", spectrum)
			bad++
			continue
		}
		if mid < midFloor && treble < trebleFloor {
			r.warn(ruleAudible, where, "energy sits in neither usable band — %s; it will read thinner on the cabinet than in the workshop", spectrum)
			bad++
			continue
		}
		r.info(where, "%s, peak %.2f", spectrum, wav.Peak())
	}

	if bad == 0 {
		r.summarize("%d WAV(s) clear the %.0f Hz / %.0f%% floor", len(wavs), model.hz, 100*model.frac)
	} else {
		r.summarize("%d of %d WAV(s) sit under the %.0f Hz / %.0f%% floor — worth an audition, not a blocker", bad, len(wavs), model.hz, 100*model.frac)
	}
}

const ruleAudible = "docs/GAME_AUTHORING_AI.md 'Peripherals & sound': the cabinet's small speakers cannot reproduce sub-bass, so a sound with most of its energy below ~150 Hz plays correctly through the whole chain and is simply inaudible"
