// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import "math"

// MFCC turns one frame of samples into mel-frequency cepstral coefficients:
// Hann window, power spectrum, triangular HTK-mel filterbank, natural log of
// each band energy (floored at 1e-10), then an orthonormal DCT-II keeping
// coeffs coefficients including c0.
type MFCC struct {
	bands, coeffs int

	window []float64
	fft    *FFT

	filterStart []int
	filterEnd   []int
	filterWts   [][]float64

	dct [][]float64 // [coeffs][bands]

	re, im []float64 // frameLen scratch, reused per Frame/LogMel call
	power  []float64 // frameLen/2+1
	logMel []float64 // bands, scratch for Frame's DCT input
}

// NewMFCC builds an MFCC extractor for frames of frameLen samples at
// sampleRate, with a mel filterbank of bands triangular filters spanning
// minHz..maxHz and coeffs cepstral coefficients kept. frameLen must be a
// power of two.
func NewMFCC(sampleRate, frameLen, bands, coeffs int, minHz, maxHz float64) *MFCC {
	if frameLen < 2 || frameLen&(frameLen-1) != 0 {
		panic("dsp: MFCC frameLen must be a power of two")
	}
	m := &MFCC{
		bands:  bands,
		coeffs: coeffs,
		window: Hann(frameLen),
		fft:    NewFFT(frameLen),
		re:     make([]float64, frameLen),
		im:     make([]float64, frameLen),
		power:  make([]float64, frameLen/2+1),
		logMel: make([]float64, bands),
	}
	m.buildFilterbank(sampleRate, frameLen, minHz, maxHz)
	m.buildDCT()
	return m
}

func melFromHz(hz float64) float64  { return 2595 * math.Log10(1+hz/700) }
func hzFromMel(mel float64) float64 { return 700 * (math.Pow(10, mel/2595) - 1) }

func (m *MFCC) buildFilterbank(sampleRate, frameLen int, minHz, maxHz float64) {
	melMin, melMax := melFromHz(minHz), melFromHz(maxHz)
	nyquistBin := frameLen / 2
	binHz := float64(sampleRate) / float64(frameLen)

	bin := make([]int, m.bands+2)
	for i := range bin {
		mel := melMin + (melMax-melMin)*float64(i)/float64(m.bands+1)
		b := int(math.Round(hzFromMel(mel) / binHz))
		bin[i] = min(max(b, 0), nyquistBin)
	}

	m.filterStart = make([]int, m.bands)
	m.filterEnd = make([]int, m.bands)
	m.filterWts = make([][]float64, m.bands)
	for b := 0; b < m.bands; b++ {
		left, center, right := bin[b], bin[b+1], bin[b+2]
		// Narrow bands collapse left==center or center==right; keep both edges.
		if center <= left {
			center = left + 1
		}
		if right <= center {
			right = center + 1
		}
		start, end := left, min(right, nyquistBin)
		wts := make([]float64, end-start+1)
		for k := start; k <= end; k++ {
			var w float64
			switch {
			case k <= center && center > left:
				w = float64(k-left) / float64(center-left)
			case k > center && right > center:
				w = float64(right-k) / float64(right-center)
			}
			wts[k-start] = max(w, 0)
		}
		m.filterStart[b], m.filterEnd[b], m.filterWts[b] = start, end, wts
	}
}

func (m *MFCC) buildDCT() {
	m.dct = make([][]float64, m.coeffs)
	for k := 0; k < m.coeffs; k++ {
		scale := math.Sqrt(2 / float64(m.bands))
		if k == 0 {
			scale = math.Sqrt(1 / float64(m.bands))
		}
		row := make([]float64, m.bands)
		for n := 0; n < m.bands; n++ {
			row[n] = scale * math.Cos(math.Pi*(float64(n)+0.5)*float64(k)/float64(m.bands))
		}
		m.dct[k] = row
	}
}

// Frame computes the coefficients of samples (len == frameLen) into out
// (len == coeffs).
func (m *MFCC) Frame(samples []float64, out []float64) {
	m.logMelInto(samples, m.logMel)
	for k := 0; k < m.coeffs; k++ {
		row := m.dct[k]
		var s float64
		for n, v := range m.logMel {
			s += row[n] * v
		}
		out[k] = s
	}
}

// LogMel computes only the log band energies into out (len == bands).
func (m *MFCC) LogMel(samples []float64, out []float64) {
	m.logMelInto(samples, out)
}

func (m *MFCC) logMelInto(samples []float64, dst []float64) {
	for i, x := range samples {
		m.re[i] = x * m.window[i]
		m.im[i] = 0
	}
	m.fft.Forward(m.re, m.im)
	for i := range m.power {
		m.power[i] = m.re[i]*m.re[i] + m.im[i]*m.im[i]
	}
	for b := 0; b < m.bands; b++ {
		wts := m.filterWts[b]
		var e float64
		for k := m.filterStart[b]; k <= m.filterEnd[b]; k++ {
			e += wts[k-m.filterStart[b]] * m.power[k]
		}
		dst[b] = math.Log(max(e, 1e-10))
	}
}
