// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"math"
	"slices"
	"testing"
)

// The canceller is fed as the cabinet feeds it: the reference in small chunks
// slightly ahead of the capture, the capture in arecord-sized blocks, the two
// declared simultaneous at the first block.

const (
	simLead   = 900  // samples the tap runs ahead of the capture clock
	simDelay  = 1800 // physical echo delay, capture relative to reference
	simBlock  = 683  // one arecord read after decimation
	simChunk  = 171  // one mixer pull after decimation
	simLength = 6 * Rate
)

// simulate builds a 6 s room: the reference plays broadband noise from 1 s
// to 5 s; the capture hears it through a two-tap echo path plus room tone,
// with a near-end 1800 Hz tone from 3.5 s to 4.5 s. Returns the residual
// and the per-frame echo-dominated flags.
func simulate(t *testing.T, cfg CancellerConfig) (residual []float64, echoed []bool, lead int, c *Canceller) {
	t.Helper()
	ref := make([]float64, simLength)
	var s uint64 = 0x9E3779B97F4A7C15
	next := func() float64 {
		s ^= s << 13
		s ^= s >> 7
		s ^= s << 17
		return float64(s%20001)/10000 - 1
	}
	for i := Rate; i < 5*Rate; i++ {
		ref[i] = 0.3 * next()
	}
	mic := make([]float64, simLength)
	for i := range mic {
		v := 0.002 * next()
		if j := i - simDelay; j >= 0 {
			v += 0.5 * ref[j]
		}
		if j := i - simDelay - 7; j >= 0 {
			v += 0.2 * ref[j]
		}
		if i >= 7*Rate/2 && i < 9*Rate/2 {
			v += 0.1 * math.Sin(2*math.Pi*1800*float64(i)/Rate)
		}
		mic[i] = v
	}

	refRing, micRing := newRing(ringSeconds*Rate), newRing(ringSeconds*Rate)
	c = NewCanceller(cfg, refRing, micRing)
	residual = make([]float64, 0, simLength)
	frame := make([]float64, 256)
	clean := make([]float64, 256)
	refAt, framed := 0, 0
	aligned := false
	for m := 0; m+simBlock <= simLength; m += simBlock {
		for refAt < m+simBlock+simLead && refAt+simChunk <= simLength {
			refRing.push(ref[refAt : refAt+simChunk])
			refAt += simChunk
		}
		micRing.push(mic[m : m+simBlock])
		if !aligned {
			lead = refRing.len() - micRing.len()
			refRing.alignTo(micRing.len())
			aligned = true
		}
		for framed+256 <= m+simBlock {
			copy(frame, mic[framed:framed+256])
			framed += 256
			c.Process(frame, clean, framed)
			residual = append(residual, clean...)
			echoed = append(echoed, c.EchoDominated(0.01))
		}
	}
	return residual, echoed, lead, c
}

func powerOver(v []float64, from, to int) float64 {
	return meanPow(v[from:min(to, len(v))])
}

func TestCancellerAlignsAndCancels(t *testing.T) {
	residual, echoed, lead, c := simulate(t, DefaultCancellerConfig())

	delay, ok := c.Delay()
	if !ok {
		t.Fatal("never aligned")
	}
	// The tap was lead samples ahead when the rings were aligned.
	if want := simDelay + lead; abs(delay-want) > 2 {
		t.Fatalf("delay %d, want %d±2 (score %.2f)", delay, want, c.Score())
	}

	// Echo only, once the filter has had a second: at least 15 dB quieter.
	from, to := 3*Rate, 7*Rate/2
	micPow := 0.5*0.5*0.3*0.3/3 + 0.2*0.2*0.3*0.3/3 // uniform noise has power a²/3
	resPow := powerOver(residual, from, to)
	if erle := 10 * math.Log10(micPow/resPow); erle < 15 {
		t.Fatalf("ERLE %.1f dB over echo-only stretch, want ≥ 15 (reported %.1f)", erle, c.ERLE())
	}
	flagged := 0
	for i := from / 256; i < to/256; i++ {
		if echoed[i] {
			flagged++
		}
	}
	if n := to/256 - from/256; flagged < n*8/10 {
		t.Fatalf("%d/%d echo-only frames judged echo-dominated, want ≥ 80%%", flagged, n)
	}

	// A tone over the echo survives cancellation and is NOT judged echo.
	from, to = 7*Rate/2+Rate/4, 9*Rate/2
	tonePow := 0.1 * 0.1 / 2
	if got := powerOver(residual, from, to); got < tonePow*0.7 || got > tonePow*1.6 {
		t.Fatalf("residual power %.4f during the tone, want about %.4f (the tone itself)", got, tonePow)
	}
	flagged = 0
	for i := from / 256; i < to/256; i++ {
		if echoed[i] {
			flagged++
		}
	}
	if n := to/256 - from/256; flagged > n/5 {
		t.Fatalf("%d/%d tone-over-echo frames judged echo-dominated, want ≤ 20%%", flagged, n)
	}

	// Silence before playback passes through untouched and unflagged.
	if got := powerOver(residual, 0, Rate/2); got > 0.002*0.002 {
		t.Fatalf("residual power %.2e in silence, want the room tone only", got)
	}
	for i := 0; i < Rate/2/256; i++ {
		if echoed[i] {
			t.Fatalf("frame %d flagged echo-dominated before anything played", i)
		}
	}
}

func TestCancellerWithoutReferenceIsTransparent(t *testing.T) {
	refRing, micRing := newRing(ringSeconds*Rate), newRing(ringSeconds*Rate)
	c := NewCanceller(DefaultCancellerConfig(), refRing, micRing)
	in := sine(1800, 0.25, 2000)
	frame := make([]float64, 256)
	out := make([]float64, 256)
	for off := 0; off+256 <= len(in); off += 256 {
		for i := range frame {
			frame[i] = float64(in[off+i]) / 32768
		}
		micRing.push(frame)
		c.Process(frame, out, micRing.len())
		for i := range out {
			if out[i] != frame[i] {
				t.Fatalf("sample %d altered with no reference: %v -> %v", off+i, frame[i], out[i])
			}
		}
		if c.EchoDominated(0.01) {
			t.Fatal("echo-dominated with no reference at all")
		}
	}
}

func TestGateCatchesPlaybackAtFrequenciesTheFilterNeverHeard(t *testing.T) {
	cfg := DefaultConfig()
	gate := newEchoGate(cfg, DefaultCancellerConfig().MinRefRMS)
	refRing, micRing := newRing(ringSeconds*Rate), newRing(ringSeconds*Rate)
	c := NewCanceller(DefaultCancellerConfig(), refRing, micRing)

	// 3 s of a 1500 Hz tone to align and train on, then a broadband burst.
	tone := sine(1500, 0.3, 3000)
	burst := noise(0.3, 400, 0, 20)
	ref := make([]float64, 0, 5*Rate)
	for _, s := range tone {
		ref = append(ref, float64(s)/32768)
	}
	ref = append(ref, make([]float64, Rate/2)...)
	for _, s := range burst {
		ref = append(ref, float64(s)/32768)
	}
	ref = append(ref, make([]float64, Rate/2)...)
	mic := make([]float64, len(ref))
	for i := range mic {
		if j := i - simDelay; j >= 0 {
			mic[i] = 0.5 * ref[j]
		}
	}

	frame := make([]float64, 256)
	clean := make([]float64, 256)
	refAt, framed := 0, 0
	aligned := false
	// The burst reaches the microphone simDelay later than the reference
	// plays it; judge the frames that actually contain it.
	burstStart := (len(tone)+Rate/2+simDelay)/256 + 1
	burstEnd := burstStart + len(burst)/256 - 1
	flagged, total := 0, 0
	for m := 0; m+simBlock <= len(mic); m += simBlock {
		for refAt < m+simBlock+simLead && refAt+simChunk <= len(ref) {
			refRing.push(ref[refAt : refAt+simChunk])
			refAt += simChunk
		}
		micRing.push(mic[m : m+simBlock])
		if !aligned {
			refRing.alignTo(micRing.len())
			aligned = true
		}
		for framed+256 <= m+simBlock {
			copy(frame, mic[framed:framed+256])
			framed += 256
			c.Process(frame, clean, framed)
			echo := gate.judge(c.Reference(), frame, clean)
			if f := framed/256 - 1; f >= burstStart && f < burstEnd {
				total++
				if echo {
					flagged++
				}
			}
		}
	}
	if _, ok := c.Delay(); !ok {
		t.Fatal("never aligned on the tone")
	}
	if total == 0 || flagged < total*8/10 {
		t.Fatalf("%d/%d burst frames judged the cabinet's own, want ≥ 80%%", flagged, total)
	}
}

func TestHistoryReadsNewestFirstAcrossTheWrap(t *testing.T) {
	h := newHistory(4)
	for i := 1; i <= 10; i++ {
		h.push(float64(i))
		var want []float64
		for v := i; v > i-4; v-- {
			want = append(want, float64(max(v, 0)))
		}
		if got := h.newestFirst(); !slices.Equal(got, want) {
			t.Fatalf("after pushing 1..%d: %v, want %v", i, got, want)
		}
		pow := 0.0
		for _, v := range want {
			pow += v * v
		}
		if h.pow != pow {
			t.Fatalf("after pushing 1..%d: pow %v, want %v", i, h.pow, pow)
		}
	}
	h.reset()
	if got := h.newestFirst(); !slices.Equal(got, make([]float64, 4)) || h.pow != 0 {
		t.Fatalf("after reset: %v pow %v", got, h.pow)
	}
}
