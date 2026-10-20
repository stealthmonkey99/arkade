# Årkade asset files

Every asset lab writes the same kind of file: a standard container any
ordinary program can open, carrying one **envelope** of Årkade metadata
that the lab can reopen and the engine can load. One shape, one reader,
one naming rule, so a new tool fits without a new format. The host
imports involved are part of ABI 1.

This file is the design rationale. Field-by-field detail is canonical in
the SDK reference: [`reference-assets.md`](../sdk/docs/reference-assets.md)
for the envelope and containers, then
[sprite](../sdk/docs/reference-sprite.md),
[sound](../sdk/docs/reference-sound.md),
[glyph](../sdk/docs/reference-glyph.md),
[hid](../sdk/docs/reference-hid.md),
[lights](../sdk/docs/reference-lights.md),
[mic](../sdk/docs/reference-mic.md) and
[levels](../sdk/docs/reference-levels.md). The labs are hosted at
arkade.build/dev/tools.

## Naming

`<name>.arkade<kind>.<container>`:

| Kind   | File                      | Container | Written by | Consumed by |
| ------ | ------------------------- | --------- | ---------- | ----------- |
| sprite | `hero.arkadesprite.png`   | PNG       | spritelab  | both hosts, via the ABI |
| sound  | `jump.arkadesound.wav`    | WAV       | soundlab   | both hosts, via `host_load_sound` |
| glyph  | `heart.arkadeglyph.json`  | JSON      | glyphlab   | both hosts at mount, session-scoped CGRAM |
| hid    | `dancepad.arkadehid.json` | JSON      | hidlab     | the cabinet host at mount, session-scoped profile |
| levels | `caves.arkadelevels`      | its own text format | levellab | compiled into the cartridge (`sdk/levels`); left out of the `.rcart` |
| lights | `pulse.arkadelights.json` | JSON      | periphlab  | both hosts at mount, played on the tick |
| mic    | `commands.arkademic.json` | JSON      | periphlab or `micmodel` | the cabinet host at mount |

The double extension keeps the file openable by every image viewer and
audio player while saying what it is. A plain `.png` under `assets/` is
still a sprite; the envelope is what unlocks sheets, animations and
reopening in the lab. `levels` has no separate container component: its
container is the pack format itself, so `<name>.arkadelevels` is the
whole rule ([reference-levels.md](../sdk/docs/reference-levels.md)).

## The envelope

One JSON document. In a PNG it sits in a `tEXt` chunk under the keyword
`arkade` (`zTXt` when large); in a WAV, in a RIFF chunk with id `arkd`;
for JSON kinds the document is the file; for `levels` it is `//` front
matter ahead of the pack body, and a pack with none still reads. Common
fields:

```json
{
  "arkade": 1,
  "kind": "sprite",
  "name": "hero",
  "description": "The player's walk cycle and idle.",
  "tags": ["player"],
  "generator": { "tool": "spritelab", "version": "1.6.0" },
  "recipe": { }
}
```

- `arkade` is the envelope format version. A reader refuses any other
  value rather than guessing.
- `generator` is **pinned**: the tool and version that produced the
  rendered data. A tool reopening a file with a different major version
  says so and offers to re-render instead of silently doing it; the
  rendered data is the artefact of record, the recipe is how it was made.
- `recipe` is the tool's own document: everything needed to re-render the
  payload byte for byte. Rendering is deterministic (engine RNG only,
  fixed sample rates, no wall clock), the rule cartridges live by.

Kind-specific fields sit beside `recipe`. The reasons that shape them:

- **sprite.** A **cel** is one drawing; a **frame** is one showing of one
  cel for a hold time, so a walk can name a cel twice. A sheet is any
  packing of named rectangles. The reader checks every rectangle against
  the pixels (outside the image is an error; a silly hold time is
  repaired).
- **sound.** The WAV is the render; `host_load_sound` never reads the
  envelope. `params` belongs to the pinned generator and `assetfile`
  validates shape, never contents, so both soundlab's knobs and a
  cartridge's own generator writing a bare voice name are honest; the
  shared `patch` grammar (`sfxlib.Patch`) makes a cue editable as data.
  A `mix` block states how nodes combine, because a lone loop pushed
  through a mixer is no longer the loop its generator produced. The
  `render` block is checked against the WAV's `fmt ` chunk. Nothing
  repairs: a recipe that renders something other than the audio beside
  it makes the record meaningless. Bit-identity is decided at the file,
  not in float64: differences below one 16-bit step produce identical
  files, which is what `tools/assetdiff` compares, per cue. The grammar
  pins its multiply order (`gain · source · env`) because Go fuses
  `a*b+c` on arm64 and differently shaped expressions disagree by an ULP.
- **glyph.** The same 5×8 rows `host_define_lcd_glyph` takes, refused for
  the same reasons (`internal/lcdrune`), so a file that packs is a file
  that mounts.
- **hid.** Report fields mapped onto named channels, plus a `standard`
  block aliasing some onto the panel. Declarative data: the host
  interprets it and never executes anything from a bundle. `standard` is
  many-to-one (a panel button is held while any of its sources is), and
  the report grammar's `map`, `bias` and `divide` verbs exist so the
  engine's built-in profiles are expressible in the format with no code
  path a bundle could not use.
- **mic.** A sound model: a few MFCC templates per class plus the front
  end they were measured under. A matched class fires its own `cue`
  number. It is measured from recordings, not hand-written, and declarative
  like `hid`.
- **lights.** Frames of per-seat lamp masks with hold times and a loop
  mode, the sprite animation's frame model over lamps. An absent seat key
  is a third state, not a mask of 0: it leaves that seat alone, which is
  what lets a one-seat cue run without touching the other panel.

## Packaging

Asset files live in the cartridge's `assets/` tree and travel in the
`.rcart`. `sdk/tools/rcartpack` validates every envelope it packs under
the bundle size caps, so a broken file fails at pack time, not on the
cabinet. The manifest lists what the host must act on at mount
(`assets.glyphs`, `assets.hid`, `assets.mic`, `assets.lights`), with the
`hid` and `mic` capabilities gating those two because they ask the
cabinet to read a device or listen to the room. Sprites and sounds need
no listing: the cartridge asks for them by path. Details:
[reference-assets.md](../sdk/docs/reference-assets.md#packaging-and-validation).

## Host imports

Additive imports on module `"arkade"`; none of them moves
`cart_abi_version` ([CARTRIDGE.md](CARTRIDGE.md#versioning)). The
per-import rationale is in [CARTRIDGE.md](CARTRIDGE.md#asset-channel-and-lamp-imports).

### Sprites from the bundle

`host_load_sheet`, `host_sheet_cel`, `host_cel_info`, `host_cel_read`
and `host_draw_cel` let the bundle be the source at runtime, the way a
console reads its ROM, while `ui.Sprite` stays the guest's type so
drawing and collision stay in the guest and golden-testable.
`Setup.LoadSheet` and `Sheet.Cel` build `ui.Sprite` values at `Init`
only, allocating once, and build the mask with `ui.OpaqueMask`, so a
sheet-loaded sprite and a `ui.Sprite` compiled in by hand or by a
cartridge's own generator behave identically. cartlib does not wrap
`host_draw_cel`.

### Glyphs on demand

No new import. At mount the host reads every file under
`assets.glyphs` into the same per-cartridge table `host_define_lcd_glyph`
writes, so the ASCII refusal, the surrogate check and the 32-rune cap
cannot differ between a defined glyph and a shipped one; the cap is
shared. Files load before `cart_init`, so a `DefineLCDGlyph` call wins
for a rune the bundle also ships. They clear at teardown. Preflight
judges runes against the engine palette, `DefineLCDGlyph` calls and the
listed files. A manifest description may use only engine runes: the
shell scrolls it before the cartridge is mounted.

### Input channels

Four channel kinds, integers only, because floats would make replays
platform-dependent: `button` (0/1), `axis` (`int16`), `delta` (`int32`,
accumulated between frames), `blob` (a declared fixed length). A
cartridge binds by name at `Init` and degrades honestly when a bind
returns -1. The panel is derived from the input records, so remapping is
already applied and a replay reproduces it with nothing extra recorded.
Bundled profiles are registered for the session and dropped at teardown;
one for a device the host already knows wins for that session only. The
browser host validates profiles but cannot read raw reports, so it binds
only the panel.

### Lamp sequences

`host_lights_play` and `host_lights_stop` walk a `.arkadelights.json`
listed under `assets.lights` on the tick's own `dt_micros`, so a replay
lights the same lamps on the same frames. The manifest list is the play
call's namespace: an unlisted sequence cannot be played, which makes a
mistyped path a pack-time error. An explicit `host_set_button_lights` or
`host_set_lights` stops a sequence on the seats it addresses, so the two
writers never fight.

## Kinds that are not assets

FSMs are source. A cartridge's screen machine is Go code, its states'
behaviour is code, so there is no file to load at runtime and no
envelope.

## Shared code

`assetfile` (Go) reads and writes the envelope for every container,
validates each kind and exposes the recipe untouched. The labs,
`rcartpack` and the cabinet host use it; the browser host carries its own
reader, tested against the Go fixtures.
