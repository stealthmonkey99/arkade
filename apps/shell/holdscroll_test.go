// SPDX-License-Identifier: GPL-2.0-or-later

package shell_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// holdScrollItemCount dummy apps give enough travel to observe auto-repeat without clamping.
const holdScrollItemCount = 12

// holdScrollNames end in letters, not digits, so "Item1" cannot match inside "Item10" in lcdShows.
var holdScrollNames = func() []string {
	names := make([]string, holdScrollItemCount)
	for i := range names {
		names[i] = fmt.Sprintf("HoldRepeat-%c", rune('A'+i))
	}
	return names
}()

// init registers the dummy apps, never launched, sorted after every real app so Clock stays first.
func init() {
	for i, name := range holdScrollNames {
		i, name := i, name
		engine.Register(engine.Manifest{
			Slug:      fmt.Sprintf("test/holdscroll-%c", rune('A'+i)),
			Name:      name,
			Type:      engine.TypeGame,
			SortOrder: 1000 + i,
			New:       func(*engine.AppContext) engine.App { return nil },
		})
	}
}

// holdScrollIndex reports which dummy app the LCD shows, or -1.
func holdScrollIndex(hw *hardware.Virtual) int {
	for i, name := range holdScrollNames {
		if lcdShows(hw, name) {
			return i
		}
	}
	return -1
}

// TestShellControllerHeldDirectionAutoRepeats sends one joystickchange then only ticks,
// as a real held stick does, and checks the selection keeps stepping.
func TestShellControllerHeldDirectionAutoRepeats(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.RT.Input.Push(input.Event{
		Kind:     input.KindJoystickChange,
		Position: input.JoyDown,
		Source: input.PlayerSource{
			Type: input.SourceController,
			ID:   "test-stick",
			// A seatless stick keeps the engine's seat remap a no-op.
			Slot: input.SlotOf(2), Seat: input.NoSeat,
		},
	})

	rig.Tick(20) // ~667ms at the rig's default 30fps
	mid := holdScrollIndex(hw)
	if mid < 1 {
		t.Fatalf("held direction did not auto-repeat: selection index %d (LCD:\n%s\n) after ~667ms of holding down with no further events",
			mid, strings.Join(hw.StatusLines(), "\n"))
	}
	if mid >= holdScrollItemCount-1 {
		t.Fatalf("selection already clamped at the bottom (index %d) after only ~667ms — the continued-advance check below would be meaningless; widen holdScrollItemCount", mid)
	}

	rig.Tick(20) // another ~667ms (~1.33s total)
	later := holdScrollIndex(hw)
	if later <= mid {
		t.Fatalf("held direction stopped repeating: index was %d after ~667ms, still %d after ~1.33s of continuous hold — want continued advance from ticking alone", mid, later)
	}
}

// TestShellHeldStickAcceleratesOnALongShelf holds down from the top of ALL
// and times the steps: after twelve the repeat interval halves, so the gap
// before the last tile is shorter than a gap among the first few.
func TestShellHeldStickAcceleratesOnALongShelf(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	focusTile(t, rig, "SEARCH")

	rig.RT.Input.Push(input.Event{
		Kind:     input.KindJoystickChange,
		Position: input.JoyDown,
		Source:   input.PlayerSource{Type: input.SourceController, ID: "test-stick", Slot: input.SlotOf(2)},
	})
	// Step 14 from SEARCH lands on the last tile, ZZ-Web; steps 2..13 are the dummies.
	labels := append(append([]string{}, holdScrollNames...), "ZZ-Web")
	arrived := map[string]int{}
	for tick := 1; tick <= 120 && len(arrived) < len(labels); tick++ {
		rig.Tick(1)
		for _, name := range labels {
			if _, seen := arrived[name]; !seen && lcdShows(hw, name) {
				arrived[name] = tick
			}
		}
	}
	if len(arrived) < len(labels) {
		t.Fatalf("the hold reached only %d of %d tiles in 4s", len(arrived), len(labels))
	}
	early := arrived["HoldRepeat-D"] - arrived["HoldRepeat-C"]
	late := arrived["ZZ-Web"] - arrived["HoldRepeat-L"]
	if late >= early {
		t.Fatalf("step 14 took %d ticks after step 13, step 5 took %d after step 4; want the hold to speed up after %d steps", late, early, 12)
	}
}

// TestShellKeyboardHeldDirectionDoesNotDoubleStep checks keyboard input steps once per keypress;
// hold tracking arms only on joystickchange, which the keyboard driver never emits.
func TestShellKeyboardHeldDirectionDoesNotDoubleStep(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	first := holdScrollIndex(hw)
	if first != 0 {
		t.Fatalf("one keyboard keypress should move exactly one step (to dummy index 0); got index %d (LCD:\n%s\n)", first, strings.Join(hw.StatusLines(), "\n"))
	}

	rig.TickFor("1500ms")
	if got := holdScrollIndex(hw); got != first {
		t.Fatalf("selection advanced from %d to %d from ticking alone with no new keypress — keyboard input must never auto-repeat from Menu's hold-tracking", first, got)
	}

	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	if got := holdScrollIndex(hw); got != first+1 {
		t.Fatalf("second keyboard keypress advanced to %d, want exactly %d (one step per keypress, no double-step)", got, first+1)
	}
}
