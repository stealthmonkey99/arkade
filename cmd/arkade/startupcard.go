// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"strings"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/hardware/pi"
)

// The card arkade-lcdboot leaves latched on exit, carried through the seconds
// between pi.New and the boot splash. Columns and backlight match its last
// frame so the handover is invisible.
const (
	startupBacklight = hardware.LampLevelMax * 30 / 100
	startupBarCells  = 8
	startupStageCols = 7 // what is left of the row beside the bar
)

// Startup milestones in order, abbreviated to fit beside the bar. "engine" is
// the word arkade-lcdboot ends on.
const (
	stageEngine     = "engine"
	stageCartridges = "carts"
	stageDashboard  = "web"
	stageMicrophone = "mic"
)

// startupCard repaints P1 for one milestone; a nil suite does nothing. Both
// rows are exactly 16 columns so the driver neither centres nor scrolls them.
func startupCard(hw *pi.Suite, stage string) {
	if hw == nil {
		return
	}
	hw.SetText(0, padCols("Årkade", hardware.LcdCols),
		strings.Repeat("█", startupBarCells)+" "+padCols(stage, startupStageCols))
	hw.SetBacklightLevel(startupBacklight)
}

func padCols(s string, n int) string {
	if pad := n - utf8.RuneCountInString(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
