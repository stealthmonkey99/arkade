// SPDX-License-Identifier: GPL-2.0-or-later

// Command gen-sfx synthesizes Catcher's five audio assets into
// sdk/tutorial/assets. Run it from the cartridge directory:
//
//	go run ./tools/gen-sfx      # from sdk/tutorial
//
// This is host-only, offline, build-time code — never compiled into the
// .wasm — so it is free to use math, sfxlib and musiclib. The cartridge
// runtime's golden rules about libm and allocation do not apply here.
//
// # Where these numbers came from
//
// Every cue below is an sfxlib.Patch — a sound written as data, a stack
// of layers over one clock — started from one of sfxlib's templates and
// tuned by ear in soundlab's SFX tab
// (arkade.build/dev/tools). That is honest for a structural reason: the
// tab renders a patch through sfxlib.Patch.Render, the exact code this
// program renders with, so what you heard in the browser is what this
// program writes. A hand-written loop of oscillator calls would be a
// second implementation, free to drift — and one nothing could reopen.
//
// The theme is the same story one level up: soundlab's Loops tab exports
// a musiclib.Song literal, and musiclib.WriteLoopWAV renders it the way
// the loop pad auditioned it, with the loop seam guaranteed continuous.
//
// # Why every asset is asserted before it is written
//
// The cabinet's speakers reproduce nothing below about 150 Hz. A sound
// whose energy sits down there plays "correctly" through the entire
// engine chain and is simply inaudible on the machine — the failure is
// silent, and it has shipped. So the rule is that a generator
// asserts its own output's spectrum with a Goertzel band check and
// refuses to write an asset that fails: the bug class fails the build
// instead of reaching a cabinet.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/stealthmonkey99/arkade/tools/musiclib"
	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

const sampleRate = sfxlib.SampleRate44100

func main() {
	_, thisFile, _, _ := runtime.Caller(0)
	assets := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "assets"))
	if err := os.MkdirAll(assets, 0o755); err != nil {
		fatal(err)
	}

	write(assets, "catch.wav", genCatch())
	write(assets, "miss.wav", genMiss())
	write(assets, "dash.wav", genDash())
	write(assets, "gameover.wav", genGameOver())

	// The music bed goes through musiclib, which renders a seamless loop
	// rather than a one-shot — the engine loops the WAV verbatim, so the
	// last sample has to meet the first one without a click.
	//
	// Its spectrum check uses the FRAMED analysis, not the one the
	// transients above use. AssertSpectrum spot-samples Goertzel bins
	// every 20 Hz across the whole signal, which is right for a 130 ms
	// hit whose bins are that wide and quietly meaningless for a signal
	// seconds long, where the same grid steps straight over every peak.
	// FrameSignal restores the short-sound assumption by measuring Hann
	// windows spread along the loop.
	song := themeSong()
	theme := musiclib.RenderLoop(song, musiclib.SampleRate22050)
	frames := sfxlib.FrameSignal(theme, musiclib.SampleRate22050, 0, 0)
	if err := frames.AssertSpectrum(20, 10000, []sfxlib.SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Max: 0.5},
		{Name: "mid-200-2000Hz", Lo: 200, Hi: 2000, Min: 0.3},
	}); err != nil {
		fatal(fmt.Errorf("theme.wav: %w", err))
	}
	if err := musiclib.WriteLoopWAV(filepath.Join(assets, "theme.wav"), song, musiclib.SampleRate22050); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote theme.wav (%.1fs loop, %d notes, %d analysis frames)\n",
		song.LengthSeconds(), song.NoteCount(), frames.Count())
}

// genCatch is the pluck: a short struck-bar hit at 880 Hz, bright and
// dry, so a fast run of catches reads as a run rather than a smear. A
// Tap is a decaying body under a few milliseconds of noise; the only
// change is that the body is a wooden bar rather than a sine.
func genCatch() sfxlib.Patch {
	p := sfxlib.Tap(880, 30, 0.13)
	p.Layers[0].Wave = sfxlib.WaveWood
	return p
}

// genMiss is the thud: a square tone sliding down fast, the classic
// "you dropped it". The pitch stays inside the band the cabinet can
// actually reproduce — a real thud's fundamental would be inaudible
// there, so the weight comes from the slide, not from depth. The sweep
// is integrated rather than pointwise so the tone arrives at 190 Hz
// instead of diving under it.
func genMiss() sfxlib.Patch {
	p := sfxlib.Sweep(420, 190, 12, 0.22)
	p.Layers[0].Wave = sfxlib.WaveSquare
	p.Layers[0].SlideCurve = 0.45
	p.Layers[0].Sweep = sfxlib.SweepPhase
	return p
}

// genDash is the whoosh: noise through a resonance, band-limited hard
// so it reads as air rather than as static. A Scrape strikes; this one
// swells and fades instead, the shape of something passing.
func genDash() sfxlib.Patch {
	p := sfxlib.Scrape(1500, 1.2, 0.16)
	p.Layers[0].Env = sfxlib.Env{Kind: sfxlib.EnvArch}
	return p
}

// genGameOver is the sting: a longer falling triangle tone with a slow
// wobble on it. Structurally it is a separate asset from the miss that
// precedes it, because a game over deserves its own beat rather than a
// louder version of the thing that caused it.
func genGameOver() sfxlib.Patch {
	p := sfxlib.Sweep(660, 220, 4, 0.8)
	p.Layers[0].Wave = sfxlib.WaveTriangle
	p.Layers[0].SlideCurve = 1.6
	p.Layers[0].Sweep = sfxlib.SweepPhase
	p.Layers[0].Vibrato = &sfxlib.Vibrato{Hz: 6, DepthHz: 5}
	return p
}

// themeSong is the bed: a seamless 8-beat loop at 104 BPM in E
// hirajoshi, drawn on soundlab's Loops tab. Hirajoshi is the loop pad's
// default scale for a good reason — its minor seconds make a four-note
// doodle sound intended rather than random, which is exactly what a
// background loop nobody is listening to needs.
func themeSong() musiclib.Song {
	return musiclib.Song{
		BPM:   104,
		Beats: 8,
		Tracks: []musiclib.Track{
			{
				Name: "pulse1",
				Inst: musiclib.Instrument{Wave: musiclib.WavePulse, Duty: 0.25, AttackMs: 2, DecayMs: 60, SustainLevel: 0.35, ReleaseMs: 90, Gain: 0.85},
				Notes: []musiclib.Note{
					{Midi: 64, Start: 0, Dur: 0.5, Vel: 1},     // E4
					{Midi: 71, Start: 0.5, Dur: 0.5, Vel: 0.7}, // B4
					{Midi: 72, Start: 1, Dur: 0.5, Vel: 1},     // C5
					{Midi: 71, Start: 1.5, Dur: 0.5, Vel: 0.7}, // B4
					{Midi: 67, Start: 2, Dur: 1, Vel: 1},       // G4
					{Midi: 66, Start: 3, Dur: 1, Vel: 0.7},     // F#4
					{Midi: 64, Start: 4, Dur: 0.5, Vel: 1},     // E4
					{Midi: 67, Start: 4.5, Dur: 0.5, Vel: 0.7}, // G4
					{Midi: 71, Start: 5, Dur: 0.5, Vel: 1},     // B4
					{Midi: 72, Start: 5.5, Dur: 0.5, Vel: 0.7}, // C5
					{Midi: 76, Start: 6, Dur: 1, Vel: 1},       // E5
					{Midi: 71, Start: 7, Dur: 1, Vel: 0.7},     // B4
				},
			},
			{
				Name: "triangle",
				Inst: musiclib.Instrument{Wave: musiclib.WaveTriangle, AttackMs: 4, DecayMs: 80, SustainLevel: 0.5, ReleaseMs: 120, Gain: 0.7},
				Notes: []musiclib.Note{
					{Midi: 52, Start: 0, Dur: 2, Vel: 0.9}, // E3
					{Midi: 55, Start: 2, Dur: 2, Vel: 0.8}, // G3
					{Midi: 52, Start: 4, Dur: 2, Vel: 0.9}, // E3
					{Midi: 59, Start: 6, Dur: 2, Vel: 0.8}, // B3
				},
			},
			{
				Name: "noise",
				Inst: musiclib.Instrument{Wave: musiclib.WaveNoise, NoiseTone: 0.5, AttackMs: 1, DecayMs: 40, SustainLevel: 0, ReleaseMs: 30, Gain: 0.4},
				Notes: []musiclib.Note{
					{Midi: 72, Start: 0.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 1.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 2.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 3.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 4.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 5.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 6.5, Dur: 0.25, Vel: 0.6},
					{Midi: 72, Start: 7.5, Dur: 0.25, Vel: 0.6},
				},
			},
		},
		Echo: musiclib.Echo{DelayBeats: 0.75, Feedback: 0.25, Mix: 0.14},
	}
}

// write renders the patch, asserts its spectrum, then writes the WAV.
// The order matters: nothing reaches assets/ that has not been proven
// audible on the hardware it is for.
func write(dir, name string, p sfxlib.Patch) {
	samples := p.Render(sampleRate)
	if err := sfxlib.AssertSpectrum(samples, sampleRate, 20, 10000, []sfxlib.SpectrumCheck{
		{Name: "sub-150Hz", Lo: 20, Hi: 150, Max: 0.5},
		{Name: "mid-200-2000Hz", Lo: 200, Hi: 2000, Min: 0.3},
	}); err != nil {
		fatal(fmt.Errorf("%s: %w", name, err))
	}
	path := filepath.Join(dir, name)
	if err := sfxlib.WriteFile(path, samples, sampleRate); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%d samples, %.0fms)\n", name, len(samples), 1000*float64(len(samples))/sampleRate)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen-sfx:", err)
	os.Exit(1)
}
