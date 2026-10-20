// SPDX-License-Identifier: GPL-2.0-or-later

package ui_test

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/sdk/ui"
	"github.com/stealthmonkey99/arkade/sdk/ui/input"
)

func ExampleHoldConfirm() {
	var confirm ui.HoldConfirm
	const dt = 100_000 // 100 ms per tick, in microseconds

	down := input.Record{Kind: input.KindKeydown, Key: input.KeyJoyB}
	up := input.Record{Kind: input.KindKeyup, Key: input.KeyJoyB}

	// B pressed during play opens the overlay; the opening press must be
	// released before the hold can begin.
	confirm.Open()
	confirm.Feed(down)
	confirm.Feed(up)

	// Hold B: the bar fills over HoldMicros (one second by default).
	confirm.Feed(down)
	for i := 0; i < 5; i++ {
		confirm.Update(dt)
	}
	fmt.Printf("progress %.1f\n", confirm.Progress())
	for i := 0; i < 5; i++ {
		confirm.Update(dt)
	}
	fmt.Println("confirmed", confirm.Confirmed())

	// Release B; once the confirmation blink has played Update reports true.
	confirm.Feed(up)
	quit := false
	for i := 0; i < 10 && !quit; i++ {
		quit = confirm.Update(dt)
	}
	fmt.Println("quit", quit)

	// Output:
	// progress 0.5
	// confirmed true
	// quit true
}
