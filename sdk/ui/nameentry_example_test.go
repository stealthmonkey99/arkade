// SPDX-License-Identifier: GPL-2.0-or-later

package ui_test

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/sdk/ui"
)

func ExampleNameEntry() {
	entry := ui.NewNameEntry(0, 0, 3) // in Init: three letter slots
	const dt = 16_667                 // one 60 Hz tick, in microseconds

	// Move takes the held direction every tick; Do takes each fresh press.
	tap := func(dir ui.Direction) {
		entry.Move(dir)
		entry.Update(dt)
		entry.Move(ui.DirNone)
		entry.Update(dt)
	}

	tap(ui.DirUp) // blank -> A
	entry.Do(ui.ActionActivate)
	tap(ui.DirUp) // blank -> A
	tap(ui.DirUp) // A -> B
	entry.Do(ui.ActionActivate)
	fmt.Printf("typed %q, done %v\n", entry.Name(), entry.Done())

	entry.Do(ui.ActionActivate) // empty third slot jumps to OK
	entry.Do(ui.ActionActivate) // Activate on OK submits
	fmt.Printf("submitted %q, done %v\n", entry.Name(), entry.Done())

	// Output:
	// typed "AB", done false
	// submitted "AB", done true
}
