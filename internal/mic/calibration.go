// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"fmt"
	"sync"
)

// Calibration is what a stinger taught the pipeline.
type Calibration struct {
	Pending bool    `json:"pending"`
	Heard   bool    `json:"heard"`
	DelayMs float64 `json:"delayMs"`
	ERLE    float64 `json:"erleDb"`
}

// calibrateWindowMs is how long after Calibrate the pipeline waits for the
// stinger's alignment before calling the microphone deaf.
const calibrateWindowMs = 3000

// calibrationWatcher follows the canceller's alignment for a fixed number
// of analysis frames after a stinger, then reports whether it was heard.
type calibrationWatcher struct {
	mu        sync.Mutex
	result    Calibration
	window    int
	remaining int // frames left in the open window; 0 when none is open
	log       func(string)
}

func newCalibrationWatcher(frameLen int, log func(string)) *calibrationWatcher {
	return &calibrationWatcher{window: calibrateWindowMs * Rate / 1000 / frameLen, log: log}
}

func (w *calibrationWatcher) arm() {
	w.mu.Lock()
	w.result = Calibration{Pending: true}
	w.remaining = w.window
	w.mu.Unlock()
}

// observe takes one analysis frame's alignment; the last aligned frame in
// the window is the one reported.
func (w *calibrationWatcher) observe(aligned bool, delay int, erle float64) {
	w.mu.Lock()
	if w.remaining == 0 {
		w.mu.Unlock()
		return
	}
	if aligned {
		w.result = Calibration{Pending: true, Heard: true, DelayMs: float64(delay) * 1000 / Rate, ERLE: erle}
	}
	w.remaining--
	closed := w.remaining == 0
	if closed {
		w.result.Pending = false
	}
	r := w.result
	w.mu.Unlock()

	switch {
	case !closed:
	case r.Heard:
		w.log(fmt.Sprintf("mic: calibrated on the stinger: echo at %.0f ms, %.0f dB cancelled", r.DelayMs, r.ERLE))
	default:
		w.log("mic: the stinger played and the microphone heard nothing it could align on — check the capture device and its gain")
	}
}

func (w *calibrationWatcher) snapshot() Calibration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.result
}
