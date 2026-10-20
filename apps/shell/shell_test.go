// SPDX-License-Identifier: GPL-2.0-or-later

package shell_test

import (
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"

	_ "github.com/stealthmonkey99/arkade/apps/clock"
	_ "github.com/stealthmonkey99/arkade/apps/shell"
)

// lcdShows reports whether either player LCD currently shows text.
func lcdShows(virt *hardware.Virtual, text string) bool {
	return strings.Contains(strings.Join(virt.StatusLines(), "\n"), text)
}

// TestShellShowsNameAndScoreWhenPresent stores a legacy-shape score with a name, which scores.Load migrates on read.
func TestShellShowsNameAndScoreWhenPresent(t *testing.T) {
	dir := t.TempDir()
	if err := storage.New(dir).Write("apps/clock", "scores", struct {
		HighScore int    `json:"highScore"`
		Name      string `json:"name"`
	}{HighScore: 4242, Name: "ada"}); err != nil {
		t.Fatalf("seed score: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	if !lcdShows(hw, "ADA 4242") {
		t.Errorf("expected LCD to show the uppercased name and score together, status:\n%s",
			strings.Join(hw.StatusLines(), "\n"))
	}
}

func TestShellShowsBestOfTopTenTable(t *testing.T) {
	dir := t.TempDir()
	table := scores.Table{}
	table.Insert(9000, "zoe")
	table.Insert(4242, "ada")
	if err := scores.Save(storage.New(dir), "apps/clock", table); err != nil {
		t.Fatalf("seed table: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	if !lcdShows(hw, "ZOE 9000") {
		t.Errorf("expected LCD to show the table's best entry, status:\n%s",
			strings.Join(hw.StatusLines(), "\n"))
	}
	if lcdShows(hw, "ADA") {
		t.Error("only the best entry should render on the shell LCD")
	}
}

func TestShellShowsPlainHighScoreWhenNameMissing(t *testing.T) {
	dir := t.TempDir()
	if err := storage.New(dir).Write("apps/clock", "scores", struct {
		HighScore int `json:"highScore"`
	}{HighScore: 4242}); err != nil {
		t.Fatalf("seed score: %v", err)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	if !lcdShows(hw, "high: 4242") {
		t.Errorf("expected LCD to show the plain high score line for a nameless save, status:\n%s",
			strings.Join(hw.StatusLines(), "\n"))
	}
	if lcdShows(hw, "ADA") {
		t.Error("no name was stored, so none should render")
	}
}
