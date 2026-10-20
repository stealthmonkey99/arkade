// SPDX-License-Identifier: GPL-2.0-or-later

package dsp

import (
	"math"
	"testing"
)

func TestHann(t *testing.T) {
	for _, n := range []int{4, 8, 256} {
		w := Hann(n)
		if len(w) != n {
			t.Fatalf("Hann(%d) len = %d", n, len(w))
		}
		if w[0] != 0 {
			t.Errorf("Hann(%d)[0] = %v, want 0", n, w[0])
		}
		for i, v := range w {
			if v < -1e-12 || v > 1+1e-12 {
				t.Errorf("Hann(%d)[%d] = %v, out of [0,1]", n, i, v)
			}
		}
		mid := w[n/2]
		if math.Abs(mid-1) > 1e-9 {
			t.Errorf("Hann(%d)[n/2] = %v, want 1", n, mid)
		}
	}
}

func TestSymmetricHann(t *testing.T) {
	for _, n := range []int{5, 9, 257} {
		w := SymmetricHann(n)
		if len(w) != n {
			t.Fatalf("SymmetricHann(%d) len = %d", n, len(w))
		}
		for i := range w {
			if math.Abs(w[i]-w[n-1-i]) > 1e-12 {
				t.Errorf("SymmetricHann(%d)[%d] = %v, mirror %v", n, i, w[i], w[n-1-i])
			}
		}
		if w[0] != 0 || math.Abs(w[n-1]) > 1e-12 || math.Abs(w[n/2]-1) > 1e-9 {
			t.Errorf("SymmetricHann(%d) ends %v,%v peak %v; want 0,0 and 1", n, w[0], w[n-1], w[n/2])
		}
	}
}
