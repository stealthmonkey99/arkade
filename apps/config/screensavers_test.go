// SPDX-License-Identifier: GPL-2.0-or-later

package config_test

import (
	"strings"
	"testing"
	"time"

	_ "github.com/stealthmonkey99/arkade/apps/evening"
	_ "github.com/stealthmonkey99/arkade/apps/morning"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

const screensaversItem = 9 // SCREENSAVERS' index in the menu

func openScreensavers(rig *testkit.Rig) {
	rig.TickFor("600ms")
	for i := 0; i < screensaversItem; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
}

// openScreensaver picks the nth entry: 0 is MORNING, 1 is EVENING.
func openScreensaver(rig *testkit.Rig, n int) {
	openScreensavers(rig)
	for i := 0; i < n; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Press(input.KeyEnter)
	rig.TickFor("600ms")
}

type storedSaver struct {
	Schedule   engine.Schedule `json:"schedule"`
	Brightness int             `json:"brightness"`
	Location   string          `json:"location,omitempty"`
	Lat        *float64        `json:"lat,omitempty"`
	Lon        *float64        `json:"lon,omitempty"`
}

func TestConfigScreensaversListGolden(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config"})
	openScreensavers(rig)
	rig.Golden("config-screensavers")
}

func TestConfigScreensaverEditsEveningAndSaves(t *testing.T) {
	dir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config", DataDir: dir})
	openScreensaver(rig, 1)
	rig.Golden("config-screensaver-evening")
	if lcd := lcdLines(rig); !strings.Contains(lcd, "EVENING") {
		t.Errorf("LCD should name the screensaver, got %q", lcd)
	}

	for i := 0; i < 3; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.PressKeyboard(input.KeyRight, false)
	rig.Press(input.KeyJoyA)
	rig.Tick(2)
	rig.Golden("config-screensaver-evening-days")
	if lcd := lcdLines(rig); !strings.Contains(lcd, "GREEN DAY RED OK") {
		t.Errorf("the DAYS row should say what green does, got %q", lcd)
	}

	rig.Press(input.KeyJoyB)
	rig.TickFor("600ms")
	rig.Golden("config-screensavers-return")

	var saved storedSaver
	if err := storage.New(dir).Read("apps/evening", "", &saved); err != nil {
		t.Fatalf("red should save evening's settings: %v", err)
	}
	want := engine.EveryDay()
	want[1] = false
	if saved.Schedule.Days != want || saved.Schedule.Priority != 20 || saved.Brightness != 40 {
		t.Errorf("expected evening's defaults with Monday off, got %+v", saved)
	}
}

func TestConfigScreensaverMorningKeepsTheDashboardsCoordinates(t *testing.T) {
	dir := t.TempDir()
	lat, lon := 37.77, -122.42
	seed := storedSaver{
		Schedule:   engine.Schedule{Enabled: true, From: "06:00", To: "09:00", Days: engine.EveryDay(), Priority: 10},
		Brightness: 15,
		Location:   "manual",
		Lat:        &lat,
		Lon:        &lon,
	}
	if err := storage.New(dir).Write("apps/morning", "", seed); err != nil {
		t.Fatal(err)
	}
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config", DataDir: dir})
	openScreensaver(rig, 0)
	rig.Golden("config-screensaver-morning")

	for i := 0; i < 4; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.PressKeyboard(input.KeyRight, false)
	rig.Press(input.KeyJoyB)
	rig.Tick(2)

	var saved storedSaver
	if err := storage.New(dir).Read("apps/morning", "", &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Brightness != 20 || saved.Lat == nil || *saved.Lat != lat || saved.Location != "manual" {
		t.Errorf("expected brightness 20 with the coordinates kept, got %+v", saved)
	}
}

// A schedule saved from SETTINGS reaches the next pick inside the engine's
// minute-long schedule cache, as a save from the screensaver does.
func TestConfigScreensaverScheduleTakesEffectWithoutARestart(t *testing.T) {
	dir := t.TempDir()
	seed := storedSaver{
		Schedule:   engine.Schedule{Enabled: false, From: "06:00", To: "09:00", Days: engine.EveryDay(), Priority: 10},
		Brightness: 15,
		Location:   "manual", // no coordinates: no weather fetch, no location lookup
	}
	if err := storage.New(dir).Write("apps/morning", "", seed); err != nil {
		t.Fatal(err)
	}
	rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/config", DataDir: dir, AttractAfter: time.Second})
	rig.Clock.Advance(6*time.Hour + 42*time.Minute)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("idling should reach ATTRACT and prime the schedule cache, got %v", got)
	}
	if got := rig.RT.CurrentSlug(); got == "apps/morning" {
		t.Fatal("a disabled morning should not show")
	}
	rig.RT.RequestLaunchNow("apps/config")
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "apps/config" {
		t.Fatalf("SETTINGS should be back up, got %q", got)
	}

	openScreensaver(rig, 0)
	rig.PressKeyboard(input.KeyRight, false)
	rig.Press(input.KeyJoyB)
	rig.Tick(2)

	rig.TickFor("2500ms")
	if got := rig.RT.CurrentSlug(); got != "apps/morning" {
		t.Errorf("the enabled window should show morning at the next attract, got %q", got)
	}
}
