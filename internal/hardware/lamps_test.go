// SPDX-License-Identifier: GPL-2.0-or-later

package hardware

import "testing"

// The ABI's four bits span the hardware's eight with no gap at either end.
func TestLampLevelFromABI(t *testing.T) {
	cases := []struct{ in, want uint8 }{
		{0, 0},
		{1, 17},
		{8, 136},
		{LampLevelABIMax, LampLevelMax},
		{LampLevelABIMax + 1, LampLevelMax},
		{255, LampLevelMax},
	}
	for _, c := range cases {
		if got := LampLevelFromABI(c.in); got != c.want {
			t.Errorf("LampLevelFromABI(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
