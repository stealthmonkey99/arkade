// SPDX-License-Identifier: GPL-2.0-or-later

package evening

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/mic"
)

const (
	// bandFrame is the cadence internal/mic publishes a spectrum at.
	bandFrame = 50 * time.Millisecond
	// barTick is the engine's frame budget at the default 30 fps.
	barTick = time.Second / 30
)

// tvSamples synthesises four seconds of television: speech-like syllables
// (gliding fundamental, harmonics rolling off 6 dB per octave) with pauses,
// over pink noise.
func tvSamples(t *testing.T) []float64 {
	t.Helper()
	const (
		seconds = 4
		voice   = 0.004
		room    = 0.0015
	)
	rng := rand.New(rand.NewSource(1))
	out := make([]float64, seconds*mic.Rate)
	var phase float64
	for i := 0; i < len(out); {
		voiced := mic.Rate * (120 + rng.Intn(180)) / 1000
		pause := mic.Rate * (60 + rng.Intn(190)) / 1000
		f0 := 85 + 100*rng.Float64()
		glide := (rng.Float64() - 0.5) * 60
		for j := 0; j < voiced && i < len(out); j, i = j+1, i+1 {
			k := float64(j) / float64(voiced)
			f := f0 + glide*k
			phase += 2 * math.Pi * f / mic.Rate
			var v float64
			for h := 1.0; h*f < mic.Rate/2; h++ {
				v += math.Sin(h*phase) / h
			}
			out[i] = voice * math.Sin(math.Pi*k) * v
		}
		i += pause
	}
	pink := make([]float64, len(out))
	var b0, b1, b2 float64 // Paul Kellet's economy pink filter
	for i := range pink {
		w := rng.NormFloat64()
		b0 = 0.99765*b0 + 0.0990460*w
		b1 = 0.96300*b1 + 0.2965164*w
		b2 = 0.57000*b2 + 1.0526913*w
		pink[i] = b0 + b1 + b2 + 0.1848*w
	}
	gain := room / rms(pink)
	for i := range out {
		out[i] += gain * pink[i]
	}
	return out
}

func rms(samples []float64) float64 {
	var sum float64
	for _, s := range samples {
		sum += s * s
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// knock mixes a 40 ms burst 20 dB over the room into samples at index at:
// white noise through two one-pole low passes, mostly under 1 kHz.
func knock(samples []float64, at int) {
	const (
		length = mic.Rate / 25 // 40 ms
		cutoff = 700.0
	)
	a := 1 - math.Exp(-2*math.Pi*cutoff/mic.Rate)
	rng := rand.New(rand.NewSource(7))
	burst := make([]float64, length)
	var lp1, lp2 float64
	for i := range burst {
		lp1 += a * (rng.NormFloat64() - lp1)
		lp2 += a * (lp1 - lp2)
		burst[i] = lp2 * math.Exp(-4*float64(i)/length)
	}
	gain := 10 * rms(samples[at:at+length]) / rms(burst)
	for i, v := range burst {
		samples[at+i] += gain * v
	}
}

// play drives the columns as the app does, at 30 fps with the last published spectrum.
func play(b *barsEl, frames [][hardware.MicBandCount]float64, fn func(tick int)) {
	stats := clock.FrameTime{ElapsedMs: float64(barTick.Microseconds()) / 1000}
	end := time.Duration(len(frames)) * bandFrame
	for tick, at := 0, time.Duration(0); at < end; tick, at = tick+1, at+barTick {
		b.advance(stats, frames[int(at/bandFrame)], true)
		fn(tick)
	}
}

func columnsOver(b *barsEl, frac float64) int {
	n := 0
	for _, v := range b.level {
		if v >= frac {
			n++
		}
	}
	return n
}

// A television excites the whole bottom half of the spectrum, not one slice.
func TestBarsSpreadTheTelevisionAcrossTheLowAndMiddleColumns(t *testing.T) {
	bars := newBars()
	frames := mic.BandFrames(tvSamples(t))
	var peak [barColumns]float64
	play(bars, frames, func(int) {
		for c, v := range bars.level {
			peak[c] = math.Max(peak[c], v)
		}
	})

	for c := 0; c < 10; c++ {
		if peak[c] < 0.25 {
			t.Errorf("column %d only reached %.2f on a television; the low and middle columns should move", c, peak[c])
		}
	}
	var low, high float64
	for c := 0; c < 10; c++ {
		low += peak[c] / 10
	}
	for c := 10; c < barColumns; c++ {
		high += peak[c] / 6
	}
	if low <= high {
		t.Errorf("the television moved the right end (%.2f) more than the low and middle columns (%.2f)", high, low)
	}
}

// An evening of television must leave headroom for something happening in the room.
func TestBarsDoNotSaturateOnTelevisionAlone(t *testing.T) {
	bars := newBars()
	frames := mic.BandFrames(tvSamples(t))
	worst := 0
	play(bars, frames, func(int) {
		if n := columnsOver(bars, 0.9); n > worst {
			worst = n
		}
	})
	if worst == barColumns {
		t.Errorf("the television alone filled all %d columns; nothing in the room could read louder", barColumns)
	}
}

// A knock is far louder than the television and over in 40 ms; the columns must still answer it.
func TestBarsAnswerAKnockOnTheCabinet(t *testing.T) {
	samples := tvSamples(t)
	at := len(samples) - mic.Rate // one second from the end, floors settled
	knock(samples, at)

	bars := newBars()
	frames := mic.BandFrames(samples)
	hit := at / (mic.Rate / 20)
	best := 0
	play(bars, frames, func(tick int) {
		if at := time.Duration(tick) * barTick; at >= time.Duration(hit)*bandFrame && at < time.Duration(hit+2)*bandFrame {
			if n := columnsOver(bars, 0.6); n > best {
				best = n
			}
		}
	})
	if best < 10 {
		t.Errorf("a knock lifted only %d of %d columns over 60%%", best, barColumns)
	}
}
