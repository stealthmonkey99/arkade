# Årkade SDK documentation

Welcome! Årkade games are **cartridges**: a single `.wasm` file you
write in Go and compile with TinyGo. The same file runs unchanged on the
cabinet — a 64×64 LED panel, two small LCDs, two joysticks and lit
arcade buttons — and in the browser player at arkade.build, which
emulates the cabinet on your screen.

Cartridges are **deterministic**: the same inputs always produce the
same frames. That is what makes replays and frame-exact tests work; the
[Game Engine Primer](engine-primer.md#6-randomness-and-determinism)
explains it, and the few rules it asks of you, in depth.

Ready to build one? Start with the [Quick Start Guide](getting-started.md).

## How a cartridge fits together

```figure cartridge-parts
A .rcart is one file with three parts inside. The host loads it, runs
its loop and owns every output.
```

- **The `.wasm`** is your code implementing `cartlib.Game` — `Manifest`,
  `Init`, `HandleInput`, `Update`, `Render` — drawing with the
  [`ui`](ui.md) toolkit, with `cartlib` doing all the wasm plumbing.
- **`manifest.json`** names the game: slug, title, version,
  description, players, icon. [Packaging](packaging.md) has the full
  schema.
- **`assets/`** holds the files the game loads in `Init`: sprites,
  sounds, glyphs, lamp sequences, art. See [Asset Files](reference-assets.md).
- **The host**, the cabinet or the browser player, runs `HandleInput →
  Update(dt) → Render` about 30 times a second and drives the panel,
  the LCDs, the lit buttons and the speaker.

`rcartpack` builds the `.rcart` for you.

## The documentation set

The sidebar follows these tables, top to bottom.

**Learn** — read in order the first time.

| Doc | What it covers |
| --- | --- |
| [Quick Start Guide](getting-started.md) | Build your first cartridge: template → build → play in the browser and terminal → pack a `.rcart` |
| [Game Engine Primer](engine-primer.md) | How game engines work in general — loop, frames, sprites, input, state, determinism, audio — each idea tied straight to where it lives here |
| [Tutorial](tutorial.md) | Build one complete game start to finish, in four parts: [the game](tutorial.md), [sprites & sound](tutorial-assets.md), [authored waves](tutorial-levels.md), [play & ship](tutorial-debug.md) |

**Guides** — read these for understanding: what to reach for, in what
order, and where it trips you.

| Doc | What it covers |
| --- | --- |
| [cartlib](cartlib.md) | The cartridge library: the `Game` interface, drawing, input, sound, storage, scores, peripherals |
| [ui](ui.md) | The widget toolkit: canvas, fonts, widgets, menus, name entry, compositor, and `sdk/fsm`, the screen-state machine |
| [Authoring Patterns](patterns.md) | Hard-won design lessons, one paragraph each, on six pages: [the hardware](patterns-hardware.md), [art & sound](patterns-art-sound.md), [loading & attract](patterns-loading-attract.md), [seating & rotation](seating.md), [testing & debugging](patterns-testing.md) and [gotchas](gotchas.md) |

**References** — look things up here rather than read them through.

*[API Reference](api.md)* — one page per package, generated from the Go
doc comments (never hand-edited): every exported name with its exact
contract. [cartlib](api/cartlib.md), [cartlib/hosttest](api/cartlib-hosttest.md),
[ui](api/ui.md), [ui/input](api/ui-input.md), [fsm](api/fsm.md),
[levels](api/levels.md), [levels/packfile](api/levels-packfile.md).

*Asset Formats* — the files the lab tools write, documented for anyone
who would rather edit them by hand.

| Doc | What it covers |
| --- | --- |
| [Asset File Reference](reference-assets.md) | The envelope every asset kind shares: the naming rule, the JSON fields, where it lives in a PNG/WAV/JSON file, determinism, packaging |
| [Sprites](reference-sprite.md) | The `sprite` kind in full: cels, animations, what's a structural error versus repaired, generated identifiers |
| [Sounds](reference-sound.md) | The `sound` kind in full: the `arkd` chunk, the render block, the recipe's composition tree, one node versus a mix, loading and generating one |
| [Glyphs](reference-glyph.md) | The `glyph` kind in full: the 5×8 row bit layout, why ASCII is refused, the 32-rune cap, listing glyphs in the manifest, shadowing and teardown |
| [Levels](reference-levels.md) | The `levels` kind: the optional front-matter grammar, the naming rule (`.arkadelevels`), a file with no envelope, generator pinning — the pack body itself is `sdk/levels/packfile`'s own spec |
| [HID & Input Channels](reference-hid.md) | The `hid` kind in full — the report field grammar, the `standard` block, seat and mount remapping — and the input-channel ABI a cartridge reads it through, including what a browser tab cannot do |
| [Lamp Sequences](reference-lights.md) | The `lights` kind in full: the frame model, the mask bit table, the three loop modes, naming one seat or both, and the two imports that play one on the tick |
| [Microphone](reference-mic.md) | The `mic` kind in full — the front end, the segmenter, classes and cues, how a model is trained — and the microphone keys a cartridge reads it through, including why a gesture is never the only way to do something |

*[Platform Contract](platform.md)* — the layer under cartlib. Most
authors never need it; read it to debug at the boundary, write a host,
or target the ABI from another language.

| Doc | What it covers |
| --- | --- |
| [Manifests & Packaging](packaging.md) | The manifest schema, the `.rcart` bundle format, `rcartpack`, installing on a cabinet |
| [ABI](abi.md) | The wasm contract itself — what `cartlib` implements for you: exports, host imports, input records, determinism rules |

*[Glossary](glossary.md)* — every term this doc set uses, alphabetical,
one to three sentences each.

If you are new, read the [Quick Start Guide](getting-started.md) first — it
walks the whole path and links into the references as it goes.

## Where the code lives

[`sdk/README.md`](../README.md) maps the SDK's folders and modules. The
two best teachers are real games in the
[arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
repo: `cartridges/life` is the simplest complete cartridge, and
`cartridges/snake` shows everything — menus, two-player, high scores,
sound, an attract mode.

## How to read these pages

A few things on these pages are more than text. Here is each one once,
so you know what it is when you meet it.

**Commands you run in a terminal** come in a small dark window. The
copy button in its corner copies the exact text, with nothing added, so
you can paste it straight into your own terminal. Unless a page says
otherwise, a command is run from the engine checkout or from your
cartridge's own directory, and the page says which.

```sh
make build
```

**Files you create** have their name in the title bar and their lines
numbered. The copy button copies the file's contents, never the numbers.
Where a listing is one of several versions of a file the tutorial builds
up, it is always the *whole* file at that point — paste over what you
have.

```go file=hello.go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// A cartridge with no game in it: it registers, draws one pixel, and
// leaves when asked — cartlib answers the quit keys for any game that does
// not take them over. Everything else on these pages builds on this.
type hello struct{}

func (h *hello) Manifest() []byte { return manifestBytes }
func (h *hello) Init(s *cartlib.Setup) {}
func (h *hello) HandleInput(records []cartlib.InputRecord) {}
func (h *hello) Update(dtMicros uint64) {}
func (h *hello) Render(fb *cartlib.Framebuffer) { fb.SetPixel(32, 32, 255, 255, 255) }

var game = &hello{}
var _ = cartlib.Run(game)

func main() {}
```

**A few lines out of a file** are coloured like code but have no title
bar, no line numbers and no copy button: they show a shape, and the
words around them say where it goes.

```go fragment
if r.IsPress() && r.Key == cartlib.KeyJoyA {
	cartlib.RejectKey()
}
```

**What a command printed** is plain, with no button — it is there to
compare against, not to run.

```text
wasmfix: "cart_manifest" now returns (ptr=0x1c588, len=167) as a genuine 2xi32 wasm export
```

**The panel** is the cabinet's 64×64 LED matrix, drawn the way the real
lamps look. When a preview holds several frames it cycles through them
by itself; click it (or press Enter with it focused) to step to the next
frame. Small ones show a single sprite or icon at a size you can read.

```panel
tutorial/testdata/golden/catcher-title.txt
tutorial/testdata/golden/catcher-playing.txt
```
*The tutorial's game: its title screen, then a run in progress.*

**The LCD** is the 16×2 character display in front of each seat, shown
with the same centring, glyph table and eight-slot budget the cabinet
applies, so what you see here is what the glass would show.

```lcd
glyph ֍ 00 0e 11 1f 15 1f 0e 00
CATCHER ֍
GREEN TO PLAY
```
*A title line using one glyph the cartridge defined itself.*

**Words with a dotted underline** are glossary terms. Hover for the
definition, click to keep it open, and the arrow in the corner of the
popup opens the [glossary](glossary.md) entry in full. Each spelling of
a term is marked once per page, the first time it appears.

Every Go example on these pages is compiled against the real SDK by the
engine's own test suite, so what you read is what builds. (If you are
editing these pages rather than reading them,
[`sdk/docs/AUTHORING.md`](https://github.com/stealthmonkey99/arkade/blob/main/sdk/docs/AUTHORING.md)
in the engine repo has the conventions behind all of the above.)
