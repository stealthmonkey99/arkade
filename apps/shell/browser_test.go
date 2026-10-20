// SPDX-License-Identifier: GPL-2.0-or-later

package shell_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"

	_ "github.com/stealthmonkey99/arkade/apps/attract"
)

func statusOf(v *hardware.Virtual) string { return strings.Join(v.StatusLines(), "\n") }

// lcdSettlesOn ticks until the LCD shows text; the Bluetooth app reaches BlueZ on a goroutine, so its screen settles a few frames late.
func lcdSettlesOn(rig *testkit.Rig, v *hardware.Virtual, text string) bool {
	for i := 0; i < 120; i++ {
		if lcdShows(v, text) {
			return true
		}
		rig.Tick(1)
		time.Sleep(time.Millisecond)
	}
	return lcdShows(v, text)
}

// focusTile walks the current shelf from its top until the LCD shows label.
func focusTile(t *testing.T, rig *testkit.Rig, label string) {
	t.Helper()
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	for i := 0; i < 40; i++ {
		rig.PressKeyboard(input.KeyUp, false)
	}
	for i := 0; i < 40; i++ {
		rig.Tick(1)
		if lcdShows(hw, label) {
			return
		}
		rig.PressKeyboard(input.KeyDown, false)
	}
	t.Fatalf("no %q tile on this shelf, status:\n%s", label, statusOf(hw))
}

// init registers a public dummy game for the web-play QR path and a scrollable
// card. It sorts last, matches no HoldRepeat query, and declares no tags, which would add shelves.
func init() {
	engine.Register(engine.Manifest{
		Slug: "test/zz-web", Name: "ZZ-Web", Type: engine.TypeGame,
		Visibility: "public", SortOrder: 2000,
		Author:  "Test",
		Players: []string{"1p", "2p-coop"},
		Description: "A long enough description that the detail card has to " +
			"wrap it across several lines and let the player scroll to read " +
			"the rest of it, and then some more after that.",
		New: func(*engine.AppContext) engine.App { return nil },
	})
}

func TestShellDetailScrollsToLeaderboard(t *testing.T) {
	dir := t.TempDir()
	table := scores.Table{}
	table.Insert(9000, "zoe")
	table.Insert(4242, "ada")
	if err := scores.Save(storage.New(dir), "test/zz-web", table); err != nil {
		t.Fatalf("seed table: %v", err)
	}
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	focusTile(t, rig, "ZZ-Web")
	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	if !lcdShows(hw, "GREEN = PLAY") {
		t.Fatalf("detail card not open, status:\n%s", statusOf(hw))
	}

	// The card advertises the scroll on the LCD's duty cycle.
	scrollHint := false
	for i := 0; i < 200 && !scrollHint; i++ {
		rig.Tick(1)
		scrollHint = lcdShows(hw, "PUSH DOWN")
	}
	if !scrollHint {
		t.Fatalf("card never hinted the scroll on the LCD, status:\n%s", statusOf(hw))
	}

	// The thumb is the one part of the frame that does not animate on its own.
	thumb := func() (first, last int) {
		frame := rig.Frame()
		first, last = -1, -1
		for y := 12; y < 54; y++ {
			if p := frame[y][63]; p != colSeparatorRGB {
				if first < 0 {
					first = y
				}
				last = y
			}
		}
		return first, last
	}
	if first, last := thumb(); first != 12 || last >= 53 {
		t.Fatalf("card opened with the thumb at y%d..y%d, want it parked at the top of a longer document", first, last)
	}

	rig.PressKeyboard(input.KeyDown, false)
	rig.TickFor("500ms") // let the eased scroll settle
	if first, _ := thumb(); first == 12 {
		t.Fatal("stick down did not scroll the detail card's body")
	}

	rig.PressKeyboard(input.KeyUp, false)
	rig.TickFor("500ms")
	if first, _ := thumb(); first != 12 {
		t.Fatalf("stick up did not return the card to the top (thumb at y%d)", first)
	}

	for i := 0; i < 30; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.TickFor("500ms")
	first, last := thumb()
	if last != 53 {
		t.Fatalf("thumb at y%d..y%d after scrolling to the end, want it against the bottom", first, last)
	}
	// Gold ink below the halfway line is the leaderboard heading: the
	// metadata cycle's own gold "HI" line has long scrolled away.
	gold := 0
	frame := rig.Frame()
	for y := 30; y < 54; y++ {
		for x := 0; x < 63; x++ {
			if p := frame[y][x]; p == colGoldRGB {
				gold++
			}
		}
	}
	if gold == 0 {
		t.Fatal("scrolled to the end without reaching the gold HIGH SCORES heading")
	}

	// Clamped: more down does nothing, and B still backs out one level.
	rig.PressKeyboard(input.KeyDown, false)
	rig.TickFor("500ms")
	if f, l := thumb(); f != first || l != last {
		t.Fatalf("card scrolled past the end of its document (thumb y%d..y%d, was y%d..y%d)", f, l, first, last)
	}
	rig.Press(input.KeyJoyB)
	rig.Tick(2)
	if !lcdShows(hw, "ZZ-Web") {
		t.Fatalf("B should still close the card back to the shelf, status:\n%s", statusOf(hw))
	}
}

// TestShellDetailHeldStickScrolls sends one joystickchange then only ticks, as a real held stick does.
func TestShellDetailHeldStickScrolls(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: t.TempDir()})

	focusTile(t, rig, "ZZ-Web")
	rig.Press(input.KeyJoyD)
	rig.TickFor("500ms") // card open, layer faded in
	thumbTop := func() int {
		frame := rig.Frame()
		for y := 12; y < 54; y++ {
			if p := frame[y][63]; p != colSeparatorRGB {
				return y
			}
		}
		return -1
	}
	if got := thumbTop(); got != 12 {
		t.Fatalf("card opened with the thumb at y%d, want the top", got)
	}

	// A seatless stick keeps the engine's seat remap a no-op.
	rig.RT.Input.Push(input.Event{
		Kind: input.KindJoystickChange, Position: input.JoyDown,
		Source: input.PlayerSource{
			Type: input.SourceController, ID: "test-stick",
			Slot: input.SlotOf(2), Seat: input.NoSeat,
		},
	})

	rig.TickFor("300ms")
	mid := thumbTop()
	if mid <= 12 {
		t.Fatalf("held stick did not scroll the card (thumb still at y%d)", mid)
	}
	rig.TickFor("300ms")
	if later := thumbTop(); later <= mid {
		t.Fatalf("held stick stopped repeating: thumb y%d then y%d", mid, later)
	}
	rig.TickFor("3s") // ride the hold well past the end of the document
	end := thumbTop()
	rig.TickFor("1s")
	if got := thumbTop(); got != end {
		t.Fatalf("thumb kept moving (y%d -> y%d) after the document ended", end, got)
	}
}

func TestShellDetailScrollOmitsLeaderboardWithoutScores(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: t.TempDir()})

	focusTile(t, rig, "ZZ-Web")
	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	for i := 0; i < 30; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.TickFor("500ms")

	frame := rig.Frame()
	for y := 12; y < 54; y++ {
		for x := 0; x < 63; x++ {
			if p := frame[y][x]; p == colGoldRGB {
				t.Fatalf("gold leaderboard ink at (%d,%d) for a game with no stored scores", x, y)
			}
		}
	}
}

// colGold and colSeparator as they appear in the frame.
var (
	colGoldRGB      = display.Pixel{R: 252, G: 192, B: 32, A: 255}
	colSeparatorRGB = display.Pixel{R: 52, G: 54, B: 60, A: 255}
)

// TestShellDLightIndicatesEligibility checks D is lit only on games; system apps and control tiles have no card.
func TestShellDLightIndicatesEligibility(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	if !lcdShows(hw, "Clock") {
		t.Fatalf("expected to boot on ALL with Clock (tile 2) focused, status:\n%s", statusOf(hw))
	}
	if hw.ButtonLit(0, "joyd") {
		t.Fatal("D lit while a system app (no detail card) is focused")
	}
	if !hw.ButtonLit(0, "joya") || !hw.ButtonLit(0, "joyb") {
		t.Fatal("A/B must stay lit while browsing")
	}

	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	if !lcdShows(hw, "HoldRepeat-A") {
		t.Fatalf("expected the first game focused after Down, status:\n%s", statusOf(hw))
	}
	if !hw.ButtonLit(0, "joyd") {
		t.Fatal("D unlit while a game (with a detail card) is focused")
	}

	rig.PressKeyboard(input.KeyUp, false)
	rig.PressKeyboard(input.KeyUp, false)
	rig.Tick(1)
	if !lcdShows(hw, "find any game") {
		t.Fatalf("expected the search tile focused after two Ups, status:\n%s", statusOf(hw))
	}
	if hw.ButtonLit(0, "joyd") {
		t.Fatal("D lit on the search control tile, where it does nothing")
	}
	rig.Press(input.KeyJoyD)
	rig.Tick(1)
	if lcdShows(hw, "GREEN = TYPE") {
		t.Fatalf("D engaged a control tile; that is A's job now, status:\n%s", statusOf(hw))
	}
}

func TestShellDetailOpensAndBCloses(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyDown, false) // Clock -> first game
	rig.Tick(1)
	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	if !lcdShows(hw, "GREEN = PLAY") {
		t.Fatalf("detail card not open after D, status:\n%s", statusOf(hw))
	}
	if hw.ButtonLit(0, "joyd") {
		t.Error("D still lit inside the detail card, where it does nothing")
	}
	// A game with no web page leaves C dark.
	if hw.ButtonLit(0, "joyc") {
		t.Error("C lit on a game with no public web play page")
	}

	rig.Press(input.KeyJoyB)
	rig.Tick(2)
	if lcdShows(hw, "GREEN = PLAY") {
		t.Fatalf("detail card still open after B, status:\n%s", statusOf(hw))
	}
	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	if !lcdShows(hw, "HoldRepeat-B") {
		t.Fatalf("browse dead after closing the card, status:\n%s", statusOf(hw))
	}
}

func TestShellDetailQRDeeplink(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	focusTile(t, rig, "ZZ-Web")
	rig.Press(input.KeyJoyD)
	rig.Tick(2)
	if !hw.ButtonLit(0, "joyc") {
		t.Fatal("C unlit on a public game's detail card")
	}
	rig.Press(input.KeyJoyC)
	rig.Tick(2)
	if !lcdShows(hw, "arkade.build/tes") && !lcdShows(hw, "arkade.build/") {
		t.Fatalf("QR overlay should show the readable URL on the LCD, status:\n%s", statusOf(hw))
	}
	if hw.ButtonLit(0, "joyc") {
		t.Error("C still lit while the QR is already showing")
	}
	// The QR is a modal: white card in the middle, the detail card's green hint still visible behind.
	frame := rig.Frame()
	white := 0
	for y := 20; y < 44; y++ {
		for x := 20; x < 44; x++ {
			p := frame[y][x]
			if p.R > 200 && p.G > 200 && p.B > 200 {
				white++
			}
		}
	}
	if white < 100 {
		t.Fatalf("expected a rendered QR modal (white quiet zone + modules), found %d bright pixels", white)
	}
	context := false
	for y := 55; y < 64 && !context; y++ {
		for x := 0; x < 64; x++ {
			p := frame[y][x]
			if p.G > 40 && int(p.G) > int(p.R)+20 {
				context = true
				break
			}
		}
	}
	if !context {
		t.Fatal("detail card context (green A hint) not visible behind the QR modal")
	}

	rig.Press(input.KeyJoyB) // close the overlay, back to the card
	rig.Tick(2)
	if !lcdShows(hw, "GREEN = PLAY") {
		t.Fatalf("B should close the QR back to the card, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyB) // and the card, back to the shelf
	rig.Tick(2)
	if !lcdShows(hw, "ZZ-Web") {
		t.Fatalf("B should close the card back to the shelf, status:\n%s", statusOf(hw))
	}
}

// TestShellResumeRestoresTheHUDAndDropsTheHold parks the shell in standby
// while a stick is held and wakes it: the HUD is back on the wake tick, and
// the hold, whose release the shell never saw, does not scroll the shelf.
func TestShellResumeRestoresTheHUDAndDropsTheHold(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "shell",
		AttractAfter: 500 * time.Millisecond,
		StandbyAfter: time.Minute,
	})
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	rig.Tick(1)

	rig.RT.Input.Push(input.Event{
		Kind:     input.KindJoystickChange,
		Position: input.JoyDown,
		Source:   input.PlayerSource{Type: input.SourceController, ID: "test-stick", Slot: input.SlotOf(2)},
	})
	// The stick keeps stepping until the shell idles; parked is the HUD of its last active frame.
	parked := ""
	for i := 0; i < 60 && rig.RT.CurrentSlug() != "attract"; i++ {
		parked, _ = hw.Lines(0)
		rig.Tick(1)
	}
	if got := rig.RT.CurrentSlug(); got != "attract" {
		t.Fatalf("running %q, want attract to have taken over", got)
	}
	if l1, _ := hw.Lines(0); l1 == parked {
		t.Fatalf("attract left the shell's %q on the LCD, so a restore would prove nothing", parked)
	}

	rig.Press(input.KeyJoyC) // green in attract is the hold-to-launch chord, not a wake
	rig.Tick(1)
	if got := rig.RT.StandbyState(); got != engine.StateActive {
		t.Fatalf("StandbyState = %v after the wake press, want active", got)
	}
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("running %q after the wake, want the shell back (the wake press must not launch)", got)
	}
	if l1, _ := hw.Lines(0); l1 != parked {
		t.Fatalf("LCD shows %q on the wake tick, want the shelf's %q restored", l1, parked)
	}

	// Long enough for the parked hold's repeat delay to have run out, short of idling again.
	rig.TickFor("400ms")
	if l1, _ := hw.Lines(0); l1 != parked {
		t.Fatalf("LCD shows %q after the wake, want %q: the stale hold scrolled the shelf", l1, parked)
	}
}

// TestShellCategorySwitch flips ALL to SYSTEM and back; SLEEP leads SYSTEM because the volume tile takes left/right.
func TestShellCategorySwitch(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(1)
	if !lcdShows(hw, "SLEEP") {
		t.Fatalf("expected the SYSTEM shelf with SLEEP leading after Right, status:\n%s", statusOf(hw))
	}
	rig.PressKeyboard(input.KeyLeft, false)
	rig.Tick(1)
	if !lcdShows(hw, "SEARCH") {
		t.Fatalf("expected Left to wrap back to ALL, status:\n%s", statusOf(hw))
	}
}

// TestShellLCDPanelsMirror checks both LCD panels always carry the same lines, including a focused game's best score.
func TestShellLCDPanelsMirror(t *testing.T) {
	dir := t.TempDir()
	table := scores.Table{}
	table.Insert(4242, "ada")
	if err := scores.Save(storage.New(dir), "test/zz-web", table); err != nil {
		t.Fatalf("seed table: %v", err)
	}
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	assertMirrored := func(msg string) {
		p1a, p1b := hw.Lines(0)
		p2a, p2b := hw.Lines(1)
		if p1a != p2a || p1b != p2b {
			t.Fatalf("%s: P1=%q/%q P2=%q/%q, want them to match", msg, p1a, p1b, p2a, p2b)
		}
	}

	assertMirrored("booting on a non-game tile")

	focusTile(t, rig, "ZZ-Web")
	if !lcdShows(hw, "ADA 4242") {
		t.Fatalf("focused game's best score missing from the LCDs, status:\n%s", statusOf(hw))
	}
	assertMirrored("focused on a game with a best score")

	rig.PressKeyboard(input.KeyRight, false) // SYSTEM, SLEEP leading
	rig.Tick(1)
	assertMirrored("on the SYSTEM shelf")
}

// favStarYellow is colButtonC as the footer hint dots paint it.
func favStarYellow(r, g, b uint8) bool { return r == 240 && g == 200 && b == 40 }

// favBadgeWarm matches the badge yellow at any point of its fade by hue.
func favBadgeWarm(p display.Pixel) bool {
	return int(p.R) > int(p.G)+8 && int(p.G) > int(p.B)+40 && p.G > 40
}

// badgeOverhang returns the brightest badge pixel in the gutter between the tile icon and the label, 0 for none.
func badgeOverhang(frame display.Matrix) int {
	best := 0
	for y := 13; y < 53; y++ {
		for x := 11; x < 14; x++ {
			if p := frame[y][x]; favBadgeWarm(p) && int(p.G) > best {
				best = int(p.G)
			}
		}
	}
	return best
}

func TestShellFavorites(t *testing.T) {
	dir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	if hw.ButtonLit(0, "joyc") {
		t.Fatal("C lit on a non-game tile")
	}
	rig.PressKeyboard(input.KeyDown, false) // first game
	rig.Tick(1)
	if !hw.ButtonLit(0, "joyc") {
		t.Fatal("C unlit on a focused game")
	}

	rig.Press(input.KeyJoyC)
	rig.Tick(1)
	if !lcdShows(hw, "FAVORITED") {
		t.Fatalf("favorite toggle not announced, status:\n%s", statusOf(hw))
	}
	rig.TickFor("2s") // ride out the announce + pop animation
	if badgeOverhang(rig.Frame()) == 0 {
		t.Fatal("no yellow star badge on the favorited tile")
	}

	rig.PressKeyboard(input.KeyLeft, false)
	rig.Tick(1)
	if !lcdShows(hw, "HoldRepeat-A") {
		t.Fatalf("FAVORITES shelf missing after starring, status:\n%s", statusOf(hw))
	}

	// Read straight from storage, the same file as the MRU.
	var saved struct {
		Favorites []string `json:"favorites"`
	}
	if err := storage.New(dir).Read("shell", "", &saved); err != nil || len(saved.Favorites) != 1 {
		t.Fatalf("favorites not persisted with the shell settings (err=%v, saved=%+v)", err, saved)
	}

	rig.Press(input.KeyJoyC)
	rig.Tick(1)
	if !lcdShows(hw, "UN-FAVORITED") {
		t.Fatalf("un-favorite not announced, status:\n%s", statusOf(hw))
	}
	rig.TickFor("2s")
	if !lcdShows(hw, "HoldRepeat-A") {
		t.Fatalf("emptied FAVORITES shelf should vanish back to ALL, status:\n%s", statusOf(hw))
	}
}

// TestShellFavoriteBadgeFades checks the badge overhangs into the gutter and fades without ever vanishing.
func TestShellFavoriteBadgeFades(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: t.TempDir()})

	rig.PressKeyboard(input.KeyDown, false) // first game
	rig.Tick(1)
	if got := badgeOverhang(rig.Frame()); got != 0 {
		t.Fatalf("badge ink in the gutter (brightest %d) with nothing favorited", got)
	}

	rig.Press(input.KeyJoyC)
	rig.TickFor("1s") // past the pop, onto the resting badge

	dim, bright := 255, 0
	for i := 0; i < 60; i++ {
		got := badgeOverhang(rig.Frame())
		if got == 0 {
			t.Fatalf("badge went out %d samples into its fade cycle", i)
		}
		dim, bright = min(dim, got), max(bright, got)
		rig.TickFor("50ms")
	}
	if bright-dim < 30 {
		t.Fatalf("badge brightness barely moved over a fade cycle (%d..%d), want a visible oscillation", dim, bright)
	}
	if dim*100 < bright*35 {
		t.Fatalf("badge faded to %d against a peak of %d — below the legibility floor", dim, bright)
	}
}

func TestShellVolumeSlider(t *testing.T) {
	dir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false) // SYSTEM shelf, SLEEP leads
	rig.PressKeyboard(input.KeyDown, false)  // down one to VOLUME
	rig.Tick(1)
	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if !lcdShows(hw, "BLUE = MUTE") {
		t.Fatalf("volume panel not engaged, status:\n%s", statusOf(hw))
	}

	rig.PressKeyboard(input.KeyRight, false) // 50 -> 55
	rig.Tick(1)
	var cfg struct {
		Volume int  `json:"volume"`
		Muted  bool `json:"muted"`
	}
	if err := storage.New(dir).Read("apps/config", "", &cfg); err != nil {
		t.Fatalf("volume change not persisted to the settings app's storage: %v", err)
	}
	if cfg.Volume != 55 || cfg.Muted {
		t.Fatalf("persisted audio settings = %+v, want volume 55 unmuted", cfg)
	}

	rig.Press(input.KeyJoyD) // mute
	rig.Tick(1)
	if !lcdShows(hw, "- muted -") {
		t.Fatalf("mute not reflected, status:\n%s", statusOf(hw))
	}
	if err := storage.New(dir).Read("apps/config", "", &cfg); err != nil || !cfg.Muted {
		t.Fatalf("mute not persisted (err=%v, cfg=%+v)", err, cfg)
	}

	rig.Press(input.KeyJoyB) // back to the shelf
	rig.Tick(1)
	if !lcdShows(hw, "VOLUME") {
		t.Fatalf("B did not return to the SYSTEM shelf on the volume tile, status:\n%s", statusOf(hw))
	}
}

// TestShellVolumeQuickAdjust checks left/right on the VOLUME tile adjusts in place without flipping shelves.
func TestShellVolumeQuickAdjust(t *testing.T) {
	dir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false) // SYSTEM, on SLEEP
	rig.PressKeyboard(input.KeyDown, false)  // one down: VOLUME
	rig.Tick(1)
	if !lcdShows(hw, "level: 50%") {
		t.Fatalf("expected the volume tile focused, status:\n%s", statusOf(hw))
	}

	volumeOf := func() int {
		var cfg struct {
			Volume int `json:"volume"`
		}
		if err := storage.New(dir).Read("apps/config", "", &cfg); err != nil {
			t.Fatalf("read persisted volume: %v", err)
		}
		return cfg.Volume
	}

	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(1)
	if !lcdShows(hw, "level: 55%") || !lcdShows(hw, "VOLUME") {
		t.Fatalf("right on the volume tile should adjust in place, not change shelf, status:\n%s", statusOf(hw))
	}
	if got := volumeOf(); got != 55 {
		t.Errorf("persisted volume = %d, want 55 straight from the shelf", got)
	}
	rig.PressKeyboard(input.KeyLeft, false)
	rig.PressKeyboard(input.KeyLeft, false)
	rig.Tick(1)
	if got := volumeOf(); got != 45 {
		t.Errorf("persisted volume = %d after two lefts, want 45", got)
	}
	if !lcdShows(hw, "VOLUME") {
		t.Fatalf("left on the volume tile changed shelf, status:\n%s", statusOf(hw))
	}

	rig.PressKeyboard(input.KeyUp, false)
	rig.Tick(1)
	if !lcdShows(hw, "SLEEP") {
		t.Fatalf("up should walk off the volume tile, status:\n%s", statusOf(hw))
	}
	rig.PressKeyboard(input.KeyLeft, false)
	rig.Tick(1)
	if !lcdShows(hw, "SEARCH") {
		t.Fatalf("left off the volume tile should flip shelves again, status:\n%s", statusOf(hw))
	}
}

// brightnessOf reads the persisted brightness.
func brightnessOf(t *testing.T, dir string) int {
	t.Helper()
	var cfg struct {
		Brightness int `json:"brightness"`
	}
	if err := storage.New(dir).Read("apps/config", "", &cfg); err != nil {
		t.Fatalf("read persisted brightness: %v", err)
	}
	return cfg.Brightness
}

// TestShellBrightnessQuickAdjust checks the BRIGHT tile adjusts in place and stops at the engine floor, not zero.
func TestShellBrightnessQuickAdjust(t *testing.T) {
	dir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false) // SYSTEM, on SLEEP
	rig.PressKeyboard(input.KeyDown, false)  // VOLUME
	rig.PressKeyboard(input.KeyDown, false)  // BRIGHT
	rig.Tick(1)
	if !lcdShows(hw, "BRIGHTNESS") || !lcdShows(hw, "level: 100%") {
		t.Fatalf("expected the brightness tile focused at full, status:\n%s", statusOf(hw))
	}

	rig.PressKeyboard(input.KeyLeft, false)
	rig.Tick(1)
	if !lcdShows(hw, "level: 95%") || !lcdShows(hw, "BRIGHTNESS") {
		t.Fatalf("left on the brightness tile should dim in place, not change shelf, status:\n%s", statusOf(hw))
	}
	if got := rig.RT.PanelBrightness(); got != 95 {
		t.Errorf("runtime brightness = %d, want 95 applied live from the shelf", got)
	}
	if got := brightnessOf(t, dir); got != 95 {
		t.Errorf("persisted brightness = %d, want 95", got)
	}

	for i := 0; i < 25; i++ {
		rig.PressKeyboard(input.KeyLeft, false)
	}
	rig.Tick(1)
	if got := rig.RT.PanelBrightness(); got != engine.MinPanelBrightness {
		t.Errorf("runtime brightness = %d after bottoming out, want the %d floor", got, engine.MinPanelBrightness)
	}
	if !lcdShows(hw, "level: 10%") {
		t.Fatalf("brightness should floor at 10%%, never 0, status:\n%s", statusOf(hw))
	}
	if got := brightnessOf(t, dir); got != engine.MinPanelBrightness {
		t.Errorf("persisted brightness = %d, want the floor", got)
	}
}

func TestShellBrightnessPanelLamps(t *testing.T) {
	dir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyDown, false) // BRIGHT
	rig.Tick(1)
	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if !lcdShows(hw, "BLUE: LAMPS AUTO") {
		t.Fatalf("brightness panel not engaged by A, status:\n%s", statusOf(hw))
	}
	if !hw.ButtonLit(0, "joyd") {
		t.Error("D unlit on the brightness panel, where it cycles the lamps")
	}

	rig.PressKeyboard(input.KeyRight, false) // already at 100: the rail rejects
	rig.PressKeyboard(input.KeyLeft, false)
	rig.Tick(1)
	if got := rig.RT.PanelBrightness(); got != 95 {
		t.Errorf("runtime brightness = %d after a left on the panel, want 95", got)
	}

	rig.Press(input.KeyJoyD)
	rig.Tick(1)
	if !lcdShows(hw, "BLUE: LAMPS ON") {
		t.Fatalf("lamp policy not reflected, status:\n%s", statusOf(hw))
	}
	if got := rig.RT.LampPolicy(); got != engine.LampsOn {
		t.Errorf("runtime lamp policy = %q, want %q", got, engine.LampsOn)
	}
	rig.Press(input.KeyJoyD)
	rig.Tick(1)
	if !lcdShows(hw, "BLUE: LAMPS OFF") {
		t.Fatalf("second press should park the lamps, status:\n%s", statusOf(hw))
	}
	if got := rig.RT.LampPolicy(); got != engine.LampsOff {
		t.Errorf("runtime lamp policy = %q, want %q", got, engine.LampsOff)
	}
	var cfg struct {
		Lamps    string `json:"lamps"`
		LampsOff *bool  `json:"lampsOff"`
	}
	if err := storage.New(dir).Read("apps/config", "", &cfg); err != nil || cfg.Lamps != string(engine.LampsOff) {
		t.Fatalf("lamp policy not persisted (err=%v, cfg=%+v)", err, cfg)
	}
	if cfg.LampsOff != nil {
		t.Error("the superseded lampsOff key should be dropped on save")
	}

	rig.Press(input.KeyJoyB)
	rig.Tick(1)
	if !lcdShows(hw, "BRIGHTNESS") {
		t.Fatalf("B did not return to the SYSTEM shelf on the brightness tile, status:\n%s", statusOf(hw))
	}
}

// TestShellCoolingPanel cycles the fan mode auto, on, off, auto and checks both mode and threshold persist to the settings app's keys.
func TestShellCoolingPanel(t *testing.T) {
	dir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	focusTile(t, rig, "CPU TEMP")
	// The virtual suite has no sensor.
	if !lcdShows(hw, "CPU TEMP") || !lcdShows(hw, "-- fan>=120") {
		t.Fatalf("expected the temp tile with a dashed reading, status:\n%s", statusOf(hw))
	}

	rig.Press(input.KeyJoyA) // green opens every control tile
	rig.Tick(1)
	if !lcdShows(hw, "BLUE: FANS AUTO") {
		t.Fatalf("cooling panel not engaged, status:\n%s", statusOf(hw))
	}
	if !hw.ButtonLit(0, "joyd") {
		t.Error("D unlit on the cooling panel, where it cycles the fan mode")
	}

	var cfg struct {
		Threshold int    `json:"fanThresholdF"`
		Mode      string `json:"fanMode"`
	}
	read := func() {
		if err := storage.New(dir).Read("apps/config", "", &cfg); err != nil {
			t.Fatalf("fan config not persisted to the settings app's storage: %v", err)
		}
	}

	rig.PressKeyboard(input.KeyRight, false) // 120 -> 125
	rig.Tick(1)
	if !lcdShows(hw, "fan at 125F") {
		t.Fatalf("threshold not adjusted, status:\n%s", statusOf(hw))
	}
	read()
	if cfg.Threshold != 125 || cfg.Mode != string(hardware.FanAuto) {
		t.Fatalf("persisted cooling settings = %+v, want 125°F auto", cfg)
	}

	rig.Press(input.KeyJoyD) // auto -> on
	rig.Tick(1)
	if !lcdShows(hw, "BLUE: FANS ON") || !lcdShows(hw, "- fans on -") {
		t.Fatalf("fan-on mode not reflected, status:\n%s", statusOf(hw))
	}
	read()
	if cfg.Mode != string(hardware.FanOn) {
		t.Fatalf("fan-on mode not persisted (%+v)", cfg)
	}

	rig.Press(input.KeyJoyD) // on -> off
	rig.Tick(1)
	if !lcdShows(hw, "BLUE: FANS OFF") || !lcdShows(hw, "- fans off -") {
		t.Fatalf("fan-off mode not reflected, status:\n%s", statusOf(hw))
	}
	read()
	if cfg.Mode != string(hardware.FanOff) {
		t.Fatalf("fan-off mode not persisted (%+v)", cfg)
	}

	rig.Press(input.KeyJoyD) // off -> auto: the cycle wraps rather than dead-ending
	rig.Tick(1)
	if !lcdShows(hw, "BLUE: FANS AUTO") {
		t.Fatalf("fan mode did not wrap back to auto, status:\n%s", statusOf(hw))
	}
	read()
	if cfg.Mode != string(hardware.FanAuto) {
		t.Fatalf("auto mode not persisted (%+v)", cfg)
	}

	rig.Press(input.KeyJoyD) // auto -> on
	rig.Tick(1)
	rig.Press(input.KeyJoyD) // on -> off, to check what persists across a reboot below
	rig.Tick(1)

	rig.Press(input.KeyJoyB) // back to the shelf
	rig.Tick(1)
	if !lcdShows(hw, "CPU TEMP") {
		t.Fatalf("B did not return to the SYSTEM shelf on the temp tile, status:\n%s", statusOf(hw))
	}

	rig2 := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw2 := rig2.RT.RawHardware().(*hardware.Virtual)
	rig2.PressKeyboard(input.KeyRight, false)
	focusTile(t, rig2, "CPU TEMP")
	if !lcdShows(hw2, "-- fans off") {
		t.Fatalf("stored cooling config not restored on a fresh shell, status:\n%s", statusOf(hw2))
	}
}

// TestShellCoolingPanelMigratesFanDisabled checks a legacy fanDisabled bool reads as fans off and a save replaces it with fanMode.
func TestShellCoolingPanelMigratesFanDisabled(t *testing.T) {
	dir := t.TempDir()
	if err := storage.New(dir).Write("apps/config", "", map[string]any{"fanDisabled": true}); err != nil {
		t.Fatalf("seed pre-upgrade settings: %v", err)
	}
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: dir})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	focusTile(t, rig, "CPU TEMP")
	if !lcdShows(hw, "-- fans off") {
		t.Fatalf("legacy fanDisabled:true should read as fans off, status:\n%s", statusOf(hw))
	}

	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	rig.Press(input.KeyJoyD) // off -> auto, forces a save
	rig.Tick(1)

	var cfg struct {
		Mode     string `json:"fanMode"`
		Disabled *bool  `json:"fanDisabled"`
	}
	if err := storage.New(dir).Read("apps/config", "", &cfg); err != nil || cfg.Mode != string(hardware.FanAuto) {
		t.Fatalf("fan mode not persisted after migration (err=%v, cfg=%+v)", err, cfg)
	}
	if cfg.Disabled != nil {
		t.Error("the superseded fanDisabled key should be dropped on save")
	}
}

func TestShellWifiTile(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	focusTile(t, rig, "JOIN WIFI")
	if !lcdShows(hw, "scan to connect") {
		t.Fatalf("expected the wifi tile focused, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyA) // green opens it
	rig.Tick(1)
	if !lcdShows(hw, "scan with phone") {
		t.Fatalf("wifi QR modal not shown, status:\n%s", statusOf(hw))
	}
	// White card in the middle with the shelf header still visible above it.
	frame := rig.Frame()
	if p := frame[32][32]; p.R < 150 || p.G < 150 || p.B < 150 {
		t.Fatalf("expected the wifi QR's white card at center, got %+v", p)
	}
	shelf := false
	for x := 0; x < 64 && !shelf; x++ {
		p := frame[3][x]
		if p.R != 0 || p.G != 0 || p.B != 0 {
			shelf = true
		}
	}
	if !shelf {
		t.Fatal("shelf header not visible behind the wifi modal")
	}
	rig.Press(input.KeyJoyB)
	rig.Tick(1)
	if !lcdShows(hw, "JOIN WIFI") {
		t.Fatalf("B did not close the wifi modal, status:\n%s", statusOf(hw))
	}
}

func TestShellShutdownConfirm(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	focusTile(t, rig, "SHUT DOWN")
	if !lcdShows(hw, "SHUT DOWN") {
		t.Fatalf("expected the shutdown tile focused, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if !lcdShows(hw, "GREEN YES RED NO") {
		t.Fatalf("shutdown confirm not shown, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyB)
	rig.Tick(1)
	if !lcdShows(hw, "SHUT DOWN") {
		t.Fatalf("B did not back out of the confirm, status:\n%s", statusOf(hw))
	}

	rig.Press(input.KeyJoyA) // re-open confirm
	rig.Tick(1)
	rig.Press(input.KeyJoyA) // and confirm
	rig.Tick(1)
	if rig.RT.QuitRequested() {
		t.Fatal("shutdown quit immediately, want the power-off sequence first")
	}
	rig.TickFor("2300ms")
	if !lcdShows(hw, "Power OFF") {
		t.Fatalf("power-off message missing, status:\n%s", statusOf(hw))
	}
	if !rig.RT.QuitRequested() {
		t.Fatal("the power-off sequence did not request an engine quit")
	}
}

func TestShellSearch(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	focusTile(t, rig, "SEARCH")
	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if !lcdShows(hw, "GREEN = TYPE") {
		t.Fatalf("search keyboard not open, status:\n%s", statusOf(hw))
	}
	if hw.ButtonLit(0, "joyd") || hw.ButtonLit(0, "joyc") {
		t.Fatal("C/D lit with an empty query and no results")
	}

	// The cursor starts on A; H is 7 steps right.
	for i := 0; i < 7; i++ {
		rig.PressKeyboard(input.KeyRight, false)
	}
	rig.Tick(1)
	rig.Press(input.KeyJoyA)
	rig.Tick(1)
	if !lcdShows(hw, "SEARCH: H") || !lcdShows(hw, "12 matches") {
		t.Fatalf("typing H should live-match the 12 HoldRepeat games, status:\n%s", statusOf(hw))
	}
	if !hw.ButtonLit(0, "joyd") || !hw.ButtonLit(0, "joyc") {
		t.Fatal("C (delete) and D (go) should be lit with a query and results")
	}

	rig.Press(input.KeyJoyD) // to the results shelf
	rig.Tick(1)
	if !lcdShows(hw, "HoldRepeat-A") {
		t.Fatalf("results shelf not shown, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyD) // detail card straight from results
	rig.Tick(2)
	if !lcdShows(hw, "GREEN = PLAY") {
		t.Fatalf("detail card not available from results, status:\n%s", statusOf(hw))
	}

	rig.Press(input.KeyJoyB)
	rig.Tick(2)
	if !lcdShows(hw, "HoldRepeat-A") {
		t.Fatalf("B from the card should return to results, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyB)
	rig.Tick(1)
	if !lcdShows(hw, "SEARCH: H") {
		t.Fatalf("B from results should return to the keyboard, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyB)
	rig.Tick(1)
	if !lcdShows(hw, "SEARCH") {
		t.Fatalf("B from the keyboard should return to the shelf, status:\n%s", statusOf(hw))
	}
}

// infoChipBlue is colButtonD, the corner hint's fill.
func infoChipBlue(r, g, b uint8) bool { return r == 70 && g == 140 && b == 255 }
func playDotGreen(r, g, b uint8) bool { return r == 60 && g == 220 && b == 90 }

// TestShellCornerHintCycle checks a focused game's corner hint cycles PLAY, PIN, INFO and no per-row D chip appears.
func TestShellCornerHintCycle(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})

	rig.PressKeyboard(input.KeyDown, false) // focus a game
	rig.Tick(2)

	cornerHas := func(match func(r, g, b uint8) bool) bool {
		frame := rig.Frame()
		for y := 55; y < 64; y++ {
			for x := 32; x < 64; x++ {
				if p := frame[y][x]; match(p.R, p.G, p.B) {
					return true
				}
			}
		}
		return false
	}
	waitFor := func(match func(r, g, b uint8) bool, what string) {
		for i := 0; i < 200; i++ {
			if cornerHas(match) {
				return
			}
			rig.Tick(1)
		}
		t.Fatalf("%s never appeared in the corner", what)
	}

	if !cornerHas(playDotGreen) {
		t.Fatal("green PLAY dot missing at cycle start with a game focused")
	}
	waitFor(favStarYellow, "yellow PIN dot")
	waitFor(infoChipBlue, "blue INFO dot")

	frame := rig.Frame()
	for y := 12; y < 53; y++ {
		for x := 0; x < 64; x++ {
			if p := frame[y][x]; infoChipBlue(p.R, p.G, p.B) {
				t.Fatal("per-row D chip still rendered in the list area")
			}
		}
	}
}

// TestShellNonGameHintIsGreenOnly checks a tile with no card and no pin advertises only green.
func TestShellNonGameHintIsGreenOnly(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	if !lcdShows(hw, "Clock") {
		t.Fatalf("expected to boot on Clock, status:\n%s", statusOf(hw))
	}
	cornerHas := func(match func(r, g, b uint8) bool) bool {
		frame := rig.Frame()
		for y := 55; y < 64; y++ {
			for x := 32; x < 64; x++ {
				if p := frame[y][x]; match(p.R, p.G, p.B) {
					return true
				}
			}
		}
		return false
	}

	// Run well past a full three-slot cycle.
	for i := 0; i < 300; i++ {
		if !cornerHas(playDotGreen) {
			t.Fatalf("green OPEN dot missing on a non-game tile at tick %d", i)
		}
		if cornerHas(infoChipBlue) || cornerHas(favStarYellow) {
			t.Fatalf("a dead button advertised itself on a non-game tile at tick %d", i)
		}
		rig.Tick(1)
	}
}

// TestShellSleepTile checks A on SLEEP reaches EnterSleep, leaving StateDark with the shell suspended, not unloaded.
func TestShellSleepTile(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "shell",
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false) // SYSTEM, SLEEP leading
	rig.Tick(1)
	if !lcdShows(hw, "rest the screen") {
		t.Fatalf("expected the sleep tile focused, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyA)
	rig.Tick(1)

	if got := rig.RT.StandbyState(); got != engine.StateDark {
		t.Fatalf("StandbyState = %v after SLEEP, want StateDark", got)
	}
	if got := rig.RT.Status().SuspendedSlug; got != "shell" {
		t.Fatalf("SuspendedSlug = %q, want the shell suspended rather than unloaded", got)
	}
}

func TestShellSleepTileExitsWithoutStandby(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(1)
	if !lcdShows(hw, "rest the screen") {
		t.Fatalf("expected the sleep tile focused, status:\n%s", statusOf(hw))
	}
	rig.Press(input.KeyJoyA)
	rig.Tick(2) // the unload lands a tick after the request

	if !rig.RT.QuitRequested() {
		t.Fatal("SLEEP with standby disabled should exit the engine")
	}
}

func TestShellBackAtTheRootExitsWithoutStandby(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	rig.Tick(1)
	rig.Press(input.KeyJoyB)
	rig.Tick(2)
	if !rig.RT.QuitRequested() {
		t.Fatal("B at the shelf root with standby disabled should exit the engine")
	}
}

// TestShellListClipsAtViewportBands checks that mid-scroll no list pixels land in the bands at y11, y12 and y53.
func TestShellListClipsAtViewportBands(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})

	for i := 0; i < 8; i++ {
		rig.PressKeyboard(input.KeyDown, false)
	}
	rig.Tick(2) // the eased scroll settles in about 6 ticks
	frame := rig.Frame()
	for _, y := range []int{11, 12, 53} {
		for x := 0; x < 64; x++ {
			p := frame[y][x]
			if p.R != 0 || p.G != 0 || p.B != 0 {
				t.Fatalf("list content leaked into reserved band at (%d,%d) = %+v mid-scroll", x, y, p)
			}
		}
	}
}

// TestShellBluetoothTileLaunchesTheApp checks the tile carries its own manifest because the app is hidden.
func TestShellBluetoothTileLaunchesTheApp(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: t.TempDir()})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	focusTile(t, rig, "BLUETOOTH")

	rig.Press(input.KeyJoyA)
	rig.TickFor("600ms")

	// A suite with no BlueZ shows "No Bluetooth", which means the app is running.
	if !lcdSettlesOn(rig, hw, "No Bluetooth") {
		t.Fatalf("green on the BLUETOOTH tile did not launch the app, status:\n%s", statusOf(hw))
	}
}

// TestShellBluetoothAppBacksOut checks B leaves the app and returns to the BLUETOOTH tile.
func TestShellBluetoothAppBacksOut(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell", DataDir: t.TempDir()})
	hw := rig.RT.RawHardware().(*hardware.Virtual)

	rig.PressKeyboard(input.KeyRight, false)
	focusTile(t, rig, "BLUETOOTH")
	rig.Press(input.KeyJoyA)
	rig.TickFor("600ms")
	if !lcdSettlesOn(rig, hw, "No Bluetooth") {
		t.Fatalf("the app did not open, status:\n%s", statusOf(hw))
	}

	rig.Press(input.KeyJoyB)
	rig.TickFor("600ms")
	if lcdShows(hw, "No Bluetooth") {
		t.Fatalf("red did not leave the app, status:\n%s", statusOf(hw))
	}
	if !lcdShows(hw, "BLUETOOTH") {
		t.Fatalf("red did not return to the BLUETOOTH tile, status:\n%s", statusOf(hw))
	}
}
