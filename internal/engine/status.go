// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"runtime"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// MotionActiveWindow is how recent presence must be to count as motion: Status's MotionActive and the
// settings app's MOTION line. It matches the PIR's own debounce, so the virtual and real cabinets agree.
const MotionActiveWindow = 5 * time.Second

// Status is a point-in-time snapshot of engine state for the HTTP dashboard, and the only Runtime
// state handlers may read. Numeric fields come from the engine clock, not wall time.
type Status struct {
	// Slug is the running app's manifest slug, or "" (dark, or quit with no shell).
	Slug string `json:"slug"`
	// SuspendedSlug is the app parked behind the attract screensaver, or "".
	SuspendedSlug string `json:"suspendedSlug"`
	// StandbyState is "active", "attract" or "dark".
	StandbyState string `json:"standbyState"`
	// StandbyEnabled is false when Options.AttractAfter is 0: the state is then pinned "active" and attract
	// previews are dropped.
	StandbyEnabled bool `json:"standbyEnabled"`
	// ExternalLaunchReady reports whether a RequestLaunch would be honored: shell menu foreground and
	// cabinet awake.
	ExternalLaunchReady bool `json:"externalLaunchReady"`
	// LabPreviewActive reports whether an asset-lab preview owns any peripheral.
	LabPreviewActive bool `json:"labPreviewActive"`
	// LightShowActive reports whether a lab lamp choreography owns the lamps; independent of LabPreviewActive.
	LightShowActive bool `json:"lightShowActive"`
	// DisplayCols/DisplayRows come from the live Display; a preview frame must match them exactly.
	DisplayCols int `json:"displayCols"`
	DisplayRows int `json:"displayRows"`
	// Ticks is the current app's frame counter (resets on app switch).
	Ticks uint64 `json:"ticks"`
	// UptimeMs is engine-clock time since the Runtime was constructed; unlike DurationMs it survives app
	// switches.
	UptimeMs float64 `json:"uptimeMs"`
	// DurationMs is engine-clock time since the current app loaded.
	DurationMs float64 `json:"durationMs"`
	// DroppedFrames counts frames skipped because a tick overran its budget.
	DroppedFrames uint64 `json:"droppedFrames"`
	// HeapMB/SysMB are runtime.MemStats readings sampled about once a second. Nondeterministic, so
	// golden-tested screens omit them.
	HeapMB float64 `json:"heapMB"`
	SysMB  float64 `json:"sysMB"`
	// TempF is the SoC temperature in °F, or nil without a thermal sensor or sample.
	TempF   *float64 `json:"tempF,omitempty"`
	FanOn   bool     `json:"fanOn"`
	FanMode string   `json:"fanMode,omitempty"`
	// MotionAgoMs is nil until the hardware reports a motion edge. Motion and direct input both feed
	// rt.PresenceFor.
	MotionActive bool     `json:"motionActive"`
	MotionAgoMs  *float64 `json:"motionAgoMs,omitempty"`
}

// memSample is the heap and system size, read about once a second because
// runtime.ReadMemStats stops the world.
type memSample struct{ heapMB, sysMB float64 }

func (m *memSample) take() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.heapMB = float64(ms.HeapAlloc) / (1 << 20)
	m.sysMB = float64(ms.Sys) / (1 << 20)
}

func (rt *Runtime) samplePerf() {
	if rt.frames.ticks%uint64(rt.targetFPS) != 0 {
		return
	}
	rt.mem.take()
}

// PerfStats returns the last sampled heap/system MB for status displays.
func (rt *Runtime) PerfStats() (heapMB, sysMB float64) { return rt.mem.heapMB, rt.mem.sysMB }

// Status returns the latest published snapshot; safe from any goroutine. Handlers must observe the
// engine only through it: other Runtime state belongs to the engine goroutine.
func (rt *Runtime) Status() Status {
	if p := rt.status.Load(); p != nil {
		return *p
	}
	return Status{}
}

// publishStatus builds and atomically publishes the snapshot. Deferred at the top of Tick; runs on the
// engine goroutine. Keep it cheap: about one allocation per tick.
func (rt *Runtime) publishStatus() {
	stats := rt.Stats()

	var tempF *float64
	if tc, ok := rt.Hardware.(hardware.ThermalSensor); ok {
		if degF, ok2 := tc.CPUTempF(); ok2 {
			tempF = &degF
		}
	}

	fanOn := false
	fanMode := ""
	if fc, ok := rt.Hardware.(hardware.CoolingFan); ok {
		fanOn = fc.FanOn()
		fanMode = string(fc.FanMode())
	}

	var motionAgoMs *float64
	motionActive := false
	if lm, ok := rt.Hardware.(hardware.MotionHistory); ok {
		if _, everFired := lm.LastMotion(); everFired {
			// The hardware timestamp is wall-clock; derive the age from the engine presence clock.
			ago := rt.PresenceFor()
			ms := float64(ago.Microseconds()) / 1000
			motionAgoMs = &ms
			motionActive = ago < MotionActiveWindow
		}
	}

	launchReady := rt.atShellMenu()

	cols, rows := rt.Display.Size()

	rt.status.Store(&Status{
		Slug:                rt.CurrentSlug(),
		SuspendedSlug:       rt.SuspendedSlug(),
		StandbyState:        rt.StandbyState().String(),
		StandbyEnabled:      rt.StandbyEnabled(),
		ExternalLaunchReady: launchReady,
		LabPreviewActive:    rt.LabPreviewActive(),
		LightShowActive:     rt.LightShowActive(),
		DisplayCols:         cols,
		DisplayRows:         rows,
		Ticks:               stats.Ticks,
		UptimeMs:            float64(rt.Clock.Now().Sub(rt.bootTime).Microseconds()) / 1000,
		DurationMs:          stats.DurationMs,
		DroppedFrames:       stats.DroppedFrames,
		HeapMB:              rt.mem.heapMB,
		SysMB:               rt.mem.sysMB,
		TempF:               tempF,
		FanOn:               fanOn,
		FanMode:             fanMode,
		MotionActive:        motionActive,
		MotionAgoMs:         motionAgoMs,
	})
}
