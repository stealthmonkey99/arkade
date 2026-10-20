// SPDX-License-Identifier: GPL-2.0-or-later

// Package clock provides frame timing types and an injectable time source:
// SystemClock in production, ManualClock for deterministic tests.
package clock

import "time"

// FrameTime carries timing stats into every input/update/render phase.
type FrameTime struct {
	// Ticks is the number of ticks since the current app started.
	Ticks uint64
	// DurationMs is the total time since the current app started.
	DurationMs float64
	// ElapsedMs is the time since the previous tick began.
	ElapsedMs float64
	// LagMs is the time since the current tick began.
	LagMs float64
	// EtaNextTickInMs estimates time remaining until the next tick.
	EtaNextTickInMs float64
	// DroppedFrames counts frames skipped because a tick overran its budget.
	DroppedFrames uint64
}

// Clock is the time source for the engine and everything driven by it.
type Clock interface {
	Now() time.Time
	// Sleep blocks for roughly d (no-op or instant for manual clocks).
	Sleep(d time.Duration)
}

// SystemClock is the real wall clock.
type SystemClock struct{}

func (SystemClock) Now() time.Time        { return time.Now() }
func (SystemClock) Sleep(d time.Duration) { time.Sleep(d) }

// ManualClock is a test clock that only moves when told to.
type ManualClock struct {
	t time.Time
}

// NewManualClock starts at an arbitrary fixed epoch.
func NewManualClock() *ManualClock {
	return &ManualClock{t: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (m *ManualClock) Now() time.Time { return m.t }

// Sleep advances the clock instead of blocking.
func (m *ManualClock) Sleep(d time.Duration) { m.t = m.t.Add(d) }

// Advance moves the clock forward by d.
func (m *ManualClock) Advance(d time.Duration) { m.t = m.t.Add(d) }
