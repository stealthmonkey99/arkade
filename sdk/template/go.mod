// This is a separate Go module from the main arkade repo (and from
// sdk/cartlib) because TinyGo's
// go:wasmimport/`//export`-driven cartlib package needs to be built for
// wasm without ever being pulled into `go build ./...`/`go test ./...` at
// the arkade repo root — see sdk/cartlib's package doc, "Build-tag split".
//
// The replace directive below points at ../cartlib, a sibling directory
// inside this same arkade checkout.
module arkade-cart-template

go 1.23

require github.com/stealthmonkey99/arkade/sdk/cartlib v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/cartlib => ../cartlib

// The template draws through the shared UI toolkit directly, and cartlib
// also depends on it. Replace directives are not transitive, so this module
// resolves ui to its local copy.
require github.com/stealthmonkey99/arkade/sdk/ui v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/ui => ../ui
