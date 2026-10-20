// SPDX-License-Identifier: GPL-2.0-or-later

package wasm_test

import (
	"context"
	_ "embed"
	"errors"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
)

//go:embed testdata/trap.wasm
var trapWasm []byte

func TestAFaultedCartridgeIsNeverReentered(t *testing.T) {
	ctx := context.Background()
	host := carttest.NewFakeHost()
	cart, err := wasm.NewCartridge(ctx, trapWasm, host, wasm.Options{Width: 8, Height: 8})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(ctx) })
	if err := cart.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := cart.Render(ctx); err != nil {
		t.Fatalf("Render before the trap: %v", err)
	}
	rects := len(host.Rects)

	err = cart.Update(ctx, 16_000)
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("first Update = %v, want the trap", err)
	}
	fault := cart.Fault()
	if fault == nil || fault.Export != "cart_update" {
		t.Fatalf("Fault() = %v, want cart_update's trap", fault)
	}

	if err := cart.Update(ctx, 16_000); !errors.Is(err, wasm.ErrFaulted) {
		t.Errorf("second Update = %v, want ErrFaulted without re-entering the guest", err)
	}
	if err := cart.Render(ctx); !errors.Is(err, wasm.ErrFaulted) {
		t.Errorf("Render after the trap = %v, want ErrFaulted", err)
	}
	if got := len(host.Rects); got != rects {
		t.Errorf("cart_render ran after the trap: %d fill_rect calls, want %d", got, rects)
	}
	if err := cart.DispatchInput(ctx, []wasm.InputRecord{{Kind: wasm.RecordKindKeypress, Player: 1}}); !errors.Is(err, wasm.ErrFaulted) {
		t.Errorf("DispatchInput after the trap = %v, want ErrFaulted", err)
	}
	if err := cart.Teardown(ctx); err != nil {
		t.Errorf("Teardown entered the faulted guest: %v", err)
	}
	if cart.Fault() != fault {
		t.Error("a later call replaced the first fault")
	}
}
