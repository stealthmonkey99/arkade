// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

// TestCartlibQuitsForAGameThatDoesNotOwnQuit pins cartlib's default: the
// template declares host_exit (every cartlib cartridge does, which hands it
// the quit keys on the cabinet) yet has no quit code of its own, so a quit
// press must still reach host_exit — and only a press, not its keydown.
func TestCartlibQuitsForAGameThatDoesNotOwnQuit(t *testing.T) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	cart, err := wasm.NewCartridge(ctx, loadTemplateWasm(t), host, wasm.Options{})
	if err != nil {
		t.Fatalf("NewCartridge(template.wasm): %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(ctx) })
	if !cart.ImportsExit() {
		t.Fatal("template.wasm does not declare host_exit; the default under test does not apply")
	}
	if err := cart.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}

	const keyJoyB = 13
	if err := cart.DispatchInput(ctx, []wasm.InputRecord{
		{Kind: wasm.RecordKindKeydown, Key: keyJoyB, Player: 1, Source: wasm.RecordSourceController},
		{Kind: wasm.RecordKindKeyup, Key: keyJoyB, Player: 1, Source: wasm.RecordSourceController},
	}); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}
	if host.Exits != 0 {
		t.Fatalf("host_exit calls after a keydown/keyup pair = %d, want 0", host.Exits)
	}

	if err := cart.DispatchInput(ctx, []wasm.InputRecord{
		{Kind: wasm.RecordKindKeypress, Key: keyJoyB, Player: 1, Source: wasm.RecordSourceController},
	}); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}
	if host.Exits != 1 {
		t.Fatalf("host_exit calls after a B press = %d, want 1", host.Exits)
	}
}
