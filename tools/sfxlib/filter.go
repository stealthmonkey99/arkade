// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// OnePoleCoef returns the smoothing coefficient of a one-pole filter with
// its -3 dB point at cutoffHz. A non-positive rate or cutoff returns 0,
// which every filter here reads as off.
func OnePoleCoef(sampleRate int, cutoffHz float64) float64 {
	if sampleRate <= 0 || cutoffHz <= 0 {
		return 0
	}
	a := 1 - math.Exp(-2*math.Pi*cutoffHz/float64(sampleRate))
	if a > 1 {
		return 1
	}
	return a
}

// Lowpass applies a one-pole lowpass at cutoffHz to s in place; cutoffHz
// <= 0 is a no-op.
func Lowpass(s []float64, sampleRate int, cutoffHz float64) {
	a := OnePoleCoef(sampleRate, cutoffHz)
	if a <= 0 {
		return
	}
	y := 0.0
	for i, v := range s {
		y += a * (v - y)
		s[i] = y
	}
}

// Highpass applies a one-pole highpass at cutoffHz to s in place (the
// signal minus its lowpass); cutoffHz <= 0 is a no-op.
func Highpass(s []float64, sampleRate int, cutoffHz float64) {
	a := OnePoleCoef(sampleRate, cutoffHz)
	if a <= 0 {
		return
	}
	y := 0.0
	for i, v := range s {
		y += a * (v - y)
		s[i] = v - y
	}
}

// echoFloor is the wet gain below which Echo stops adding repeats, under
// the noise floor of a 16-bit asset.
const echoFloor = 0.002

// maxFeedback keeps Echo's tap loop terminating.
const maxFeedback = 0.95

// Echo adds a feedback delay to s in place: the dry signal repeated every
// delaySec seconds, each repeat scaled by feedback (clamped to [0, 0.95])
// and mixed in at mix. A non-positive delaySec or mix is a no-op. The delay
// is linear, so repeats past the end of s are dropped.
func Echo(s []float64, sampleRate int, delaySec, feedback, mix float64) {
	if len(s) == 0 || mix <= 0 || delaySec <= 0 || sampleRate <= 0 {
		return
	}
	d := int(math.Round(delaySec * float64(sampleRate)))
	if d < 1 || d >= len(s) {
		return
	}
	if feedback < 0 {
		feedback = 0
	}
	if feedback > maxFeedback {
		feedback = maxFeedback
	}
	dry := make([]float64, len(s))
	copy(dry, s)
	for gain, off := mix, d; gain >= echoFloor && off < len(s); off += d {
		for i := off; i < len(s); i++ {
			s[i] += gain * dry[i-off]
		}
		if feedback == 0 {
			break
		}
		gain *= feedback
	}
}

// FadeOut ramps the last sec seconds of s linearly to zero in place,
// clamped to the length of s. It removes the end-of-buffer click.
func FadeOut(s []float64, sampleRate int, sec float64) {
	if len(s) == 0 || sec <= 0 || sampleRate <= 0 {
		return
	}
	n := int(math.Round(sec * float64(sampleRate)))
	if n <= 0 {
		return
	}
	if n > len(s) {
		n = len(s)
	}
	start := len(s) - n
	for i := start; i < len(s); i++ {
		s[i] *= float64(len(s)-1-i) / float64(n)
	}
}

// LowpassVarying is Lowpass with a per-sample cutoff. A shorter cutoffHz
// holds its last value; an empty one is a no-op. A non-positive cutoff
// holds the output at its current value rather than switching the filter off.
func LowpassVarying(s []float64, sampleRate int, cutoffHz []float64) {
	if sampleRate <= 0 || len(cutoffHz) == 0 {
		return
	}
	y := 0.0
	for i, v := range s {
		j := i
		if j >= len(cutoffHz) {
			j = len(cutoffHz) - 1
		}
		y += OnePoleCoef(sampleRate, cutoffHz[j]) * (v - y)
		s[i] = y
	}
}

// BandPass returns s through an RBJ band-pass biquad (peak gain q) whose
// centre frequency is centreHz per sample, recomputed every sample. A
// shorter centreHz holds its last value. A non-positive q, empty centreHz or
// non-positive rate returns an unfiltered copy. It returns a new slice so
// several formants can be fed from one source.
func BandPass(s []float64, sampleRate int, centreHz []float64, q float64) []float64 {
	out := make([]float64, len(s))
	if sampleRate <= 0 || len(centreHz) == 0 || q <= 0 {
		copy(out, s)
		return out
	}
	var x1, x2, y1, y2 float64
	for i, x := range s {
		j := i
		if j >= len(centreHz) {
			j = len(centreHz) - 1
		}
		w := 2 * math.Pi * centreHz[j] / float64(sampleRate)
		alpha := math.Sin(w) / (2 * q)
		cw := math.Cos(w)
		y := (alpha*x - alpha*x2 + 2*cw*y1 - (1-alpha)*y2) / (1 + alpha)
		x2, x1 = x1, x
		y2, y1 = y1, y
		out[i] = y
	}
	return out
}
