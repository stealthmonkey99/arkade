// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"testing"
	"time"
)

func TestMotionDebounce(t *testing.T) {
	t0 := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	var d debounce

	d.edge(true, t0)
	if !d.motion || !d.sensor {
		t.Fatal("rising edge should assert motion and sensor state")
	}

	d.edge(false, t0.Add(time.Second))
	d.expire(t0.Add(3 * time.Second))
	if !d.motion {
		t.Error("motion should persist inside the timeout window")
	}
	if d.sensor {
		t.Error("sensor state should track the raw level")
	}
	d.expire(t0.Add(6 * time.Second))
	if d.motion {
		t.Error("motion should clear once the timeout elapses")
	}
}

func TestMotionRisingEdgeCancelsPendingClear(t *testing.T) {
	t0 := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	var d debounce

	d.edge(true, t0)
	d.edge(false, t0.Add(time.Second))
	d.edge(true, t0.Add(2*time.Second)) // re-trigger cancels the timer

	d.expire(t0.Add(10 * time.Second))
	if !d.motion {
		t.Error("re-trigger should cancel the pending clear")
	}

	d.edge(false, t0.Add(11*time.Second))
	d.expire(t0.Add(15 * time.Second))
	if !d.motion {
		t.Error("timeout should measure from the latest falling edge")
	}
	d.expire(t0.Add(16 * time.Second))
	if d.motion {
		t.Error("motion should clear after the re-armed timeout")
	}
}

func TestMotionExpireWithoutDeadlineIsNoop(t *testing.T) {
	var d debounce
	d.edge(true, time.Now())
	d.expire(time.Now().Add(time.Hour))
	if !d.motion {
		t.Error("expire must not clear motion while the sensor is high")
	}
}
