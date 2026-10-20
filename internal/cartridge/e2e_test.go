// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"strings"

	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"

	// testkit registers no apps; quit needs a shell to land on.
	_ "github.com/stealthmonkey99/arkade/apps/shell"
)

// e2eWasmPath is a fixture with a visible render, cart_attract and a
// two-press host_exit quit, so these tests depend on no real game.
const e2eWasmPath = "wasm/testdata/e2e.wasm"

// hasVisibleContent reports any lit pixel. Alpha alone proves nothing: a
// painted frame is composed over opaque black.
func hasVisibleContent(frame display.Matrix) bool {
	for _, row := range frame {
		for _, p := range row {
			if p.A != 0 && p.R|p.G|p.B != 0 {
				return true
			}
		}
	}
	return false
}

func TestAttractRendererDrivesCartridge(t *testing.T) {
	wasmBytes, err := os.ReadFile(e2eWasmPath)
	if err != nil {
		t.Fatalf("read e2e fixture cartridge: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "e2e.wasm"), wasmBytes, 0o644); err != nil {
		t.Fatalf("seed scan dir: %v", err)
	}
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	t.Cleanup(func() { engine.Unregister("cart/e2e") })
	reg.Scan()

	var m engine.Manifest
	for _, a := range engine.Apps() {
		if a.Slug == "cart/e2e" {
			m = a
		}
	}
	if m.NewAttract == nil {
		t.Fatal("e2e fixture manifest has no NewAttract (cart_attract not wired)")
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	attractCtx := &engine.AppContext{
		Manifest: engine.Manifest{Slug: "attract", Name: "Attract"},
		RT:       rig.RT,
	}

	r, ok := m.NewAttract(attractCtx)
	if !ok {
		t.Fatal("NewAttract returned ok=false for a cart_attract cartridge")
	}

	for i := 0; i < 20; i++ {
		r.Attract(33333)
	}
	if !hasVisibleContent(rig.RT.Display.Compose(clock.FrameTime{})) {
		t.Fatal("attract renderer drew nothing into the runtime display")
	}

	r.Close()
}

func TestEndToEndScanRegisterLaunchCartridge(t *testing.T) {
	wasmBytes, err := os.ReadFile(e2eWasmPath)
	if err != nil {
		t.Fatalf("read e2e fixture cartridge (regenerate via go:generate in wasm/generate.go): %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "e2e.wasm"), wasmBytes, 0o644); err != nil {
		t.Fatalf("seed scan dir: %v", err)
	}

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	t.Cleanup(func() { engine.Unregister("cart/e2e") })

	res := reg.Scan()
	if len(res.Added) != 1 || res.Added[0] != "cart/e2e" {
		t.Fatalf("Scan: Added = %v, want [cart/e2e]", res.Added)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "cart/e2e"})

	// cart_render runs once at load, before any cart_update.
	if !hasVisibleContent(rig.Frame()) {
		t.Fatal("init render: expected visible content, got a blank frame")
	}
	if got := rig.RT.CurrentSlug(); got != "cart/e2e" {
		t.Fatalf("CurrentSlug() = %q, want cart/e2e", got)
	}

	rig.Press(input.KeyJoyA)
	rig.Tick(30)
	if !hasVisibleContent(rig.Frame()) {
		t.Fatal("mid-game: expected visible content, got a blank frame")
	}
	if got := rig.RT.CurrentSlug(); got != "cart/e2e" {
		t.Fatalf("CurrentSlug() after ticking = %q, want cart/e2e (should not have crashed/unloaded)", got)
	}

	// Importing host_exit gives the cartridge the quit keys; it swallows
	// the first press as a confirm-exit dialog.
	rig.Press(input.KeyEscape)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "cart/e2e" {
		t.Fatalf("CurrentSlug() after one quit press = %q, want cart/e2e (the cartridge owns quit keys: one press never unloads)", got)
	}

	// Two ticks: the unload host_exit queues takes effect on the next Tick.
	rig.Press(input.KeyEscape)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("CurrentSlug() after confirming quit = %q, want shell", got)
	}
}

func TestEndToEndBadCartridgeBouncesToShell(t *testing.T) {
	wasmBytes, err := os.ReadFile("wasm/testdata/abimismatch.wasm")
	if err != nil {
		t.Fatalf("read abimismatch.wasm: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "badcart.wasm"), wasmBytes, 0o644); err != nil {
		t.Fatalf("seed scan dir: %v", err)
	}

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	t.Cleanup(func() { engine.Unregister("cart/badcart") })
	if res := reg.Scan(); len(res.Added) != 1 {
		t.Fatalf("Scan: Added = %v, want one registration", res.Added)
	}

	rig := testkit.NewRig(t, testkit.Options{StartApp: "cart/badcart"})

	rig.Tick(1) // processes the unload queued during Init
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("CurrentSlug() after a bad cartridge's forced exit = %q, want shell", got)
	}
}

func TestATrappedCartridgeGoesDarkUntilQuit(t *testing.T) {
	wasmBytes, err := os.ReadFile("wasm/testdata/trap.wasm")
	if err != nil {
		t.Fatalf("read trap.wasm: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "trap.wasm"), wasmBytes, 0o644); err != nil {
		t.Fatalf("seed scan dir: %v", err)
	}
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	t.Cleanup(func() { engine.Unregister("cart/trap") })
	if res := reg.Scan(); len(res.Added) != 1 {
		t.Fatalf("Scan: Added = %v, want one registration", res.Added)
	}

	var logs []string
	rig := testkit.NewRig(t, testkit.Options{
		StartApp: "cart/trap",
		Logger:   func(level, msg string) { logs = append(logs, level+": "+msg) },
	})
	if !hasVisibleContent(rig.Frame()) {
		t.Fatal("init render: expected the fixture's square")
	}

	rig.Tick(10)
	faults := 0
	for _, l := range logs {
		if strings.Contains(l, "cartridge update:") {
			faults++
		}
	}
	if faults != 1 {
		t.Errorf("the trap was reported %d times over 10 ticks, want once; logs = %v", faults, logs)
	}
	if hasVisibleContent(rig.Frame()) {
		t.Errorf("a trapped cartridge still shows its last frame, want dark:\n%s", testkit.ASCII(rig.Frame()))
	}
	if got := rig.RT.CurrentSlug(); got != "cart/trap" {
		t.Fatalf("CurrentSlug() after the trap = %q, want cart/trap until the player quits", got)
	}

	// The fixture imports host_exit, so the quit keys are its own; trapped,
	// it can no longer honour them and the host must.
	rig.Press(input.KeyEscape)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("CurrentSlug() after quitting a trapped cartridge = %q, want shell", got)
	}
	for _, l := range logs {
		if strings.Contains(l, "teardown") {
			t.Errorf("unloading a trapped cartridge re-entered it: %s", l)
		}
	}
}

type seedApp struct{ ctx *engine.AppContext }

func (a *seedApp) Init() error                                 { return nil }
func (a *seedApp) HandleInput(clock.FrameTime)                 {}
func (a *seedApp) Update(clock.FrameTime)                      {}
func (a *seedApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *seedApp) Teardown() error                             { return nil }

// The shell reads engine.Apps() once per Init, so a hot-mounted cartridge
// appears only after the shell reloads. A known limitation.
func TestShellMenuHotMountVisibility(t *testing.T) {
	const seedSlug = "test/hotmount-seed"
	engine.Register(engine.Manifest{
		Slug: seedSlug, Name: "Seed", Type: engine.TypeGame,
		New: func(ctx *engine.AppContext) engine.App { return &seedApp{ctx: ctx} },
	})
	t.Cleanup(func() { engine.Unregister(seedSlug) })

	var logs bytes.Buffer
	rig := testkit.NewRig(t, testkit.Options{
		StartApp: "shell",
		Logger:   func(level, msg string) { fmt.Fprintf(&logs, "[%s] %s\n", level, msg) },
	})

	if !bytes.Contains(logs.Bytes(), []byte("1 apps found in library.")) {
		t.Fatalf("baseline shell log missing expected app count; got:\n%s", logs.String())
	}

	dir := t.TempDir()
	wasmBytes, err := os.ReadFile(e2eWasmPath)
	if err != nil {
		t.Fatalf("read e2e fixture cartridge: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "e2e.wasm"), wasmBytes, 0o644); err != nil {
		t.Fatalf("seed scan dir: %v", err)
	}
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	t.Cleanup(func() { engine.Unregister("cart/e2e") })
	if res := reg.Scan(); len(res.Added) != 1 {
		t.Fatalf("Scan: Added = %v, want one registration", res.Added)
	}

	apps := engine.Apps()
	found := false
	for _, m := range apps {
		if m.Slug == "cart/e2e" {
			found = true
		}
	}
	if !found {
		t.Fatalf("engine.Apps() after Scan = %v, missing cart/e2e", apps)
	}

	// The running shell's shelf is unchanged. Its frames animate, so the
	// witness is behavioural: Down from the seed app clamps unless the shelf
	// grew.
	rig.Tick(1)
	hw := rig.RT.RawHardware().(*hardware.Virtual)
	if !strings.Contains(strings.Join(hw.StatusLines(), "\n"), "Seed") {
		t.Fatalf("expected the shell still focused on the seed app before testing the clamp; status:\n%s",
			strings.Join(hw.StatusLines(), "\n"))
	}
	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	if !strings.Contains(strings.Join(hw.StatusLines(), "\n"), "Seed") {
		t.Fatalf("the shell's shelf count changed after a hot-mount with no reload — expected Down to stay clamped on the seed app; status:\n%s",
			strings.Join(hw.StatusLines(), "\n"))
	}

	// Leaving and coming back re-runs the shell's Init.
	rig.Press(input.KeyEnter)
	rig.Tick(2) // one to dispatch the select, one to load the picked app
	if got := rig.RT.CurrentSlug(); got != seedSlug {
		t.Fatalf("CurrentSlug() after selecting the seed app = %q, want %q", got, seedSlug)
	}
	rig.Press(input.KeyEscape)
	rig.Tick(2) // one to dispatch the quit, one to actually load the shell
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("CurrentSlug() after quitting the seed app = %q, want shell", got)
	}
	if !bytes.Contains(logs.Bytes(), []byte("2 apps found in library.")) {
		t.Fatalf("reloaded shell log missing the updated app count (seed + cartridge); got:\n%s", logs.String())
	}
}
