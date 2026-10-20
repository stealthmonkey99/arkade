// Fixture: LCD text that fits. The pass path for check_lcd.go — every
// shape the check understands, all of them legal.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// Exactly 16 columns: the boundary is inclusive, so this must pass.
const titleLine = "SIXTEEN CHARS!!!"

// Custom runes the cabinet's CGRAM palette really does carry, four of
// them — inside the 8-slot budget.
const meter = "░▒▓█"

func showTitle() {
	cartlib.SetText(0, titleLine, "SELECT MODE")
	// A const resolved through one level of indirection.
	cartlib.SetText(1, titleLine, meter)
	// A concatenation of literals, still under 16.
	cartlib.SetText(0, "P1 "+"READY", "")
	// A runtime piece: the literal floor is 7 columns, so nothing is
	// decidable here and nothing must be reported.
	cartlib.SetText(0, "SCORE "+itoa(score), "")
	// SetTextBytes carries no static text at all.
	cartlib.SetTextBytes(0, scratch[:], nil)
}

var (
	score   int
	scratch [16]byte
)

func itoa(n int) string { return "" }

func main() {}
