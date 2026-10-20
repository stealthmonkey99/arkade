// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// Every Remote method is called from HTTP handlers while the engine ticks;
// run under -race this pins that none of them reaches live engine state.
func TestRemoteIsSafeWhileTheEngineTicks(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: lcdAppSlug, FlightDir: t.TempDir()})
	var remote engine.Remote = rig.RT
	frame := solidFrame(t, rig.RT, display.Red)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		release := remote.WatchPeriph()
		defer release()
		var cursor uint64
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = remote.Status()
			_ = remote.PeriphSnapshot()
			_, cursor, _ = remote.DrainPeriphInputs(cursor)
			_, _ = remote.DumpFlight("test", "")
			remote.RequestAttractPreview(lcdAppSlug)
			_ = remote.ShowScreensaver("nope")
			remote.RequestLabPreview(engine.LabPreview{Frames: []engine.LabPreviewFrame{{Data: frame}}, Timeout: time.Second})
			remote.CancelLabPreview()
			remote.RequestLightShow(engine.LightShow{Steps: []engine.LightStep{{Seats: seats(1, 1)}}})
			remote.CancelLightShow()
			_ = remote.Microphone()
		}
	}()
	rig.Tick(30)
	close(stop)
	wg.Wait()
}
