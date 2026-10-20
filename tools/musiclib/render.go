// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"math"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// DefaultPeak is the default normalization target, leaving headroom for sounds
// mixed concurrently by the host.
const DefaultPeak = 0.9

const (
	defaultAttackMs  = 2.0
	defaultReleaseMs = 10.0
)

// WriteLoopWAV renders song at sampleRate and writes it as a 16-bit mono PCM
// WAV to path.
func WriteLoopWAV(path string, song Song, sampleRate int) error {
	return sfxlib.WriteFile(path, RenderLoop(song, sampleRate), sampleRate)
}

// RenderLoop renders song at sampleRate into exactly the song's loop length,
// normalized to song.PeakLevel(). The result loops seamlessly and is
// deterministic.
func RenderLoop(song Song, sampleRate int) []float64 {
	return RenderLoopPeak(song, sampleRate, song.PeakLevel())
}

// RenderLoopPeak is RenderLoop with an explicit normalization peak; peak <= 0
// returns the raw mix.
func RenderLoopPeak(song Song, sampleRate int, peak float64) []float64 {
	spb := 60 / song.BPM * float64(sampleRate)
	total := int(math.Round(song.LengthBeats() * spb))
	if total <= 0 {
		return nil
	}
	buf := make([]float64, total)
	for ti := range song.Tracks {
		tr := &song.Tracks[ti]
		trGain := tr.Gain
		if trGain == 0 {
			trGain = 1
		}
		for ni := range tr.Notes {
			renderNote(buf, sampleRate, spb, ti, ni, tr.Inst, trGain, tr.Notes[ni])
		}
	}
	applyEcho(buf, song, sampleRate)
	if peak > 0 {
		sfxlib.Normalize(buf, peak)
	}
	return buf
}

// applyEcho adds the song's echo in place, summing taps until the gain falls
// below 0.002 with indices wrapped modulo the loop length.
func applyEcho(buf []float64, song Song, sampleRate int) {
	e := song.Echo
	if e.DelayBeats <= 0 || e.Mix <= 0 {
		return
	}
	n := len(buf)
	if n == 0 {
		return
	}
	spb := 60 / song.BPM * float64(sampleRate)
	d := int(math.Round(e.DelayBeats * spb))
	if d < 1 {
		d = 1
	}
	fb := e.Feedback
	if fb < 0 {
		fb = 0
	}
	if fb > 0.95 {
		fb = 0.95
	}
	dry := make([]float64, n)
	copy(dry, buf)
	gain := e.Mix
	for t := 1; gain >= 0.002; t++ {
		off := (t * d) % n
		for i := range buf {
			buf[i] += gain * dry[((i-off)%n+n)%n]
		}
		gain *= fb
		if fb == 0 {
			break
		}
	}
}

func renderNote(buf []float64, sampleRate int, spb float64, ti, ni int, inst Instrument, trGain float64, n Note) {
	start := int(math.Round(n.Start * spb))
	dur := int(math.Round(n.Dur * spb))
	if dur < 1 {
		return
	}

	amp := n.Vel
	if amp == 0 {
		amp = 1
	}
	g := inst.Gain
	if g == 0 {
		g = 1
	}
	amp *= g * trGain

	env := noteEnvelope(inst, sampleRate, dur)

	switch inst.Wave {
	case WaveNoise:
		renderNoise(buf, start, dur, n.NoiseSeed(ti, ni), inst, amp, env, n.Midi)
	default:
		renderTone(buf, sampleRate, start, dur, inst, amp, env, n)
	}
}

// envelopeFn returns the envelope value in [0,1] for sample i of a note.
type envelopeFn func(i int) float64

// noteEnvelope builds the note's ADSR with the release finishing inside dur;
// when attack and release together exceed dur, both shrink.
func noteEnvelope(inst Instrument, sampleRate, dur int) envelopeFn {
	ms := func(v, def float64) int {
		if v == 0 {
			v = def
		}
		return sfxlib.MsToSamples(sampleRate, int(math.Round(v)))
	}
	a := ms(inst.AttackMs, defaultAttackMs)
	d := ms(inst.DecayMs, 0)
	r := ms(inst.ReleaseMs, defaultReleaseMs)
	sus := inst.SustainLevel
	if sus == 0 {
		sus = 1
	}
	if a+r >= dur {
		scale := float64(dur) / float64(a+r+1)
		a = int(float64(a) * scale)
		r = dur - a - 1
		if r < 0 {
			r = 0
		}
	}
	gate := dur - r
	return func(i int) float64 {
		if i < 0 || i >= dur {
			return 0
		}
		var v float64
		switch {
		case i < a:
			v = float64(i) / float64(a)
		case d > 0 && i < a+d:
			frac := float64(i-a) / float64(d)
			v = 1 + (sus-1)*frac
		default:
			v = sus
			if d == 0 {
				v = sus
			}
		}
		if i >= gate {
			rel := float64(i-gate) / float64(dur-gate)
			v *= 1 - rel
		}
		return v
	}
}

// renderTone synthesizes a pulse or triangle note, starting at phase 0.
func renderTone(buf []float64, sampleRate, start, dur int, inst Instrument, amp float64, env envelopeFn, n Note) {
	total := len(buf)
	duty := inst.Duty
	if duty <= 0 || duty >= 1 {
		duty = 0.5
	}
	baseFreq := MidiFreq(float64(n.Midi))
	lpA := 0.0
	if inst.LowpassHz > 0 {
		lpA = 1 - math.Exp(-2*math.Pi*inst.LowpassHz/float64(sampleRate))
	}
	lpY := 0.0
	ratios := []float64{1}
	if inst.DetuneCents > 0 {
		d := math.Pow(2, inst.DetuneCents/1200)
		ratios = []float64{1, d, 1 / d}
	}
	phs := make([]float64, len(ratios))
	voiceAmp := 1 / float64(len(ratios))
	for i := 0; i < dur; i++ {
		f := baseFreq
		if inst.VibratoHz > 0 && inst.VibratoSemis > 0 {
			t := float64(i) / float64(sampleRate)
			f = MidiFreq(float64(n.Midi) + inst.VibratoSemis*math.Sin(2*math.Pi*inst.VibratoHz*t))
		}
		v := 0.0
		for vi, ratio := range ratios {
			ph := phs[vi]
			switch inst.Wave {
			case WaveTriangle:
				v += voiceAmp * trianglePhase(ph)
			default:
				if ph < duty {
					v += voiceAmp
				} else {
					v -= voiceAmp
				}
			}
			ph += f * ratio / float64(sampleRate)
			if ph >= 1 {
				ph -= math.Floor(ph)
			}
			phs[vi] = ph
		}
		if lpA > 0 {
			lpY += lpA * (v - lpY)
			v = lpY
		}
		j := start + i
		buf[((j%total)+total)%total] += amp * env(i) * v
	}
}

// trianglePhase evaluates a unit triangle at phase ph in [0,1), phase-aligned
// with a sine so the first sample is 0.
func trianglePhase(ph float64) float64 {
	switch {
	case ph < 0.25:
		return 4 * ph
	case ph < 0.75:
		return 2 - 4*ph
	default:
		return 4*ph - 4
	}
}

// renderNoise synthesizes an LFSR noise note from the given start state.
func renderNoise(buf []float64, start, dur int, lfsr uint16, inst Instrument, amp float64, env envelopeFn, midi int) {
	total := len(buf)
	// LFSR clock as a fraction of the sample rate: full at MIDI 60, halved per octave down.
	step := math.Pow(2, float64(midi-60)/12)
	if step > 1 {
		step = 1
	}
	acc := 0.0
	cur := lfsrOut(lfsr)
	lp := 0.0
	const lpCoef = 0.2
	tone := inst.NoiseTone
	for i := 0; i < dur; i++ {
		acc += step
		for acc >= 1 {
			acc -= 1
			// Taps 16,14,13,11.
			bit := ((lfsr >> 0) ^ (lfsr >> 2) ^ (lfsr >> 3) ^ (lfsr >> 5)) & 1
			lfsr = (lfsr >> 1) | (bit << 15)
			cur = lfsrOut(lfsr)
		}
		x := cur
		lp += lpCoef * (x - lp)
		hp := x - lp
		v := x
		if tone > 0 {
			v = x + tone*(hp-x)
		} else if tone < 0 {
			v = x + (-tone)*(lp-x)
		}
		j := start + i
		buf[((j%total)+total)%total] += amp * env(i) * v
	}
}

func lfsrOut(l uint16) float64 {
	if l&1 == 1 {
		return 1
	}
	return -1
}
