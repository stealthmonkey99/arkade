//go:build !tinygo

// Fixture: a HOST-ONLY file, the shape a host-side rules file has. It
// breaks two GOLDEN RULES on purpose — an init() and a
// package-level initializer that does work — and the tidy cartridge must
// still come back clean, because none of this is in the TinyGo build.
package main

import "fmt"

var pieceNames = map[int]string{0: "empty", 1: "pawn"}

var describe = fmt.Sprintf("%d pieces", len(pieceNames))

func init() {
	pieceNames[2] = "knight"
}
