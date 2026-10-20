# Glossary

One-to-three-sentence definitions of the terms this SDK's documentation
uses, alphabetical, each linking to where it's covered in depth. If a term
here is new to you, read [Getting started](getting-started.md) or the
[engine primer](engine-primer.md) first — this page assumes the vocabulary
exists, it doesn't build it from scratch.

## ABI (and its version)

The wasm contract between a cartridge and its host: the eight `cart_*`
exports every cartridge implements, the `host_*` imports the host
provides, and the input-record and determinism rules that keep one
`.wasm` file running byte-for-byte identical on the cabinet and in the
browser. The contract is versioned — the current one is 1, and
additive imports never move it — and a cartridge reporting a version
the host does not accept fails with one clear "unsupported ABI version"
error instead of a confusing crash. See the
[ABI reference](abi.md).

## Asset envelope

The one JSON document every asset file carries — `arkade` (format
version), `kind`, `name`, a pinned `generator` `{tool, version}`, and
the kind's own fields — embedded where the container allows: a PNG's
`tEXt` chunk under the keyword `arkade`, a WAV's `arkd` RIFF chunk, a
`.arkadelevels` file's `// --- arkade ---` front matter, or the whole file for
the JSON kinds. It is what lets the lab tool that wrote an asset reopen
it later and lets `rcartpack` refuse a broken one at pack time; the
hosts never read it. See
[The envelope](reference-assets.md#the-envelope).

## Asset kinds

The seven values `kind` can take, each with its own file spelling under
the rule `<name>.arkade<kind>.<container>`: `sprite`
(`.arkadesprite.png`), `sound` (`.arkadesound.wav`), `glyph`
(`.arkadeglyph.json`), `hid` (`.arkadehid.json`), `levels`
(`.arkadelevels` — the container is the pack format itself), `lights`
(`.arkadelights.json`) and `mic` (`.arkademic.json`). Every kind is read and written through
the one `assetfile` package, so a lab tool, `rcartpack` and both hosts
agree on what the bytes mean. See
[The naming rule](reference-assets.md#the-naming-rule).

## Asset lab (the)

The suite of seven browser-based tools for authoring a cartridge's
non-code assets: spritelab (pixel art and sprite sheets), soundlab
(synthesized sound effects and loops), glyphlab (LCD glyphs), hidlab
(HID controller debugging and profiles), periphlab (the cabinet's
peripherals live, and button-light choreography), levellab (`.arkadelevels`
packs) and fsmlab (`sdk/fsm` screen machines). All seven are hosted at
arkade.build/dev/tools; each keeps its files in the browser, with
Import and Export to move them in and out of a cartridge. See
[Open the lab](tutorial-assets.md#open-the-lab).

## Attract screen

The cabinet's idle screensaver slot for a cartridge: a dedicated instance
that calls only `cart_init`, the optional `cart_attract` export, and
`cart_teardown` — no load phase, no `Update`. A cartridge that
implements it draws over the engine's starfield and, by house rule,
plays zero sounds and touches no LCDs or lights. See
[Attract](ui.md#attract).

## Browser player (the)

The wasm host running at arkade.build: a browser tab that instantiates a
cartridge's `.wasm` (or `.rcart`) directly and plays it via drag-and-drop,
with keyboard arrows standing in for the joystick. It implements the same
ABI as the cabinet, so a cartridge needs no browser-specific code, and it
ports the engine's player strategies, gating and rotation included. See [4. Play it](getting-started.md#4-play-it).

## Button lamps / eligibility light

The six lit arcade buttons in front of each seat, addressed through
`host_set_button_lights` by a per-button bitmask. The platform rule is to
light a button exactly when pressing it would do something right now —
an "eligibility light" — never statically, so an always-lit button never
reads as a mystery control. See
[Buttons and lights](patterns.md#buttons-and-lights).

## cartlib

The Go SDK library (`sdk/cartlib`) every cartridge imports: an ergonomic
wrapper around the raw ABI that supplies every `cart_*` export and wraps
every `host_*` import, so an author implements one `Game` interface
instead of writing wasm plumbing by hand. See the
[cartlib reference](cartlib.md).

## Cartridge

An Årkade game: a single `.wasm` module — optionally bundled with a
manifest and assets into a `.rcart` — that runs unmodified on both hosts,
the cabinet and the browser player. See the
[SDK documentation](README.md).

## Cartridge root

A directory whose subdirectories are cartridges — each one a folder
holding a `manifest.json`. Every fleet tool (preflight, the lab's
cartridge picker, the manifest gate) finds cartridges by scanning the
roots named in `ARKADE_CARTRIDGE_ROOTS`, comma-separated, falling back to
`cartridges/` beside the engine; the reader's own library of games is one
such root. See [Preflight](tutorial-debug.md#preflight).

## CGRAM glyph

A custom 5×8 pixel character uploaded into one of the cabinet's 16×2
LCD's eight CGRAM slots, because the display's own chip can only draw
ASCII from its mask ROM. `Setup.DefineLCDGlyph` lets a cartridge add
glyphs the engine's shared palette doesn't carry (up to 32 per cartridge,
8 drawn on one screen at once), on any plain, single-width, monochrome
rune — never an emoji or a CJK-wide one, which both hosts refuse — and
every definition is cleared when the cartridge unloads. See [Your own
glyphs](cartlib.md#your-own-glyphs).

## Cheat code

The one debug combo shared by every cartridge — Aux1 held plus Up Up
Down Down Left Right Left Right — wired through `cartlib.CheatCode`
instead of each game inventing its own. Completing it taints the running
instance so it can never submit a score again, a one-way flag a
cartridge is expected to show on screen rather than hide. See the
[cartlib reference](cartlib.md).

## Cue (microphone)

One class of a cartridge's own [sound model](#sound-model), delivered as
a keypress on `mic.cue0`…`mic.cue7` when the cabinet matches a sound
against it. A cue means whatever the bundling cartridge's model says it
means — one game's cue 0 is "come", the next game's is a kazoo note —
which is why the keys are numbered rather than named, and it is never
the only way to reach an action a button already reaches. See
[What a cartridge receives](reference-mic.md#what-a-cartridge-receives).

## Determinism

The invariant that everything a frame shows is a pure function of that
tick's time delta, its input records, and the host's seeded RNG draws —
no wall clock, no OS randomness, no goroutines. It's what makes a
recorded input log replayable, lets golden tests diff frames
byte-for-byte, and is enforced identically on both hosts. See
[Determinism](abi.md#determinism).

## dtMicros

The argument to `Update`, in microseconds: the delta for this tick only,
never a running total, and not always exactly 33,333 on the cabinet (the
browser's fixed-step accumulator is exact). A cartridge accumulates it to
drive its own fixed-timestep logic. See
[The Game interface](cartlib.md#the-game-interface).

## Echo cancellation

How the cabinet avoids hearing itself: the mixer's output is tapped as a
reference, aligned against the microphone's capture, predicted by an
adaptive filter and subtracted, so the detectors see the room minus the
cabinet. It exists because a synthesised sound effect is a *purer* tone
than any person — no threshold separates a game's own whistle from a
player's, but the host knows exactly what it is playing. Its known
blind spot is the first sound after boot, before there is an onset to
align on. See
[The pipeline the events come out of](reference-mic.md#the-pipeline-the-events-come-out-of).

## Engine clock (the)

A cartridge's only notion of time: `cartlib.NowMicros()`, fed by the
host, never a wall clock — there is no `time.Now` available to guest
code, by design. See [1. The game loop](engine-primer.md#1-the-game-loop).

## Fleet

Informal shorthand for the whole set of cartridges under the configured
[cartridge roots](#cartridge-root), as in "fleet-wide" — a change made,
or a rule enforced, across every cartridge at once, the way `preflight -all` does. See
[Preflight](tutorial-debug.md#preflight).

## Flight recorder

The cabinet's always-on ring buffer of what a running cartridge saw:
every tick's `dtMicros`, every input record, and every `RandomU64` draw,
under a header carrying the cartridge's slug, version, and wasm hash.
When something breaks, the ring becomes a file and one command replays
the exact session at a desk. See
[the flight recorder](tutorial-debug.md#when-something-goes-wrong-the-flight-recorder).

## Framebuffer

The `cartlib.Framebuffer` a cartridge draws into every frame: a fixed
64×64 array of straight-alpha RGBA pixels, blitted whole to the host once
per frame in a single `host_blit` call. It also satisfies `ui.Canvas`,
which is why the shared widget toolkit composites straight into it. See
[2. Frames and the display](engine-primer.md#2-frames-and-the-display).

## FSM (the screen machine)

`sdk/fsm`'s `Machine`: a small state variable plus the obligations that
hang off entering a state — one `OnEnter` hook that fires on every
arrival, and a real `Push`/`Pop` stack for modals. It doesn't replace the
`switch` a cartridge already writes over its own screen enum; it owns
only the variable and the entry hook. See
[Screen state](ui.md#screen-state-sdkfsm).

## Golden frame / golden test

A test that drives a cartridge's checked-in `.wasm` with a scripted RNG
stream, input, and timing, and asserts an ASCII snapshot of the resulting
frame against a checked-in "golden" file, plus that two runs are
byte-identical. It's the determinism guarantee made concrete, and the
same golden files are what the browser host is checked against. See
[5. Test as you go](getting-started.md#5-test-as-you-go).

## HID profile / input channels

A `.arkadehid.json` asset file says what a physical device's input
report *means* — the device's vendor/product, named channels (`button`,
`axis`, `delta`, `blob`) mapped to bytes and bits of the report, and a
`standard` block aliasing channels onto the panel's own names — as data
rather than driver code. A cartridge reads it through input channels:
`host_input_bind` once at init, then one call per value per frame.
hidlab captures a device over WebHID, decodes it live while you name
the channels, and writes the file. See
[The profile file](reference-hid.md#the-profile-file).

## Hold-to-confirm

The platform-standard mid-game exit flow, `ui.HoldConfirm`: B opens an
overlay over the frozen game, release arms it, and a press-and-hold of
about a second confirms — firing only on release, so the shell never
inherits a held button. See [HoldConfirm](ui.md#holdconfirm).

## Host fake (hosttest)

`sdk/cartlib/hosttest`, the in-process stand-in for the cabinet that a
cartridge's own `go test` drives its `Game` against under plain Go: it
runs `Init` with a live `Setup`, feeds input, ticks the loop, and records
every LCD, lamp, sound, storage and score call for the test to read. See
[5. Test as you go](getting-started.md#5-test-as-you-go).

## Host import

One of the `host_*` functions a cartridge calls into module `"arkade"` —
drawing, sound, storage, the score table, peripherals, time, and
randomness. Every host import is bounds-checked and warn-and-no-op on a
bad call, so a misbehaving cartridge can never take down its host. See
[Host imports](abi.md#host-imports-module-arkade).

## Host vs guest

The host is the runtime driving a cartridge — the cabinet or the browser
player — and it owns the outer loop, the framebuffer blit, and every
`host_*` import. The guest is the cartridge itself: sandboxed wasm with
no filesystem, no sockets, and no way to see another cartridge's storage
or scores. See [Design rules](abi.md#design-rules).

## Hosted vs local

Every asset-lab tool is hosted at arkade.build/dev/tools, compiled to
wasm and keeping its files in the browser, which is why each has
Import/Export buttons: import a cartridge's files, edit them, and export
them back into the cartridge's directory. See [Open the lab](tutorial-assets.md#open-the-lab).

## Keypress vs keydown

Two of the four input record kinds a cartridge receives. A cabinet
controller emits a keydown-and-keypress pair per physical press (and a
keyup on release); keypress is the canonical "acted once" record, and a
cartridge that also acts on keydown double-fires on real hardware.
Keydown/keyup exist for held-state tracking, which should go through
`ui.HeldDirs` rather than a hand-rolled tracker. See
[Input records](abi.md#input-records).

## LCD

Each cabinet seat's 16×2 character display, addressed by
`cartlib.SetText` and capped at 16 displayed runes per line. It draws
ASCII from a mask ROM and anything else — hearts, suits, shade ramps, a
cartridge's own CGRAM glyphs — from a small uploaded bitmap table. See
[Peripherals](cartlib.md#peripherals-lcds-and-button-lights).

## Levels pack

A hand-authored `.arkadelevels` text file — a grid, a legend, and typed
metadata — compiled by `tools/genlevelpack` into Go a cartridge commits
alongside it; the `levels` asset kind. The pack file is the only thing a
human edits, the generated `levels_gen.go` is build output that every
cartridge build refreshes, and TinyGo compiles that Go. An optional asset-envelope block may sit at
the top as `// --- arkade ---` front matter that the pack parser never sees.
See [The pack](tutorial-levels.md#the-pack).

## Lights cue

A `.arkadelights.json` asset file holding one piece of button-lamp
choreography: frames of per-seat lamp masks, each with a hold time, and
a loop mode. A cartridge lists its cues under `assets.lights` in the
bundle manifest and plays one with `cartlib.PlayLights`
(`host_lights_play`), after which the host walks the frames on the tick
— a flourish costs a call instead of a timer in `Update`. periphlab's
Lights room draws, previews on a real cabinet, and writes them. See
[Playing one](reference-lights.md#playing-one).

## Manifest

A cartridge describes itself twice, in two schemas: the small runtime
manifest `cart_manifest()` returns (`abi`, `slug`, `name`, `version`,
`players`), and the richer bundle `manifest.json` packed into the
`.rcart` (title, tags, audience, visibility, and more) that everything
lists games by without instantiating them. The bundle manifest is the
one an author hand-edits; the runtime one is `manifest_gen.go`,
generated from it on every build. See
[One manifest, two forms](packaging.md#one-manifest-two-forms).

## Mount remap / raw directions

The cabinet remaps a controller's physical directions to logical ones
before a cartridge ever sees them, based on which seat mount it's plugged
into and the panel's display orientation — right for screen-spatial
controls (a cursor moving toward an edge) and wrong for egocentric ones
(tank steering). `rawDirections` in the manifest, or
`host_set_raw_directions` at runtime, opts a cartridge out of the remap.
See
[Player strategy, rotation, and raw directions](cartlib.md#player-strategy-rotation-and-raw-directions).

## MRU / recent names

The per-physical-seat memory of who's been playing, fed by
`cartlib.NoteRecentName` on every confirmed name entry. Two different
questions live behind it: the one-hour auto-suggest prefill
(`RecentName`), and the five-deep, never-expiring "most recently used"
list of regulars (`RecentNameAt`/`RecentNames`) that a name-entry widget
offers as a pick-list. See
[Scores and recent names](cartlib.md#scores-and-recent-names).

## Name entry

The high-score initials flow, built on `ui.NameEntry`: a widget wired to
prefill a returning player's last name from their seat's MRU and accept
it with one press. See [NameEntry](ui.md#nameentry).

## Panel (64×64)

The cabinet's actual display: a 64×64 RGB LED matrix, 4,096 individually
lit pixels, with no palette and no tile layer between a cartridge's
framebuffer and the lamps themselves. See
[2. Frames and the display](engine-primer.md#2-frames-and-the-display).

## Player strategy

The engine's affordance for gating input to one seat and, for a
one-player mode, rotating the panel toward whoever is playing. A push
(`host_push_player_strategy`) alone is inert — the gate isn't enforced
until locked — and it never gates a keyboard, so a cartridge must still
demux on the input record's player byte for correctness. See
[Player strategy, rotation, and raw directions](cartlib.md#player-strategy-rotation-and-raw-directions).

## Preflight

`go run ./tools/preflight <slug>` — one command that grades a whole
cartridge against the platform's rules in a single pass, eight checks:
`config`, `golden-rules`, `wav-audibility`, `sprite-alpha`, `manifest`,
`bundle`, `lcd` and `tests` (`-list` prints them with the rule each
enforces; `wav-audibility` is informative and never fails a run). It's
the last thing an author runs before calling a cartridge finished. See
[Preflight](tutorial-debug.md#preflight).

## `.rcart` bundle

A cartridge's one distributable file: a tar (optionally gzip) named
`<slug>.rcart`, holding the bundle manifest, the `.wasm`, an optional
manual, and the `assets/` a cartridge references by bundle-relative path.
`rcartpack` builds one and runs the cabinet loader's own validation on it
before writing anything, so a bad bundle fails at pack time instead of on
hardware. See [The .rcart format](packaging.md#the-rcart-format).

## RandomU64

`cartlib.RandomU64()` — a cartridge's only source of randomness, drawing
from the engine's seeded RNG. `math/rand` and `crypto/rand` are both
off-limits in cartridge code, because either one would break the
determinism the platform depends on. See
[Environment](cartlib.md#environment).

## Score table

The cabinet's shared top-ten table, one per cartridge slug, kept
host-side — not in a cartridge's own storage — so the shell's LCD
readout, the attract screen, and a future leaderboard can all read it
from outside the game. the `cartlib.Score*` calls are the only way to touch it; there's no way for a cartridge to see
another game's table. See
[Scores and recent names](cartlib.md#scores-and-recent-names).

## Seat vs player

Two different bytes on every input record, and conflating them is the
classic bug: `player` is the *pairing slot* — who is P1 versus P2,
deciding whose piece moves — while `physical` is the *cabinet port*,
deciding which seat's LCD and button lights light up. They agree by
default and diverge the moment someone picks up the other controller.
They are distinct types, `cartlib.Slot` (`player`) and `cartlib.Seat`
(`physical`), so one cannot be passed for the other. See
[Input](cartlib.md#input).

## Setup

The `*cartlib.Setup` that `Game.Init` receives: the only way to load a
sound or a sprite sheet, bind an input channel or define an LCD glyph.
It is dead once `Init` returns, so the compiler keeps those allocating
calls out of the frame loop, and a stale one fails loudly in tests. See
[The Game interface](cartlib.md#the-game-interface).

## Shell (the)

The cabinet's own menu program: the screen that boots with every mounted
cartridge listed, launches whichever one a player selects, and is where
`cartlib.Exit()` (or the platform's default quit map, for a cartridge
that never declares it) sends a player back to. See
[4. Play it](getting-started.md#4-play-it).

## Slug

A cartridge's unique lowercase-hyphen identifier — its storage and score
namespace, and the filename stem its `.rcart` must match. It appears in
both the runtime manifest and the bundle manifest, and the two must
agree. See
[The bundle manifest schema](packaging.md#the-bundle-manifest-schema).

## Sound model

A `.arkademic.json` asset file — the `mic` kind — holding the words or
noises one cartridge wants to recognise: the feature front end they were
measured under, the segmenter that decides when a sound starts and
stops, and a few classes, each a handful of MFCC templates and the match
distance it accepts. The cabinet matches a captured sound against them
and fires the matched class's [cue](#cue-microphone) key; no audio ever
reaches the cartridge. periphlab's Listen room records the clips and
trains it, `micmodel` does the same on the command line, and nothing in
the file is hand-written. See
[The sound model file](reference-mic.md#the-sound-model-file).

## Sprite / cel / sheet

`ui.Sprite` is gameplay art in guest memory: a pixel buffer plus a
collision mask built from its fully-opaque pixels. A *cel* is one named
drawing; a *sheet* is a `.arkadesprite.png` packing several cels — and
zero or more animations, each a list of frames over them — with the
layout in its envelope. The sheet ships in the `.rcart` and
`Setup.LoadSheet` at Init hands back each cel by name; a cartridge may
also compile `ui.Sprite` literals in, by hand or with its own generator. See [3. Sprites](engine-primer.md#3-sprites)
and
[Loading a sheet from a cartridge](reference-sprite.md#loading-a-sheet-from-a-cartridge).

## TinyGo

The Go compiler this SDK targets: it produces the small
`-scheduler=none -gc=leaking` wasm binaries a cartridge ships, at the
cost of several hard rules (no goroutines, no time/math-rand, no
allocation after `Init`; [Gotchas](gotchas.md) lists them)
that ordinary Go code doesn't have to follow. See
[0. Prerequisites](getting-started.md#0-prerequisites).

## ui (the widget toolkit)

The `sdk/ui` package: a host-free widget library — menus, fonts, name
entry, hold-to-confirm, layers — shared by every cartridge and the
cabinet's own shell, so a cartridge that draws with it speaks the
platform's native visual language and renders identical bytes on both
hosts. See the [ui reference](ui.md).

## Virtual flag (the)

Bit 0 of an input record's flags byte, set on keyboard and other
keyboard-driven sources. A real joystick push emits both a joystick
record and a synthetic, *non*-virtual direction keypress for the same
press; a cartridge must act on exactly one of the two or it
double-fires — `r.PressDir()` does it, and the raw gate is
`kind == joystick || flags & virtual != 0`. See
[Input records](abi.md#input-records).

## wasmfix

The mandatory post-build pass (`go run ./tools/wasmfix`) that rewrites a
TinyGo-compiled `cart_manifest` export into the genuine two-result form
the ABI specifies, since TinyGo can't emit one itself. `make build`
always runs it after `tinygo build`; skipping it produces a cartridge
that fails to load on either host. See
[3. Build it](getting-started.md#3-build-it).
