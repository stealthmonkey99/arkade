// SPDX-License-Identifier: GPL-2.0-or-later

package main

import "testing"

func TestParseThermal(t *testing.T) {
	if got, ok := parseThermal("41234\n"); !ok || got != 41 {
		t.Errorf("parseThermal = %d, %v; want 41", got, ok)
	}
	for _, raw := range []string{"", "warm\n"} {
		if _, ok := parseThermal(raw); ok {
			t.Errorf("parseThermal(%q) should report no reading", raw)
		}
	}
}

// The level is the third field after the interface name's colon, with a trailing dot.
const procNetWireless = `Inter-| sta-|   Quality        |   Discarded packets               | Missed | WE
 face | tus | link level noise |  nwid  crypt   frag  retry   misc | beacon | 22
 wlan0: 0000   58.  -62.  -256        0      0      0      0     31        0
`

func TestParseWireless(t *testing.T) {
	if got, ok := parseWireless(procNetWireless); !ok || got != -62 {
		t.Errorf("parseWireless = %d, %v; want -62", got, ok)
	}
	if _, ok := parseWireless(""); ok {
		t.Error("no file should report no reading")
	}
	idle := "Inter-| sta-|\n face | tus |\n wlan0: 0000    0.    0.    0.        0\n"
	if _, ok := parseWireless(idle); ok {
		t.Error("a link-less interface should report no reading")
	}
}
