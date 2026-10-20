// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// TestStartupCardRowsFillThePanel: rows shorter than 16 columns get centred
// and longer ones scroll, either of which makes the handover visible.
func TestStartupCardRowsFillThePanel(t *testing.T) {
	for _, stage := range []string{stageEngine, stageCartridges, stageDashboard, stageMicrophone} {
		if n := utf8.RuneCountInString(stage); n > startupStageCols {
			t.Errorf("stage %q is %d columns, the row has room for %d", stage, n, startupStageCols)
		}
		rows := [2]string{
			padCols("Årkade", hardware.LcdCols),
			padCols(stage, startupBarCells+1+startupStageCols),
		}
		for row, line := range rows {
			if n := utf8.RuneCountInString(line); n != hardware.LcdCols {
				t.Errorf("stage %q row %d = %q (%d columns), want %d", stage, row, line, n, hardware.LcdCols)
			}
		}
	}
}

// TestStartupBacklightMatchesTheTool: arkade-lcdboot's idle level must equal startupBacklight.
func TestStartupBacklightMatchesTheTool(t *testing.T) {
	if want := hardware.LampLevelMax * 30 / 100; startupBacklight != want {
		t.Errorf("startupBacklight = %d, want the tool's idle level %d", startupBacklight, want)
	}
}

func TestPadColsCountsRunesNotBytes(t *testing.T) {
	// "Årkade" is seven bytes and six columns.
	if got := padCols("Årkade", 16); utf8.RuneCountInString(got) != 16 {
		t.Errorf("padCols(%q, 16) = %q, %d columns", "Årkade", got, utf8.RuneCountInString(got))
	}
	if got := padCols("overlong text here", 4); got != "overlong text here" {
		t.Errorf("padCols should never truncate, got %q", got)
	}
}
