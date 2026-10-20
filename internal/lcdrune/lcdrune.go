// SPDX-License-Identifier: GPL-2.0-or-later

// Package lcdrune decides which runes a cartridge may define a 5×8 LCD
// glyph on (host_define_lcd_glyph, sdk/docs/abi.md; a bundled
// <name>.arkadeglyph.json, docs/ASSETS.md).
//
// A custom glyph rune must be a plain, single-column, monochrome character,
// so it renders evenly on the panel, in terminals and in the browser host.
// Refused are:
//
//   - below U+0080 (the HD44780 mask ROM draws ASCII)
//   - emoji, including variation selectors and the zero-width joiner
//   - East Asian wide and fullwidth forms, and every plane from U+20000 up
//   - surrogates, noncharacters and anything past U+10FFFF
//
// The ranges are a checked-in table rather than a Unicode dependency so the
// cabinet, the browser host (which mirrors this file) and the tools agree
// exactly. The package imports nothing.
package lcdrune

// Reason strings, short because hosts splice them into warning sentences.
const (
	ReasonASCII         = "control/ASCII"
	ReasonEmoji         = "emoji"
	ReasonWide          = "wide"
	ReasonNotACodePoint = "not a code point"
)

// MaxCartGlyphs is how many distinct runes one cartridge may define. It caps
// the table, not the screen: the chip still shows at most 8 custom glyphs at
// once. sdk/cartlib, which cannot import this package, repeats it, and
// tools/preflight's tests pin the two together.
const MaxCartGlyphs = 32

func Allowed(r rune) bool { return Reason(r) == "" }

// Reason says why r is refused ("control/ASCII", "emoji", "wide" or "not a
// code point"), or "" when it is allowed. Not-a-code-point wins where classes
// overlap, so U+1FFFE and U+1FFFF are not reported as emoji.
func Reason(r rune) string {
	switch {
	case r < 0x80:
		return ReasonASCII
	case r > 0x10FFFF, r >= 0xD800 && r <= 0xDFFF,
		r >= 0xFDD0 && r <= 0xFDEF, r&0xFFFE == 0xFFFE:
		return ReasonNotACodePoint
	case r >= 0x20000:
		return ReasonWide
	case r >= 0x1F000:
		return ReasonEmoji
	case r == 0x200D, r >= 0xFE00 && r <= 0xFE0F:
		// Zero-width joiner and variation selectors.
		return ReasonEmoji
	}
	for _, span := range emojiPresentation {
		if r >= span.lo && r <= span.hi {
			return ReasonEmoji
		}
	}
	for _, span := range eastAsianWide {
		if r >= span.lo && r <= span.hi {
			return ReasonWide
		}
	}
	return ""
}

type span struct{ lo, hi rune }

// emojiPresentation is every Basic Multilingual Plane code point with
// Emoji_Presentation=Yes in Unicode's emoji-data.txt. Text-presentation
// symbols such as U+2665 and U+2714 are not in it and stay allowed.
var emojiPresentation = []span{
	{0x231A, 0x231B}, // ⌚ ⌛ watch, hourglass
	{0x23E9, 0x23EC}, // ⏩ ⏪ ⏫ ⏬ media controls
	{0x23F0, 0x23F0}, // ⏰ alarm clock
	{0x23F3, 0x23F3}, // ⏳ hourglass with flowing sand
	{0x25FD, 0x25FE}, // ◽ ◾ medium small squares
	{0x2614, 0x2615}, // ☔ ☕ umbrella with rain drops, hot beverage
	{0x2648, 0x2653}, // ♈ … ♓ the zodiac
	{0x267F, 0x267F}, // ♿ wheelchair symbol
	{0x2693, 0x2693}, // ⚓ anchor
	{0x26A1, 0x26A1}, // ⚡ high voltage
	{0x26AA, 0x26AB}, // ⚪ ⚫ medium circles
	{0x26BD, 0x26BE}, // ⚽ ⚾ soccer ball, baseball
	{0x26C4, 0x26C5}, // ⛄ ⛅ snowman, sun behind cloud
	{0x26CE, 0x26CE}, // ⛎ ophiuchus
	{0x26D4, 0x26D4}, // ⛔ no entry
	{0x26EA, 0x26EA}, // ⛪ church
	{0x26F2, 0x26F3}, // ⛲ ⛳ fountain, flag in hole
	{0x26F5, 0x26F5}, // ⛵ sailboat
	{0x26FA, 0x26FA}, // ⛺ tent
	{0x26FD, 0x26FD}, // ⛽ fuel pump
	{0x2705, 0x2705}, // ✅ check mark button
	{0x270A, 0x270B}, // ✊ ✋ raised fist, raised hand
	{0x2728, 0x2728}, // ✨ sparkles
	{0x274C, 0x274C}, // ❌ cross mark
	{0x274E, 0x274E}, // ❎ cross mark button
	{0x2753, 0x2755}, // ❓ ❔ ❕ question and exclamation marks
	{0x2757, 0x2757}, // ❗ exclamation mark
	{0x2795, 0x2797}, // ➕ ➖ ➗ plus, minus, divide
	{0x27B0, 0x27B0}, // ➰ curly loop
	{0x27BF, 0x27BF}, // ➿ double curly loop
	{0x2B1B, 0x2B1C}, // ⬛ ⬜ large squares
	{0x2B50, 0x2B50}, // ⭐ star
	{0x2B55, 0x2B55}, // ⭕ hollow red circle
}

// eastAsianWide is the Basic Multilingual Plane's East_Asian_Width Wide and
// Fullwidth blocks. Planes 2 and up are handled in Reason.
var eastAsianWide = []span{
	{0x1100, 0x115F}, // Hangul Jamo (the wide leading consonants)
	{0x2E80, 0xA4CF}, // CJK Radicals through Yi: kana, bopomofo, CJK Unified Ideographs and everything between
	{0xAC00, 0xD7A3}, // Hangul Syllables
	{0xF900, 0xFAFF}, // CJK Compatibility Ideographs
	{0xFE30, 0xFE4F}, // CJK Compatibility Forms
	{0xFF00, 0xFF60}, // Fullwidth Forms
	{0xFFE0, 0xFFE6}, // Fullwidth signs (¢ £ ¬ ¯ ¦ ¥ ₩)
}
