// SPDX-License-Identifier: GPL-2.0-or-later

package ui_test

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/sdk/ui"
	"github.com/stealthmonkey99/arkade/sdk/ui/input"
)

func ExampleHeldDirs() {
	var held ui.HeldDirs
	const dt = 100_000 // 100 ms per tick, in microseconds

	// A real button reports both edges.
	held.Feed(input.Record{Kind: input.KindKeydown, Key: input.KeyLeft})
	held.Update(dt)
	fmt.Println("left held:", held.Held(ui.DirLeft), "current is left:", held.Current() == ui.DirLeft)
	held.Feed(input.Record{Kind: input.KindKeyup, Key: input.KeyLeft})
	fmt.Println("after release:", held.Held(ui.DirLeft))

	// A console keyboard only sends keypresses; each grants 250 ms of credit
	// that decays with time.
	held.Feed(input.Record{Kind: input.KindKeypress, Key: input.KeyRight, Flags: input.FlagVirtual})
	held.Update(dt)
	fmt.Println("right held:", held.Held(ui.DirRight))
	held.Update(3 * dt)
	fmt.Println("right held later:", held.Held(ui.DirRight))

	// Output:
	// left held: true current is left: true
	// after release: false
	// right held: true
	// right held later: false
}
