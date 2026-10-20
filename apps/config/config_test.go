// SPDX-License-Identifier: GPL-2.0-or-later

package config_test

import (
	"strings"
	"testing"
	"time"

	_ "github.com/stealthmonkey99/arkade/apps/config"
	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func lcdLines(rig *testkit.Rig) string {
	return strings.Join(rig.RT.Hardware.StatusLines(), "\n")
}

// Menu order: JOIN WIFI (focused first), VOLUME, TEST INPUTS, RESET CONTROLS,
// SYSTEM STATS, SHUTDOWN, CRASH TEST, FAN, ROTATION, SCREENSAVERS.

func TestConfigMenuGolden(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms") // ride out the menu fade-in
	rig.Golden("config-menu")
}

func TestConfigInputTestShowsKeypress(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")

	// Keyboard, because controller directions are remapped by cabinet position.
	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyEnter, false)
	rig.TickFor("600ms")
	rig.Golden("config-inputtest")

	rig.Press(input.KeyJoyA)
	rig.Tick(2)
	rig.Golden("config-inputtest-joya")
	if lcd := lcdLines(rig); !strings.Contains(lcd, "0:0:joya") {
		t.Errorf("LCD should echo the pressed key, got %q", lcd)
	}

	rig.PressKeyboard(input.KeyUp, false)
	rig.Tick(2)
	rig.Golden("config-inputtest-keyboard-up")

	rig.Press(input.KeyJoyB)
	rig.Tick(2)
	if lcd := lcdLines(rig); !strings.Contains(lcd, "press RED again") {
		t.Errorf("first back press should warn on the LCD, got %q", lcd)
	}
	rig.Press(input.KeyJoyB)
	rig.TickFor("600ms")
	rig.Golden("config-menu-return")
}

func TestConfigAudioAdjustsAndPersistsVolume(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")

	rig.PressKeyboard(input.KeyDown, false)
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Golden("config-audio") // shows "volume: 50%"

	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(2)
	rig.Golden("config-audio-40")

	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	rig.Golden("config-audio-muted")
	rig.Press(input.KeyJoyD)
	rig.Tick(2)

	// the second escape exits from the menu, persisting settings on teardown
	rig.Press(input.KeyEscape)
	rig.Tick(2)
	rig.Press(input.KeyEscape)
	rig.Tick(3)

	var saved struct {
		Volume int  `json:"volume"`
		Muted  bool `json:"muted"`
	}
	if err := rig.RT.Store.Read("apps/config", "", &saved); err != nil {
		t.Fatalf("settings should be persisted on exit: %v", err)
	}
	if saved.Volume != 40 || saved.Muted {
		t.Errorf("expected volume 40 unmuted persisted, got %+v", saved)
	}
}

func audioPlayEvents(rig *testkit.Rig) []audio.Event {
	var out []audio.Event
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "play" && ev.Owner == "apps/config" && ev.File == "boop.wav" {
			out = append(out, ev)
		}
	}
	return out
}

func TestConfigAudioVolumeStepPlaysBoopOnce(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")

	rig.PressKeyboard(input.KeyDown, false)
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	if got := len(audioPlayEvents(rig)); got != 0 {
		t.Fatalf("no boop expected before any volume step, got %d", got)
	}

	rig.PressKeyboard(input.KeyUp, false)
	rig.Tick(2)
	if got := len(audioPlayEvents(rig)); got != 1 {
		t.Fatalf("one UP step should play exactly one boop, got %d", got)
	}

	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(2)
	if got := len(audioPlayEvents(rig)); got != 2 {
		t.Fatalf("one DOWN step should play exactly one more boop, got %d", got)
	}

	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	if got := len(audioPlayEvents(rig)); got != 2 {
		t.Fatalf("muting should not play a boop, got %d", got)
	}

	for i := 0; i < 3; i++ {
		rig.PressKeyboard(input.KeyDown, false)
		rig.Tick(2)
	}
	if got := len(audioPlayEvents(rig)); got != 5 {
		t.Fatalf("expected 5 total boops after 5 volume steps, got %d", got)
	}
}

func TestConfigFanAdjustsThresholdAndPersists(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")

	for i := 0; i < 7; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Golden("config-fan") // shows "fan: >=120°F" and "cpu: ~" (virtual suite)

	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(2)
	rig.Golden("config-fan-110")

	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	rig.Golden("config-fan-on")
	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	rig.Golden("config-fan-disabled")
	rig.Press(input.KeyJoyD)
	rig.Tick(2)

	// the second escape exits from the menu, persisting settings on teardown
	rig.Press(input.KeyEscape)
	rig.Tick(2)
	rig.Press(input.KeyEscape)
	rig.Tick(3)

	var saved struct {
		FanThresholdF int    `json:"fanThresholdF"`
		FanMode       string `json:"fanMode"`
		FanDisabled   bool   `json:"fanDisabled,omitempty"`
	}
	if err := rig.RT.Store.Read("apps/config", "", &saved); err != nil {
		t.Fatalf("settings should be persisted on exit: %v", err)
	}
	if saved.FanThresholdF != 110 || saved.FanMode != string(hardware.FanAuto) || saved.FanDisabled {
		t.Errorf("expected threshold 110 auto persisted, got %+v", saved)
	}
}

// The clamp test asserts only that the label stops moving, not the key-reject bell.
func TestConfigFanThresholdClampsToRange(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")
	for i := 0; i < 7; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")

	for i := 0; i < 30; i++ { // 120 -> 180 in steps of 5, then clamp
		rig.PressKeyboard(input.KeyUp, false)
	}
	rig.Tick(2)
	rig.Golden("config-fan-max")

	for i := 0; i < 40; i++ { // 180 -> 80 in steps of 5, then clamp
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Tick(2)
	rig.Golden("config-fan-min")
}

func TestConfigFanDefaultsWhenSettingsPredateIt(t *testing.T) {
	dataDir := t.TempDir()
	old := map[string]any{"volume": 40, "muted": false}
	if err := storage.New(dataDir).Write("apps/config", "", old); err != nil {
		t.Fatalf("seed pre-upgrade settings: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config", DataDir: dataDir})
	rig.TickFor("600ms")
	for i := 0; i < 7; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Golden("config-fan")
}

func TestConfigFanMigratesFanDisabled(t *testing.T) {
	dataDir := t.TempDir()
	old := map[string]any{"volume": 40, "fanThresholdF": 120, "fanDisabled": true}
	if err := storage.New(dataDir).Write("apps/config", "", old); err != nil {
		t.Fatalf("seed pre-upgrade settings: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config", DataDir: dataDir})
	rig.TickFor("600ms")
	for i := 0; i < 7; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Golden("config-fan-disabled") // same rendering as the off state reached by cycling

	rig.Press(input.KeyEscape)
	rig.Tick(2)
	rig.Press(input.KeyEscape)
	rig.Tick(3)

	var saved struct {
		FanMode     string `json:"fanMode"`
		FanDisabled bool   `json:"fanDisabled,omitempty"`
	}
	if err := rig.RT.Store.Read("apps/config", "", &saved); err != nil {
		t.Fatalf("settings should be persisted on exit: %v", err)
	}
	if saved.FanMode != string(hardware.FanOff) || saved.FanDisabled {
		t.Errorf("expected migrated fanMode=off with fanDisabled dropped, got %+v", saved)
	}
}

func TestConfigWifiConnectShowsQR(t *testing.T) {
	// read at scene Init, so set before the rig boots
	t.Setenv("WIFI_SSID", "arkade")
	t.Setenv("WIFI_PASS", "secret123")
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")

	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Golden("config-wifi")

	if v, ok := rig.RT.RawHardware().(*hardware.Virtual); ok {
		if !v.ButtonLit(0, "joyb") || v.ButtonLit(0, "joya") {
			t.Error("wifi scene should light only JOY B")
		}
	} else {
		t.Fatal("rig hardware should be the virtual suite")
	}

	rig.Press(input.KeyJoyB)
	rig.TickFor("600ms")
	rig.Golden("config-wifi-return")
}

func TestConfigSystemStatsGolden(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")

	for i := 0; i < 4; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms") // ride out the scene fade-in

	// past engine.MotionActiveWindow: MOTION reads IDLE
	rig.Tick(160)
	rig.Golden("config-systemstats")

	rig.Motion()
	rig.Tick(1)
	rig.Golden("config-systemstats-motion")
}

func TestConfigResetControllersClearsPairings(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})

	pairing := map[string]string{"player1": "hid-abc123", "player2": "hid-def456"}
	if err := rig.RT.Store.Write("boot", "", pairing); err != nil {
		t.Fatalf("seed boot pairing: %v", err)
	}
	rig.TickFor("600ms")

	for i := 0; i < 3; i++ { // RESET CONTROLS is the 4th item
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Golden("config-reset") // "are you sure?" confirmation

	rig.Press(input.KeyJoyB)
	rig.TickFor("600ms")
	var kept map[string]string
	if err := rig.RT.Store.Read("boot", "", &kept); err != nil || kept["player1"] != "hid-abc123" {
		t.Fatalf("backing out must keep the pairing, got %v (err %v)", kept, err)
	}

	// No boot app is registered here, so the scene falls back to exiting.
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Press(input.KeyJoyA)
	rig.Tick(3)

	cleared := map[string]string{"player1": "sentinel-untouched"}
	if err := rig.RT.Store.Read("boot", "", &cleared); err != nil {
		t.Fatalf("read cleared pairing: %v", err)
	}
	if cleared["player1"] != "sentinel-untouched" {
		t.Errorf("boot pairing file should be deleted, but read back %v", cleared)
	}
}

// Keyboard directions are never remapped, so RIGHT aims Clockwise and DOWN
// aims UpsideDown.
func TestConfigRotationAimsAppliesAndPersists(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	rig.TickFor("600ms")

	for i := 0; i < 8; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
	rig.Golden("config-rotation") // ROOM, TOP marker on the top edge

	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(2)
	rig.Golden("config-rotation-aim-right") // TOP marker moved to right edge

	rig.Press(input.KeyJoyA)
	rig.Tick(2)
	rig.Golden("config-rotation-preview") // whole panel rotated

	rig.Press(input.KeyJoyD)
	rig.Tick(2)

	rig.PressKeyboard(input.KeyDown, false)
	rig.Press(input.KeyJoyA)
	rig.Tick(2)

	// the second escape exits from the menu, persisting settings on teardown
	rig.Press(input.KeyEscape)
	rig.Tick(2)
	rig.Press(input.KeyEscape)
	rig.Tick(3)

	var saved struct {
		RoomOrientation   int `json:"roomOrientation"`
		SeatedOrientation int `json:"seatedOrientation"`
	}
	if err := rig.RT.Store.Read("apps/config", "", &saved); err != nil {
		t.Fatalf("settings should be persisted on exit: %v", err)
	}
	if saved.RoomOrientation != int(display.Clockwise) || saved.SeatedOrientation != int(display.UpsideDown) {
		t.Errorf("expected room=Clockwise seated=UpsideDown persisted, got %+v", saved)
	}
}

func TestConfigInputRemapsForSeatedOrientation(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:          "apps/config",
		AttractAfter:      5 * time.Minute, // keep the active window open
		SeatedOrientation: display.Clockwise,
	})
	rig.TickFor("600ms")

	// Keyboard presses count as real input, so the cabinet reads as seated.
	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyDown, false)
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")

	rig.Press(input.KeyUp)
	rig.Tick(2)
	if lcd := lcdLines(rig); !strings.Contains(lcd, "0:0:down") {
		t.Errorf("seated Clockwise should remap controller UP to DOWN, LCD = %q", lcd)
	}

	rig.PressKeyboard(input.KeyUp, false)
	rig.Tick(2)
	if lcd := lcdLines(rig); !strings.Contains(lcd, ":up") {
		t.Errorf("keyboard input must not be remapped, LCD = %q", lcd)
	}
}
