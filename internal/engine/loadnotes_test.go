// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import "testing"

func TestLoadNotesAreReadOnce(t *testing.T) {
	var n loadNotes
	n.quiet, n.bootSettle, n.attractFocus = true, true, "games/x"
	if !n.takeQuiet() || n.takeQuiet() {
		t.Error("quiet was not consumed by its first read")
	}
	if !n.takeBootSettle() || n.takeBootSettle() {
		t.Error("bootSettle was not consumed by its first read")
	}
	if n.takeAttractFocus() != "games/x" || n.takeAttractFocus() != "" {
		t.Error("attractFocus was not consumed by its first read")
	}
}

func TestLoadNotesSettingsOpenIsForOneApp(t *testing.T) {
	var n loadNotes
	n.settingsOpen = "savers/a"
	if n.takeSettingsOpen("savers/b") {
		t.Error("another app's load read the note as its own")
	}
	if n.takeSettingsOpen("savers/a") {
		t.Error("the note survived a load that was not the screensaver's")
	}
	n.settingsOpen = "savers/a"
	if !n.takeSettingsOpen("savers/a") {
		t.Error("the screensaver's own load did not read the note")
	}
	n.settingsOpen = ""
	if n.takeSettingsOpen("") {
		t.Error("an empty slug matched an empty note")
	}
}
