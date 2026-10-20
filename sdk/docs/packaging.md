# Manifests & packaging

A finished cartridge ships as one file: `<slug>.rcart`. The bundle is
the game's **one informational packet** — binary, metadata, artwork,
and manual travel together, so the cabinet shell, the website, and any
future registry all read a single source of truth.

## One manifest, two forms

A cartridge describes itself at two layers, in two different schemas:

1. **The runtime manifest** — the JSON your `Manifest()` method
   returns through `cart_manifest()`, compiled into the wasm as the
   `manifestJSON` constant in `manifest_gen.go`. Small, read by the
   *hosts* at load time: `abi`, `slug`, `name`, `description`,
   `version`, `players` (integers), optional `icon`, optional
   `rawDirections`. See [ABI reference](abi.md#runtime-manifest).
2. **The bundle manifest** — the `manifest.json` *file* packed into
   the `.rcart`. The full metadata schema below (schema-1: `title`,
   `players` as strings like `"1p"`), read by everything that lists
   games *without* instantiating them: the shell menu, the
   arkade.build library, registries.

You author only the second of these: **`manifest.json` is the single
source of truth, and `manifest_gen.go` is generated from it.** Every
build (`make build` in the cartridge, `make cart-<slug>` in the
arkade-cartridges repo) runs the generator before `tinygo`, and it can be run alone:

```sh
go -C ../../arkade run ./sdk/tools/manifestgen .       # from the cartridge directory
go run ./sdk/tools/manifestgen -check <dir>...         # exit 1 on a stale or missing file
```

The mapping is fixed: `title` becomes `name`, the player modes become
the counts they support (`["1p", "2p-versus"]` is `[1, 2]`), and `abi`,
`slug`, `version`, `description`, `icon` and `rawDirections` are
copied. Running it twice is a no-op, so the generated file is committed
beside the `.wasm` and a clean clone builds without the engine's tools.

Never edit `manifest_gen.go`. Preflight's `manifest` check **fails** a
cartridge whose generated file differs from what its `manifest.json`
generates, and `carttest.ManifestCopiesAgree` fails a wasm that reports
anything else, so a hand edit or a skipped rebuild cannot ship.

## The bundle manifest schema

`schemaVersion: 1` is required for anything newly packed (the loader
still mounts old pre-schema manifests so deployed bundles keep
playing). Annotated example:

```jsonc
{
  "schemaVersion": 1,
  "abi": 1,                     // mirrors cart_manifest()'s abi (informational here)
  "slug": "my-game",            // unique per publisher; global id = publisher/slug
  "title": "My Game",
  "publisher": "arkade",        // registry namespace
  "author": "your name",
  "version": "1.0.0",           // MAJOR.MINOR.PATCH
  "description": "One card-sized sentence about the game.",  // ≤160 bytes
  "tags": ["puzzle", "retro"],  // ≥1; lowercase-hyphen words; drives category browsing
  "players": ["1p"],            // check all that apply — see vocabulary below
  "audience": "all-ages",       // all-ages | teen | mature
  "capabilities": [],           // future permission-style requests
  "visibility": "public",       // public | play-only | private
  "icon": "assets/icon.png",    // 8×8 PNG, packed in the bundle — required
  "logo": "assets/logo.png",    // optional larger art for the landing page
  "manual": "MANUAL.md",        // optional long-form manual, packed in the bundle
  "links": {
    "source": null              // https:// URL once source is published, else null
  }
}
```

Field rules (validated by `internal/cartridge/rcart.Manifest.Validate`
— the *same* code path `rcartpack` and the cabinet's mount both run):

- **slug / tags / capabilities** — lowercase words joined by single
  hyphens (`[a-z0-9]+(-[a-z0-9]+)*`). The slug must match the bundle
  filename stem.
- **players** vocabulary is about how people share the cabinet:
  `1p`, `2p-coop` (simultaneous, same side), `2p-versus`
  (simultaneous, opposed), `alternating` (two players, one at the
  controls at a time). Check all that apply; a bare legacy `2` is
  rejected under schema 1 — say which.
- **audience** — our own vocabulary (`all-ages` | `teen` | `mature`),
  deliberately not anyone's rating marks.
- **visibility** drives registry listing and web gating (`public` |
  `play-only` | `private`); an installed bundle plays on a cabinet
  regardless.
- **description** is the short card text (≤160 bytes) — the manual is
  the long-form home.
- **Artwork is referenced by bundle-relative path, never embedded**,
  so the JSON stays diffable; the web layer extracts and serves it.
- **links** is required — use `{"source": null}` while unpublished;
  a non-null source must be `https://`.

Everything except `logo` and `manual` is required under schema 1.

Player manuals have their own format contract — a short skeleton
(what the game is, controls, scoring), a consistent tone, a length
that fits the shell's manual viewer, and an icon. Preflight's `manifest`
check enforces the heading, the icon and a footer quoting the manifest
version, so a version bump forces a manual re-pass
([Getting started](getting-started.md#8-write-the-manual-and-run-preflight));
the cartridges repo's `TestManualsCurrent` holds its own fleet to the
rest.

## The .rcart format

A `.rcart` is a tar (optionally gzip) named `<slug>.rcart`:

```text
my-game.rcart
├── manifest.json          # the bundle manifest above
├── game.wasm              # the cartridge binary
├── MANUAL.md              # if the manifest references one
└── assets/                # PNGs and WAVs, referenced by bundle-relative path
    ├── icon.png
    └── pop.wav
```

Everything under the project's `assets/` is packed except level packs
(`*.arkadelevels`, `*.levels`): those are compile-time input, built into
the wasm as `levels_gen.go`, so a bundled copy would be dead weight.

Asset paths in host calls are exactly the tar entry names —
`s.LoadSound("assets/pop.wav")` on Init's Setup, no leading `./`. On the cabinet
a mounted bundle's assets never touch disk (they're served from
memory); on the web the bundle is fetched and untarred client-side.

A bare `.wasm` with no bundle is also a valid cartridge — no assets,
no metadata card — and is the everyday dev artifact you drag into the
player.

### Caps

Enforced at pack time and at mount time, identically: `game.wasm`
8 MiB, `manifest.json` 1 MiB, the whole bundle 64 MiB uncompressed, 512
entries max (`internal/cartridge/rcart`).

## rcartpack

`sdk/tools/rcartpack` packs a project directory and — the important
part — **runs the cabinet loader's own validation on the bytes it just
built** before writing anything. A bundle that would fail on a
cabinet fails at pack time instead, with the identical error message.

```sh
# from your cartridge directory — its Makefile's rcart target
make rcart

# or from the engine checkout — DIR must contain manifest.json + a .wasm
make rcart DIR=../my-cartridges/my-game

# or directly, with explicit paths:
go run ./sdk/tools/rcartpack -dir ../my-cartridges/my-game -wasm ../my-cartridges/my-game/my-game.wasm -out my-game.rcart
```

The wasm is found at `<dir>/game.wasm`, then `<dir>/<slug>.wasm`, or
wherever `-wasm` points. rcartpack refuses to pack a legacy
(pre-schema) manifest — new bundles ratchet the schema forward while
old deployed ones keep mounting.

**Repack after every wasm rebuild.** Rebuilding the `.wasm` does not
touch an already-packed `.rcart`; shipping the stale bundle is a
classic mistake (the cabinet restart even looks healthy, because the
old wasm still registers fine).

## Installing

- **Cabinet**: copy the `.rcart` into `data/cartridges/` (or the root
  of a USB stick — mounts are watched). Copy to a temp name and `mv`
  into place: the folder watcher will happily read a half-copied tar.
  Then restart the service. Health-check the journal for
  `cartridge: cart/<slug> registered` and the absence of per-frame
  `wasm error` spam.
- **Console renderer**: no install — `./arkade --cartridges
  'path/to/dir'` scans directories/globs for `.rcart` and `.wasm`
  files directly.
- **Browser**: drag the `.rcart` onto the player page, or serve it
  from a registry entry (arkade.build packs each listed cartridge's
  bundle and serves it at `/{slug}/cart.rcart`).
