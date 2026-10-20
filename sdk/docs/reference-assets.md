# Asset file reference

The lab tools (hosted at
[arkade.build/dev/tools](https://arkade.build/dev/tools)) are the easy
path to every asset kind on this page — draw a sprite, synthesise a
sound, page a glyph, map a pad, and let the tool write the file. This
page is for the other path: what the file actually contains, precisely
enough to write or edit one by hand with a text editor (and, for a
sprite, an image editor that preserves PNG chunks). It documents
`assetfile`, the Go package every lab tool, `rcartpack` and both
engine hosts read this format through, and its
read-only counterpart in the browser host.

## The naming rule

Every asset file is `<name>.arkade<kind>.<container>`:

| Kind     | File                       | Container | Written by | Consumed by |
| -------- | -------------------------- | --------- | ---------- | ----------- |
| `sprite` | `hero.arkadesprite.png`    | PNG       | spritelab  | both hosts via the ABI |
| `sound`  | `jump.arkadesound.wav`     | WAV       | soundlab   | both hosts via `host_load_sound` |
| `glyph`  | `heart.arkadeglyph.json`   | JSON      | glyphlab   | both hosts at mount |
| `hid`    | `dancepad.arkadehid.json`  | JSON      | hidlab     | the cabinet host at mount |
| `lights` | `win.arkadelights.json`    | JSON      | periphlab  | both hosts via `host_lights_play` |
| `mic`    | `commands.arkademic.json` | JSON      | periphlab (or `micmodel`) | the cabinet host at mount |
| `levels` | `waves.arkadelevels` | the fleet's own text format (`sdk/levels/packfile`) | levellab | compiled in by every cartridge build (`genlevelpack`); left out of the `.rcart` |

`levels` is the one exception to that shape: its container IS the
fleet's own text format rather than a wrapper around one, so the naming
rule collapses to a single extension — see
[Levels reference](reference-levels.md#the-naming-rule) for both
spellings and what a writer chooses between them. A pack still lives in
`assets/` with every other kind; it is the one kind the packer does not
bundle, because the cartridge compiles it in.

The double extension is deliberate for the other six: every image viewer still opens
`hero.arkadesprite.png` as a PNG, every audio player still plays
`jump.arkadesound.wav`, and the middle component says what the file
means to the platform. A plain `.png` under `assets/` — no
`.arkadesprite` — is still an ordinary sprite, and is read for an
envelope all the same.

All seven kinds get their own full reference page; this page documents
only the envelope they share. [Sprite reference](reference-sprite.md),
[Sound reference](reference-sound.md), [Glyph reference](reference-glyph.md),
[HID reference](reference-hid.md), [Lights reference](reference-lights.md),
[Microphone reference](reference-mic.md) and
[Levels reference](reference-levels.md) document each kind's own
fields.

## The envelope

Every asset file carries one JSON document, the **envelope** —
embedded in the container for `sprite` and `sound`, *is* the file for
`glyph`, `hid`, `lights` and `mic`, and for `levels` sits as optional front
matter ahead of the pack format's own text (see
[Levels reference](reference-levels.md)). Fields common to every kind:

```json
{
  "arkade": 1,
  "kind": "sprite",
  "name": "hero",
  "description": "The player's walk cycle and idle.",
  "tags": ["player"],
  "generator": { "tool": "spritelab", "version": "1.6.0" },
  "recipe": {}
}
```

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `arkade` | integer | yes | The envelope format version. A reader refuses a value it does not know rather than guessing at a future format; this page documents version `1`. |
| `kind` | string | yes | One of `sprite`, `sound`, `glyph`, `hid`, `lights`, `mic`, `levels`. Must match the file name's `<kind>` component. |
| `name` | string | no | The asset's own name — not necessarily the file's name, though the lab tools keep them in step. The lab tools always write it; `assetfile` does not check it. |
| `description` | string | no | Free text for whoever reopens the file later. |
| `tags` | array of strings | no | Free-form labels; nothing in the toolchain currently filters on them. |
| `generator` | object `{tool, version}` | yes | **Pinned.** The tool and version that produced the rendered payload — see below. |
| `recipe` | object | no | The tool's own document: everything needed to re-render the payload byte for byte. Opaque to every reader except the tool that wrote it. |

Beside these sit the fields particular to `kind` — `cels` and
`animations` for a sprite, `render` for a sound, `codepoint`/`rows`/
`category` for a glyph, `device`/`channels`/`standard` for a HID
profile, `frames`/`loop` for a lamp sequence,
`features`/`segment`/`classes` for a sound model, and nothing at all for
levels (the pack body already says everything a levels envelope could)
— flat in the same document, not nested under a `"sprite"` or `"hid"`
key. [Sprite reference](reference-sprite.md), [Sound reference](reference-sound.md),
[Glyph reference](reference-glyph.md), [HID reference](reference-hid.md),
[Lights reference](reference-lights.md),
[Microphone reference](reference-mic.md) and
[Levels reference](reference-levels.md) document every kind's fields in
full.

### Why `generator` is pinned

The rendered container payload — the PNG's pixels, the WAV's samples —
is the artefact of record; `recipe` is how a tool would re-derive it.
A tool reopening a file whose `generator.tool`/`generator.version`
disagree with its own says so and offers to re-render, rather than
silently trusting a recipe it did not write. Rendering itself is
required to be deterministic — engine RNG only, fixed sample rates, no
wall clock — the same rule cartridges live by, so re-rendering the
same recipe with the same tool version always reproduces the same
bytes.

### Determinism

Writing the same envelope twice always produces identical bytes, in
every container. The mechanism: the envelope is rendered as compact
JSON with a fixed field order (a Go struct's fields encode in
declaration order) and every non-ASCII code point escaped as `\uXXXX`
(a surrogate pair above U+FFFF) — so nothing depends on how a
particular Unicode string happens to be represented, or on map key
iteration order (the one map in the format, a HID profile's
`standard` block, is a `string` → `string` map, and `encoding/json`
itself always emits those in sorted key order). A container's own
bytes around the envelope are written by an equally fixed procedure —
see each container below.

## Where the envelope lives

### PNG (`sprite`)

The envelope is a PNG `tEXt` chunk under the keyword `arkade`, placed
directly after `IHDR` — legal anywhere a `tEXt` chunk may appear, and
early enough that a streaming reader knows the envelope before it has
decoded a single pixel. `strings`, `exiftool -TextualData`, `pngcheck
-t` and ImageMagick's `identify -verbose` all print it directly; it is
plain ASCII JSON either way (see Determinism above), so no tool needs
to understand Latin-1 to read it by eye.

Once the JSON passes roughly 1&nbsp;KiB (a sheet with many named cels
and animations can get there), it moves to a `zTXt` chunk instead —
same keyword, zlib-compressed — so a large sheet's metadata does not
bloat the file past what a `tEXt` chunk should reasonably carry. A
reader checks `tEXt` first, then any `zTXt` chunk under the same
keyword; a writer keeps exactly one of the two, replacing whichever
existed in place.

Every other chunk — pixels, palette, any other tool's own text — is
preserved byte for byte across a write. Nothing here uses Go's
`image/png` to rewrite the file, because that package silently drops
every chunk it does not itself understand (see
`assetfile/pngchunk`'s package doc for the rest of that
argument); a hand edit with any tool that does the same is safe, one
that re-encodes the whole PNG from its own decoded pixels is not.

### WAV (`sound`)

The envelope is a RIFF chunk with id `arkd` (four ASCII bytes, chosen
the same way `arkade`'s `tEXt` keyword was: a made-up id, not anything
the WAV format itself defines), inserted between the `fmt ` chunk and
the `data` chunk:

```text
offset  bytes  content
0       4      "arkd"                     — chunk id, ASCII, not NUL-terminated
4       4      chunk size N, little-endian u32
8       N      the envelope JSON, ASCII (see Determinism above)
8+N     0/1    a single zero pad byte, present only if N is odd
```

That is the ordinary RIFF chunk framing every other chunk in the file
uses — no new rule to learn, and any RIFF-aware tool that preserves
unknown chunks (most do; `ffmpeg -i` and `file` both print it) keeps
it across an edit for you. A write drops any existing `arkd` chunk and
inserts the new one immediately before `data` (appended if the file has no `data` chunk); `fmt `, `data` and every other
chunk cross byte for byte, `data`'s audio samples included. Both
decoders in use skip a chunk they do not recognise, so a
`.arkadesound.wav` plays exactly as the same audio without an envelope
does. [Sound reference](reference-sound.md) has the byte layout and the
recipe's fields.

### JSON (`glyph`, `hid`, `lights`, `mic`)

For these four kinds the envelope *is* the file — no container framing
at all, just the JSON above, indented two spaces with a trailing
newline for a human reading it directly. Each is named per the naming
rule (`heart.arkadeglyph.json`, `dancepad.arkadehid.json`,
`win.arkadelights.json`, `commands.arkademic.json`) so the `<kind>`
component still tells a reader what the document holds without opening
it.

### Front matter (`levels`)

`levels` fits neither shape: its container is already a text format
with opinions about its own first line, so the envelope — when present
at all, since it's optional — sits as a fenced comment block ahead of
the pack body rather than wrapping or being the file. See
[Levels reference](reference-levels.md#where-the-envelope-lives-front-matter-not-the-file)
for the exact grammar.

## Packaging and validation

`rcartpack` validates every envelope it packs — format version, `kind`
against the file's own naming, a sprite's cel rectangles against the
sheet's actual pixel bounds, a sound's recipe structure and its `fmt `
chunk against its own `render` block, a glyph's code point, a HID
profile's channel names and report bounds, a lamp sequence's frame
count and mask bits, a sound model's class cues and template shape —
under the existing bundle size caps,
so a broken asset file fails at pack time rather than on a cabinet. The
bundle manifest lists what the host must act on at mount for the kinds
that need it:

```json
"assets": {
  "glyphs": ["assets/heart.arkadeglyph.json"],
  "hid":    ["assets/dancepad.arkadehid.json"],
  "lights": ["assets/win.arkadelights.json"],
  "mic":    ["assets/commands.arkademic.json"]
},
"capabilities": ["hid", "mic"]
```

Packing checks the manifest against the kinds: a `hid` or `mic` list
needs the matching capability, `lights` needs none, and a bundle mounts at
most 8 HID profiles, 4 mic models and 16 lamp sequences.

A sprite or a sound needs no manifest entry — a cartridge asks for
either by its bundle-relative path, exactly as it does for a plain
`.png`/`.wav` today (see [Manifests & packaging](packaging.md)). Neither
does a levels pack: it never leaves the source tree — the build's
`genlevelpack` step compiles it into `levels_gen.go`, which the cartridge imports directly, so nothing about it
survives into the bundle at all.
