# Glyph reference

This documents the `glyph` kind of the asset envelope introduced on
[Asset file reference](reference-assets.md): a `.arkadeglyph.json` file
is one 5×8 bitmap for one Unicode code point, plus the prose that says
what it is. A cartridge lists these in its manifest and both hosts
register them at mount, so a game can print a rune the character LCD's
own ROM has never heard of without an engine change and without a line
of loading code. Read [Asset file reference](reference-assets.md) first
for the envelope fields every kind shares (`arkade`, `kind`, `name`,
`generator`, `recipe`, ...); for a JSON kind, the document **is** the
file.

glyphlab is still the easy way to produce one of these — it draws the
5×8 grid, shows it inside real 16×2 panel text next to the ASCII the ROM
already draws, and writes the file straight into the cartridge's
`assets/glyphs/`. This page is for reading or hand-editing the JSON.

## Fields

A glyph envelope adds three fields beside the common ones, and gives
`description` a job the other kinds do not.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `codepoint` | integer | yes | The Unicode code point this bitmap draws, as a **number** — `9829`, not `"♥"`. Must be a plain, single-width, monochrome character: at or above `128` (`0x80`), not an emoji or an East Asian wide rune, and not a surrogate or noncharacter; see [Why ASCII is refused](#why-ascii-is-refused). |
| `rows` | array of 8 integers | yes | The bitmap, top row first. Each value is `0`–`31`: five pixels, **bit 4 leftmost**. See [Row bit layout](#row-bit-layout). |
| `category` | string | no | The family this glyph belongs to, for grouping a gallery: `arrows`, `shades`, `suits`, `dice`, `marks`, `symbols` are the ones the engine's own table uses, and a cartridge is free to invent its own (a falling-block game might ship `blocks`). Free text; nothing branches on it. |
| `name` | string | no | The envelope's shared `name`, but for a glyph it is the *thing's* name rather than the file's — `"half-block-lower"`, `"die-6"`. glyphlab derives the filename from it for a new glyph and always writes it; `assetfile` does not check it. |
| `description` | string | no | One line saying what the glyph is **for**, not what it looks like. "Filled heart: the hearts suit, and the pip a lives counter spends" earns its place; "a heart" does not. |

`recipe` has no meaning for this kind and is normally absent: the eight
rows ARE the artefact, there is nothing to re-render them from.

### Row bit layout

Eight rows, top to bottom, five pixels each. Within a row the **low five
bits** are the pixels and **bit 4 is the leftmost column**:

```text
bit    4  3  2  1  0
       ↓  ↓  ↓  ↓  ↓
row 0  ·  █  █  █  ·     0x0e = 14
row 1  █  ·  ·  ·  █     0x11 = 17
row 2  █  ·  █  ·  █     0x15 = 21
row 3  █  ·  █  ·  █     0x15 = 21
row 4  █  ·  █  ·  █     0x15 = 21
row 5  █  ·  ·  ·  █     0x11 = 17
row 6  ·  █  █  █  ·     0x0e = 14
row 7  ·  ·  ·  ·  ·     0x00 = 0
```

That is the HD44780's own CGRAM layout, so the bytes travel from the
file to the chip unaltered — and it is the same encoding
`host_define_lcd_glyph` packs into its `u64` and the same one
`internal/hardware/pi/glyphs.go`'s table is written in. A value above
`31` is an error, not a truncation: a row with junk in its top three bits
would draw one thing on the chip and something else on a host that
masked it, so it is refused rather than quietly fixed.

Row 7 is conventionally blank. The HD44780 draws a cursor there, and a
glyph that fills it looks like it is sitting on a line the ones around it
are not — the engine's own table leaves it clear for everything except
the full-block shade `█`, where the point is that there is no gap.

### Why ASCII is refused

A code point below `0x80` is refused by every reader in the chain — this
format, `host_define_lcd_glyph`, `rcartpack`, and both hosts at mount —
for one reason: **the chip already draws ASCII, out of its own mask
ROM**. Redefining a letter, a digit, or (worst) a space would move that
character into CGRAM, where it would take one of the eight slots on every
line that contains it and thrash the cache for everything else on screen.
There is no upside to trade against that, so it is not offered.

The refusal reads `assetfile: glyph codepoint U+0041 is below U+0080`,
and it is the same sentence on both hosts, because both run the same
ported validator.

Emoji and East Asian wide runes are refused by the same chain, for a
different reason: **a glyph rune must be a plain, single-width,
monochrome character** — the kind a terminal draws white-on-black in one
cell. A terminal draws an emoji in colour and two cells wide, and a CJK
ideograph or a fullwidth form is two cells by definition, so no surface
that ever shows LCD text (the glass, a log line, a test golden, the
browser widget) could draw them evenly. Refused: all of
`U+1F000`–`U+1FFFF`; the Basic Multilingual Plane's
`Emoji_Presentation=Yes` symbols (⚡ ⭐ ✅ ❌ ⌚ …); variation selectors
`U+FE00`–`U+FE0F` and the zero-width joiner `U+200D`; the wide blocks
`U+1100`–`U+115F`, `U+2E80`–`U+A4CF`, `U+AC00`–`U+D7A3`,
`U+F900`–`U+FAFF`, `U+FE30`–`U+FE4F`, `U+FF00`–`U+FF60`,
`U+FFE0`–`U+FFE6`; and every code point from `U+20000` up. `♥ ☺ ✔`, the
suits, the die faces, arrows, shades, box drawing and the private-use
area are all fine — that is where the engine's own table lives. The
refusal reads `… U+1F9FA is emoji — an LCD glyph rune must be a plain,
single-width, monochrome character`; the table is
`internal/lcdrune`, mirrored line for line by the browser
host, and `preflight`'s `lcd` check applies it to
`DefineLCDGlyph` literals and `SetText` text before the cabinet does.

## The 32-rune cap

One cartridge may hold **32 distinct glyphs** at a time, counting its
bundled files and its `host_define_lcd_glyph` calls together — they are
one table (see [Shadowing and teardown](#shadowing-and-teardown)). Past
32, a new rune is dropped with a warning and the table is left alone; a
rune already in the table can always be redefined. `Setup.DefineLCDGlyph`
returns false when a definition is refused, a 33rd distinct rune
included.

That is a *table* cap, not a render budget. The chip still shows at most
**8 distinct custom glyphs across both rows at once**, and pages them in
through an LRU — so a game may define a themed alphabet of thirty and use
eight of them at once, and the row being drawn shows `?` where a ninth
distinct glyph would go (the other row's glyphs stay pinned). Both numbers are in
[CARTRIDGE.md](../../docs/CARTRIDGE.md).

## Listing them in the manifest

Sprites and sounds need no listing: the cartridge asks for them by path
while it runs. A glyph is different — it has to be registered before the
first frame draws, and the guest never mentions it at all — so the
bundle manifest names them:

```json
"assets": {
  "glyphs": [
    "assets/glyphs/half-block-lower.arkadeglyph.json",
    "assets/glyphs/quadrant-ll.arkadeglyph.json"
  ]
}
```

Paths are bundle-relative, the same path space `host_load_sheet` and
`host_load_sound` use, and are read in the order listed. A cartridge with
no `assets` block at all is untouched by any of this.

**`rcartpack` fails the build** if a listed path is not in the bundle or
its envelope does not validate — a manifest that lists a glyph is
promising the host will register it, and a promise the bundle cannot keep
is a build error while the fix is still one edit away. At *mount* the
same check is a warning and a skip: a game whose fourth glyph file is
broken starts, missing that one rune, rather than refusing to launch with
a player standing in front of it.

Nothing writes the manifest for you, glyphlab included: it is the
cartridge's own contract. Saving a glyph prints the line to add.

## Shadowing and teardown

A bundled glyph and one defined by `Setup.DefineLCDGlyph` land in the
**same per-cartridge table**, and that is the whole design: shadowing and
cleanup are not new behaviour for shipped glyphs, they are the behaviour
the ABI import already had.

Three layers resolve a rune, in this order:

1. the operator's glyphlab **preview** overlay, if a lab is pushing one
   at the cabinet (the dashboard's `/api/preview/lcd` route,
   [DASHBOARD.md](../../docs/DASHBOARD.md));
2. the **cartridge's** table — bundled files and `DefineLCDGlyph` calls;
3. the **engine's** palette, `internal/hardware/pi/glyphs.go`.

So a cartridge may redefine any engine glyph — ship a `♥` of your own and
your `♥` is what draws, for your session only. ASCII stays the chip's,
always; it is not part of this stack.

Files are read **at mount, before `cart_init`**, so the first frame can
already print the rune. A `DefineLCDGlyph` call in `cart_init` therefore
runs *after* the bundle and wins for a rune both name — the same
last-writer-wins rule two `DefineLCDGlyph` calls follow.

The whole table is dropped in `Cartridge.Teardown`, *after* `cart_teardown`
returns so a farewell `SetText` still draws with it. The overlay is global
to the hardware suite, so leaving it installed would have the shell,
attract, and the next cartridge drawing a departed game's private
bitmaps.

## Worked example

`assetfile/testdata/heart.arkadeglyph.json`, which
`assetfile`'s own tests read back field for field:

```json file=heart.arkadeglyph.json
{
  "arkade": 1,
  "kind": "glyph",
  "name": "heart",
  "description": "Filled heart for the lives counter.",
  "generator": {
    "tool": "assetfile-fixtures",
    "version": "1.0.0"
  },
  "codepoint": 9829,
  "rows": [
    4,
    10,
    31,
    31,
    31,
    14,
    4,
    0
  ],
  "category": "hud"
}
```

`9829` is `U+2665`, `♥`. Reading the rows as bit patterns gives the
familiar shape, point down, with the blank cursor row at the bottom:

```text
·  ·  █  ·  ·      4
·  █  ·  █  ·     10
█  █  █  █  █     31
█  █  █  █  █     31
█  █  █  █  █     31
·  █  █  █  ·     14
·  ·  █  ·  ·      4
·  ·  ·  ·  ·      0
```

```lcd glyph
glyph ♥ 04 0a 1f 1f 1f 0e 04 00
```

A cartridge shipping this would list
`"assets/glyphs/heart.arkadeglyph.json"` under `assets.glyphs` and then
simply `cartlib.SetText(cartlib.SeatBoth, "♥♥♥  SCORE 40", "")` — no load call, no
handle, no branch for the host that could not draw it.

## A worked cartridge: a falling-block game

Take the fourteen block runes a falling-block game's next-piece preview
draws (`▄`, `▐`, `▌`, `▟`, `▜`, `▖`, `▘` and seven more). They belong in the
game's own files rather than the engine's table, where every other
cartridge would carry them and none would use them, and declaring them
there needs **no change to the game's Go source** —
`pieceGlyph` still returns `"▟▖"` and `SetText` still prints it. What
changed is where the host learns the bitmap, which is the point: a
game's private alphabet belongs to the game.

The reverse is worth stating too. The suits, the six die faces and
score-peg markers live in the engine, because they are general enough
for another game to want. **The test is not "who uses it today" but
"whose vocabulary is it".**

## Editing one

Open glyphlab at `arkade.build/dev/tools` and import the cartridge's
glyph files; they appear in the gallery beside the engine's, one list,
because that is how the cabinet resolves them. Click either kind to open it; the metadata panel is the same three
fields for both. **Save to cart** writes the file into the cartridge's
`assets/glyphs/` through this same `assetfile` package, so what the tool
writes and what an author hand-writes are the same bytes.

The panel simulator is the part worth using before you commit: it renders
your glyph inside real 16×2 text, through the same substitution the
cabinet performs, with a readout of how many of the eight CGRAM slots the
two lines together are asking for.
