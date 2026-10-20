// SPDX-License-Identifier: GPL-2.0-or-later

package evening

import (
	"math"
	"time"
)

// Sensitivity is how much louder than the room's own quiet the evening gate
// wants before it calls the room noisy.
type Sensitivity string

const (
	SensLow    Sensitivity = "low"
	SensMedium Sensitivity = "medium"
	SensHigh   Sensitivity = "high"
)

// loudRatio is the multiple of the quiet reference the fast level must reach.
// A TV two rooms away measured 1.5-5x the microphone floor
// (testdata/tv-rms.csv), so medium sits just under that range.
func (s Sensitivity) loudRatio() float64 {
	switch s {
	case SensLow:
		return 2.2
	case SensHigh:
		return 1.3
	default:
		return 1.6
	}
}

func (s Sensitivity) next() Sensitivity {
	switch s {
	case SensLow:
		return SensMedium
	case SensMedium:
		return SensHigh
	default:
		return SensLow
	}
}

func (s Sensitivity) prev() Sensitivity {
	switch s {
	case SensHigh:
		return SensMedium
	case SensMedium:
		return SensLow
	default:
		return SensHigh
	}
}

const (
	// fastTau smooths the level so a door closing is not "loud" but a sentence of dialogue is.
	fastTau = 2 * time.Second
	// quietRiseTau is the upward drift of the quiet reference: slow enough
	// that an evening of television cannot become the new quiet, fast enough
	// to absorb a fan within the hour.
	quietRiseTau = 10 * time.Minute
	// minLoudRMS is the absolute floor under the ratio test, which a near-silent room would pass trivially.
	minLoudRMS = 0.0012
	// quietFloor keeps the reference off zero.
	quietFloor = 1e-5

	triggerWindow   = 20 * time.Second
	triggerFraction = 0.6
	releaseWindow   = 3 * time.Minute
	releaseFraction = 0.2
	// windowCoverage is the sampled share of a window needed before its fraction counts.
	windowCoverage = 0.75

	// sampleGapReset is a feed gap long enough that the history either side describes different rooms.
	sampleGapReset = 5 * time.Second

	bucketSeconds = 1
	bucketCount   = int64(releaseWindow/time.Second) / bucketSeconds
)

type bucket struct{ loud, total float64 }

// tracker turns microphone level frames into whether the room has been loud
// enough for long enough. It keeps a fast exponential average and a quiet
// reference that follows it down instantly and drifts up slowly.
type tracker struct {
	ratio float64

	have  bool
	start time.Time
	last  time.Time
	fast  float64
	quiet float64

	buckets [bucketCount]bucket
	// idx is the second, counted from start, that buckets[idx%len] holds.
	idx int64

	wants bool
}

func newTracker(s Sensitivity) *tracker { return &tracker{ratio: s.loudRatio()} }

// setSensitivity changes the ratio and keeps the level history.
func (t *tracker) setSensitivity(s Sensitivity) { t.ratio = s.loudRatio() }

func (t *tracker) reset() {
	t.have = false
	t.wants = false
	t.fast, t.quiet = 0, 0
	t.idx = 0
	t.buckets = [bucketCount]bucket{}
}

// Push folds one level frame into the tracker. Callers de-duplicate by hardware.MicLevel.Seq.
func (t *tracker) Push(now time.Time, rms float64) {
	if !t.have || now.Sub(t.last) > sampleGapReset || now.Before(t.last) {
		t.reset()
		t.have = true
		t.start, t.last = now, now
		t.fast, t.quiet = rms, math.Max(rms, quietFloor)
		return
	}
	dt := now.Sub(t.last).Seconds()
	if dt <= 0 {
		return
	}
	t.last = now

	t.fast += (1 - math.Exp(-dt/fastTau.Seconds())) * (rms - t.fast)
	// The reference follows the raw frame: smoothing would hide the pauses it measures.
	if rms < t.quiet {
		t.quiet = rms
	} else {
		t.quiet *= math.Exp(dt / quietRiseTau.Seconds())
	}
	t.quiet = math.Max(t.quiet, quietFloor)

	loud := t.fast > t.ratio*t.quiet && t.fast > minLoudRMS
	t.record(now, dt, loud)
	t.decide()
}

func (t *tracker) record(now time.Time, dt float64, loud bool) {
	idx := int64(now.Sub(t.start).Seconds()) / bucketSeconds
	if idx < t.idx {
		idx = t.idx
	}
	for t.idx < idx {
		t.idx++
		t.buckets[t.idx%bucketCount] = bucket{}
	}
	b := &t.buckets[t.idx%bucketCount]
	b.total += dt
	if loud {
		b.loud += dt
	}
}

// fraction reports the loud share of the last d, and whether enough was sampled to trust it.
func (t *tracker) fraction(d time.Duration) (float64, bool) {
	n := int64(d/time.Second) / bucketSeconds
	if t.idx < n-1 {
		return 0, false
	}
	var loud, total float64
	for i := int64(0); i < n; i++ {
		b := t.buckets[(t.idx-i)%bucketCount]
		loud += b.loud
		total += b.total
	}
	if total < windowCoverage*d.Seconds() {
		return 0, false
	}
	return loud / total, true
}

// decide applies asymmetric thresholds: quick to trigger, slow to release.
func (t *tracker) decide() {
	if !t.wants {
		if f, ok := t.fraction(triggerWindow); ok && f >= triggerFraction {
			t.wants = true
		}
		return
	}
	if f, ok := t.fraction(releaseWindow); ok && f < releaseFraction {
		t.wants = false
	}
}

// Wants reports whether the room has been loud enough for long enough; false before a full trigger window.
func (t *tracker) Wants() bool { return t.wants }
