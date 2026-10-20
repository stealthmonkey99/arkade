# Writing the SDK docs

How `sdk/docs/*.md`, and the SDK doc comments its API reference is
generated from, are written so that `go test ./sdk/docs/` can keep the
examples honest and the site at arkade.build/dev/docs renders the pages
the way [How to read these pages](README.md#how-to-read-these-pages)
promises. Readers never need this page; authors do. It is not one of the
site's pages.

## What the tests check

`sdk/docs/snippets_test.go` runs over every `.md` file in this directory,
this one included:

- Every fenced `go` block is compiled against the real
  `cartlib`, `ui`, `fsm` and `levels` modules, so an example cannot
  silently rot. Blocks in other languages, and `go fragment`,
  are not compiled.
- The tutorial's listings carry `file=<name>.go step=<n>` and are replayed
  in step order into one accumulating project, compiled at every step
  boundary; the final step is byte-compared with `sdk/tutorial/*.go`. The
  package doc on that test has the full mechanism.
- `TestDocLinksResolve` checks every relative link outside code fences:
  a same-page `#anchor` must match a heading or an `<a id>` on that page;
  a file link (`packaging.md`, `../template/main.go`,
  `../../docs/CARTRIDGE.md`) must resolve to a file that exists, relative
  to this directory; and an anchor into a `.md` target must match a
  heading or `<a id>` in that file. Absolute URLs are not checked.

## Fence tags

A tag is the word after a fence's opening backticks. Something the reader
is meant to type, run or paste in full gets its real language tag: `sh`
for a shell command, `go` for a complete Go file, `json`/`jsonc` for a
manifest, `make` for a Makefile, `levels` for a wave pack, `go-mod` for a
`go.mod`.

- Output, transcripts, logs and diagrams are `text` (or
  untagged).
- A block that mixes a command with what it prints is two fences: the
  command as `sh`, what it prints as `text`.
- Never write a `$ ` prompt into an `sh` block; the copy button copies the
  text verbatim.
- Every listing of a COMPLETE file the reader creates or pastes carries
  `file=<path>` after the tag, relative to the cartridge root when nested
  (for example `json file=manifest.json`,
  `go file=tools/gen-sfx/main.go` or
  `levels file=assets/waves.arkadelevels`). `start=<n>` numbers
  an excerpt from the middle of a file from `n`. On a `go`
  fence, `file=` without `step=` is a labelled standalone example to the
  test; `step=N` is consumed by the test alone and never renders.
- A few lines out of a file — a Go function without its package clause,
  two `require` lines out of a `go.mod` — are tagged
  `go fragment` (or `go-mod fragment`, and so on):
  highlighted as that language, never compiled.

## Doc comments and the API reference

The SDK's Go doc comments are the API reference. `sdk/tools/sdkdoc`
loads each SDK package with `go/doc` and renders one page per package
into `api/` (`api/cartlib.md`, `api/ui-input.md`, ...), plus the export
and import tables in [abi.md](abi.md) between its `<!-- sdkdoc:... -->`
markers; `go doc` and pkg.go.dev render the same comments. The pages are
never edited by hand: change the comment, then run `make sdk-docs`.
`sdkdoc -check`, `make test`, CI and `TestAPIReferenceIsCurrent` all fail while a page is
stale. Where a package's files are split by build tag, the page shows the
plain-Go (`!tinygo`) build and names the symbols only one build has.

The comments follow Go's conventions
([go.dev/doc/comment](https://go.dev/doc/comment)):

- A doc comment is complete sentences beginning with the symbol's name
  (`PlaySound plays ...`; a type may begin `A Seat is ...`). A package
  doc begins `Package cartlib ...`, a command's `Command sdkdoc ...`.
- `[Symbol]`, `[Type.Method]` and `[pkg.Symbol]` are links, rendered as
  links on the pages, in `go doc` and on pkg.go.dev. Use them wherever a
  reader would click; a cross-package link resolves only when the file
  imports that package.
- `# Heading` lines head sections of a long package doc; a tab-indented
  block is code; `- ` and `1. ` start lists; a paragraph beginning
  `Deprecated:` marks the symbol deprecated.
- Related constants are one `const ( ... )` block with one leading
  sentence on the group; a per-name comment only where a name needs
  more. Exported struct fields carry a doc line or a trailing comment.
- Runnable examples are `Example`, `ExampleT` and `ExampleT_Method`
  functions in a `_test.go` file of the package's `_test` package.
  `go test` compiles and runs them, checking the output against a
  closing `// Output:` comment, and the pages, `go doc` and pkg.go.dev
  show each beside its symbol. A file holding exactly one example plus
  other declarations is shown whole, which is how an example brings its
  own `Game` type.

On top of those, the SDK's own rule: document the
contract, not the implementation. Say the units, the ranges, the phase a
call is valid in (`Init`, `Update`, `Render`), and what invalid input does,
above all where the ABI fails silently by returning a failure value and
logging. Say where the TinyGo constraints bind the caller: no allocation
after `Init`, no goroutines, time only from `dtMicros`, randomness only
from `RandomU64`. No narration of how the code works and no history.
The first sentence of each cartlib wrapper's doc doubles as its import's
one-line contract in abi.md's import table, so it must stand alone.

`TestSDKDocCommentsConform` in `api_test.go` enforces the mechanical half
(`gen.Lint` in `sdk/tools/sdkdoc/gen` lists the rules): an exported symbol,
interface method or field without a doc, a doc that does not begin with
its name, an undocumented constant group, a missing package doc. The
contract half is review.

To link from a hand-written page to a symbol, use the generated
anchors: `api/cartlib.md#type-setup`, `#func-playsound`, a method as
`#func-setup-loadsheet` (receiver, then method), an interface method or
field through its type's anchor, and package-level constants and
variables through `#constants` and `#variables`. `TestDocLinksResolve`
checks those links like any other.

## For site maintainers

The site renderer is not in this repo; this section records what it does
with the conventions above, so a page renders the way
[How to read these pages](README.md#how-to-read-these-pages) describes.

- **Copy buttons.** Every fenced block gets a copy button *except* an
  untagged fence, one marked `text`, or one carrying the word
  `fragment` (the renderer's `copyable` check). `sh` (and
  `bash`/`console`) renders as a small terminal window; every other
  language tag gets a "file" panel, with the `file=` path in its title bar
  and line numbers in a gutter the copy button never picks up.
- **`panel`** is not code: its body lists frame files, one per
  line, relative to `sdk/` — a golden ASCII frame
  (`tutorial/testdata/golden/catcher-title.txt`) or a pixel PNG
  (`tutorial/assets/icon.png`); blank lines and `#` comments are ignored.
  The site draws them with the cabinet's LED look, cycling several frames
  with a crossfade (`hold=<ms>` sets the per-frame hold; `inline` makes a
  small floated version for an icon beside text; `zoom=<px>` asks for
  bigger lamps on a tiny frame). The build fails on a missing file or on
  frames of different sizes. Write the caption as the paragraph right
  after the fence, in italics.
- **`lcd`** is the cabinet's 16×2 character LCD: its body is
  the display's text, one line per row (at most two rows of at most 16
  columns; the build fails beyond that), and any line of the form
  `glyph <rune> <8 hex bytes>` defines a custom 5×8 glyph for that rune in
  `cartlib.GlyphRows`' encoding (bit 4 = leftmost pixel, row 0 on top).
  Runes neither the engine's table nor the block covers show as `?`, as on
  the cabinet. `lcd glyph` is the blow-up form — one `glyph`
  line and no text, drawn as a single large cell; `scale=<n>` (1–3)
  enlarges the 16×2 form. Caption it like a panel.
- **`figure <name>`** inlines `sdk/docs/figures/<name>.svg` with
  the body as its caption; **`demo <name>`** mounts one of the
  site's interactive demos. Both are first-party only.
- **Glossary terms.** `glossary.md`'s `##` headings are the term table.
  The site links the first mention of each spelling of a term on every
  other page to a popup carrying that entry's definition, so a new entry
  needs no markup anywhere else — and a page should not hand-link a term
  into the glossary, or that mention navigates instead of popping up.
  Variants come from the heading (split on " / ", parentheticals
  stripped), with an override map in the site's build for the ambiguous
  ones.
