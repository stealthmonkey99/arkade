// SPDX-License-Identifier: GPL-2.0-or-later

package lcdrune

import "testing"

// The first four cases are pinned by every host and tool; the rest are the
// edges of each class.
func TestReason(t *testing.T) {
	cases := []struct {
		r    rune
		want string
	}{
		// the four shared cases
		{'A', ReasonASCII},
		{0x1F9FA, ReasonEmoji}, // 🧺
		{0x4E00, ReasonWide},   // 一 CJK Unified Ideographs
		{0x058D, ""},           // ֍ ARMENIAN ETERNITY SIGN

		// control/ASCII
		{0, ReasonASCII},
		{' ', ReasonASCII},
		{0x7F, ReasonASCII},
		{0x80, ""},
		{0xA4, ""}, // ¤

		// not a code point
		{0xD800, ReasonNotACodePoint},
		{0xDFFF, ReasonNotACodePoint},
		{0xFDD0, ReasonNotACodePoint},
		{0xFDEF, ReasonNotACodePoint},
		{0xFFFE, ReasonNotACodePoint},
		{0xFFFF, ReasonNotACodePoint},
		{0x1FFFE, ReasonNotACodePoint}, // not "emoji", even though it sits in that range
		{0x10FFFF, ReasonNotACodePoint},
		{0x110000, ReasonNotACodePoint},

		// emoji
		{0x1F000, ReasonEmoji}, // 🀀 the first mahjong tile
		{0x1F600, ReasonEmoji}, // 😀
		{0x1FA99, ReasonEmoji}, // 🪙 the old coin fixture
		{0x1FFFD, ReasonEmoji},
		{0x200D, ReasonEmoji}, // zero-width joiner
		{0xFE0F, ReasonEmoji}, // variation selector 16
		{0x26A1, ReasonEmoji}, // ⚡
		{0x2B50, ReasonEmoji}, // ⭐
		{0x231A, ReasonEmoji}, // ⌚
		{0x2705, ReasonEmoji}, // ✅
		{0x2665, ""},          // ♥ the engine's heart, text-presentation
		{0x263A, ""},          // ☺
		{0x2714, ""},          // ✔
		{0x2680, ""},          // ⚀ die face
		{0x26A0, ""},          // ⚠ text-presentation by default
		{0x2764, ""},          // ❤ likewise

		// wide
		{0x1100, ReasonWide},
		{0x115F, ReasonWide},
		{0x1160, ""},
		{0x2E7F, ""},
		{0x2E80, ReasonWide},
		{0x3000, ReasonWide}, // ideographic space
		{0x9FFF, ReasonWide},
		{0xA4CF, ReasonWide},
		{0xA4D0, ""},
		{0xAC00, ReasonWide},
		{0xD7A3, ReasonWide},
		{0xF900, ReasonWide},
		{0xFE30, ReasonWide},
		{0xFF01, ReasonWide}, // ！
		{0xFF61, ""},         // ｡ halfwidth
		{0xFFE0, ReasonWide},
		{0xFFE6, ReasonWide},
		{0x20000, ReasonWide},
		{0xE0000, ReasonWide},

		// the rest of the plain world
		{0x2190, ""}, // ←
		{0x2588, ""}, // █
		{0x2500, ""}, // ─ box drawing
		{0xE000, ""}, // private use
		{0xF8FF, ""},
	}
	for _, tc := range cases {
		if got := Reason(tc.r); got != tc.want {
			t.Errorf("Reason(%#x) = %q, want %q", tc.r, got, tc.want)
		}
		if got := Allowed(tc.r); got != (tc.want == "") {
			t.Errorf("Allowed(%#x) = %v, disagrees with Reason", tc.r, got)
		}
	}
}
