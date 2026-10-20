// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadPadSeatsReadsWhatTheAppWrote parses the Bluetooth app's on-disk
// bytes; a drifted struct tag would fail silently, with the pad attaching unseated.
func TestLoadPadSeatsReadsWhatTheAppWrote(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "apps", "bluetooth")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const onDisk = `{"seats":{"aa:bb:cc:dd:ee:01":0}}`
	if err := os.WriteFile(filepath.Join(appDir, "settings.json"), []byte(onDisk), 0o644); err != nil {
		t.Fatal(err)
	}

	seats := loadPadSeats(dir)
	if len(seats) != 1 {
		t.Fatalf("read %d seats from %s, want 1", len(seats), onDisk)
	}
	if got, ok := seats["aa:bb:cc:dd:ee:01"]; !ok || got != 0 {
		t.Errorf("seat = %d (present=%v), want 0 for the cabinet's own pad", got, ok)
	}
}

func TestLoadPadSeatsToleratesNoFile(t *testing.T) {
	if seats := loadPadSeats(t.TempDir()); len(seats) != 0 {
		t.Errorf("got %v from a data dir with no settings at all", seats)
	}
}
