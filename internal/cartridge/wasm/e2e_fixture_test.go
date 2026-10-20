// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	_ "embed"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

//go:embed testdata/e2e.wasm
var e2eWasm []byte

func newE2ECartridge(t *testing.T, host wasm.Host) *wasm.Cartridge {
	t.Helper()
	cart, err := wasm.NewCartridge(context.Background(), e2eWasm, host, wasm.Options{Width: 4, Height: 4})
	if err != nil {
		t.Fatalf("NewCartridge(e2e.wasm): %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return cart
}

func TestFixtureAttract(t *testing.T) {
	ctx := context.Background()
	if !wasm.ExportsAttract(ctx, e2eWasm) {
		t.Fatal("ExportsAttract(e2e.wasm) = false, want true")
	}
	if wasm.ExportsAttract(ctx, goodWasm) {
		t.Fatal("ExportsAttract(good.wasm) = true, want false (good.wat exports no cart_attract)")
	}

	host := carttest.NewFakeHost()
	cart := newE2ECartridge(t, host)
	if !cart.SupportsAttract() {
		t.Fatal("SupportsAttract() = false for a cart_attract-exporting module")
	}

	for i := 0; i < 5; i++ {
		if err := cart.Attract(ctx, 33_333); err != nil {
			t.Fatalf("Attract tick %d: %v", i, err)
		}
	}
	if len(host.Blits) != 5 {
		t.Fatalf("attract blits = %d, want 5 (one per tick)", len(host.Blits))
	}
	frame := host.LastBlit(t)
	if len(frame) != 16 {
		t.Fatalf("attract frame has %d pixels, want 16 (4x4)", len(frame))
	}
	var transparent, opaque int
	for _, p := range frame {
		switch p.A {
		case 0:
			transparent++
		case 0xFF:
			opaque++
		}
	}
	if opaque != 4 || transparent != 12 {
		t.Fatalf("attract overlay = %d opaque / %d transparent pixels, want 4/12", opaque, transparent)
	}
	if len(host.Sounds) != 0 {
		t.Fatalf("attract made %d sound calls, want 0", len(host.Sounds))
	}
	if len(host.Rects) != 5 {
		t.Fatalf("attract fill_rect calls = %d, want 5", len(host.Rects))
	}

	plain := newGoodCartridge(t, carttest.NewFakeHost())
	if plain.SupportsAttract() {
		t.Fatal("good.wasm SupportsAttract() = true, want false")
	}
}

func TestFixtureAttractDeterminism(t *testing.T) {
	run := func() *carttest.FakeHost {
		host := carttest.NewFakeHost()
		cart := newE2ECartridge(t, host)
		for i := 0; i < 5; i++ {
			if err := cart.Attract(context.Background(), 33_333); err != nil {
				t.Fatalf("Attract: %v", err)
			}
		}
		return host
	}
	carttest.AssertBlitsIdentical(t, run(), run())
}

// The first Escape is swallowed as a confirm dialog; the second reaches host_exit.
func TestFixtureQuitKeyTwoPressContract(t *testing.T) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	cart := newE2ECartridge(t, host)

	press := []wasm.InputRecord{{Kind: wasm.RecordKindKeypress, Key: carttest.KeyEscape, Player: 1}}
	if err := cart.DispatchInput(ctx, press); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}
	if host.Exits != 0 {
		t.Fatalf("host_exit calls after ONE Escape = %d, want 0 (first press is the confirm step)", host.Exits)
	}
	if err := cart.DispatchInput(ctx, press); err != nil {
		t.Fatalf("DispatchInput: %v", err)
	}
	if host.Exits != 1 {
		t.Fatalf("host_exit calls after TWO Escapes = %d, want 1", host.Exits)
	}
}

func TestFixtureRenderVisible(t *testing.T) {
	host := carttest.NewFakeHost()
	cart := newE2ECartridge(t, host)
	if err := cart.Render(context.Background()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(host.Rects) != 1 || host.Rects[0].C.A != 0xFF {
		t.Fatalf("render rects = %+v, want one opaque fill", host.Rects)
	}
	frame := host.LastBlit(t)
	if frame[0].A != 0xFF {
		t.Fatalf("render blit pixel 0 = %+v, want opaque white", frame[0])
	}
}
