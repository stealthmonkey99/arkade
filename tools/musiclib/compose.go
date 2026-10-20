// SPDX-License-Identifier: GPL-2.0-or-later

package musiclib

import (
	"math"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// Composition arranges already-rendered audio on a timeline. Looping segments
// are treated as cycles: each fades in over JoinMs at its start and keeps
// sounding for JoinMs past its end, so every junction is a short crossfade.
// Writes are modulo the composition length, so the whole piece loops
// seamlessly.
//
// The crossfade is linear rather than equal-power: ramps over identical
// material sum back to exactly that material, where equal-power would bulge
// 3 dB and pull down the normalized level.
type Composition struct {
	Segments []Segment
	// Seconds is the loop length; 0 means the end of the last segment.
	// A shorter value folds the overhang back onto the start.
	Seconds float64
	// JoinMs is the crossfade length at segment junctions; 0 takes
	// DefaultJoinMs. It applies only to segments marked Loop.
	JoinMs float64
}

// Segment is one placed piece of pre-rendered audio.
type Segment struct {
	// Samples is the material, already at the rate the composition renders at.
	Samples []float64
	// Start is the placement, in seconds from the composition's start.
	Start float64
	// Repeat lays Samples down this many times back to back; 0 and 1
	// both mean once. Repeats of a Loop segment are not crossfaded.
	Repeat int
	// Loop declares that Samples is a seamless cycle, so it may be tiled by
	// Repeat and crossfaded at its edges. One-shots leave it false and are
	// written verbatim.
	Loop bool
	// Gain scales the segment; 0 means 1.
	Gain float64
}

// DefaultJoinMs is the default junction crossfade length in milliseconds.
const DefaultJoinMs = 6.0

// WriteCompositionWAV renders c at sampleRate and writes it as a 16-bit mono
// PCM WAV to path.
func WriteCompositionWAV(path string, c Composition, sampleRate int) error {
	return sfxlib.WriteFile(path, RenderComposition(c, sampleRate), sampleRate)
}

// RenderComposition renders c at sampleRate, normalized to DefaultPeak.
// The result loops seamlessly and is deterministic: the same segments at
// the same rate produce identical samples.
func RenderComposition(c Composition, sampleRate int) []float64 {
	return RenderCompositionPeak(c, sampleRate, DefaultPeak)
}

// RenderCompositionPeak is RenderComposition with an explicit normalization
// peak; peak <= 0 returns the raw mix.
func RenderCompositionPeak(c Composition, sampleRate int, peak float64) []float64 {
	total := c.LengthSamples(sampleRate)
	if total <= 0 {
		return nil
	}
	fade := int(math.Round(c.joinMs() / 1000 * float64(sampleRate)))
	if fade < 1 {
		fade = 1
	}
	buf := make([]float64, total)
	for _, s := range c.Segments {
		writeSegment(buf, s, sampleRate, fade)
	}
	if peak > 0 {
		sfxlib.Normalize(buf, peak)
	}
	return buf
}

// LengthSamples returns the composition's length in samples at
// sampleRate: Seconds if set, otherwise the end of the last segment.
func (c Composition) LengthSamples(sampleRate int) int {
	if c.Seconds > 0 {
		return int(math.Round(c.Seconds * float64(sampleRate)))
	}
	end := 0
	for _, s := range c.Segments {
		if e := s.startSample(sampleRate) + s.Len(); e > end {
			end = e
		}
	}
	return end
}

// LengthSeconds returns the composition's length in seconds at sampleRate.
func (c Composition) LengthSeconds(sampleRate int) float64 {
	return float64(c.LengthSamples(sampleRate)) / float64(sampleRate)
}

func (c Composition) joinMs() float64 {
	if c.JoinMs > 0 {
		return c.JoinMs
	}
	return DefaultJoinMs
}

// Len is the segment's sample count, material times repeats, excluding the
// join tail.
func (s Segment) Len() int { return len(s.Samples) * s.repeat() }

func (s Segment) repeat() int {
	if s.Repeat < 1 {
		return 1
	}
	return s.Repeat
}

func (s Segment) startSample(sampleRate int) int {
	return int(math.Round(s.Start * float64(sampleRate)))
}

// writeSegment adds one segment into the cyclic buffer. A Loop segment ramps
// in over its first fade samples and rings out for fade more samples of its
// own continuation; a one-shot gets neither ramp.
func writeSegment(buf []float64, s Segment, sampleRate, fade int) {
	n, m := len(buf), len(s.Samples)
	if n == 0 || m == 0 {
		return
	}
	g := s.Gain
	if g == 0 {
		g = 1
	}
	start := s.startSample(sampleRate)
	body := s.Len()
	at := func(i int, v float64) {
		buf[((start+i)%n+n)%n] += v
	}
	if !s.Loop {
		for i := 0; i < body; i++ {
			at(i, g*s.Samples[i%m])
		}
		return
	}
	f := fade
	if f > body {
		f = body
	}
	for i := 0; i < body; i++ {
		v := g * s.Samples[i%m]
		if i < f {
			v *= float64(i) / float64(f)
		}
		at(i, v)
	}
	for i := 0; i < f; i++ {
		at(body+i, g*s.Samples[(body+i)%m]*(1-float64(i)/float64(f)))
	}
}
