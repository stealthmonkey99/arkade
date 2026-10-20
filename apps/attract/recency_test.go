// SPDX-License-Identifier: GPL-2.0-or-later

package attract_test

import (
	"fmt"
	"testing"

	_ "github.com/stealthmonkey99/arkade/apps/attract"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// testShellSlug backs a throwaway TypeShell manifest, registered per test so
// other tests still see the no-shell path.
const testShellSlug = "test/shell"

func registerTestShell(t *testing.T) {
	t.Helper()
	engine.Register(engine.Manifest{
		Slug: testShellSlug,
		Name: "Test Shell",
		Type: engine.TypeShell,
		New:  func(ctx *engine.AppContext) engine.App { return nil },
	})
	t.Cleanup(func() { engine.Unregister(testShellSlug) })
}

// seedMRU writes the shell settings file with the most recent slug first.
func seedMRU(t *testing.T, dir string, mru []string) {
	t.Helper()
	if err := storage.New(dir).Write(testShellSlug, "", engine.ShellRecents{MRU: mru}); err != nil {
		t.Fatalf("seed MRU: %v", err)
	}
}

func seedScore(t *testing.T, dir, slug string, score int) {
	t.Helper()
	if err := storage.New(dir).Write(slug, "scores", struct {
		HighScore int `json:"highScore"`
	}{HighScore: score}); err != nil {
		t.Fatalf("seed score for %s: %v", slug, err)
	}
}

// soloIcon is an opaque 8x8 sprite of one colour, so a single pixel
// identifies which game's slot is showing.
func soloIcon(c display.Pixel) display.Sprite {
	row := make([]display.Pixel, 8)
	for i := range row {
		row[i] = c
	}
	sprite := make(display.Sprite, 8)
	for i := range sprite {
		sprite[i] = row
	}
	return sprite
}

// registerScoredGame registers a TypeGame stand-in with a soloIcon of the
// given colour and unregisters it on cleanup.
func registerScoredGame(t *testing.T, slug string, color display.Pixel) {
	t.Helper()
	engine.Register(engine.Manifest{
		Slug: slug,
		Name: slug,
		Type: engine.TypeGame,
		Icon: soloIcon(color),
		New:  func(ctx *engine.AppContext) engine.App { return nil },
	})
	t.Cleanup(func() { engine.Unregister(slug) })
}

// iconColorAt reads the pixel at the showcase icon's screen position.
func iconColorAt(frame display.Matrix) display.Pixel {
	return frame[12][28]
}

// attractScheduleFrom ticks the rig forward by lead, then through n 8s
// one-entry slots, mapping each slot's icon colour to a slug. A lead of "20s"
// lands in the first game slot, after the two ambient slots.
func attractScheduleFrom(t *testing.T, rig *testkit.Rig, colors map[display.Pixel]string, n int, lead string) []string {
	t.Helper()
	slugAt := func() string {
		c := iconColorAt(rig.Frame())
		slug, ok := colors[c]
		if !ok {
			t.Fatalf("icon color %+v at showcase position doesn't match any registered game", c)
		}
		return slug
	}
	rig.TickFor(lead)
	out := make([]string, n)
	out[0] = slugAt()
	for i := 1; i < n; i++ {
		rig.TickFor("8s")
		out[i] = slugAt()
	}
	return out
}

func TestAttractRecencyOrdersKnownGamesFirst(t *testing.T) {
	dir := t.TempDir()
	registerTestShell(t)

	const gA, gB, gC = "test/recent-a", "test/recent-b", "test/recent-c"
	colors := map[display.Pixel]string{
		{R: 200, G: 20, B: 20, A: 255}: gA,
		{R: 20, G: 200, B: 20, A: 255}: gB,
		{R: 20, G: 20, B: 200, A: 255}: gC,
	}
	for c, slug := range colors {
		registerScoredGame(t, slug, c)
		seedScore(t, dir, slug, 100)
	}
	seedMRU(t, dir, []string{gC, gA}) // gB has never been launched

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	got := attractScheduleFrom(t, rig, colors, 3, "20s")
	want := []string{gC, gA, gB}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schedule = %v, want %v", got, want)
		}
	}
}

func TestAttractRecencyUnplayedGamesFollowKnown(t *testing.T) {
	dir := t.TempDir()
	registerTestShell(t)

	const known = "test/recent-known"
	knownColor := display.Pixel{R: 200, G: 20, B: 20, A: 255}
	registerScoredGame(t, known, knownColor)
	seedScore(t, dir, known, 100)

	unplayedColors := []display.Pixel{
		{R: 20, G: 200, B: 20, A: 255},
		{R: 20, G: 20, B: 200, A: 255},
		{R: 200, G: 200, B: 20, A: 255},
	}
	colors := map[display.Pixel]string{knownColor: known}
	var unplayed []string
	for i, c := range unplayedColors {
		slug := fmt.Sprintf("test/recent-u%d", i+1)
		registerScoredGame(t, slug, c)
		seedScore(t, dir, slug, 50)
		colors[c] = slug
		unplayed = append(unplayed, slug)
	}
	seedMRU(t, dir, []string{known})

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	got := attractScheduleFrom(t, rig, colors, 1+len(unplayed), "20s")
	if got[0] != known {
		t.Fatalf("first slot = %q, want the known game %q", got[0], known)
	}
	tail := map[string]bool{}
	for _, slug := range got[1:] {
		tail[slug] = true
	}
	for _, slug := range unplayed {
		if !tail[slug] {
			t.Errorf("unplayed game %q missing from the tail %v", slug, got[1:])
		}
	}
}

func TestAttractRecencyDegradesWithoutShell(t *testing.T) {
	dir := t.TempDir()
	// Deliberately no registerTestShell(t): RecentSlugs must return nil.

	colors := map[display.Pixel]string{
		{R: 200, G: 20, B: 20, A: 255}: "test/recent-nohist-a",
		{R: 20, G: 200, B: 20, A: 255}: "test/recent-nohist-b",
	}
	for c, slug := range colors {
		registerScoredGame(t, slug, c)
		seedScore(t, dir, slug, 42)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	got := attractScheduleFrom(t, rig, colors, len(colors), "20s")
	seen := map[string]bool{}
	for _, slug := range got {
		seen[slug] = true
	}
	for _, slug := range colors {
		if !seen[slug] {
			t.Errorf("game %q missing from the schedule %v", slug, got)
		}
	}
}

func TestAttractRecencyOrderIsStableAcrossLoad(t *testing.T) {
	dir := t.TempDir()
	registerTestShell(t)

	const known = "test/recent-stable-known"
	knownColor := display.Pixel{R: 200, G: 20, B: 20, A: 255}
	registerScoredGame(t, known, knownColor)
	seedScore(t, dir, known, 100)

	colors := map[display.Pixel]string{knownColor: known}
	unplayedColors := []display.Pixel{
		{R: 20, G: 200, B: 20, A: 255},
		{R: 20, G: 20, B: 200, A: 255},
	}
	for i, c := range unplayedColors {
		slug := fmt.Sprintf("test/recent-stable-u%d", i+1)
		registerScoredGame(t, slug, c)
		seedScore(t, dir, slug, 10)
		colors[c] = slug
	}
	seedMRU(t, dir, []string{known})

	rig := testkit.NewRig(t, testkit.Options{StartApp: "attract", DataDir: dir})
	n := len(colors)
	first := attractScheduleFrom(t, rig, colors, n, "20s")
	// One lap later: two ambient slots plus n game slots.
	second := attractScheduleFrom(t, rig, colors, n, "24s")
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("slot %d: first lap %q, second lap %q; rotation reshuffled mid-load", i, first[i], second[i])
		}
	}
}
