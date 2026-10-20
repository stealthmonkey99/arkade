// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"math"
	"testing"
)

const testRate = Rate

// sine renders a tone with a 20 ms raised-cosine fade at each end; a hard
// onset is a broadband transient and would read as a clap.
func sine(hz, amp float64, ms int) []int16 {
	n := testRate * ms / 1000
	out := make([]int16, n)
	fade := testRate * 20 / 1000
	for i := range out {
		a := amp
		if i < fade {
			a *= 0.5 - 0.5*math.Cos(math.Pi*float64(i)/float64(fade))
		}
		if j := n - 1 - i; j < fade {
			a *= 0.5 - 0.5*math.Cos(math.Pi*float64(j)/float64(fade))
		}
		out[i] = int16(a * 32767 * math.Sin(2*math.Pi*hz*float64(i)/testRate))
	}
	return out
}

// noise is a deterministic xorshift broadband source. lowpassHz > 0 tilts it
// toward breath; fadeMS ramps the onset, which separates a blow from a clap.
func noise(amp float64, ms int, lowpassHz float64, fadeMS int) []int16 {
	n := testRate * ms / 1000
	out := make([]int16, n)
	var s uint64 = 0x2545F4914F6CDD1D
	var prev float64
	k := 0.0
	if lowpassHz > 0 {
		k = math.Exp(-2 * math.Pi * lowpassHz / testRate)
	}
	fade := testRate * fadeMS / 1000
	for i := range out {
		s ^= s << 13
		s ^= s >> 7
		s ^= s << 17
		v := (float64(s%20001)/10000 - 1) * amp
		if k > 0 {
			// Keeps the level above the detector's absolute floor.
			prev = k*prev + (1-k)*v
			v = prev * 6
		}
		if fade > 0 {
			if i < fade {
				v *= 0.5 - 0.5*math.Cos(math.Pi*float64(i)/float64(fade))
			}
			if j := n - 1 - i; j < fade {
				v *= 0.5 - 0.5*math.Cos(math.Pi*float64(j)/float64(fade))
			}
		}
		out[i] = int16(math.Max(-1, math.Min(1, v)) * 32767)
	}
	return out
}

func silence(ms int) []int16 { return make([]int16, testRate*ms/1000) }

func cat(parts ...[]int16) []int16 {
	var out []int16
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func collect(t *testing.T, cfg Config, samples []int16) []Frame {
	t.Helper()
	d := NewDetector(cfg)
	var got []Frame
	d.Write(samples, func(f Frame) {
		if f.Event != EventNone {
			got = append(got, f)
		}
	})
	return got
}

func clapCfg() Config { c := DefaultConfig(); c.EnableClap = true; return c }
func blowCfg() Config { c := DefaultConfig(); c.EnableBlow = true; return c }

func count(fs []Frame, e Event) int {
	n := 0
	for _, f := range fs {
		if f.Event == e {
			n++
		}
	}
	return n
}

func TestSilenceIsSilent(t *testing.T) {
	if got := collect(t, DefaultConfig(), silence(2000)); len(got) != 0 {
		t.Fatalf("silence produced %d events: %+v", len(got), got)
	}
}

func TestWhistleDetected(t *testing.T) {
	cfg := DefaultConfig()
	for _, hz := range []float64{900, 1400, 1800, 2500, 3200} {
		got := collect(t, cfg, cat(silence(300), sine(hz, 0.25, 600), silence(300)))
		if n := count(got, EventWhistle); n != 1 {
			t.Errorf("%.0f Hz: got %d whistles, want 1 (%+v)", hz, n, got)
			continue
		}
		for _, f := range got {
			if f.Event != EventWhistle {
				continue
			}
			// One DFT bin is 62.5 Hz; two bins of slack covers the Hann lobe.
			if math.Abs(f.PitchHz-hz) > 130 {
				t.Errorf("%.0f Hz: reported %.0f Hz", hz, f.PitchHz)
			}
		}
	}
}

func TestSustainedWhistleFiresOnce(t *testing.T) {
	got := collect(t, DefaultConfig(), cat(silence(200), sine(1800, 0.25, 3000), silence(200)))
	if n := count(got, EventWhistle); n != 1 {
		t.Fatalf("3s whistle fired %d times, want 1", n)
	}
}

func TestTwoWhistlesFireTwice(t *testing.T) {
	got := collect(t, DefaultConfig(), cat(
		silence(200), sine(1800, 0.25, 400),
		silence(600), sine(2200, 0.25, 400), silence(200)))
	if n := count(got, EventWhistle); n != 2 {
		t.Fatalf("two whistles fired %d times, want 2 (%+v)", n, got)
	}
}

func TestClapDetected(t *testing.T) {
	got := collect(t, clapCfg(), cat(silence(500), noise(0.6, 40, 0, 0), silence(500)))
	if n := count(got, EventClap); n != 1 {
		t.Fatalf("got %d claps, want 1 (%+v)", n, got)
	}
}

func TestBlowDetected(t *testing.T) {
	got := collect(t, blowCfg(), cat(silence(300), noise(0.5, 700, 180, 80), silence(300)))
	// A breath may re-fire once across a level dip.
	if n := count(got, EventBlow); n < 1 || n > 2 {
		t.Fatalf("got %d blows, want 1 or 2 (%+v)", n, got)
	}
	if n := count(got, EventWhistle); n != 0 {
		t.Errorf("breath registered as %d whistles", n)
	}
}

func TestBroadbandIsNotAWhistle(t *testing.T) {
	got := collect(t, DefaultConfig(), cat(silence(200), noise(0.35, 2000, 0, 30), silence(200)))
	if n := count(got, EventWhistle); n != 0 {
		t.Fatalf("broadband noise produced %d whistles (%+v)", n, got)
	}
}

func TestOutOfBandToneRejected(t *testing.T) {
	for _, hz := range []float64{300, 5200} {
		got := collect(t, DefaultConfig(), cat(silence(300), sine(hz, 0.3, 600), silence(300)))
		if n := count(got, EventWhistle); n != 0 {
			t.Errorf("%.0f Hz produced %d whistles, want 0", hz, n)
		}
	}
}

func TestQuietToneRejected(t *testing.T) {
	got := collect(t, DefaultConfig(), cat(silence(300), sine(1800, 0.006, 800), silence(300)))
	if n := count(got, EventWhistle); n != 0 {
		t.Fatalf("a -44 dBFS tone produced %d whistles", n)
	}
}

func TestReplayIsDeterministic(t *testing.T) {
	in := cat(silence(200), sine(1600, 0.25, 400), noise(0.6, 40, 0, 0),
		silence(400), sine(2400, 0.25, 400), silence(200))
	cfg := DefaultConfig()

	d := NewDetector(cfg)
	var a []Frame
	d.Write(in, func(f Frame) {
		if f.Event != EventNone {
			a = append(a, f)
		}
	})
	d.Reset()
	var b []Frame
	d.Write(in, func(f Frame) {
		if f.Event != EventNone {
			b = append(b, f)
		}
	})

	if len(a) != len(b) {
		t.Fatalf("replay produced %d events, first pass %d", len(b), len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("event %d differs:\n first %+v\nsecond %+v", i, a[i], b[i])
		}
	}
	if len(a) == 0 {
		t.Fatal("test signal produced no events, so determinism proved nothing")
	}
}

func TestChunkingDoesNotChangeResult(t *testing.T) {
	in := cat(silence(200), sine(1600, 0.25, 500), silence(300))
	cfg := DefaultConfig()

	whole := collect(t, cfg, in)

	d := NewDetector(cfg)
	var split []Frame
	for i := 0; i < len(in); i += 333 { // deliberately coprime with FrameLen
		end := min(i+333, len(in))
		d.Write(in[i:end], func(f Frame) {
			if f.Event != EventNone {
				split = append(split, f)
			}
		})
	}

	if len(whole) != len(split) {
		t.Fatalf("chunked read gave %d events, whole gave %d", len(split), len(whole))
	}
	for i := range whole {
		if whole[i] != split[i] {
			t.Fatalf("event %d differs:\n whole %+v\nchunked %+v", i, whole[i], split[i])
		}
	}
}

func TestDefaultBankLayout(t *testing.T) {
	cfg := DefaultConfig()
	if ms := 1000 * cfg.FrameLen / cfg.SampleRate; ms != 17 {
		t.Errorf("frame is %d ms, want 17", ms)
	}
	l := newBankLayout(cfg)
	if low := l.lowK1 - l.lowK0 + 1; low != 8 {
		t.Errorf("low bank has %d bins, want 8", low)
	}
	if tone := l.toneBins(); tone != 63 {
		t.Errorf("tone bank has %d bins, want 63", tone)
	}
}

func TestSetTaintRejectsAMaskOfTheWrongLength(t *testing.T) {
	cfg := DefaultConfig()
	d := NewDetector(cfg)
	if err := d.SetTaint(newEchoGate(cfg, 0.003).taint); err != nil {
		t.Fatalf("the gate's own mask was rejected: %v", err)
	}
	if err := d.SetTaint(make([]bool, 3)); err == nil {
		t.Fatal("a 3-bin mask was accepted")
	}
	if d.taint != nil {
		t.Error("a rejected mask left the previous taint in place")
	}
}
