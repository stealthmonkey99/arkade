# Årkade cartridge authoring SDK

A Go SDK for writing Årkade cartridges: `.wasm` binaries that run
unmodified on both the physical cabinet (`internal/cartridge/wasm`,
wazero) and the browser player (the arkade.build package's `host/` —
not part of this repo).

**The documentation lives in [`sdk/docs/`](docs/README.md).** Start
with [Getting started](docs/getting-started.md) (template → build →
play → pack), keep [Gotchas](docs/gotchas.md) open beside it, then read
the [cartlib](docs/cartlib.md) and [ui](docs/ui.md) references,
[Manifests & packaging](docs/packaging.md) and the
[ABI contract](docs/abi.md). Every Go example in those pages is
compile-checked by `go test ./sdk/docs/`, and the same set is rendered on
arkade.build at `/dev/docs`. The API reference is generated from the Go
doc comments into [`docs/api/`](docs/api/cartlib.md), one page per
package, rendered on the site with the other docs; `go doc` and
pkg.go.dev show the same comments. Regenerate with `make sdk-docs`;
`sdkdoc -check` and `make test` fail when a page is stale. (`docs/CARTRIDGE.md` at the repo root is the
ABI's design record — the rationale behind each rule.)

## Layout

```
sdk/
  cartlib/     the SDK library (its own Go module) — the Game interface and
               its Setup, framebuffer, font, input decoding, host_* wrappers,
               the cart_* export harness
    hosttest/  the in-process fake cabinet a cartridge's own `go test`
               drives its Game against
  ui/          host-free widget toolkit (its own Go module) shared by the
               cabinet AND cartridges — Canvas, fonts, widgets, compositor
    input/     the 8-byte input record, its key and seat types, and their
               decodes; cartlib re-exports all of it
  fsm/         the screen-state machine (its own Go module): which screen
               is up, one entry hook per screen, a modal push/pop stack.
               Opt-in; see fsm/README.md for when it pays
  levels/      the authored-level pack format (its own Go module): the
               runtime half compiles into cartridges, packfile/ is the
               host-side parser and generator
  template/    a complete minimal cartridge project to copy (its own Go
               module)
  tutorial/    Catcher, the tutorial's finished game (its own Go module)
  tools/
    manifestgen/ writes manifest_gen.go, the runtime manifest the wasm
               reports, from manifest.json; every build runs it first
    rcartpack/ packs a project directory into a .rcart bundle,
               enforcing the same caps the cabinet's loader applies
  docs/        the SDK/ABI documentation set (rendered at /dev/docs)
```

Real cartridges built with this SDK: `life` (the simplest —
`cartlib.Game` + a grid) and `snake` (menus, two players, scores, sound,
attract mode) live in the separate
[arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
repo, at `cartridges/<slug>`; a bare `cartridges/…` path anywhere in these
docs is relative to that repo, not this one.

Each module with more to say has its own README:
[fsm](fsm/README.md), [levels](levels/README.md),
[template](template/README.md) and [tutorial](tutorial/README.md).
[docs/AUTHORING.md](docs/AUTHORING.md) is the guide to writing the doc
pages themselves.

## Requirements

[TinyGo](https://tinygo.org) produces a cartridge's `.wasm`; plain
`go build`/`go test` on a cartridge never need it. The exact TinyGo and Go
versions are in the engine's [Requirements](../README.md#requirements).

## Quickstart

From the engine checkout, with a cartridge library beside it:

```sh
brew install tinygo-org/tools/tinygo   # macOS; other platforms: https://tinygo.org/getting-started/install/
mkdir -p ../my-cartridges
cp -r sdk/template ../my-cartridges/my-game
rm ../my-cartridges/my-game/template.wasm
```

Edit the copy's `go.mod` (see [Module resolution](#module-resolution)),
`manifest.json` and the Makefile's `ARKADE` line, then:

```sh
make -C ../my-cartridges/my-game build   # -> my-game.wasm
make build                               # the engine, once
./arkade --cartridges ../my-cartridges/my-game
```

Or drag `my-game.wasm` onto the player at [arkade.build](https://arkade.build).
[Getting started](docs/getting-started.md) walks the same path in full.

## Module resolution

`sdk/cartlib`, `sdk/ui`, `sdk/fsm`, `sdk/levels`, `sdk/template` and
`sdk/tutorial` are each their own Go module (see their `go.mod` files),
so a cartridge can depend on the SDK without depending on the engine and
its wazero runtime, and so the root `go test ./...` does not descend into
the guest-side code. The library modules are tagged together
(`sdk/cartlib/v0.1.0`, `sdk/ui/v0.1.0`, …).

- **A cartridge of your own** requires the tagged modules. Require `ui`
  even if you import only `cartlib`, which depends on it; add `fsm` and
  `levels` when the cartridge imports them:

  ```go-mod
  require (
  	github.com/stealthmonkey99/arkade/sdk/cartlib v0.1.0
  	github.com/stealthmonkey99/arkade/sdk/ui v0.1.0
  )
  ```

  The Makefile still runs four build tools from an engine checkout, the one
  its `ARKADE` line names: `sdk/tools/manifestgen`, `sdk/tools/rcartpack`,
  `tools/genlevelpack` and `tools/wasmfix`.
- **Inside this checkout**, or to build against unreleased SDK changes,
  `replace` each SDK module with a path to the checkout's copy:
  `sdk/template/go.mod` uses `../cartlib` and `../ui`, and a library
  beside the checkout uses `../../arkade/sdk/cartlib` and so on. Declare
  a `replace` for every SDK module the build uses, `ui` included:
  `replace` directives are not transitive.

Cartridge tests and tools also import two root-module packages,
[`carttest`](../carttest/) (the golden-test harness) and
[`assetfile`](../assetfile/) (the asset envelope), via
`require github.com/stealthmonkey99/arkade`.

## Testing

Cartridge logic is plain Go, so `make test` (`go test ./...`) runs without
TinyGo. Code that calls the host runs against
[`cartlib/hosttest`](cartlib/hosttest), an in-process fake cabinet that
drives a `Game` the way the cabinet does.
