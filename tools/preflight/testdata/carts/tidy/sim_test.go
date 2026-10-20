// Fixture: a cartridge's own test file, which runs under host Go and is
// full of exactly what the GOLDEN RULES forbid. It must be invisible to
// the golden-rules check, or every cartridge would light up
// for its own hintstyle_test.go.
package main

import (
	"fmt"
	"testing"
	"time"
)

var scratch = map[string]int{}

func init() { scratch["x"] = 1 }

func TestElapses(t *testing.T) {
	start := time.Now()
	fmt.Println(start)
}
