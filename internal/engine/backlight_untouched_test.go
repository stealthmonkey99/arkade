// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"reflect"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/render"
)

type backlightRecorder struct {
	*hardware.Virtual
	writes []uint8
}

func (r *backlightRecorder) SetBacklightLevel(level uint8) {
	r.writes = append(r.writes, level)
	r.Virtual.SetBacklightLevel(level)
}

// The backlight is written only once something explicitly sets it; the startup brightness restore must not relight panels the boot screen fades in.
func TestBacklightUntouchedUntilSet(t *testing.T) {
	rec := &backlightRecorder{Virtual: hardware.NewVirtual()}
	s := newShadowHardware(rec)
	s.setPanelBrightness(60)
	s.setLampPolicy(LampsOn)
	s.setLampPolicy(LampsMatchScreen)
	if len(rec.writes) != 0 {
		t.Fatalf("startup re-pushes wrote the backlight %v, want nothing", rec.writes)
	}
	s.SetBacklightLevel(0)
	s.fadeBacklight(hardware.LampLevelMax, 0, Linear)
	if want := []uint8{0, scaleLevel(hardware.LampLevelMax, 60)}; !reflect.DeepEqual(rec.writes, want) {
		t.Fatalf("explicit sets wrote %v, want %v", rec.writes, want)
	}
}

// Neither startup restore (panel brightness, lamp policy) may relight the LCDs before the boot bloom.
func TestStartupRestoresLeaveTheBacklightAlone(t *testing.T) {
	rec := &backlightRecorder{Virtual: hardware.NewVirtual()}
	rt, err := New(Options{
		Renderer: render.NewCapture(true),
		Clock:    clock.NewManualClock(),
		Audio:    audio.NewRecorder(),
		DataDir:  t.TempDir(),
		Hardware: rec,
	})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetPanelBrightness(60)
	rt.SetLampPolicy(LampsMatchScreen)
	rt.SetLampPolicy(LampsOn)
	if len(rec.writes) != 0 {
		t.Fatalf("startup restores wrote the backlight %v, want nothing until the boot screen sets it", rec.writes)
	}
}
