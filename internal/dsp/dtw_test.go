// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import (
	"math"
	"math/rand"
	"testing"
)

func makeSeq(n, dim int, rng *rand.Rand) [][]float64 {
	seq := make([][]float64, n)
	for i := range seq {
		frame := make([]float64, dim)
		for j := range frame {
			frame[j] = rng.Float64()*2 - 1
		}
		seq[i] = frame
	}
	return seq
}

func stretch2x(seq [][]float64) [][]float64 {
	out := make([][]float64, 0, 2*len(seq))
	for _, f := range seq {
		out = append(out, f, f)
	}
	return out
}

func TestDTWIdenticalIsZero(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	a := makeSeq(20, 4, rng)

	dist, _ := DTW(a, a, 0, nil)
	if dist != 0 {
		t.Fatalf("DTW(a, a) = %v, want 0", dist)
	}
}

func TestDTWTimeStretchIsCloserThanUnrelated(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	a := makeSeq(30, 4, rng)
	stretched := stretch2x(a)
	unrelated := makeSeq(60, 4, rng)

	stretchedDist, _ := DTW(a, stretched, 0, nil)
	unrelatedDist, _ := DTW(a, unrelated, 0, nil)

	if stretchedDist >= unrelatedDist {
		t.Fatalf("stretched distance %v not smaller than unrelated %v", stretchedDist, unrelatedDist)
	}
	if stretchedDist > 1e-9 {
		t.Fatalf("stretched distance %v, want ~0", stretchedDist)
	}
}

func TestDTWBandWidensForLengthMismatch(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	a := makeSeq(3, 2, rng)
	b := makeSeq(30, 2, rng)

	tight, _ := DTW(a, b, 1, nil)
	unconstrained, _ := DTW(a, b, 0, nil)

	if math.IsInf(tight, 1) {
		t.Fatal("band=1 with |len(a)-len(b)|=27 produced +Inf; widening did not kick in")
	}
	if math.Abs(tight-unconstrained) > 1e-12 {
		t.Fatalf("widened band result %v != unconstrained result %v", tight, unconstrained)
	}
}

func TestDTWEmptyIsInf(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	b := makeSeq(5, 3, rng)

	if dist, _ := DTW(nil, b, 0, nil); !math.IsInf(dist, 1) {
		t.Fatalf("DTW(nil, b) = %v, want +Inf", dist)
	}
	if dist, _ := DTW(b, nil, 0, nil); !math.IsInf(dist, 1) {
		t.Fatalf("DTW(b, nil) = %v, want +Inf", dist)
	}
	if dist, _ := DTW(nil, nil, 0, nil); !math.IsInf(dist, 1) {
		t.Fatalf("DTW(nil, nil) = %v, want +Inf", dist)
	}
}

func TestDTWScratchReuse(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	a := makeSeq(10, 2, rng)
	b := makeSeq(12, 2, rng)

	var scratch []float64
	d1, scratch := DTW(a, b, 0, scratch)
	if len(scratch) < 11*13 {
		t.Fatalf("scratch too small: %d", len(scratch))
	}
	d2, scratch2 := DTW(a, b, 0, scratch)
	if d1 != d2 {
		t.Fatalf("reused scratch changed result: %v vs %v", d1, d2)
	}
	if &scratch2[0] != &scratch[0] {
		t.Fatal("scratch buffer was reallocated when capacity already sufficed")
	}
}
