# Årkade SDK documentation

Årkade games are **cartridges**: a single `.wasm` binary that runs
unmodified on two hosts — the physical cabinet (a 64×64 LED matrix,
two joysticks, two little LCDs, lit arcade buttons) and the browser
player at arkade.build. You write ordinary Go, compile it with TinyGo,
and the same file plays in both places, pixel for pixel.

That promise rests on one idea: **everything a frame shows is a pure
function of the frame's time delta, its input records, and the host's
seeded RNG**. No wall clocks, no OS randomness, no hidden state. It is
what makes a recorded game replayable, lets golden tests diff frames
byte-for-byte across hosts, and will one day let a leaderboard verify a
score by replaying its inputs.

## The 30-second model

- You implement `cartlib.Game` — five methods: `Manifest`, `Init`,
  `HandleInput`, `Update`, `Render`. `Init` receives the `Setup` that
  loads assets and binds input, and only `Init` ever gets one.
- The host drives the loop ~30 times a second:
  `HandleInput → Update(dt) → Render(framebuffer)`.
- You draw with the shared [`ui` widget toolkit](ui.md) (the same code
  the cabinet's own menus use) or raw framebuffer pixels.
- `cartlib` supplies all the wasm plumbing — you never write a raw
  export or import.
- `rcartpack` bundles your wasm + manifest + assets into a `.rcart`
  file anyone can drop onto a cabinet or a browser.

## The documentation set

| Doc | What it covers |
| --- | --- |
| [Engine primer](engine-primer.md) | How game engines work in general — loop, frames, sprites, input, state, determinism, audio — each idea tied straight to where it lives here |
| [Getting started](getting-started.md) | Build your first cartridge: template → build → play in the browser and terminal → pack a `.rcart` |
| [Gotchas](gotchas.md) | The platform traps that compile fine and fail later, one line each, with the symbol that documents or enforces it |
| [Tutorial](tutorial.md) | Build one complete game start to finish, in four parts: [the game](tutorial.md), [sprites & sound](tutorial-assets.md), [authored waves](tutorial-levels.md), [play & ship](tutorial-debug.md) |
| [API reference](api/cartlib.md) | Generated from the Go doc comments, one page per package: [cartlib](api/cartlib.md), [cartlib/hosttest](api/cartlib-hosttest.md), [ui](api/ui.md), [ui/input](api/ui-input.md), [fsm](api/fsm.md), [levels](api/levels.md), [levels/packfile](api/levels-packfile.md). Never hand-edited |
| [cartlib reference](cartlib.md) | The SDK library: the `Game` interface, drawing, input, sound, storage, scores, peripherals |
| [ui reference](ui.md) | The shared widget toolkit: canvas, fonts, widgets, menus, name entry, compositor - and sdk/fsm, the screen-state machine |
| [ABI reference](abi.md) | The wasm contract itself: exports, host imports, input records, determinism rules |
| [Manifests & packaging](packaging.md) | The manifest schema, the `.rcart` bundle format, `rcartpack`, installing on a cabinet |
| [Authoring patterns](patterns.md) | Hard-won design lessons, distilled, with pointers to the full reference pages |
| [Seating & rotation](seating.md) | Where each player sits, which way the board faces, and who is allowed to press what |
| [Glossary](glossary.md) | Every term this doc set uses, alphabetical, one to three sentences each |

**Reference: hand-editable file formats**, for anyone who would rather
not use the lab tools:

| Doc | What it covers |
| --- | --- |
| [Asset file reference](reference-assets.md) | The envelope every asset kind shares: the naming rule, the JSON fields, where it lives in a PNG/WAV/JSON file, determinism, packaging |
| [Sprite reference](reference-sprite.md) | The `sprite` kind in full: cels, animations, what's a structural error versus repaired, generated identifiers |
| [Sound reference](reference-sound.md) | The `sound` kind in full: the `arkd` chunk, the render block, the recipe's composition tree, one node versus a mix, loading and generating one |
| [Glyph reference](reference-glyph.md) | The `glyph` kind in full: the 5×8 row bit layout, why ASCII is refused, the 32-rune cap, listing glyphs in the manifest, shadowing and teardown |
| [Levels reference](reference-levels.md) | The `levels` kind: the optional front-matter grammar, the naming rule (`.arkadelevels`), a file with no envelope, generator pinning — the pack body itself is `sdk/levels/packfile`'s own spec |
| [HID profile and input channel reference](reference-hid.md) | The `hid` kind in full — the report field grammar, the `standard` block, seat and mount remapping — and the input-channel ABI a cartridge reads it through, including what a browser tab cannot do |
| [Lamp sequence reference](reference-lights.md) | The `lights` kind in full: the frame model, the mask bit table, the three loop modes, naming one seat or both, and the two imports that play one on the tick |
| [Microphone and sound model reference](reference-mic.md) | The `mic` kind in full — the front end, the segmenter, classes and cues, how a model is trained — and the microphone keys a cartridge reads it through, including why a gesture is never the only way to do something |

If you are new, read [Getting started](getting-started.md) first — it
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
