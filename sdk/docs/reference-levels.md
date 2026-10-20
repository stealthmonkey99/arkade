# Levels reference

This documents the `levels` kind of the asset envelope introduced on
[Asset file reference](reference-assets.md): a `.arkadelevels` file is a hand-authored level pack — the text format
`sdk/levels/packfile` defines and `tools/genlevelpack` turns into the Go
a cartridge compiles — with one optional envelope as **front matter**
ahead of the pack itself. A cartridge has no filesystem to read a pack from
at run time, and static data folds into the wasm at compile time where a
runtime parser would cost code size and allocations after `Init`, so the
pack is compiled to Go (`levels_gen.go`) by every `make build`; the
`.arkadelevels` file in the cartridge's `assets/` is the only one an author
edits, and the packer leaves it out of the `.rcart`. Read [Asset file reference](reference-assets.md)
first for the fields every kind shares (`arkade`, `kind`, `name`,
`generator`, ...); this page is only about how those fields sit inside a
`.arkadelevels` file, since the file is not JSON and the envelope is not the
whole of it.

This page does not restate the pack file format — the grid, the legend,
`ent`/`exit`, metadata typed by spelling. That is `sdk/levels/packfile`'s
own spec (`go doc ./sdk/levels/packfile`, and `sdk/levels/README.md` for
the tour), unchanged by anything here. What follows is the one thing
`packfile` gained: a place at the top of the file for the metadata every
other asset kind already carries.

levellab (the lab's level editor) is still the easy way to produce and edit one of these — it
opens a pack's grid, legend and metadata in a visual editor, and shows
and edits the envelope's name, description and tags too. This page is for reading or hand-editing the front matter directly.

## Where the envelope lives: front matter, not the file

Every other JSON kind (`glyph`, `hid`) makes the envelope *be* the file.
A `.arkadelevels` file cannot: it is already a text format with its own first
line (`pack NAME`), read by a real parser that has opinions about what
comes first. So a levels envelope is **front matter** — an optional
block that, when present, is the very first thing in the file, written
in the pack format's own comment syntax:

```text
// --- arkade ---
// {
//   "arkade": 1,
//   "kind": "levels",
//   "name": "walls",
//   "description": "Six wall layouts for the arena.",
//   "tags": ["arena"],
//   "generator": { "tool": "levellab", "version": "1.0.0" }
// }
// --- arkade ---

pack walls

legend
  . floor
  # wall

level L01
  |###|
  |#.#|
  |###|
```

Two sentinel lines, each spelled exactly `// --- arkade ---`, with one
`//`-prefixed comment line per line of the envelope's JSON document
between them. That is the whole grammar, and it was chosen for one
reason: `sdk/levels/packfile`'s own format already says "comments start
with `//` and take the whole line... blank lines and comment lines are
dropped before anything else is parsed" — so an envelope written this
way is invisible to the real parser (`packfile.Parse`) by construction,
not by a special case the parser had to learn. A pack file with this
block and the same file with the block deleted decode to the byte-identical
`levels.Pack`.

`sdk/levels/packfile`'s `SplitEnvelope` finds the block (or reports there
is none) and returns its JSON, undecoded, plus the pack's own body — the
bytes after the block, exactly what the file would have been before front
matter existed. `FormatEnvelope` is the inverse: JSON in, the fenced
comment block out. `assetfile.ReadBytes`/`WriteBytes` are what actually
decode and validate that JSON as an `Envelope` (below) — `packfile` itself
has no opinion on what the JSON means, only on where its bytes end and the
pack's begin.

### Why front matter and not, say, a trailing block

Because a reader — a human skimming the file, or a tool listing packs —
should not have to read to the end of a 64×64 grid to learn what a pack
is *for*. Every other kind's metadata is the first thing a reader of the
file encounters (a PNG's `tEXt` chunk sits right after `IHDR`; a WAV's
`arkd` chunk sits before `data`); a levels envelope keeps that promise by
sitting before `pack` rather than after the last `level`.

## A file with no front matter still parses — and still reads

A pack may carry no envelope at all, and that is not a migration
debt: **the block is optional, permanently**.
`packfile.Parse` never required it (it does not know it exists) and
`tools/genlevelpack` does not either — a pack without front matter
generates the identical Go it always has (see "Proof" below). What changes is only `assetfile.Read`, which is new: reading a
plain pack through it returns a **synthesized** envelope — `kind: "levels"`,
`name` taken from the file's own stem, and a generator that says so
rather than a pinned tool that did not write the file:

```json
{
  "arkade": 1,
  "kind": "levels",
  "name": "plain",
  "generator": { "tool": "assetfile", "version": "legacy-levels" }
}
```

Nothing is written back to disk by this — it is a value assetfile hands
back for the duration of one read, so a reader elsewhere in the
toolchain (`Validate`'s "generator present" rule, below) need not
special-case "this envelope was not really written by anyone."

## Fields

A levels envelope adds **no fields** beyond the ones every kind shares —
this is the one kind on [Asset file reference](reference-assets.md)'s
table with nothing kind-specific, because there is nothing left to say:
the pack body already states its own name (`pack NAME`), its own shape
(the grid, the legend), and everything a cartridge reads back
(`ent`/`exit`, metadata). Repeating any of that in the envelope would be
a second place for it to go stale.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `arkade` | integer | yes | The envelope format version; this page documents version `1`. |
| `kind` | string | yes | Always `"levels"`. |
| `name` | string | yes | The asset's own display name — not necessarily the `pack` directive's identifier, which has its own, stricter syntax (`[A-Za-z][A-Za-z0-9_-]*`). |
| `description` | string | no | Free text for whoever reopens the file later. |
| `tags` | array of strings | no | Free-form labels; nothing in the toolchain currently filters on them. |
| `generator` | object `{tool, version}` | yes | **Pinned.** See [Asset file reference](reference-assets.md#why-generator-is-pinned) — the same rule, applied to whichever tool last wrote the front matter (levellab, today). A file with no front matter gets the synthesized placeholder above, which still satisfies this rule. |

`recipe` has no meaning for this kind and is always absent: a level pack
is authored, not rendered from a recipe.

## The naming rule

`<name>.arkadelevels` is this kind's own spelling of [Asset file
reference](reference-assets.md#the-naming-rule)'s rule — with one
difference from every other kind: **there is no separate container
component**. A sprite's container is PNG, a glyph's is JSON; a levels
file's container IS the fleet's own text format, so the naming rule
collapses to one extension rather than two.

`.arkadelevels` is the name to write. Readers still accept the format's
original `.levels` extension for old files (`assetfile.KindFromFilename`,
`tools/genlevelpack`, levellab); `sdk/levels/packfile.HasExt` is the one
place that rule is stated. levellab writes `.arkadelevels` for a new pack,
and `tools/genlevelpack` never writes a pack file at all (only the
generated Go); its `-fmt` canonicalizer rewrites whichever file it is
pointed at in place, name unchanged.

## Proof: two cartridges' generated Go, unchanged

The central claim above — that front matter is invisible to the real
pipeline — is checked two ways.

`sdk/levels/packfile`'s own tests parse a fixture with a front-matter
block prepended and the same fixture without one, and assert the decoded
`levels.Pack` is byte-for-byte identical between the two
(`TestSplitEnvelopeDoesNotChangeWhatParseSees`). That is the format-level
proof.

The proof is regenerating real cartridges. Snake's
`assets/walls.arkadelevels` (in the arkade-cartridges repo) carries a
front-matter block as the worked example; running `tools/genlevelpack` over
it produces a `levels_gen.go` identical to the one already checked in. A second
cartridge with no front matter at all regenerates identically too — the
unremarkable case, which is the point: nothing about the envelope requires
touching a pack file that does not want the metadata.

## Editing one

Open levellab at `arkade.build/dev/tools` and import the pack.

The **Asset info** panel shows and edits `name`, `description` and
`tags` for the open pack, next to the grid, legend and metadata panels
it already had. A brand-new pack (the **New** button) gets one from the
start. A pack without asset info opens with the panel empty and a
note that it carries none yet; filling in a name there and saving writes
the envelope, never silently: an author who never opens that panel never
triggers it, and levellab is otherwise happy to open, edit and save a pack that carries no
envelope at all, forever, exactly as it already did.

`generator` is never a field the panel offers to type: like every other
lab tool, levellab pins its own tool name and version on every save that
writes an envelope, and reports — rather than silently overwriting —
when an opened file's generator is a newer major version than the build
you are running, the same rule [Asset file reference](reference-assets.md#why-generator-is-pinned)
states and soundlab already applies to `.arkadesound.wav`.

## Worked example

`assetfile/testdata/walls.arkadelevels`, which
`assetfile`'s own tests read back field for field (its sibling,
`plain.levels`, is the same pack with no front matter at all, exercising
the synthesized-envelope path above):

```text
// --- arkade ---
// {
//   "arkade": 1,
//   "kind": "levels",
//   "name": "walls",
//   "description": "Fixture pack: one tiny room.",
//   "tags": [
//     "fixture"
//   ],
//   "generator": {
//     "tool": "assetfile-fixtures",
//     "version": "1.0.0"
//   }
// }
// --- arkade ---

pack walls

legend
  . floor
  # wall

level L01
  |###|
  |#.#|
  |###|
```

A cartridge never lists a levels pack in its manifest and never loads
one by path at runtime. `pack walls`
compiles to `var wallsPack = levels.Pack{...}` through `tools/genlevelpack`; the envelope above is metadata for
whoever is reading or editing the file, not something `sdk/levels`' guest
runtime ever sees.
