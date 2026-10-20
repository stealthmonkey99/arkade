// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import "sync"

// ring is a fixed-size sample history addressed by absolute index, so two
// rings fed by different goroutines can be aligned once and read as one
// timeline. Logical index i is the (i+off)-th sample ever pushed.
type ring struct {
	mu    sync.Mutex
	buf   []float64
	count int
	off   int
}

func newRing(size int) *ring { return &ring{buf: make([]float64, size)} }

func (r *ring) push(v []float64) {
	r.mu.Lock()
	for _, x := range v {
		r.buf[r.count%len(r.buf)] = x
		r.count++
	}
	r.mu.Unlock()
}

// len is the logical count: the index the next pushed sample will get.
func (r *ring) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count - r.off
}

// alignTo declares that the ring's newest sample is simultaneous with
// logical index n of some other ring, i.e. len() == n from now on.
func (r *ring) alignTo(n int) {
	r.mu.Lock()
	r.off = r.count - n
	r.mu.Unlock()
}

func (r *ring) at(i int) (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := i + r.off
	if p < 0 || p >= r.count || p < r.count-len(r.buf) {
		return 0, false
	}
	return r.buf[p%len(r.buf)], true
}

// window copies logical [start, start+n) into dst (grown as needed),
// returning nil if any of it has already been overwritten or not yet
// written.
func (r *ring) window(start, n int, dst []float64) []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := start + r.off
	if p < 0 || p+n > r.count || p < r.count-len(r.buf) {
		return nil
	}
	if cap(dst) < n {
		dst = make([]float64, n)
	}
	dst = dst[:n]
	for i := range dst {
		dst[i] = r.buf[(p+i)%len(r.buf)]
	}
	return dst
}
