# Getting started: your first cartridge

This walkthrough takes you from the SDK's starter template to a packed
`.rcart` bundle playing in your browser and in a terminal. Nothing here
needs a cabinet, a Raspberry Pi, or a USB stick.

## 0. Prerequisites

- **TinyGo 0.42.0 with Go 1.25–1.27** — TinyGo compiles the cartridge
  to wasm and refuses any other Go version. You only need it to produce
  a `.wasm`; plain `go build` / `go test` on your game logic never touch
  it (see [Test as you go](#5-test-as-you-go)).
- **Go 1.23.4+** for host-side tests and the engine itself (its
  `go.mod` floor); the Go you installed for TinyGo covers it. Full
  requirements: [the engine README](../../README.md#requirements).
- **An engine checkout** — `github.com/stealthmonkey99/arkade`, cloned
  beside your cartridge library (layout below). Your cartridge reaches
  into it for the SDK modules and the build tools.

```sh
brew install tinygo-org/tools/tinygo   # macOS
# other platforms: https://tinygo.org/getting-started/install/
```

## 1. Copy the template

A game lives in a **cartridge library** of your own: a folder beside the
engine checkout, not inside it. The public cartridge collection is its
own repository, and what you are building is a personal library — you
are not preparing a pull request, and you can publish later. Any name
will do; each subfolder is one cartridge:

```text
arkade/                 the engine checkout
my-cartridges/          your library — a "cartridge root"
my-cartridges/my-game/  this game
```

The starter at `sdk/template` in the engine checkout is a complete,
working cartridge: a bouncing block, a button-light chase, and a
counter that survives reloads. From the engine checkout:

```sh
mkdir -p ../my-cartridges
cp -r sdk/template ../my-cartridges/my-game
cd ../my-cartridges/my-game
rm template.wasm            # the template's own checked-in build
```

Then make it yours, in order.

**First, `go.mod`** — rename the module (say
`arkade-my-game-cartridge`) and resolve the SDK. Either require the
tagged modules, or build against your engine checkout by pointing the
two `replace` paths at it, as here;
[Module resolution](../README.md#module-resolution) covers both:

```go-mod file=go.mod
module arkade-my-game-cartridge

go 1.23

require github.com/stealthmonkey99/arkade/sdk/cartlib v0.1.0
require github.com/stealthmonkey99/arkade/sdk/ui v0.1.0

replace github.com/stealthmonkey99/arkade/sdk/cartlib => ../../arkade/sdk/cartlib
replace github.com/stealthmonkey99/arkade/sdk/ui => ../../arkade/sdk/ui
```

Both `replace` lines matter even if you only import `cartlib`: it
depends on `ui`, and `replace` directives are not transitive.

**Then `manifest.json`** — set your own `slug` (`"my-game"`), `title`,
`description`, and `version`. The slug must match your directory name
and, later, your `.rcart` filename. The full schema is in
[Manifests & packaging](packaging.md).

**Then the `Makefile`** — one line, `ARKADE ?= ../../arkade`: the path
to the engine checkout. Everything else in it is derived — it reads
`slug` out of `manifest.json` to name its outputs — so there is nothing
else to edit, now or as the game grows.

**Then `main.go`** — nothing about the manifest. `make build` generates
`manifest_gen.go` from `manifest.json` before every compile; that file
declares the `manifestJSON` constant and the `manifestBytes` slice your
`Manifest` method returns, so the running cartridge always reports what
the file says (`title` there is `name` here). Commit it with the rest of
the source. Preflight's `manifest` check fails a cartridge whose
generated file has fallen behind.

## 2. Look around `main.go`

The whole game is one short file — read it top to bottom, it is
commented as a tour. The skeleton every cartridge shares:

```go file=main.go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

type myGame struct {
	x, y int
}

func (g *myGame) Manifest() []byte { return manifestBytes }

func (g *myGame) Init(s *cartlib.Setup) {
	g.x, g.y = cartlib.FBWidth/2, cartlib.FBHeight/2
}

func (g *myGame) HandleInput(records []cartlib.InputRecord) {
	for _, r := range records {
		switch r.PressDir() { // once per push, stick or keyboard
		case cartlib.DirLeft:
			g.x--
		case cartlib.DirRight:
			g.x++
		}
	}
}

func (g *myGame) Update(dtMicros uint64) {}

func (g *myGame) Render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)
	fb.SetPixel(g.x, g.y, 255, 255, 255)
}

var game = &myGame{}
var _ = cartlib.Run(game)

func main() {} // required by TinyGo, never called
```

Three things to internalize right away:

- **Registration happens at the package level**, never inside `main` —
  the host never calls `main`. `var _ = cartlib.Run(game)` is the one
  supported shape (see [cartlib reference](cartlib.md#registration-and-the-reactor-model)).
- **`Manifest` returns `manifestBytes`** from the generated
  `manifest_gen.go` and nothing else — the build's `wasmfix` pass
  depends on `cart_manifest` resolving to compile-time constants (see
  [cartlib reference](cartlib.md#the-manifest-method)).
- **Quit is handled for you.** cartlib answers q, escape and the red B
  button with `cartlib.Exit()`. A game with its own quit flow (a "hold B
  to quit" overlay) implements `cartlib.QuitOwner` and calls `Exit`
  itself.

Those are the first of a short list of traps that compile cleanly and
fail later — no allocation after `Init`, time only from `dtMicros`,
randomness only from `RandomU64`, the calls that fail silently.
[Gotchas](gotchas.md) has the whole list, one line each; keep it open.

## 3. Build it

```sh
make build      # from my-cartridges/my-game — manifestgen + tinygo build + wasmfix → my-game.wasm
```

Under the hood this is three steps, and all three always run:

```sh
go -C ../../arkade run ./sdk/tools/manifestgen "$PWD"
tinygo build -target=wasi -scheduler=none -gc=leaking -opt=z -no-debug -panic=trap -o my-game.wasm .
go -C ../../arkade run ./tools/wasmfix -in "$PWD/my-game.wasm" -out "$PWD/my-game.wasm"
```

The first and last lines run the engine's tools from the engine's own module
(`go -C` changes into it first — your cartridge is a separate module and
cannot `go run` a path outside itself), which is why the Makefile hands
them absolute paths.

`wasmfix` is a post-build pass that rewrites `cart_manifest` into the
genuine two-result export the ABI specifies (TinyGo can't emit one
itself — [ABI reference](abi.md#cartridge-exports)). Never skip it: a
cartridge without the fix-up fails to load on either host.

## 4. Play it

**In the browser** (fastest feedback loop): open the hosted player at
[arkade.build](https://arkade.build) and drag `my-game.wasm` onto the
page. Keyboard arrows are the joystick, Enter/Space are A, Escape is B.
Every change is rebuild-and-drag — no engine rebuild, ever, and nothing
to install.

**In your terminal** (the console renderer — the same engine binary
the cabinet runs, drawing the LED matrix as colored text). From the
engine checkout:

```sh
make build                                    # the engine, once
./arkade --cartridges ../my-cartridges/my-game
```

The shell menu boots with your cartridge in it; arrows + Enter to
launch, `q` to quit. `--cartridges` takes directories to scan for
`.wasm`/`.rcart` files and accepts globs, so
`./arkade --cartridges '../my-cartridges/*'` boots the whole library.
A bare `.wasm` with a `manifest.json` beside it is mounted as a bundle,
`assets/` and all, so you can play without packing first.

**Headless**, for a quick look without a window or for a script:

```sh
./arkade --cartridges ../my-cartridges/my-game --app cart/my-game --headless --ticks 90 --dump-frame -
```

launches straight into the cartridge (its slug, prefixed `cart/`), runs
90 frames with no display and prints the last one as ASCII with its
palette.

## 5. Test as you go

`cartlib` compiles under the ordinary host Go toolchain too (see
`sdk/cartlib`'s package doc, "Build tags"), so your game is testable
with plain `go test`, no TinyGo, no wasm:

```sh
go test ./...        # from my-cartridges/my-game
```

Rules, physics and board math that never call a `cartlib.*` host
wrapper test as they are — `sdk/tutorial/catcher_test.go` in the engine
checkout is the pattern. Code that does reach the host runs against the
in-process fake in `sdk/cartlib/hosttest`: `h := hosttest.New(t)`
installs it for the test, `h.Init(game)` runs your `Init` with a live
`*cartlib.Setup`, `h.AdvancePastSplash()`, `h.Input(...)` and
`h.Tick(dt)` drive the frame loop the way the cabinet does, and the
Host's fields hold the frame, LCD text, lamps, sounds, storage and scores
your game wrote. A host call with no fake installed panics, so a test
that reaches one by accident says so.

Once the game is worth protecting, add a **golden test** beside the
game as `my-game_golden_test.go`, in the cartridge's own module (copy
`sdk/tutorial/catcher_golden_test.go`): it drives your compiled `.wasm`
through the engine's `carttest` harness with a fixed RNG script and
fixed timing, asserts ASCII snapshots of the frames, and checks that two
runs are byte-identical. The harness lives in the engine module, so the
test needs the engine in `go.mod` —
`require github.com/stealthmonkey99/arkade v0.1.0` and
`replace github.com/stealthmonkey99/arkade => ../../arkade`, plus
`replace github.com/stealthmonkey99/arkade/sdk/levels => ../../arkade/sdk/levels`
because the engine's own replace for that module does not carry over —
then `go mod tidy`. Only host-side test code imports it; nothing in the
TinyGo build sees it. That determinism check is the ABI's core promise — see [ABI
reference](abi.md#determinism) for what your game must (and must not)
do to keep it. Regenerate snapshots after intentional visual changes
with `UPDATE_GOLDENS=1 go test ...`.

## 6. Pack a `.rcart`

A bare `.wasm` is a valid cartridge with no assets. To make some, open
the **asset lab** — seven browser tools: **spritelab** for pixel art
and animation, **soundlab** for synthesized effects and music loops,
plus glyphlab, hidlab, periphlab, levellab and fsmlab. It runs hosted
at `arkade.build/dev/tools`, keeping its files in the browser: import
the cartridge's files, edit them, and export them back into the
cartridge's `assets/`. A small
generator you run from the cartridge (`tools/gen-sfx` for the audio, for
instance) turns the rest into what the build consumes. The
[tutorial](tutorial-assets.md) walks the whole path.

Once you have assets under `assets/`, pack the distributable bundle —
manifest, wasm, and assets in one file:

```sh
make rcart      # from my-cartridges/my-game
```

This runs `sdk/tools/rcartpack`, which applies the **exact same
validation the cabinet's loader applies at mount time** — a bundle
that's too big or has a bad manifest is rejected at pack time with the
identical error, instead of failing days later on a cabinet. Format
details and caps: [Manifests & packaging](packaging.md).

The web player accepts the `.rcart` by drag-and-drop exactly like the
bare `.wasm`, now with working sound and sprite assets.

## 7. Put it on a cabinet

If you have one, a USB stick with the `.rcart` at its root is the
simplest route: the game appears in the shell menu, no engine rebuild.
Copying it over the network is in
[Running on a cabinet](../../docs/CABINET.md#deploying-a-cartridge).

## 8. Write the manual and run preflight

Every cartridge ships a player manual, `MANUAL.md`, beside
`manifest.json`. Preflight checks three things about it: a `# My Game`
heading matching the manifest title, a header image
`<img src="manual-icon.png" …>` with `manual-icon.png` present (the 8×8
menu icon scaled up ×16, nearest-neighbour), and a footer line
`*Manual for version 0.1.0*` quoting the manifest version, so a version
bump forces a reread. `sdk/tutorial/MANUAL.md` is a complete example,
and the style guide is
[docs/MANUALS.md](https://github.com/stealthmonkey99/arkade-cartridges/blob/main/docs/MANUALS.md)
in the cartridges repo. The template ships a `MANUAL.md` to adapt (title,
pitch, footer version) and a `manual-icon.png` of its own;
`ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/manualicon`
rewrites `manual-icon.png` for every cartridge in the library from its
`assets/icon.arkadesprite.png`, or `assets/icon.png` when there is none.

Then, from the engine checkout:

```sh
ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/preflight my-game
```

Preflight runs the checks the published cartridges pass: the TinyGo
golden rules, WAV audibility, sprite alpha, the manifest and manual, the
packed bundle, the LCD text, and your own `go test ./...` (`-fast`
skips that last one). It wants a built `my-game.wasm`, so run
`make build` first. [Part 4 of the tutorial](tutorial-debug.md) reads a
report line by line.

## Where next

- **[Build a whole game, step by step](tutorial.md)** — the follow-along
  tutorial. One small complete cartridge from this template to a packed
  bundle: sprites drawn in spritelab, sounds synthesized in soundlab,
  hot-reload iteration, golden tests, replay, and preflight. Start here
  if you would rather be shown than told.
- The symbol-by-symbol API reference is generated from the Go doc
  comments into [api/cartlib.md](api/cartlib.md) and its sibling pages
  (rendered on the site with these docs; `go doc` and pkg.go.dev show
  the same comments). Regenerate with `make sdk-docs`; `sdkdoc -check`
  and `make test` fail when a page is stale.
- Make it a real game: [cartlib reference](cartlib.md) for input,
  sound, storage, scores, and the cabinet peripherals; [ui
  reference](ui.md) for menus, fonts, name entry, and scenes.
- Steal from the reference cartridges in the public cartridge
  collection (the `arkade-cartridges` repo): `cartridges/life` (minimal)
  and `cartridges/snake` (menus, 2P, scores, sound, attract mode).
- Before you polish: [Authoring patterns](patterns.md) — the distilled
  design lessons that separate a demo from a game people finish.
