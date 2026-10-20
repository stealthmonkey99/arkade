// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
)

// frameClock is the loop's time base: the tick counter and the instants a
// FrameTime derives from. It knows nothing of the Runtime, so a test can
// drive one directly.
type frameClock struct {
	ticks     uint64
	initTime  time.Time // the loaded app's zero; shifted on resume
	prevTick  time.Time
	tickStart time.Time
	dropped   uint64
}

// reset starts a fresh app clock at now.
func (f *frameClock) reset(now time.Time) {
	*f = frameClock{initTime: now, prevTick: now, tickStart: now}
}

// begin opens a tick at now.
func (f *frameClock) begin(now time.Time) {
	f.ticks++
	f.tickStart = now
}

// end closes the tick, so the next one's ElapsedMs measures from it.
func (f *frameClock) end() { f.prevTick = f.tickStart }

// stats is the FrameTime for the instant now.
func (f *frameClock) stats(now time.Time, budget time.Duration) clock.FrameTime {
	lag := now.Sub(f.tickStart)
	return clock.FrameTime{
		Ticks:           f.ticks,
		DurationMs:      ms(now.Sub(f.initTime)),
		ElapsedMs:       ms(now.Sub(f.prevTick)),
		LagMs:           ms(lag),
		EtaNextTickInMs: ms(budget) - ms(lag),
		DroppedFrames:   f.dropped,
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// parkedClock is what a suspended app's clock needs to resume unnoticed.
type parkedClock struct {
	ticks    uint64
	initTime time.Time
	at       time.Time
}

// park records the clock for an app being suspended at now.
func (f *frameClock) park(now time.Time) parkedClock {
	return parkedClock{ticks: f.ticks, initTime: f.initTime, at: now}
}

// resume restores a parked clock with its zero shifted past the gap, so
// DurationMs carries on from where the app stopped.
func (f *frameClock) resume(p parkedClock, now time.Time) {
	f.initTime = p.initTime.Add(now.Sub(p.at))
	f.ticks = p.ticks
	f.prevTick, f.tickStart = now, now
}
