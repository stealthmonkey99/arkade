# Sprite reference

This documents the `sprite` kind of the asset envelope introduced on
[Asset file reference](reference-assets.md): the JSON that lives inside
a `.arkadesprite.png`'s `arkade` chunk, its fields, what is checked
against the sheet's actual pixels versus repaired, and how a cartridge
loads one. Read
[Asset file reference](reference-assets.md) first for the envelope
fields every kind shares (`arkade`, `kind`, `name`, `generator`,
`recipe`, ...) and exactly where this JSON sits inside the PNG.

spritelab is still the easy way to produce one of these files; this
page is for reading or hand-editing the JSON directly.

## Cels and frames

A **cel** is one drawing: a named rectangle of the sheet's pixels,
listed once in `cels`, and the thing an artist edits.

A **frame** is one step of an animation: which cel to show, and how long
to hold it. Several frames may name the same cel, so a walk cycle of
four frames can be built from three drawings.

The classic animation words, used the classic way round: the sheet holds
cels, an animation is a list of frames over them. Adding a frame does
not make a drawing; adding a cel does.

## Fields

A sprite envelope adds two fields beside the common ones: `cels`, the
sheet's named rectangles, and `animations`, zero or more named
sequences of frames over them.

### `cels`

An array of objects, one per named drawing on the sheet:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `name` | string | yes | Unique within the sheet. Free text — `"idle"`, `"walk.0"`, `"shadow"` are all legal; a dot has no special meaning. `Sheet.Cel` looks a cel up by this exact string. |
| `x`, `y` | integer | yes | The cel's top-left corner, in the sheet's own pixel coordinates. |
| `w`, `h` | integer | yes | The cel's size in pixels. Both must be positive and the rectangle must lie inside the sheet; see [Cel size and the 64px limit](#cel-size-and-the-64px-limit) for the further ceiling a drawable cel has to meet. |
| `anchor` | `[x, y]` | no; `[0, 0]` if omitted | The point (relative to the cel's own top-left corner) a drawing routine anchors it on — where the sprite's "feet" are, typically. |

A sheet is any packing of any-sized rectangles: nothing requires a
uniform grid or a left-to-right strip, though a sheet that happens to
lay cels out as a strip (as spritelab still does) is exactly as legal.

#### Cel size and the 64px limit

The ENVELOPE bounds a cel only by the sheet it sits in: any positive
rectangle inside the image is a legal cel, and `assetfile` accepts it.
The 64-per-axis limit belongs to **`ui.Mask`**, whose rows are
`uint64`: 64 is the width of the LED panel and of a mask row, so a
bigger cel has nowhere to put its collision bits. `ui.OpaqueMask`
panics on a width over 64, and the hosts refuse to hand out a cel
bigger than 64 on either axis, so `Sheet.Cel` returns `nil` for one
rather than truncating its mask.

The two are separate on purpose. Only the cels a cartridge draws have
to obey 64; a sheet also legitimately holds art that never will, which
is why spritelab lets a cel run to 256 per axis and warns rather than
forbidding.

### `animations`

An array of objects, each naming a sequence of **frames** over the
sheet's own cels:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `name` | string | yes | Unique within the sheet. |
| `frames` | array of objects | yes | The frames, in play order; at least one. See below. |
| `loop` | string | no | One of `"cycle"`, `"once"`, `"pingpong"`. Anything else (including missing) becomes `"cycle"`. |

Each entry of `frames` is one showing of one cel:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `cel` | string | yes | The name of a cel on this sheet. It may repeat across frames — a four-frame walk that goes contact/pass/contact/pass off three drawings is `walk.0`, `walk.1`, `walk.0`, `walk.2`. |
| `holdMs` | integer | no | How long this frame holds. The hold belongs to the FRAME, not to the cel, so a repeated cel may be held for a different length each time it plays. Any value under 10 (missing and 0 included) becomes the default 120ms; a value over 10000 becomes 10000ms. |

```json
"animations": [
  { "name": "walk", "frames": [
      { "cel": "walk.0", "holdMs": 90 },
      { "cel": "walk.1", "holdMs": 90 }
    ], "loop": "cycle" }
]
```

## Structural errors versus repairs

The rule: a claim the reader can check against the actual pixels is
either true or the record is meaningless, so it is a **hard error**;
anything that only affects how cels are *shown* is repaired to a sane
default instead of failing the whole sheet. Concretely:

- **Errors:** an `arkade` value other than 1; `kind` not `"sprite"`; no
  cels at all; a cel or animation with an empty `name`; a duplicate cel
  `name`; a cel rectangle (`x`, `y`, `w`, `h`) that falls outside the
  sheet's actual pixel bounds, or has non-positive width or height; an
  animation with no frames; a frame whose `cel` is not on the sheet.
- **Repaired:** a missing or out-of-range frame `holdMs`; an
  unrecognised `loop`.

An envelope that fails a structural check is refused outright; there is
no plain-sprite fallback for a broken `.arkadesprite.png`.

## Worked example

`assetfile/testdata/sheet.arkadesprite.png` is a 24×16
PNG: three solid, fully-opaque 8×16 vertical bands (a dim, a medium and
a bright shade of the same blue-grey), so the three cels are easy to
tell apart by eye. Its `arkade` chunk holds this envelope
(`assetfile`'s tests read it back byte for byte):

```json
{
  "arkade": 1,
  "kind": "sprite",
  "name": "hero",
  "description": "Fixture sheet: three cels, one animation.",
  "tags": ["fixture"],
  "generator": { "tool": "assetfile-fixtures", "version": "1.0.0" },
  "recipe": { "note": "hand-authored fixture, not rendered by any tool" },
  "cels": [
    { "name": "idle",   "x": 0,  "y": 0, "w": 8, "h": 16, "anchor": [4, 16] },
    { "name": "walk.0", "x": 8,  "y": 0, "w": 8, "h": 16, "anchor": [4, 16] },
    { "name": "walk.1", "x": 16, "y": 0, "w": 8, "h": 16, "anchor": [4, 16] }
  ],
  "animations": [
    { "name": "walk", "frames": [
        { "cel": "walk.0", "holdMs": 90 },
        { "cel": "walk.1", "holdMs": 90 }
      ], "loop": "cycle" }
  ]
}
```

Three named cels, one of which (`idle`) no animation ever plays — a
sheet's cels and its animations are independent; a cel exists to be
drawn, an animation exists to sequence some subset of them. `walk`
alternates the other two cels, 90ms each, forever.

## Loading a sheet from a cartridge

The sheet travels in the `.rcart` and the cartridge reads it at run
time, so the wasm binary stays small and the art can be re-exported
without rebuilding it. (A cartridge may still compile `ui.Sprite`
literals in, by hand or with its own generator; the result is the same
type.)

```go
package main

import (
	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// blank is what every sprite falls back to when the bundle is missing —
// a dev build run from a bare .wasm, most often. A zero ui.Sprite draws
// nothing and collides with nothing, so nothing dereferences a nil.
var blank ui.Sprite

var (
	spriteIdle  = &blank
	spriteWalk0 = &blank
)

// loadSprites runs ONCE, from Game.Init, which is the only place a
// *cartlib.Setup exists. Cel allocates, and a cartridge runs
// under -gc=leaking where nothing is ever freed.
func loadSprites(s *cartlib.Setup) {
	sheet := s.LoadSheet("assets/hero.arkadesprite.png")
	if !sheet.Ok() {
		return // the host has already logged why
	}
	if s := sheet.Cel("idle"); s != nil {
		spriteIdle = s
	}
	if s := sheet.Cel("walk.0"); s != nil {
		spriteWalk0 = s
	}
}

func main() {} // a cartridge's real main calls cartlib.Run
```

The path is bundle-relative, the same path space `Setup.LoadSound`
uses. `Cel` returns a `*ui.Sprite` with its pixels copied once and its
collision `Mask` derived once by `ui.OpaqueMask`: a pixel collides
exactly when its alpha is 255. A compiled-in sprite whose mask was
built by the same rule is indistinguishable from a loaded one.

`Cel` returns `nil` for a sheet that did not load, a cel name the sheet
does not carry, or a cel larger than `ui.Mask`'s 64-pixel rows can
represent. All three are logged host-side by name. It also returns `nil`
for a call made after Init returns, logging a stale-Setup message (a
panic under `go test`).

### Drawing without a mask

The ABI also has `host_draw_cel`, which composites a cel on the host's
surface without copying its pixels into the guest. cartlib does not wrap
it: cartlib sends the whole framebuffer after every `Render`, which
overwrites anything drawn host-side. Lift the cel with `Sheet.Cel` and
draw the `ui.Sprite`; that keeps every pixel inside the cartridge, where
a golden frame can see it.

### What the hosts enforce

Per cartridge: 8 sheets, 256 named cels, 64 pixels per cel axis, 1 MiB of
resident cel pixels, a 1 MiB sheet file decoding to at most 4 MiB of
pixels. Interlaced PNGs are refused on both hosts (save non-interlaced).
A refused load returns a zero handle and logs the reason; a zero handle is
a silent no-op everywhere it is accepted, so a cartridge never has to
branch on one to stay safe. `sdk/tools/rcartpack` runs the same envelope
validation at pack time and fails the build, so a broken sheet is caught
before it can reach a cabinet. Full detail:
[CARTRIDGE.md's "Sprites from the bundle"](../../docs/CARTRIDGE.md).
