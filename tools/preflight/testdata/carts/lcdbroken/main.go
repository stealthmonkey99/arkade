// Fixture: every LCD failure check_lcd.go can decide. The fail path.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// 19 columns — the shape found in real cartridges, where the tail is
// simply not drawn.
const tooLong = "press to play again"

func showGameOver() {
	// A plain literal over 16 columns.
	cartlib.SetText(0, "GAME OVER", "PRESS ANY KEY TO CONTINUE")
	// The same, reached through a const.
	cartlib.SetText(1, "GAME OVER", tooLong)
	// Literal parts alone already overflow, before anything computed is
	// appended — so the overflow is a fact, not an estimate.
	cartlib.SetText(0, "FINAL SCORE IS "+itoa(score)+" POINTS", "")
	// A rune the cabinet's CGRAM palette does not carry: it renders '?'.
	cartlib.SetText(0, "DANGER ☠", "")
	// Nine distinct customs on one screen, one over the CGRAM budget.
	cartlib.SetText(0, "░▒▓█♠♦", "♣♥→")
}

var score int

func itoa(n int) string { return "" }

func main() {}
