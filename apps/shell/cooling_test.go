// SPDX-License-Identifier: GPL-2.0-or-later

package shell

import "testing"

func TestLCDTempDropsDegreeSign(t *testing.T) {
	if got := lcdTemp("118°F"); got != "118F" {
		t.Errorf("lcdTemp(118°F) = %q, want 118F", got)
	}
	if got := lcdTemp(cpuTempNone); got != cpuTempNone {
		t.Errorf("lcdTemp(%q) = %q, want it untouched", cpuTempNone, got)
	}
}
