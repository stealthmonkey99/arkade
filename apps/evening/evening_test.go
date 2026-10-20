// SPDX-License-Identifier: GPL-2.0-or-later

package evening_test

import (
	"testing"
	"time"

	_ "github.com/stealthmonkey99/arkade/apps/attract"
	"github.com/stealthmonkey99/arkade/apps/evening"
	_ "github.com/stealthmonkey99/arkade/apps/shell"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// storedSettings spells out the on-disk shape so a renamed key fails a test.
type storedSettings struct {
	Schedule        engine.Schedule `json:"schedule"`
	Brightness      int             `json:"brightness"`
	Sensitivity     string          `json:"sensitivity"`
	PresenceHoldMin int             `json:"presenceHoldMinutes"`
}

const (
	quietRMS = 0.0010
	loudRMS  = 0.0040
)

// quietBands is the resting spectrum for calibrating per-band floors; loudBands
// puts each band at a distinct column height. Both are hardware.MicBander band powers.
var (
	quietBands = [hardware.MicBandCount]float64{0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02}
	loudBands  = [hardware.MicBandCount]float64{0.0800, 0.0586, 0.0314, 0.0429, 0.0255, 0.0187, 0.0152, 0.0123}
)

// scriptedMic is a hand-set microphone; every read is a new frame because the gate de-duplicates by Seq.
type scriptedMic struct {
	seq   uint64
	rms   float64
	bands [hardware.MicBandCount]float64
}

func (m *scriptedMic) MicInfo() hardware.MicInfo {
	return hardware.MicInfo{Available: true, Source: "scripted", SampleRateHz: 14700, FrameHz: 20, WindowMs: 50}
}

func (m *scriptedMic) MicLevel() (hardware.MicLevel, bool) {
	m.seq++
	return hardware.MicLevel{Seq: m.seq, RMS: m.rms, Peak: m.rms * 2}, true
}

func (m *scriptedMic) MicBands() ([hardware.MicBandCount]float64, bool) { return m.bands, true }

type eveningRig struct {
	*testkit.Rig
	mic   *scriptedMic
	store *storage.Store
}

// newEveningRig boots the shell with a scripted microphone, the clock inside
// the evening window (the manual epoch is Monday 2024-01-01 00:00), and a short presence hold.
func newEveningRig(t *testing.T, holdMin int) *eveningRig {
	t.Helper()
	evening.ResetGate()
	t.Cleanup(evening.ResetGate)

	dir := t.TempDir()
	store := storage.New(dir)
	err := store.Write("apps/evening", "", storedSettings{
		Schedule: engine.Schedule{
			Enabled: true, From: "19:00", To: "23:30",
			Days: engine.EveryDay(), Priority: 20,
		},
		Brightness:      40,
		Sensitivity:     "medium",
		PresenceHoldMin: holdMin,
	})
	if err != nil {
		t.Fatalf("seeding settings: %v", err)
	}

	mic := &scriptedMic{rms: quietRMS, bands: quietBands}
	hw := hardware.NewVirtual()
	hw.AttachMic(mic)

	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "shell",
		DataDir:      dir,
		Hardware:     hw,
		AttractAfter: time.Second,
	})
	rig.Clock.Advance(19*time.Hour + 5*time.Minute)
	rig.Tick(2)
	return &eveningRig{Rig: rig, mic: mic, store: store}
}

// listen ticks d worth of frames, keeping the motion sensor alive when occupied.
func (r *eveningRig) listen(d time.Duration, occupied bool) {
	frames := int(d / r.RT.FrameBudget())
	for i := 0; i < frames; i++ {
		if occupied && i%15 == 0 {
			r.Motion()
		}
		r.Tick(1)
	}
}

func TestEveningGateWantsPresenceAndNoise(t *testing.T) {
	rig := newEveningRig(t, 1)

	rig.listen(30*time.Second, true)
	if got := rig.RT.CurrentSlug(); got == "apps/evening" {
		t.Fatal("a quiet room must stay on the attract reel")
	}

	rig.mic.rms = loudRMS
	rig.listen(30*time.Second, true)
	if got := rig.RT.CurrentSlug(); got != "apps/evening" {
		t.Fatalf("a loud, occupied room should show the equalizer, got %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("the screensaver must stay inside ATTRACT, got %v", got)
	}

	// Presence alone keeps the panel up.
	rig.mic.rms = quietRMS
	rig.listen(45*time.Second, true)
	if got := rig.RT.CurrentSlug(); got != "apps/evening" {
		t.Fatalf("a pause in the dialogue must not bounce the panel, got %q", got)
	}

	// The room empties and the panel is handed back within the presence hold.
	rig.mic.rms = loudRMS
	rig.listen(70*time.Second, false)
	if got := rig.RT.CurrentSlug(); got == "apps/evening" {
		t.Fatal("an empty room should hand the panel back to the attract reel")
	}
}

func TestEveningGateIgnoresAnEmptyLoudRoom(t *testing.T) {
	rig := newEveningRig(t, 1)
	rig.mic.rms = loudRMS
	rig.listen(40*time.Second, false)
	if got := rig.RT.CurrentSlug(); got == "apps/evening" {
		t.Fatal("a loud room with nobody in it is a television left on, not an audience")
	}
}

// showEvening drives the rig until the equalizer has the panel.
func showEvening(t *testing.T, rig *eveningRig) {
	t.Helper()
	rig.mic.rms = loudRMS
	rig.listen(30*time.Second, true)
	if got := rig.RT.CurrentSlug(); got != "apps/evening" {
		t.Fatalf("expected the equalizer to be showing, got %q", got)
	}
}

func TestEveningGoldenBars(t *testing.T) {
	rig := newEveningRig(t, 1)
	showEvening(t, rig)

	rig.mic.bands = loudBands
	rig.listen(3*time.Second, true)
	rig.Golden("evening-bars")
}

func TestEveningPanelDimAndBlankPeripherals(t *testing.T) {
	rig := newEveningRig(t, 1)
	showEvening(t, rig)

	if got := rig.RT.PanelBrightness(); got != 40 {
		t.Errorf("expected the stored 40%% panel dim, got %d", got)
	}
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("expected the virtual suite, got %T", rig.RT.RawHardware())
	}
	if line1, line2 := hw.Lines(0); line1 != "" || line2 != "" {
		t.Errorf("expected blank LCDs, got %q / %q", line1, line2)
	}
	if got := hw.BacklightLevel(); got != 0 {
		t.Errorf("expected the LCD backlight out, got %d", got)
	}
	for _, name := range hardware.LightButtons {
		if hw.ButtonLit(0, name) {
			t.Errorf("expected every lamp dark, %s is lit", name)
		}
	}
}

func TestEveningSettingsSavesAndReturns(t *testing.T) {
	rig := newEveningRig(t, 1)
	showEvening(t, rig)

	rig.PressKeyboard(input.KeyJoyA, false) // green on a showing screensaver opens its settings
	rig.Tick(3)
	if got := rig.RT.CurrentSlug(); got != "apps/evening" {
		t.Fatalf("expected the settings screen to be the evening app itself, got %q", got)
	}
	rig.Golden("evening-settings")

	// Four downs reach the brightness row.
	for i := 0; i < 4; i++ {
		rig.PressKeyboard(input.KeyDown, false)
		rig.Tick(2)
	}
	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(2)
	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(2)

	rig.PressKeyboard(input.KeyJoyB, false) // red saves and goes back to the equalizer
	rig.Tick(4)

	var got storedSettings
	if err := rig.store.Read("apps/evening", "", &got); err != nil {
		t.Fatalf("reading back the settings: %v", err)
	}
	if got.Brightness != 50 {
		t.Errorf("expected brightness 40 stepped twice to 50, got %d", got.Brightness)
	}
	if !got.Schedule.Enabled || got.Schedule.From != "19:00" || got.Schedule.To != "23:30" {
		t.Errorf("saving must keep the rest of the schedule, got %+v", got.Schedule)
	}
	if slug := rig.RT.CurrentSlug(); slug != "apps/evening" {
		t.Fatalf("red should have gone back to the screensaver, got %q", slug)
	}
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Errorf("the return should be to ATTRACT, got %v", got)
	}
	if got := rig.RT.PanelBrightness(); got != 50 {
		t.Errorf("the reloaded equalizer should read the brightness just saved, got %d", got)
	}
}

func TestEveningSettingsTogglesADay(t *testing.T) {
	rig := newEveningRig(t, 1)
	showEvening(t, rig)
	rig.PressKeyboard(input.KeyJoyA, false)
	rig.Tick(3)

	for i := 0; i < 3; i++ { // ENABLED, FROM, TO, DAYS
		rig.PressKeyboard(input.KeyDown, false)
		rig.Tick(2)
	}
	rig.PressKeyboard(input.KeyRight, false) // the cursor walks to Monday
	rig.Tick(2)
	rig.PressKeyboard(input.KeyJoyA, false) // green turns it off
	rig.Tick(2)
	rig.PressKeyboard(input.KeyJoyB, false)
	rig.Tick(4)

	var got storedSettings
	if err := rig.store.Read("apps/evening", "", &got); err != nil {
		t.Fatalf("reading back the settings: %v", err)
	}
	want := engine.EveryDay()
	want[1] = false
	if got.Schedule.Days != want {
		t.Errorf("expected Monday switched off, got %v", got.Schedule.Days)
	}
}
