// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func sineBands(hz, amp float64) [hardware.MicBandCount]float64 {
	b := newBands()
	for i := 0; i < bandFFT; i++ {
		b.push(amp * math.Sin(2*math.Pi*hz*float64(i)/float64(Rate)))
	}
	return b.analyse()
}

func total(v [hardware.MicBandCount]float64) float64 {
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum
}

// bandProbes are three frequencies spread across a band, geometrically.
func bandProbes(i int) [3]float64 {
	e := bandEdgesHz()
	lo, hi := e[i], e[i+1]
	var out [3]float64
	for j, f := range [3]float64{0.25, 0.5, 0.75} {
		out[j] = lo * math.Pow(hi/lo, f)
	}
	return out
}

// The bottom band is only three DFT bins wide and a Hann window spreads a
// tone over four, so a sine near a band edge is shared with its neighbour;
// the bank as a whole must still read 1.0 for a full-scale sine.
func TestBandsReadFullScaleSineAnywhereInsideTheBand(t *testing.T) {
	for i := range hardware.MicBandCentersHz {
		for _, hz := range bandProbes(i) {
			got := sineBands(hz, 1)
			if math.Abs(total(got)-1) > 0.02 {
				t.Errorf("band %d, %.0f Hz: the bank reads %.3f in total, want ~1.0", i, hz, total(got))
			}
			if got[i] < 0.5 {
				t.Errorf("band %d, %.0f Hz: reads %.3f, want the band that contains it to hold most of it", i, hz, got[i])
			}
		}
		hz := hardware.MicBandCentersHz[i]
		if got := sineBands(hz, 1)[i]; math.Abs(got-1) > 0.05 {
			t.Errorf("band %d centre (%.0f Hz) reads %.3f, want ~1.0", i, hz, got)
		}
	}
}

func TestBandsReadTonesBetweenTheCentres(t *testing.T) {
	e := bandEdgesHz()
	for _, hz := range []float64{110, 320, 1000, 1800} {
		want := -1
		for i := range hardware.MicBandCentersHz {
			if hz >= e[i] && hz < e[i+1] {
				want = i
			}
		}
		got := sineBands(hz, 1)
		if got[want] < 0.8 {
			t.Errorf("%.0f Hz reads %.3f in band %d, want ~1.0", hz, got[want], want)
		}
	}
}

func TestBandsReadPowerNotAmplitude(t *testing.T) {
	hz := hardware.MicBandCentersHz[2]
	if got := sineBands(hz, 0.5)[2]; math.Abs(got-0.25) > 0.02 {
		t.Errorf("half-amplitude reads %.4f, want ~0.25 — a quarter of the power", got)
	}
}

func TestBandsSilenceIsZero(t *testing.T) {
	for i, v := range sineBands(0, 0) {
		if v != 0 {
			t.Errorf("band %d = %v on silence, want 0", i, v)
		}
	}
}

func TestBandsSpreadWhiteNoiseByBandwidth(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	b := newBands()
	for i := 0; i < bandFFT; i++ {
		b.push(rng.NormFloat64() * 0.1)
	}
	got := b.analyse()
	for i, v := range got {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			t.Fatalf("band %d = %v on noise", i, v)
		}
	}
	if got[7] <= 10*got[0] {
		t.Errorf("white noise: the top band (%.3e) should dwarf the bottom (%.3e)", got[7], got[0])
	}
}

func TestBandFramesKeepTheTransientPeak(t *testing.T) {
	const burst = Rate / 25 // 40 ms
	samples := make([]float64, 6*levelWindow)
	for i := range samples {
		samples[i] = 0.001 * math.Sin(2*math.Pi*740*float64(i)/Rate)
	}
	at := 4 * levelWindow
	for i := at; i < at+burst; i++ {
		samples[i] += math.Sin(2 * math.Pi * 740 * float64(i) / Rate)
	}
	frames := mic740(BandFrames(samples))
	if len(frames) < 4 {
		t.Fatalf("expected several frames, got %d", len(frames))
	}
	quiet, loud := frames[2], 0.0
	for _, v := range frames[3:] {
		loud = math.Max(loud, v)
	}
	if loud < 0.2 {
		t.Errorf("a 40 ms full-scale burst reached only %.3f in its band, want it plainly loud", loud)
	}
	if loud < 1000*quiet {
		t.Errorf("burst %.3e is not far enough over the quiet %.3e", loud, quiet)
	}
}

func mic740(frames [][hardware.MicBandCount]float64) []float64 {
	out := make([]float64, len(frames))
	for i, f := range frames {
		out[i] = f[4]
	}
	return out
}

func BenchmarkBandsAnalyse(b *testing.B) {
	bank := newBands()
	for i := 0; i < bandFFT; i++ {
		bank.push(math.Sin(float64(i)))
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bank.analyse()
	}
}
