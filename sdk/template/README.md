# Årkade cartridge template

A minimal, complete Årkade cartridge: a bouncing block, a "beep-free"
button-light chase, and a counter that survives a cartridge reload via
`host_storage_read`/`host_storage_write`. Read `main.go` top to bottom —
it's short and heavily commented as a tour of
[`cartlib`](../cartlib)'s [`Game`](../cartlib/game.go) interface.

See [`../docs/getting-started.md`](../docs/getting-started.md) (the
SDK's authoring walkthrough) for prerequisites, testing, and `.rcart`
packaging, and [`../docs/`](../docs/README.md) for the full reference
set.

## Use this as a starting point

A game lives in your own cartridge library, a folder beside the engine
checkout — not inside it. Any name; each subfolder is one cartridge:

```text
arkade/                 the engine checkout (this repo)
my-cartridges/          your library
my-cartridges/my-game/  this copy
```

```sh
mkdir -p ../my-cartridges
cp -r sdk/template ../my-cartridges/my-game   # from the engine checkout
cd ../my-cartridges/my-game
rm template.wasm                              # the template's own build
```

Then edit, in order:

1. `go.mod` — rename the module, then either require the tagged SDK
   modules or point both `replace` lines at the engine checkout
   (`../../arkade/sdk/cartlib` and `../../arkade/sdk/ui` from the layout
   above). [Module resolution](../README.md#module-resolution) has both
   forms.
2. `manifest.json` — your own `slug`/`title`/`version`. The Makefile
   reads `slug` from here to name its outputs (`<slug>.wasm`,
   `<slug>.rcart`), so it needs no edits of its own beyond the next step.
3. `Makefile` — one line: `ARKADE ?= ../../arkade`, the path to the
   engine checkout. Everything else is derived.
4. `main.go` — replace `demoGame`'s fields and methods with your own
   game. Keep `Manifest()` returning `manifestBytes`, which lives in
   `manifest_gen.go` — `make build` regenerates that file from
   `manifest.json`, so never edit it (see `cartlib`'s package doc,
   "Manifest and wasmfix") — and keep the `var _ = cartlib.Run(...)`
   registration at the package level, never inside `func main`.

## Build / test / package

```sh
make build   # genlevelpack + manifestgen + tinygo build + wasmfix -> <slug>.wasm
             # (the Makefile's comments say why the generators always run)
make test    # plain `go test ./...`, no TinyGo; sdk/cartlib/hosttest fakes the host
make rcart   # packages <slug>.wasm + manifest.json (+ assets/, if you
             # add one) into <slug>.rcart, rejecting anything the cabinet
             # loader would also reject (see sdk/tools/rcartpack)
```

Play it in the terminal from the engine checkout — `make build` there
once, then `./arkade --cartridges ../my-cartridges/my-game`.

`template.wasm` (built with TinyGo 0.42.0, `-opt=z`) is small enough that
this SDK checks a copy in at `template.wasm` for
`internal/cartridge/wasm/sdk_template_test.go` to exercise without
requiring TinyGo in every test environment — see that test file's doc
comment. Rebuilding via `make build` (or `make -C sdk/template build`
from the engine checkout) reproduces it byte-for-byte.
