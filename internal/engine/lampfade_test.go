// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// The fade steps on the engine's per-tick delta: the 30fps rig advances 33.333ms a frame, so a 600ms fade ends after eighteen.
func TestLampFadeRidesTheTick(t *testing.T) {
	rig := newPeriphRig(t)
	rig.Tick(1)
	// The lamp app rewrites its buttons every Update, which would cancel the
	// fade on sight.
	liveLampApp.quiet = true
	defer func() { liveLampApp.quiet = false }()

	hw := virtualHW(t, rig)
	release := rig.RT.WatchPeriph()
	defer release()
	rig.RT.SetLampPolicy(engine.LampsOn)
	rig.RT.FadeLamps(0, hardware.LampLevels{}, lampAppSeats[0], 600)

	if got := hw.LampLevels(0)[0]; got != hardware.LampLevelMax {
		t.Fatalf("lamp = %d before the first tick, want it still full", got)
	}
	rig.Tick(9)
	mid := hw.LampLevels(0)[0]
	if mid == 0 || mid == hardware.LampLevelMax {
		t.Fatalf("lamp = %d half way through the fade, want it part-way down", mid)
	}
	rig.Tick(9)
	if got := hw.LampLevels(0)[0]; got != 0 {
		t.Errorf("lamp = %d after the full 600ms, want it dark", got)
	}
	if got := rig.RT.PeriphSnapshot().Levels[0][0]; got != 0 {
		t.Errorf("published level = %d, want the intent the fade targeted", got)
	}
}
