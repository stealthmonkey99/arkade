// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"strings"
	"testing"
)

func newTestWatcher() (*calibrationWatcher, *[]string) {
	var logs []string
	return newCalibrationWatcher(DefaultConfig().FrameLen, func(s string) { logs = append(logs, s) }), &logs
}

func TestCalibrationWatcherIsIdleUntilArmed(t *testing.T) {
	w, logs := newTestWatcher()
	for range 2 * w.window {
		w.observe(true, 100, 20)
	}
	if got := w.snapshot(); got != (Calibration{}) || len(*logs) != 0 {
		t.Fatalf("an unarmed watcher recorded %+v and logged %q", got, *logs)
	}
}

func TestCalibrationWatcherReportsTheLastAlignmentInTheWindow(t *testing.T) {
	w, logs := newTestWatcher()
	w.arm()
	if got := w.snapshot(); !got.Pending || got.Heard {
		t.Fatalf("just armed: %+v", got)
	}
	w.observe(true, Rate/10, 12)
	w.observe(true, Rate/20, 18)
	for range w.window - 3 {
		w.observe(false, 0, 0)
	}
	if got := w.snapshot(); !got.Pending || !got.Heard || got.DelayMs != 50 || got.ERLE != 18 {
		t.Fatalf("one frame before the window closes: %+v", got)
	}
	if len(*logs) != 0 {
		t.Fatalf("logged before the window closed: %q", *logs)
	}
	w.observe(false, 0, 0)
	if got := w.snapshot(); got.Pending || !got.Heard {
		t.Fatalf("after the window: %+v", got)
	}
	if len(*logs) != 1 || !strings.Contains((*logs)[0], "calibrated on the stinger") {
		t.Fatalf("logs %q", *logs)
	}
	w.observe(true, 1, 1)
	if got := w.snapshot(); got.DelayMs != 50 || len(*logs) != 1 {
		t.Fatalf("a closed window kept watching: %+v %q", got, *logs)
	}
}

func TestCalibrationWatcherCallsADeafMicrophone(t *testing.T) {
	w, logs := newTestWatcher()
	w.arm()
	for range w.window {
		w.observe(false, 0, 0)
	}
	if got := w.snapshot(); got.Pending || got.Heard {
		t.Fatalf("after a window of nothing: %+v", got)
	}
	if len(*logs) != 1 || !strings.Contains((*logs)[0], "heard nothing") {
		t.Fatalf("logs %q", *logs)
	}
}

func TestCalibrationRearmingRestartsTheWindow(t *testing.T) {
	w, _ := newTestWatcher()
	w.arm()
	w.observe(true, 10, 10)
	w.arm()
	if got := w.snapshot(); !got.Pending || got.Heard {
		t.Fatalf("a re-armed watcher kept the earlier stinger's result: %+v", got)
	}
}
