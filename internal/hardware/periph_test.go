// SPDX-License-Identifier: GPL-2.0-or-later

package hardware

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestLightMaskRoundTripsThroughNames(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		mask  LightMask
		str   string
	}{
		{"dark", nil, 0, "-"},
		{"one", []string{"joya"}, 1 << 0, "joya"},
		{"two, out of roster order", []string{"joyd", "joyb"}, 1<<3 | 1<<1, "joyb+joyd"},
		{"every lit button", LightButtons, AllLights, "joya+joyb+joyc+joyd+joyaux1+joyaux2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LightMaskFromNames(tc.names)
			if err != nil {
				t.Fatalf("LightMaskFromNames(%v): %v", tc.names, err)
			}
			if got != tc.mask {
				t.Errorf("mask = %d, want %d", got, tc.mask)
			}
			if got.String() != tc.str {
				t.Errorf("String() = %q, want %q", got.String(), tc.str)
			}
			back, err := LightMaskFromNames(got.Names())
			if err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if back != tc.mask {
				t.Errorf("round trip = %d, want %d", back, tc.mask)
			}
		})
	}
}

func TestLightMaskFromNamesRefusesAnUnknownButton(t *testing.T) {
	if _, err := LightMaskFromNames([]string{"joya", "joye"}); err == nil {
		t.Fatal("an unknown button name must be an error, not a silent skip")
	}
}

func TestLightMaskButtonLightsIsComplete(t *testing.T) {
	bl := LightMask(1 << 1).ButtonLights()
	if len(bl) != len(LightButtons) {
		t.Fatalf("ButtonLights() has %d entries, want all %d buttons", len(bl), len(LightButtons))
	}
	want := ButtonLights{"joya": false, "joyb": true, "joyc": false, "joyd": false, "joyaux1": false, "joyaux2": false}
	if !reflect.DeepEqual(bl, want) {
		t.Errorf("ButtonLights() = %v, want %v", bl, want)
	}
}

func TestLightMaskOfToleratesShortAndLongStates(t *testing.T) {
	if got := LightMaskOf([]bool{true}); got != 1 {
		t.Errorf("short states: mask = %d, want 1", got)
	}
	long := make([]bool, len(LightButtons)+3)
	for i := range long {
		long[i] = true
	}
	if got := LightMaskOf(long); got != AllLights {
		t.Errorf("long states: mask = %d, want %d (extras ignored)", got, AllLights)
	}
	if LightMask(0).Has(-1) || LightMask(AllLights).Has(len(LightButtons)) {
		t.Error("Has must report false outside the roster rather than panicking")
	}
}

func TestVirtualMicIsSyntheticAndSaysSo(t *testing.T) {
	m := NewVirtualMic()
	info := m.MicInfo()
	if !info.Available || !info.Synthetic || info.Source != "virtual" {
		t.Errorf("MicInfo = %+v, want an available synthetic source", info)
	}
	if info.FrameHz <= 0 || info.WindowMs <= 0 {
		t.Errorf("MicInfo = %+v, want a frame rate and a window", info)
	}
}

func TestVirtualMicFramesAdvanceAndTransientsHappen(t *testing.T) {
	m := NewVirtualMic()
	start := time.Unix(0, 0)
	now := start
	m.setClock(start, func() time.Time { return now })

	now = start.Add(3 * time.Second)
	quiet, ok := m.MicLevel()
	if !ok {
		t.Fatal("the synthetic source should always produce a frame")
	}
	if quiet.RMS <= 0 || quiet.RMS > 0.1 {
		t.Errorf("resting RMS = %v, want a quiet room (0 < rms <= 0.1)", quiet.RMS)
	}

	now = start.Add(virtualMicClapEvery)
	clap, _ := m.MicLevel()
	if clap.RMS <= quiet.RMS*3 {
		t.Errorf("clap RMS = %v, want it well above the floor %v", clap.RMS, quiet.RMS)
	}
	if clap.Peak <= clap.RMS {
		t.Errorf("clap peak = %v, want it above its own RMS %v", clap.Peak, clap.RMS)
	}

	now = start.Add(virtualMicClapEvery + 1*time.Millisecond)
	same, _ := m.MicLevel()
	if same != clap {
		t.Errorf("two reads inside one frame differ: %+v vs %+v", same, clap)
	}
	now = start.Add(virtualMicClapEvery + time.Second/virtualMicFrameHz)
	next, _ := m.MicLevel()
	if next.Seq != clap.Seq+1 {
		t.Errorf("next frame seq = %d, want %d", next.Seq, clap.Seq+1)
	}

	for i := 0; i < 400; i++ {
		now = start.Add(time.Duration(i) * 47 * time.Millisecond)
		f, _ := m.MicLevel()
		if f.RMS < 0 || f.RMS > 1 || f.Peak < 0 || f.Peak > 1 {
			t.Fatalf("frame %+v is outside 0..1", f)
		}
	}
}

func TestVirtualMicReportsBands(t *testing.T) {
	m := NewVirtualMic()
	start := time.Unix(0, 0)
	now := start
	m.setClock(start, func() time.Time { return now })

	now = start.Add(3 * time.Second)
	bands, ok := m.MicBands()
	if !ok {
		t.Fatal("the synthetic source should always produce a spectrum")
	}
	for i, v := range bands {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			t.Errorf("band %d = %v, want a finite non-negative value", i, v)
		}
	}

	now = start.Add(3*time.Second + time.Millisecond)
	again, _ := m.MicBands()
	if again != bands {
		t.Errorf("two reads inside one frame differ: %+v vs %+v", again, bands)
	}
}

func TestVirtualSuiteShipsTheSyntheticMicAndCanDropIt(t *testing.T) {
	v := NewVirtual()
	if info := v.MicInfo(); !info.Available || !info.Synthetic {
		t.Errorf("a fresh Virtual should carry the synthetic mic, got %+v", info)
	}
	if _, ok := v.MicLevel(); !ok {
		t.Error("a fresh Virtual should produce level frames")
	}
	v.AttachMic(nil)
	if info := v.MicInfo(); info.Available {
		t.Errorf("AttachMic(nil) should leave no microphone, got %+v", info)
	}
	if _, ok := v.MicLevel(); ok {
		t.Error("a suite with no microphone must report no frame")
	}
}

func TestVirtualSuiteForwardsMicBands(t *testing.T) {
	v := NewVirtual()
	if _, ok := v.MicBands(); !ok {
		t.Error("a fresh Virtual should carry a spectrum from its synthetic mic")
	}
	v.AttachMic(nil)
	if _, ok := v.MicBands(); ok {
		t.Error("a suite with no microphone must report no spectrum")
	}
}

func TestMicSlotZeroValueHasNoMicrophone(t *testing.T) {
	var m MicSlot
	if info := m.MicInfo(); info.Available {
		t.Errorf("zero MicSlot MicInfo = %+v, want unavailable", info)
	}
	if _, ok := m.MicLevel(); ok {
		t.Error("zero MicSlot reported a level frame")
	}
	if _, ok := m.MicBands(); ok {
		t.Error("zero MicSlot reported bands")
	}
	m.AttachMic(NewVirtualMic())
	if _, ok := m.MicBands(); !ok {
		t.Error("a MicSlot holding a VirtualMic should report its bands")
	}
}

func TestVirtualOwnMethodsAreNilSafe(t *testing.T) {
	var v *Virtual
	v.SetText(-1, "a", "b")
	v.SetLights(0, true)
	v.SetButtonLights(0, ButtonLights{"joya": true}, false)
	v.SetLampLevels(0, LampLevels{})
	v.Lines(0)
	v.SetPreviewGlyphs(nil)
	v.SetCartGlyphs(nil)
	v.PreviewGlyphs()
	v.CartGlyphs()
	v.ButtonLit(0, "joya")
	v.LampLevels(0)
	v.SetBacklightLevel(1)
	v.BacklightLevel()
	v.InputSignal()
	v.SetFanConfig(0, FanAuto)
	v.LastMotion()
	v.StatusLines()
	_ = v.Close()
}
