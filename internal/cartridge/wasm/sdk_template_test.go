// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

const templateWasmPath = "../../../sdk/template/template.wasm"

func loadTemplateWasm(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(templateWasmPath)
	if err != nil {
		t.Fatalf("read checked-in sdk/template/template.wasm (run `make sdk-template` to rebuild it): %v", err)
	}
	return data
}

func TestSDKTemplateCartridge(t *testing.T) {
	host := carttest.NewFakeHost()
	host.RandScript = []uint64{1, 2, 3}

	cart, err := wasm.NewCartridge(context.Background(), loadTemplateWasm(t), host, wasm.Options{})
	if err != nil {
		t.Fatalf("NewCartridge(template.wasm): %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })

	version, err := cart.ABIVersion(context.Background())
	if err != nil {
		t.Fatalf("ABIVersion: %v", err)
	}
	if version < wasm.MinSupportedABIVersion || version > wasm.SupportedABIVersion {
		t.Fatalf("ABIVersion = %d, want %d..%d", version, wasm.MinSupportedABIVersion, wasm.SupportedABIVersion)
	}

	manifestRaw, err := cart.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	var manifest struct {
		ABI     int    `json:"abi"`
		Slug    string `json:"slug"`
		Name    string `json:"name"`
		Players []int  `json:"players"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("parsing cart_manifest JSON %q: %v", manifestRaw, err)
	}
	if manifest.ABI != 1 || manifest.Slug != "template" || manifest.Name == "" {
		t.Fatalf("cart_manifest = %+v, want abi=1 slug=template name=<non-empty>", manifest)
	}

	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Game.Render must work from Init's state alone.
	if err := cart.Render(context.Background()); err != nil {
		t.Fatalf("Render before first Update: %v", err)
	}
	if len(host.Blits) != 1 {
		t.Fatalf("blits after the init render = %d, want 1", len(host.Blits))
	}

	// JoyA keypress, ordinal 12, bumps the counter.
	if err := cart.DispatchInput(context.Background(), []wasm.InputRecord{
		{Kind: wasm.RecordKindKeypress, Key: 12, Player: 1},
	}); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}

	// Enough 30 FPS ticks for the demo's 100 ms step to move the block.
	for i := 0; i < 10; i++ {
		if err := cart.Update(context.Background(), 33333); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := cart.Render(context.Background()); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}

	last := host.Blits[len(host.Blits)-1]
	nonBlack := false
	for _, p := range last {
		if p.R != 0 || p.G != 0 || p.B != 0 {
			nonBlack = true
			break
		}
	}
	if !nonBlack {
		t.Fatal("final rendered frame is entirely black - expected the bouncing block and/or counter text to be visible")
	}

	raw, ok := host.Storage["counter"]
	if !ok || len(raw) != 4 {
		t.Fatalf(`storage["counter"] = %v, ok=%v; want a stored 4-byte counter`, raw, ok)
	}
	if got := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24; got != 1 {
		t.Errorf(`storage["counter"] decodes to %d, want 1`, got)
	}

	if err := cart.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	for _, l := range host.Logs {
		if l.Level == wasm.LogWarn && strings.HasPrefix(l.Msg, "[cartridge sandbox]") {
			t.Errorf("unexpected sandbox warning: %s", l.Msg)
		}
	}
}

// TestSDKTemplateWasmMatchesSource catches a stale checked-in template.wasm,
// which the test above would keep passing.
func TestSDKTemplateWasmMatchesSource(t *testing.T) {
	carttest.RequireWasmMatchesSource(t, "../../../sdk/template", "template.wasm", "make sdk-template")
}
