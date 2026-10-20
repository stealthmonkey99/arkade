// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"
	"time"
)

func TestFrameClockStatsMeasureFromTheTickAndTheLoad(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	budget := time.Second / 30
	var f frameClock
	f.reset(epoch)
	for i := 1; i <= 3; i++ {
		if i > 1 {
			f.end()
		}
		f.begin(epoch.Add(time.Duration(i) * budget))
	}
	now := epoch.Add(3*budget + 10*time.Millisecond)
	st := f.stats(now, budget)
	if st.Ticks != 3 {
		t.Errorf("Ticks = %d, want 3", st.Ticks)
	}
	if want := ms(3*budget + 10*time.Millisecond); st.DurationMs != want {
		t.Errorf("DurationMs = %v, want %v since the load", st.DurationMs, want)
	}
	if want := ms(budget + 10*time.Millisecond); st.ElapsedMs != want {
		t.Errorf("ElapsedMs = %v, want %v since the previous tick began", st.ElapsedMs, want)
	}
	if st.LagMs != 10 || st.EtaNextTickInMs != ms(budget)-10 {
		t.Errorf("LagMs = %v, Eta = %v; want 10 and budget-10", st.LagMs, st.EtaNextTickInMs)
	}
}

func TestFrameClockResumeShiftsTheZeroPastTheGap(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var f frameClock
	f.reset(epoch)
	f.begin(epoch.Add(time.Second))
	f.end()
	parked := f.park(epoch.Add(time.Second))

	f.reset(epoch.Add(2 * time.Second)) // attract ran meanwhile
	f.begin(epoch.Add(3 * time.Second))

	wake := epoch.Add(time.Minute)
	f.resume(parked, wake)
	st := f.stats(wake, time.Second/30)
	if st.Ticks != 1 {
		t.Errorf("Ticks = %d, want the parked app's 1", st.Ticks)
	}
	if st.DurationMs != 1000 {
		t.Errorf("DurationMs = %v after a minute away, want 1000: the gap is hidden", st.DurationMs)
	}
	if st.ElapsedMs != 0 {
		t.Errorf("ElapsedMs = %v on the wake tick, want 0", st.ElapsedMs)
	}
}

func TestFrameClockResetDropsEverything(t *testing.T) {
	epoch := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var f frameClock
	f.reset(epoch)
	f.begin(epoch.Add(time.Second))
	f.dropped = 4
	f.reset(epoch.Add(2 * time.Second))
	if f.ticks != 0 || f.dropped != 0 || !f.initTime.Equal(epoch.Add(2*time.Second)) {
		t.Errorf("after reset: %+v", f)
	}
}
