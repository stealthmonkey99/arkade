// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// crashFile is what -after-engine leaves in the state directory (on /run, so a reboot clears it).
const crashFile = "engine-exit"

// crashWindow is how recent a crash must be for the card to show.
const crashWindow = 2 * time.Minute

// crash is what systemd told ExecStopPost about the engine's exit.
type crash struct {
	result string // SERVICE_RESULT
	status string // EXIT_STATUS: the exit code, or the signal name
	at     time.Time
}

func (c crash) message() string {
	return fmt.Sprintf("engine exit %s - retry in 5s", c.status)
}

func formatCrash(c crash) string {
	return fmt.Sprintf("result=%s\nstatus=%s\nat=%s\n", c.result, c.status, c.at.UTC().Format(time.RFC3339))
}

func parseCrash(raw string) (crash, error) {
	var c crash
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "result":
			c.result = value
		case "status":
			c.status = value
		case "at":
			t, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return crash{}, fmt.Errorf("crash file: %w", err)
			}
			c.at = t
		}
	}
	if c.status == "" || c.at.IsZero() {
		return crash{}, fmt.Errorf("crash file: want status and at, got %q", raw)
	}
	return c, nil
}

func writeCrash(dir string, c crash) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, crashFile), []byte(formatCrash(c)), 0o644)
}

// readCrash consumes the file whether or not it is fresh, so a card shows once.
func readCrash(dir string, now time.Time) (crash, bool) {
	path := filepath.Join(dir, crashFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return crash{}, false
	}
	os.Remove(path)
	c, err := parseCrash(string(raw))
	if err != nil {
		logf("%v", err)
		return crash{}, false
	}
	return c, now.Sub(c.at) < crashWindow
}

// crashFromEnv reads the exit systemd exported to ExecStopPost. A clean stop
// reports no crash, so the engine's own power-off message stays.
func crashFromEnv(env func(string) string, now time.Time) (crash, bool) {
	result := env("SERVICE_RESULT")
	if result == "" || result == "success" {
		return crash{}, false
	}
	status := env("EXIT_STATUS")
	if status == "" {
		status = result
	}
	return crash{result: result, status: status, at: now}, true
}
