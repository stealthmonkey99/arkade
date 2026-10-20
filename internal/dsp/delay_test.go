// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import (
	"math"
	"math/rand"
	"testing"
)

func TestDelayEstimateRecoversLag(t *testing.T) {
	const n, maxLag, scale = 4096, 1600, 0.3

	for _, lag := range []int{37, 400, 1500} {
		t.Run("", func(t *testing.T) {
			rng := rand.New(rand.NewSource(int64(lag)))
			ref := make([]float64, n)
			for i := range ref {
				ref[i] = rng.Float64()*2 - 1
			}
			mic := make([]float64, n)
			for i := range mic {
				var v float64
				if i >= lag {
					v = scale * ref[i-lag]
				}
				mic[i] = v + 0.01*(rng.Float64()*2-1)
			}

			gotLag, score := DelayEstimate(ref, mic, maxLag)
			if gotLag != lag {
				t.Errorf("lag = %d, want %d", gotLag, lag)
			}
			if score <= 0.9 {
				t.Errorf("score = %v, want > 0.9", score)
			}
		})
	}
}

func TestDelayEstimateSilence(t *testing.T) {
	ref := make([]float64, 500)
	mic := make([]float64, 500)
	lag, score := DelayEstimate(ref, mic, 100)
	if lag != 0 || score != 0 {
		t.Fatalf("silence: got (%d, %v), want (0, 0)", lag, score)
	}
}

func TestDelayEstimateDegenerateInput(t *testing.T) {
	cases := []struct {
		name     string
		ref, mic []float64
		maxLag   int
	}{
		{"empty", nil, nil, 10},
		{"length mismatch", make([]float64, 10), make([]float64, 11), 5},
		{"negative maxLag", make([]float64, 10), make([]float64, 10), -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lag, score := DelayEstimate(c.ref, c.mic, c.maxLag)
			if lag != 0 || score != 0 {
				t.Fatalf("got (%d, %v), want (0, 0)", lag, score)
			}
		})
	}
}

func TestDelayEstimateScoreBounded(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	n := 2048
	ref := make([]float64, n)
	mic := make([]float64, n)
	for i := range ref {
		ref[i] = rng.Float64()*2 - 1
		mic[i] = rng.Float64()*2 - 1
	}
	_, score := DelayEstimate(ref, mic, 50)
	if score < 0 || score > 1 {
		t.Fatalf("score %v out of [0,1]", score)
	}
	if math.IsNaN(score) {
		t.Fatalf("score is NaN")
	}
}

func TestDelayEstimateWholeIgnoresFlatOverlaps(t *testing.T) {
	// One spike each, with the mic's spike far enough along that only
	// lags under 100 keep both spikes in the overlap.
	n := 1000
	ref, mic := make([]float64, n), make([]float64, n)
	ref[900], mic[950] = 1, 1
	for i := range ref {
		ref[i] += 1e-3 * float64(i%7-3)
		mic[i] += 1e-3 * float64(i%7-3)
	}
	lag, score := DelayEstimateWhole(ref, mic, 400)
	if lag != 50 || score < 0.9 {
		t.Fatalf("lag %d score %.2f, want 50 and ≥ 0.9", lag, score)
	}
	mic[950] = 0
	if _, score := DelayEstimateWhole(ref, mic, 400); score > 0.2 {
		t.Fatalf("score %.2f with no matching onset, want ≈ 0", score)
	}
}
