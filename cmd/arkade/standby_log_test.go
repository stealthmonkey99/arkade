// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func TestStandbyTransitionsLogStandbyLines(t *testing.T) {
	var logs []string
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "apps/clock",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
		Logger: func(level, msg string) {
			logs = append(logs, msg)
		},
	})

	hasLog := func(substr string) bool {
		for _, l := range logs {
			if strings.Contains(l, substr) {
				return true
			}
		}
		return false
	}

	rig.TickFor("5m1s")
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract, got %v", got)
	}
	if !hasLog("standby: idle") || !hasLog("starting attract") {
		t.Errorf("expected an idle/starting-attract standby log, got %v", logs)
	}

	rig.TickFor("10m1s")
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected StateDark, got %v", got)
	}
	if !hasLog("standby: no presence") || !hasLog("going dark") {
		t.Errorf("expected a no-presence/going-dark standby log, got %v", logs)
	}

	rig.Motion()
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract after motion, got %v", got)
	}
	if !hasLog("standby: motion, waking to attract") {
		t.Errorf("expected a motion-wake standby log, got %v", logs)
	}

	// The wake resumes the parked apps/clock rather than loading the shell.
	rig.PressKeyboard(input.KeyEnter, false)
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("expected StateActive after input, got %v", got)
	}
	if !hasLog("standby: suspending apps/clock") {
		t.Errorf("expected a suspend standby log, got %v", logs)
	}
	if !hasLog("standby: resuming apps/clock") {
		t.Errorf("expected a resume standby log, got %v", logs)
	}
}

// TestStandbyHardDarkLogsGreppableLines covers forced attract, hard dark and the decay to soft.
func TestStandbyHardDarkLogsGreppableLines(t *testing.T) {
	var logs []string
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:       "shell",
		AttractAfter:   5 * time.Minute,
		StandbyAfter:   10 * time.Minute,
		HardDecayAfter: 10 * time.Minute,
		Logger: func(level, msg string) {
			logs = append(logs, msg)
		},
	})

	hasLog := func(substr string) bool {
		for _, l := range logs {
			if strings.Contains(l, substr) {
				return true
			}
		}
		return false
	}

	rig.Tick(3)

	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Fatalf("expected StateAttract after shell BACK, got %v", got)
	}
	if !hasLog("standby: forced attract") {
		t.Errorf("expected a forced-attract standby log, got %v", logs)
	}

	rig.PressKeyboard(input.KeyEscape, false)
	rig.Tick(2)
	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("expected StateDark after attract BACK, got %v", got)
	}
	if !hasLog("standby: hard dark (back pressed)") {
		t.Errorf("expected a hard-dark standby log, got %v", logs)
	}

	rig.TickFor("10m1s")
	if !hasLog("standby: hard dark decayed to soft") {
		t.Errorf("expected a hard-dark-decayed standby log, got %v", logs)
	}
}
