// Catcher is the finished game from the SDK's follow-along tutorial
// (sdk/docs/tutorial.md and its sequels). It is a separate Go module for
// exactly the reason sdk/template is: TinyGo's go:wasmimport/`//export`-driven
// cartlib package must be buildable for wasm without ever being pulled into
// the arkade repo root's `go build ./...` / `go test ./...` — see
// sdk/cartlib's package doc, "Build-tag split".
//
// It is a teaching artifact: the tutorial's reader builds their own copy,
// and this directory is the finished one to diff against. The Makefile's
// sdk-tutorial / sdk-tutorial-test targets build and test it.
//
// It requires the published modules like any cartridge; inside the engine
// checkout, sdk/go.work resolves them to the local copies instead.
//
// The engine itself is required for host-side code only: the golden test
// (via carttest) and tools/gen-sfx (via tools/sfxlib and tools/musiclib).
// The cartridge package proper never imports it, so none of it enters the
// TinyGo build's import graph. One sdk/levels require covers both halves of
// that pipeline: the runtime package the cartridge compiles and the
// host-side levels/packfile that tools/genlevels runs.
module arkade-cart-tutorial

go 1.23.4

require (
	github.com/stealthmonkey99/arkade v0.1.0
	github.com/stealthmonkey99/arkade/sdk/cartlib v0.1.0
	github.com/stealthmonkey99/arkade/sdk/fsm v0.1.0
	github.com/stealthmonkey99/arkade/sdk/levels v0.1.0
	github.com/stealthmonkey99/arkade/sdk/ui v0.1.0
)

require (
	github.com/gopxl/beep/v2 v2.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/tetratelabs/wazero v1.10.1 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/term v0.28.0 // indirect
)
