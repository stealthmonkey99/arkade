// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/dsp"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// bandFFT is the analysis length: 1024 samples at Rate is 70 ms, enough to
// resolve the bottom band without diluting a knock.
const bandFFT = 1024

// bandHop is half a level window, so a burst straddling one analysis sits
// whole inside its neighbour.
const bandHop = levelWindow / 2

// hannPowerGain is the mean of a Hann window squared.
const hannPowerGain = 3.0 / 8.0

// bands measures the summed power of every DFT bin inside each
// hardware.MicBandCentersHz band, normalised so a full-scale sine inside a
// band reads ~1. Edges are the geometric means of adjacent centres, extended
// by half a step at either end. Summing the whole band lets broadband sound
// such as a voice or a knock register.
type bands struct {
	fft    *dsp.FFT
	win    []float64
	ring   []float64
	pos    int
	filled int
	re, im []float64
	lo, hi [hardware.MicBandCount]int
	scale  float64

	count int
	peak  [hardware.MicBandCount]float64
	seen  bool
}

// newBands builds an analyser; push never allocates.
func newBands() *bands {
	b := &bands{
		fft:  dsp.NewFFT(bandFFT),
		win:  dsp.Hann(bandFFT),
		ring: make([]float64, bandFFT),
		re:   make([]float64, bandFFT),
		im:   make([]float64, bandFFT),
		// Parseval: the sum of |X|² is N times the windowed power, and half
		// of a sine's power lands on the positive frequencies.
		scale: 2 / (0.5 * hannPowerGain * float64(bandFFT) * float64(bandFFT)),
	}
	edges := bandEdgesHz()
	binHz := float64(Rate) / bandFFT
	for i := range b.lo {
		b.lo[i] = edgeBin(int(math.Round(edges[i] / binHz)))
		b.hi[i] = edgeBin(int(math.Round(edges[i+1] / binHz)))
	}
	return b
}

func edgeBin(k int) int {
	if k < 1 { // bin 0 is DC, which is the capture's offset, not a sound
		return 1
	}
	if k > bandFFT/2 {
		return bandFFT / 2
	}
	return k
}

func bandEdgesHz() [hardware.MicBandCount + 1]float64 {
	c := hardware.MicBandCentersHz
	n := hardware.MicBandCount
	var e [hardware.MicBandCount + 1]float64
	for i := 1; i < n; i++ {
		e[i] = math.Sqrt(c[i-1] * c[i])
	}
	e[0] = c[0] * math.Sqrt(c[0]/c[1])
	e[n] = c[n-1] * math.Sqrt(c[n-1]/c[n-2])
	return e
}

// push adds one sample at Rate and, every levelWindow samples, returns the
// frame to publish: the peak each band reached since the last one. ok is
// false until a whole analysis window has been pushed.
func (b *bands) push(x float64) (frame [hardware.MicBandCount]float64, ok bool) {
	b.ring[b.pos] = x
	b.pos++
	if b.pos == bandFFT {
		b.pos = 0
	}
	if b.filled < bandFFT {
		b.filled++
	}

	b.count++
	if b.count%bandHop == 0 && b.filled == bandFFT {
		for i, v := range b.analyse() {
			if v > b.peak[i] {
				b.peak[i] = v
			}
		}
		b.seen = true
	}
	if b.count < levelWindow {
		return frame, false
	}
	frame, ok = b.peak, b.seen
	b.count, b.peak, b.seen = 0, [hardware.MicBandCount]float64{}, false
	return frame, ok
}

// analyse transforms the last bandFFT samples pushed.
func (b *bands) analyse() [hardware.MicBandCount]float64 {
	for i := range b.re {
		j := b.pos + i
		if j >= bandFFT {
			j -= bandFFT
		}
		b.re[i] = b.ring[j] * b.win[i]
		b.im[i] = 0
	}
	b.fft.Forward(b.re, b.im)

	var out [hardware.MicBandCount]float64
	for i := range out {
		var p float64
		for k := b.lo[i]; k < b.hi[i]; k++ {
			p += b.re[k]*b.re[k] + b.im[k]*b.im[k]
		}
		out[i] = p * b.scale
	}
	return out
}

// BandFrames replays samples at Rate through the live band analysis and
// returns the frames it would have published.
func BandFrames(samples []float64) [][hardware.MicBandCount]float64 {
	b := newBands()
	out := make([][hardware.MicBandCount]float64, 0, len(samples)/levelWindow)
	for _, s := range samples {
		if f, ok := b.push(s); ok {
			out = append(out, f)
		}
	}
	return out
}
