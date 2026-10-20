// Catcher is the finished game from the SDK's follow-along tutorial
// (sdk/docs/tutorial.md and its two sequels). It is a separate Go module
// for exactly the reason sdk/template is: TinyGo's
// go:wasmimport/`//export`-driven cartlib package must be buildable for
// wasm without ever being pulled into the arkade repo root's
// `go build ./...` / `go test ./...` — see sdk/cartlib's package doc,
// "Build-tag split".
//
// It is a teaching artifact: the tutorial's reader builds their own copy,
// and this directory is the finished one to diff against. The Makefile's
// sdk-tutorial / sdk-tutorial-test targets build and test it.
//
// The replace directives below point at ../cartlib and ../ui, siblings
// inside this same checkout.
module arkade-cart-tutorial

go 1.23.4

require github.com/stealthmonkey99/arkade/sdk/cartlib v0.1.0

require (
	github.com/gopxl/beep/v2 v2.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tetratelabs/wazero v1.10.1 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/term v0.28.0 // indirect
)

replace github.com/stealthmonkey99/arkade/sdk/cartlib => ../cartlib

// Catcher draws through the shared UI toolkit directly, and cartlib also
// depends on it. Replace directives are not transitive, so this module
// resolves ui to its local copy too.
require github.com/stealthmonkey99/arkade/sdk/ui v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/ui => ../ui

// Catcher's screens are an sdk/fsm machine. Replace
// directives are not transitive, so this module declares its own pair.
require github.com/stealthmonkey99/arkade/sdk/fsm v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/fsm => ../fsm

// Catcher's authored waves are an sdk/levels pack. One require covers both
// halves of that pipeline: the cartridge compiles the runtime half (which
// imports nothing at all), and tools/genlevels runs the host-side
// levels/packfile subpackage of the same module.
require github.com/stealthmonkey99/arkade/sdk/levels v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/levels => ../levels

// Host-side code imports the arkade repo itself: the golden test (via the
// carttest harness) and tools/gen-sfx (via tools/sfxlib and
// tools/musiclib). The cartridge package proper — main.go,
// game.go, catcher.go, render.go, peripherals.go, attract.go — never
// does, so none of it enters the TinyGo build's import graph.
require github.com/stealthmonkey99/arkade v0.1.0

replace github.com/stealthmonkey99/arkade => ../..
