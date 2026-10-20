// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// tolerance is how far two renders of one sound may drift and still count
// as the same sound. See the package doc for where the defaults come from.
type tolerance struct {
	maxDelta float64 // per-sample |old-new|, full scale = 1
	minSNR   float64 // dB, old's energy over the difference's
}

const (
	defaultMaxDelta = 0.25
	defaultMinSNR   = 15.0
	// maxShiftMs bounds the alignment search. Speech trims to a silence
	// threshold in 5 ms frames, and another CPU's arithmetic can move an
	// onset across one frame boundary, shifting the whole clip by 5 ms.
	maxShiftMs = 50
)

// audioDelta measures new against old at the shift, within [maxShiftMs],
// where they agree best. The shorter side is read as silence past its
// end, so a length change counts as error.
type audioDelta struct {
	maxDelta float64
	snr      float64 // +Inf when the samples are identical
	shiftMs  float64 // the delay added to new for the best fit
}

func measureAudio(a, b []byte) (audioDelta, error) {
	da, err := sfxlib.Decode(a)
	if err != nil {
		return audioDelta{}, fmt.Errorf("left: %w", err)
	}
	db, err := sfxlib.Decode(b)
	if err != nil {
		return audioDelta{}, fmt.Errorf("right: %w", err)
	}
	switch {
	case da.SampleRate != db.SampleRate:
		return audioDelta{}, fmt.Errorf("sample rate %d vs %d", da.SampleRate, db.SampleRate)
	case da.Channels != db.Channels:
		return audioDelta{}, fmt.Errorf("%d channel(s) vs %d", da.Channels, db.Channels)
	case da.Bits != db.Bits:
		return audioDelta{}, fmt.Errorf("%d-bit vs %d-bit", da.Bits, db.Bits)
	}
	maxShift := da.SampleRate * maxShiftMs / 1000
	best := measureAt(da.Samples, db.Samples, 0)
	for shift := 1; shift <= maxShift; shift++ {
		for _, s := range [2]int{shift, -shift} {
			if d := measureAt(da.Samples, db.Samples, s); d.snr > best.snr {
				best = d
			}
		}
	}
	best.shiftMs = 1000 * best.shiftMs / float64(da.SampleRate)
	return best, nil
}

// measureAt compares b, delayed by shift samples, against a. shiftMs holds
// the raw sample count until measureAudio scales it.
func measureAt(a, b []float64, shift int) audioDelta {
	d := audioDelta{shiftMs: float64(shift)}
	var signal, noise float64
	n := max(len(a), len(b)+shift)
	for i := min(0, shift); i < n; i++ {
		x, y := at(a, i), at(b, i-shift)
		e := x - y
		d.maxDelta = math.Max(d.maxDelta, math.Abs(e))
		signal += x * x
		noise += e * e
	}
	switch {
	case noise == 0:
		d.snr = math.Inf(1)
	case signal == 0:
		d.snr = math.Inf(-1)
	default:
		d.snr = 10 * math.Log10(signal/noise)
	}
	return d
}

func at(s []float64, i int) float64 {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return 0
}

func (d audioDelta) within(t tolerance) bool {
	return d.maxDelta <= t.maxDelta && d.snr >= t.minSNR
}

func (t tolerance) String() string {
	return fmt.Sprintf("max delta %g, SNR %g dB", t.maxDelta, t.minSNR)
}

func (d audioDelta) String() string {
	s := fmt.Sprintf("max delta %.5f, SNR %.1f dB", d.maxDelta, d.snr)
	if d.shiftMs != 0 {
		s += fmt.Sprintf(", aligned at %+.1f ms", d.shiftMs)
	}
	return s
}
