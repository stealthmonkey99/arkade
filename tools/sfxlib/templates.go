// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

// The seeds fix the noise grain of each template; set Patch.Seed to change it.
const (
	TapSeed    = 101
	ImpactSeed = 202
	ScrapeSeed = 303
)

// Tap is a struck pitched object: a decaying body under 6 ms of noise.
// hz is the body pitch, decay its rate (larger is faster).
func Tap(hz, decay, durationS float64) Patch {
	return Patch{
		Duration: durationS,
		Peak:     0.9,
		Seed:     TapSeed,
		Layers: []Layer{
			{Freq: hz, Gain: 0.8, Env: Env{Kind: EnvExpDecay, Rate: decay}},
			{
				Wave: WaveNoise, Gain: 0.35,
				Gate: &Gate{To: 0.006},
				Env:  Env{Kind: EnvExpDecay, Rate: 300},
			},
		},
	}
}

// Tom is one pitched drum body inside an Impact.
type Tom struct {
	Hz, Gain, Decay float64
}

// Impact is a band-passed noise burst with pitched bodies under it. The
// cabinet's speakers cannot reproduce sub-bass, so the noise sits in the mid
// band and the toms stay above roughly 100 Hz.
func Impact(bandHz, q, noiseDecay float64, toms []Tom, durationS float64) Patch {
	layers := make([]Layer, 0, 1+len(toms))
	layers = append(layers, Layer{
		Wave: WaveNoise,
		Env:  Env{Kind: EnvExpDecay, Rate: noiseDecay},
		Filter: Filter{
			Kind: FilterBandPass, Hz: bandHz, Q: q, Gain: 2.2, BeforeEnv: true,
		},
	})
	for _, t := range toms {
		layers = append(layers, Layer{
			Freq: t.Hz, Gain: t.Gain,
			Env: Env{Kind: EnvExpDecay, Rate: t.Decay},
		})
	}
	return Patch{Duration: durationS, Peak: 0.9, Seed: ImpactSeed, Layers: layers}
}

// Sweep is one tone gliding from fromHz to toHz.
func Sweep(fromHz, toHz, decay, durationS float64) Patch {
	return Patch{
		Duration: durationS,
		Peak:     0.9,
		Layers: []Layer{{
			Freq: fromHz, SlideTo: toHz,
			Env: Env{Kind: EnvExpDecay, Rate: decay},
		}},
	}
}

// BonkLowHz and BonkHighHz are a semitone apart, close enough to sound wrong
// without sounding broken.
const (
	BonkLowHz  = 220.0
	BonkHighHz = 233.08
)

// Bonk is two close pitches held flat and cut off without a ring.
func Bonk(lowHz, highHz float64, wave Wave) Patch {
	return Patch{
		Duration: 0.15,
		Peak:     0.9,
		Layers: []Layer{{
			Wave: wave, Freq: lowHz,
			Partials: []Partial{{Hz: highHz, Gain: 1, Wave: wave}},
			Env: Env{Kind: EnvADSR, ADSR: ADSR{
				AttackS: 0.004, DecayS: 0.02, SustainS: 0.09,
				ReleaseS: 0.05, SustainLevel: 0.8,
			}},
		}},
	}
}

// Scrape is noise through a band-pass at hz with a swelling, fading envelope.
func Scrape(hz, q, durationS float64) Patch {
	return Patch{
		Duration: durationS,
		Peak:     0.6,
		Seed:     ScrapeSeed,
		Layers: []Layer{{
			Wave:   WaveNoise,
			Filter: Filter{Kind: FilterBandPass, Hz: hz, Q: q},
			Env: Env{Kind: EnvADSR, ADSR: ADSR{
				AttackS: 0.01, DecayS: 0.02, SustainS: 0.01,
				ReleaseS: 0.02, SustainLevel: 0.75,
			}},
		}},
	}
}

// Note is one step of a Motif. Chord holds extra partials; Run gives the last
// note MotifChord.
type Note struct {
	Delay, Duration, Hz float64
	Wave                Wave
	Level               float64
	Chord               []Partial
}

// MotifChord is a fifth above and an octave below, quieter than the note.
var MotifChord = []Partial{
	{Ratio: 1.5, Gain: 0.22},
	{Ratio: 0.5, Gain: 0.18},
}

// Motif places notes on one timeline; decay is each note's decay rate.
func Motif(notes []Note, decay float64) Patch {
	layers := make([]Layer, len(notes))
	end := 0.0
	for i, n := range notes {
		layers[i] = Layer{
			Delay: n.Delay, Duration: n.Duration,
			Wave: n.Wave, Freq: n.Hz, Level: n.Level,
			Partials: n.Chord,
			Env:      Env{Kind: EnvExpDecay, Rate: decay},
		}
		if e := n.Delay + n.Duration; e > end {
			end = e
		}
	}
	return Patch{Duration: end, Peak: 0.9, Layers: layers}
}

// Run lays pitches out back to back, each beat long. The last note is
// lastScale beats long and carries MotifChord.
func Run(hz []float64, beat, lastScale float64, wave Wave) []Note {
	out := make([]Note, len(hz))
	at := 0.0
	for i, f := range hz {
		d := beat
		if i == len(hz)-1 {
			d = beat * lastScale
		}
		out[i] = Note{Delay: at, Duration: d, Hz: f, Wave: wave}
		if i == len(hz)-1 {
			out[i].Chord = MotifChord
		}
		at += d
	}
	return out
}
