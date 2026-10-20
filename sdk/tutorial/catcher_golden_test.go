// SPDX-License-Identifier: GPL-2.0-or-later

package main

// These tests drive the committed catcher.wasm, so they run without TinyGo;
// TestCatcherWasmMatchesSource checks the binary still matches the source.
// sdk/docs/tutorial-debug.md walks through its skeleton.
//
// The journey relies on the authored waves drawing no random numbers: the
// first drop is over the catcher's starting column and the next four are
// not, so an idle player catches one fruit, misses three and ends on a score
// of exactly catchScore, the smallest run that reaches a qualifying name
// entry. RandScript covers the endless finale, which draws one number per
// random spawn; FakeHost clamps at the last value, so two elements describe
// a whole run.

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
)

const (
	catcherWasmPath = "catcher.wasm"

	// the cabinet's 30 fps cadence; the sim's own 25ms step is independent
	catcherTickMicros = 33_333

	// Physical addresses the LCD and button lights; Player is the pairing slot
	catcherSeat   uint8 = 0
	catcherPlayer uint8 = 1

	// the catcher's starting left edge, (64-7)/2; the first scripted fruit
	// spawns here
	catcherStartColumn uint64 = 28
)

func loadCatcherWasm(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(catcherWasmPath)
	if err != nil {
		t.Fatalf("read the checked-in %s (run `make build` to rebuild it): %v", catcherWasmPath, err)
	}
	return data
}

// newCatcherHost is the scripted host every test here starts from. It mounts
// the cartridge's assets/ directory, since the art lives in the bundle and
// not in catcher.wasm.
func newCatcherHost(t *testing.T) *carttest.FakeHost {
	t.Helper()
	host := carttest.NewFakeHost()
	if err := host.MountAssets("."); err != nil {
		t.Fatalf("MountAssets: %v", err)
	}
	host.RandScript = []uint64{catcherStartColumn, 0}
	return host
}

// catcherOptions builds the cabinet's options from manifest.json's
// assets.lights, the lamp sequences host_lights_play can name
// (sdk/docs/reference-lights.md).
func catcherOptions(t *testing.T) carttest.Options {
	t.Helper()
	return carttest.Options{BundleLights: manifestLights(t)}
}

func manifestLights(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Assets struct {
			Lights []string `json:"lights"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m.Assets.Lights
}

func newCatcherCartridge(t *testing.T, host carttest.Host) *carttest.Cartridge {
	t.Helper()
	cart, err := carttest.NewCartridge(context.Background(), loadCatcherWasm(t), host, catcherOptions(t))
	if err != nil {
		t.Fatalf("NewCartridge(catcher.wasm): %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	carttest.AdvancePastSplash(t, cart)
	return cart
}

func catcherTick(t *testing.T, cart *carttest.Cartridge, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if err := cart.Update(ctx, catcherTickMicros); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := cart.Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
}

// catcherPress sends the keydown, keypress and keyup triple a cabinet button
// emits; ui.HoldConfirm cannot arm without the keyup.
func catcherPress(t *testing.T, cart *carttest.Cartridge, key uint8) {
	t.Helper()
	ctx := context.Background()
	for _, kind := range []uint8{carttest.RecordKindKeydown, carttest.RecordKindKeypress, carttest.RecordKindKeyup} {
		rec := carttest.InputRecord{
			Kind: kind, Key: key,
			Player: catcherPlayer, Physical: catcherSeat,
			Source: carttest.RecordSourceController, Index: 0,
		}
		if err := cart.DispatchInput(ctx, []carttest.InputRecord{rec}); err != nil {
			t.Fatalf("DispatchInput(key=%d kind=%d): %v", key, kind, err)
		}
	}
}

func TestCatcherManifestAndABI(t *testing.T) {
	host := newCatcherHost(t)
	cart, err := carttest.NewCartridge(context.Background(), loadCatcherWasm(t), host, catcherOptions(t))
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })

	version, err := cart.ABIVersion(context.Background())
	if err != nil {
		t.Fatalf("ABIVersion: %v", err)
	}
	// a host states the range it accepts
	if version < carttest.MinSupportedABIVersion || version > carttest.SupportedABIVersion {
		t.Fatalf("ABIVersion = %d, want %d..%d",
			version, carttest.MinSupportedABIVersion, carttest.SupportedABIVersion)
	}

	raw, err := cart.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	var m struct {
		ABI     int    `json:"abi"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Players []int  `json:"players"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("cart_manifest is not JSON (%q): %v", raw, err)
	}
	if m.ABI != 1 || m.Slug != "catcher" || m.Name != "Catcher" {
		t.Errorf("cart_manifest = %+v, want abi=1 slug=catcher name=Catcher", m)
	}
	if len(m.Players) != 1 || m.Players[0] != 1 {
		t.Errorf("cart_manifest.players = %v, want [1]", m.Players)
	}

	// the bundle manifest and the compiled-in one must agree on slug and version
	bundleRaw, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatalf("read manifest.json: %v", err)
	}
	var bundle struct {
		Slug    string `json:"slug"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(bundleRaw, &bundle); err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	if bundle.Slug != m.Slug || bundle.Version != m.Version {
		t.Errorf("manifest.json says %s/%s, the wasm says %s/%s — bump both",
			bundle.Slug, bundle.Version, m.Slug, m.Version)
	}

	// Render must draw a valid frame before the first Update
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cart.Render(context.Background()); err != nil {
		t.Fatalf("Render before the first Update: %v", err)
	}
	if len(host.Blits) != 1 {
		t.Fatalf("blits after the init render = %d, want 1", len(host.Blits))
	}
}

// TestCatcherQuitKeyOwnership: Catcher owns its quit keys, so Escape at the
// title must reach host_exit.
func TestCatcherQuitKeyOwnership(t *testing.T) {
	host := newCatcherHost(t)
	cart := newCatcherCartridge(t, host)
	catcherPress(t, cart, carttest.KeyEscape)
	if host.Exits != 1 {
		t.Fatalf("host_exit calls after Escape at the title = %d, want 1", host.Exits)
	}
}

// catcherJourney plays the scripted run from the title to wherever the
// caller asks it to stop, and is the shared spine of the tests below.
// Every run with the same script is byte-identical, which is what
// TestCatcherDeterminism relies on.
func catcherJourney(t *testing.T, dump bool) (*carttest.Cartridge, *carttest.FakeHost) {
	t.Helper()
	host := newCatcherHost(t)
	cart := newCatcherCartridge(t, host)

	catcherTick(t, cart, 3)
	if dump {
		carttest.GoldenFrame(t, "catcher-title", carttest.MatrixFrame(t, host.LastBlit(t)))
	}

	catcherPress(t, cart, carttest.KeyJoyA) // title -> playing

	// early enough that the first wave is still announcing itself: the only
	// frame showing the level name from assets/waves.arkadelevels
	catcherTick(t, cart, 12)
	if dump {
		carttest.GoldenFrame(t, "catcher-wave", carttest.MatrixFrame(t, host.LastBlit(t)))
	}

	catcherTick(t, cart, 78) // 90 in total: fruit in flight, nothing resolved yet
	if dump {
		carttest.GoldenFrame(t, "catcher-playing", carttest.MatrixFrame(t, host.LastBlit(t)))
	}
	return cart, host
}

// tickUntilLCD advances until the seat's LCD line 1 reads want, failing
// with what it actually said if it never does.
func tickUntilLCD(t *testing.T, cart *carttest.Cartridge, host *carttest.FakeHost, want string, budget int) {
	t.Helper()
	for i := 0; i < budget; i++ {
		catcherTick(t, cart, 1)
		host.Mu.Lock()
		got := carttest.LastLCDLine1(host, int(catcherSeat))
		host.Mu.Unlock()
		if got == want {
			return
		}
	}
	host.Mu.Lock()
	got := carttest.LastLCDLine1(host, int(catcherSeat))
	host.Mu.Unlock()
	t.Fatalf("LCD line 1 never reached %q in %d frames (it says %q)", want, budget, got)
}

// TestCatcherScoresACatchAndEndsOnThreeMisses is the correctness anchor: the
// scripted run must produce the outcome the script describes, which goldens
// alone do not prove.
func TestCatcherScoresACatchAndEndsOnThreeMisses(t *testing.T) {
	cart, host := catcherJourney(t, false)
	tickUntilLCD(t, cart, host, "GAME OVER", 3000)

	host.Mu.Lock()
	line2 := host.LCDTexts[len(host.LCDTexts)-1].Line2
	host.Mu.Unlock()
	if line2 != "SCORE 10" {
		t.Errorf("game-over LCD line 2 = %q, want %q — the scripted run catches exactly one fruit", line2, "SCORE 10")
	}

	carttest.GoldenFrame(t, "catcher-gameover", carttest.MatrixFrame(t, host.LastBlit(t)))

	// the bed stops and the game-over sting fires after the fatal miss
	if id := carttest.SoundLoadedID(host, "assets/gameover.wav"); !carttest.SoundPlayed(host, id) {
		t.Error("the game-over sting never played")
	}
}

func TestCatcherDeterminism(t *testing.T) {
	_, a := catcherJourney(t, false)
	_, b := catcherJourney(t, false)
	carttest.AssertBlitsIdentical(t, a, b)
}

// TestCatcherGoldenFrames records the frames, separate from the assertions
// above so `UPDATE_GOLDENS=1 go test -run GoldenFrames` rewrites only
// snapshots.
func TestCatcherGoldenFrames(t *testing.T) {
	catcherJourney(t, true)
}

// TestCatcherDashLightIsEligibilityOnly pins the platform's lighting rule
// on the one button this game adds: blue is lit exactly while a dash
// would do something, and dark the moment it would not.
func TestCatcherDashLightIsEligibilityOnly(t *testing.T) {
	cart, host := catcherJourney(t, false)

	const maskJoyD = "joyd"
	if !lastLightsFor(host, int(catcherSeat)).Lights[maskJoyD] {
		t.Fatal("blue is dark at the start of a run, when a dash is available")
	}

	catcherPress(t, cart, carttest.KeyJoyD)
	catcherTick(t, cart, 2)
	if lastLightsFor(host, int(catcherSeat)).Lights[maskJoyD] {
		t.Error("blue stayed lit through the dash and its cooldown — an unusable lit button is a lie")
	}
	if id := carttest.SoundLoadedID(host, "assets/dash.wav"); !carttest.SoundPlayed(host, id) {
		t.Error("the dash made no sound")
	}

	// 60 frames is 2.0s: past the dash's 0.2s plus 1.4s cooldown, short of the
	// scripted run's third miss
	catcherTick(t, cart, 60)
	if !lastLightsFor(host, int(catcherSeat)).Lights[maskJoyD] {
		t.Error("blue never came back after the cooldown")
	}
}

// TestCatcherRejectsAnUnavailableDash: pressing a dark button rings the
// cabinet's "that does nothing" bell rather than being silently dropped.
func TestCatcherRejectsAnUnavailableDash(t *testing.T) {
	cart, host := catcherJourney(t, false)
	catcherPress(t, cart, carttest.KeyJoyD)
	catcherTick(t, cart, 2)
	before := host.Rejects
	catcherPress(t, cart, carttest.KeyJoyD) // still cooling down
	if host.Rejects != before+1 {
		t.Errorf("host_reject_key calls = %d, want %d — a press with no meaning should say so", host.Rejects, before+1)
	}
}

func lastLightsFor(host *carttest.FakeHost, seat int) carttest.ButtonLightsCall {
	host.Mu.Lock()
	defer host.Mu.Unlock()
	for i := len(host.ButtonLights) - 1; i >= 0; i-- {
		if host.ButtonLights[i].Player == seat {
			return host.ButtonLights[i]
		}
	}
	return carttest.ButtonLightsCall{}
}

// TestCatcherAttractIsSilentAndTouchesNoHardware: attract runs a dedicated
// instance (cart_init, cart_attract, cart_teardown) with audio and
// peripherals live, so silence must be a property of the code.
func TestCatcherAttractIsSilentAndTouchesNoHardware(t *testing.T) {
	host := newCatcherHost(t)
	ctx := context.Background()
	cart, err := carttest.NewCartridge(ctx, loadCatcherWasm(t), host, catcherOptions(t))
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(ctx) })
	if err := cart.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	lcdAfterInit := len(host.LCDTexts)
	lightsAfterInit := len(host.ButtonLights)

	for i := 0; i < 1800; i++ { // 60 seconds of screensaver
		if err := cart.Attract(ctx, catcherTickMicros); err != nil {
			t.Fatalf("Attract tick %d: %v", i, err)
		}
	}

	if n := host.PlayCount(); n != 0 {
		t.Errorf("attract played %d sounds, want 0", n)
	}
	if len(host.LCDTexts) != lcdAfterInit {
		t.Errorf("attract wrote %d LCD lines, want 0", len(host.LCDTexts)-lcdAfterInit)
	}
	if len(host.ButtonLights) != lightsAfterInit {
		t.Errorf("attract set button lights %d times, want 0", len(host.ButtonLights)-lightsAfterInit)
	}
	if !carttest.ExportsAttract(ctx, loadCatcherWasm(t)) {
		t.Error("catcher.wasm does not export cart_attract — the //export shim in main.go is the other half of implementing Attract")
	}
}

// catcherReachNameEntry parks a fresh cartridge on a qualifying name entry;
// NameEntryB calls it repeatedly, so it builds a new instance each time.
func catcherReachNameEntry(t *testing.T) (*carttest.Cartridge, *carttest.FakeHost) {
	t.Helper()
	cart, host := catcherJourney(t, false)
	tickUntilLCD(t, cart, host, "TOP 10!", 3000)
	return cart, host
}

// TestCatcherNameEntryBareBKeepsTheScore is the platform-wide contract: a
// fumbled B over a placement that is not in the table yet must open the
// hold confirm, not throw the run away.
func TestCatcherNameEntryBareBKeepsTheScore(t *testing.T) {
	carttest.NameEntryB{
		Reach:    catcherReachNameEntry,
		Tick:     catcherTick,
		Player:   catcherPlayer,
		Physical: catcherSeat,
	}.Run(t)
}

// TestCatcherNameEntryConfirmCancelsAndSubmits: any non-B press closes the
// overlay and the entry underneath stays live and submittable. The golden
// frame is the overlay over the entry, not over a frozen playfield.
func TestCatcherNameEntryConfirmCancelsAndSubmits(t *testing.T) {
	cart, host := catcherReachNameEntry(t)

	catcherPress(t, cart, carttest.KeyJoyB) // opens the confirm
	catcherTick(t, cart, 2)
	if host.Exits != 0 {
		t.Fatalf("a single B exited the cartridge (%d times)", host.Exits)
	}
	carttest.GoldenFrame(t, "catcher-nameentry-confirm", carttest.MatrixFrame(t, host.LastBlit(t)))

	// A cancels back to typing, and the entry still submits.
	for i := 0; i < 12 && host.ScoreCount() == 0; i++ {
		catcherPress(t, cart, carttest.KeyJoyA)
		catcherTick(t, cart, 2)
	}
	if host.ScoreCount() != 1 {
		t.Fatalf("score table has %d rows after cancelling the overlay and finishing the entry, want 1",
			host.ScoreCount())
	}
	if score, _, _ := host.ScoreRead(1); score != 10 {
		t.Errorf("submitted score = %d, want 10", score)
	}
	if host.Exits != 0 {
		t.Errorf("cartridge exited %d times on the resume-and-submit path, want 0", host.Exits)
	}
}

// TestCatcherStorageKeysWouldBeAccepted: keys must match [A-Za-z0-9_.-]+,
// and a slash is rejected host-side while fake-host tests keep passing.
func TestCatcherStorageKeysWouldBeAccepted(t *testing.T) {
	_, host := catcherJourney(t, false)
	host.Mu.Lock()
	defer host.Mu.Unlock()
	for key := range host.Storage {
		if !carttest.StorageKeyOK(key) {
			t.Errorf("storage key %q would be rejected on real hardware", key)
		}
	}
}

// TestCatcherWasmMatchesSource catches a stale binary, which the goldens
// cannot: they were recorded from it. carttest.RequireWasmMatchesSource
// holds the policy.
func TestCatcherWasmMatchesSource(t *testing.T) {
	carttest.RequireWasmMatchesSource(t, ".", "catcher.wasm", "make -C sdk/tutorial build")
}

// TestCatcherManifestListsALampCue: exactly one sequence is listed and it is
// in the cartridge directory. assets.lights is host_lights_play's namespace,
// so an empty list would make the other lamp assertions vacuous.
func TestCatcherManifestListsALampCue(t *testing.T) {
	lights := manifestLights(t)
	if len(lights) != 1 {
		t.Fatalf("manifest.json assets.lights = %v, want exactly one sequence", lights)
	}
	if _, err := os.Stat(lights[0]); err != nil {
		t.Fatalf("the listed sequence is not in the cartridge: %v", err)
	}
}

// TestCatcherGameOverPlaysItsLampCue: the cue flashes the panel twice on this
// seat only, walked by the host on the engine clock, and ends dark.
func TestCatcherGameOverPlaysItsLampCue(t *testing.T) {
	cart, host := catcherJourney(t, false)
	tickUntilLCD(t, cart, host, "GAME OVER", 3000)

	// frame 0 lands on the play call, inside the tick that ended the run
	if got := lampMaskString(lastLightsFor(host, int(catcherSeat))); got != "111111" {
		t.Fatalf("the panel is %q as the run ends, want it flashed on by the cue's first frame", got)
	}
	mark := len(host.ButtonLights)

	// 400 ms of the cue at 30 fps: dark, lit, dark after the first frame
	catcherTick(t, cart, 20)
	var seen []string
	host.Mu.Lock()
	for _, c := range host.ButtonLights[mark:] {
		if c.Player != int(catcherSeat) {
			t.Errorf("the cue wrote seat %d; it was played for seat %d only", c.Player, catcherSeat)
		}
		if !c.Only {
			t.Error("a cue frame must be a complete lamp state, not a merge")
		}
		seen = append(seen, lampMaskString(c))
	}
	host.Mu.Unlock()
	want := []string{"000000", "111111", "000000"}
	if !slicesEqual(seen, want) {
		t.Fatalf("the cue's remaining frames were %v, want %v", seen, want)
	}

	// `once` rests on its last frame
	catcherTick(t, cart, 30)
	if got := lampMaskString(lastLightsFor(host, int(catcherSeat))); got != "000000" {
		t.Errorf("the panel is %q well after the cue, want it dark", got)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// lampMaskString renders a recorded lamp call as six characters in
// host_set_button_lights' bit order.
func lampMaskString(c carttest.ButtonLightsCall) string {
	var out [6]byte
	for i, name := range [6]string{"joya", "joyb", "joyc", "joyd", "joyaux1", "joyaux2"} {
		out[i] = '0'
		if c.Lights[name] {
			out[i] = '1'
		}
	}
	return string(out[:])
}
