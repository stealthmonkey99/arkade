// SPDX-License-Identifier: GPL-2.0-or-later

package evening

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// framePeriod is the level-frame cadence of testdata/tv-rms.csv.
const framePeriod = 17400 * time.Microsecond

// tvSeries is 90 s of recorded level frames with a television two metres
// away; it is the oracle for "loud", and the synthetic series is the control.
func tvSeries(t *testing.T) []float64 {
	t.Helper()
	raw, err := os.ReadFile("testdata/tv-rms.csv")
	if err != nil {
		t.Fatalf("reading the tv recording: %v", err)
	}
	var out []float64
	for _, f := range strings.Split(string(raw), ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			t.Fatalf("bad level %q: %v", f, err)
		}
		out = append(out, v)
	}
	if len(out) < 100 {
		t.Fatalf("expected a long recording, got %d frames", len(out))
	}
	return out
}

// quietSeries synthesises the room with the television off: the floor seen in
// the recording's pauses, plus jitter.
func quietSeries(n int, seed int64) []float64 {
	rng := rand.New(rand.NewSource(seed))
	out := make([]float64, n)
	for i := range out {
		out[i] = 0.0010 + 0.0003*(rng.Float64()-0.5)
	}
	return out
}

// feed pushes a series into the tracker at framePeriod from now and returns the next instant.
func feed(tr *tracker, now time.Time, series []float64) time.Time {
	for _, v := range series {
		tr.Push(now, v)
		now = now.Add(framePeriod)
	}
	return now
}

// repeat loops a series until it covers at least d.
func repeat(series []float64, d time.Duration) []float64 {
	want := int(d/framePeriod) + 1
	out := make([]float64, 0, want)
	for len(out) < want {
		out = append(out, series...)
	}
	return out[:want]
}

func TestTrackerTriggersOnTheTelevision(t *testing.T) {
	tr := newTracker(SensMedium)
	start := time.Date(2026, 9, 15, 21, 0, 0, 0, time.UTC)
	tv := tvSeries(t)

	var fired time.Duration
	now := start
	for _, v := range tv {
		tr.Push(now, v)
		if tr.Wants() && fired == 0 {
			fired = now.Sub(start)
		}
		now = now.Add(framePeriod)
	}
	if fired == 0 {
		t.Fatal("the television never triggered the tracker")
	}
	if fired > 25*time.Second {
		t.Errorf("the television took %v to trigger, want within 25s", fired)
	}
	if !tr.Wants() {
		t.Error("the tracker dropped the television before the recording ended")
	}

	// Ten more minutes of television must not become the new normal.
	feed(tr, now, repeat(tv, 10*time.Minute))
	if !tr.Wants() {
		t.Error("ten more minutes of television released the tracker")
	}
}

func TestTrackerIgnoresAQuietRoom(t *testing.T) {
	start := time.Date(2026, 9, 15, 21, 0, 0, 0, time.UTC)
	for _, s := range []Sensitivity{SensLow, SensMedium, SensHigh} {
		tr := newTracker(s)
		now := start
		for _, v := range quietSeries(int(10*time.Minute/framePeriod), 7) {
			tr.Push(now, v)
			if tr.Wants() {
				t.Fatalf("%s: a quiet room triggered at %v", s, now.Sub(start))
			}
			now = now.Add(framePeriod)
		}
	}
}

func TestTrackerReleasesWhenTheRoomGoesQuiet(t *testing.T) {
	tr := newTracker(SensMedium)
	start := time.Date(2026, 9, 15, 21, 0, 0, 0, time.UTC)

	now := feed(tr, start, tvSeries(t))
	if !tr.Wants() {
		t.Fatal("the television should have triggered before the room goes quiet")
	}

	quiet := quietSeries(int(4*time.Minute/framePeriod), 11)
	var released time.Duration
	quietStart := now
	for _, v := range quiet {
		tr.Push(now, v)
		if !tr.Wants() {
			released = now.Sub(quietStart)
			break
		}
		now = now.Add(framePeriod)
	}
	if released == 0 {
		t.Fatal("four minutes of quiet never released the tracker")
	}
	// The recording was not loud in every second, so release lands short of the full window.
	if released < 2*time.Minute {
		t.Errorf("released after only %v of quiet; the %v window should hold it longer", released, releaseWindow)
	}
	if released > releaseWindow+30*time.Second {
		t.Errorf("released after %v of quiet, want inside the %v window", released, releaseWindow)
	}
}

func TestTrackerSensitivityOrdering(t *testing.T) {
	start := time.Date(2026, 9, 15, 21, 0, 0, 0, time.UTC)
	tv := repeat(tvSeries(t), 2*time.Minute)

	fire := func(s Sensitivity) (time.Duration, bool) {
		tr := newTracker(s)
		now := start
		for _, v := range tv {
			tr.Push(now, v)
			if tr.Wants() {
				return now.Sub(start), true
			}
			now = now.Add(framePeriod)
		}
		return 0, false
	}

	high, okHigh := fire(SensHigh)
	med, okMed := fire(SensMedium)
	if !okHigh || !okMed {
		t.Fatalf("high/medium must both hear the television (high=%v medium=%v)", okHigh, okMed)
	}
	if high > med {
		t.Errorf("high sensitivity fired at %v, later than medium's %v", high, med)
	}
}

// The gate polls once a second, not per level frame; this thins the recording to match.
func TestTrackerTriggersOnTheOnceASecondPoll(t *testing.T) {
	const poll = time.Second
	every := int(poll / framePeriod)
	tv := subsample(repeat(tvSeries(t), 3*time.Minute), every)
	quiet := subsample(quietSeries(int(10*time.Minute/framePeriod), 7), every)
	start := time.Date(2026, 9, 15, 21, 0, 0, 0, time.UTC)

	tr := newTracker(SensMedium)
	now := start
	var fired time.Duration
	for _, v := range tv {
		tr.Push(now, v)
		if tr.Wants() && fired == 0 {
			fired = now.Sub(start)
		}
		now = now.Add(poll)
	}
	if fired == 0 || fired > 25*time.Second {
		t.Errorf("the television triggered the once-a-second poll at %v, want within 25s", fired)
	}

	tr = newTracker(SensMedium)
	now = start
	for _, v := range quiet {
		tr.Push(now, v)
		if tr.Wants() {
			t.Fatalf("a quiet room triggered the once-a-second poll at %v", now.Sub(start))
		}
		now = now.Add(poll)
	}
}

func subsample(series []float64, n int) []float64 {
	var out []float64
	for i := 0; i < len(series); i += n {
		out = append(out, series[i])
	}
	return out
}

func TestTrackerResetsOnAGapInTheStream(t *testing.T) {
	tr := newTracker(SensMedium)
	start := time.Date(2026, 9, 15, 21, 0, 0, 0, time.UTC)
	now := feed(tr, start, tvSeries(t))
	if !tr.Wants() {
		t.Fatal("expected the television to have triggered")
	}

	tr.Push(now.Add(sampleGapReset+time.Second), 0.002)
	if tr.Wants() {
		t.Error("a gap in the stream must start the tracker over, not carry the verdict across it")
	}
}
