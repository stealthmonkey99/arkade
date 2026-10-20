// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

import (
	"context"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

// attractSlotSlug is the layer owner for attract instances, so they never
// collide with the same game suspended under standby.
const attractSlotSlug = "attract-slot"

type attractRenderer struct {
	host *wasm.EngineHost
	cart *wasm.Cartridge
}

var _ engine.AttractRenderer = (*attractRenderer)(nil)

func (r *attractRenderer) Attract(dtMicros uint64) {
	_ = r.cart.Attract(context.Background(), dtMicros)
}

// Close runs cart_teardown and releases the wasm runtime.
func (r *attractRenderer) Close() {
	ctx := context.Background()
	_ = r.cart.Teardown(ctx)
	_ = r.cart.Close(ctx)
	r.host.Close()
}

// newAttractRenderer returns ok=false, having cleaned up, when the cartridge
// fails to instantiate, lacks cart_attract, or fails cart_init.
func newAttractRenderer(parent *engine.AppContext, slug string, wasmBytes []byte, width, height int, assets map[string][]byte, mount rcart.AssetRefs) (engine.AttractRenderer, bool) {
	ctx := context.Background()
	host := wasm.NewEngineHost(parent, wasm.Identity{CartSlug: slug, LayerOwner: attractSlotSlug}, wasm.ModeAttract, width, height, assets)

	// No HID profiles or sound models: an attract slot has no player.
	cart, err := wasm.NewCartridge(ctx, wasmBytes, host, wasm.Options{Width: width, Height: height, BundleGlyphs: mount.Glyphs, BundleLights: mount.Lights})
	if err != nil {
		host.Close()
		return nil, false
	}
	r := &attractRenderer{host: host, cart: cart}
	if !cart.SupportsAttract() || cart.Init(ctx) != nil {
		r.Close()
		return nil, false
	}
	return r, true
}
