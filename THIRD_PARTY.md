# Third-party code

Everything Årkade vendors, fetches or links that it did not write. Our own
code is GPL-2.0-or-later (see [`COPYING`](./COPYING)). Because the Go
modules below include Apache-2.0 works, which GPLv2 cannot combine with,
the binaries are distributed under GPLv3
([`LICENSES/GPL-3.0.txt`](./LICENSES/GPL-3.0.txt)), and anyone
distributing one ships wazero's NOTICE
([`LICENSES/wazero-NOTICE`](./LICENSES/wazero-NOTICE)) with it.

## Vendored

| Path | Upstream | Licence |
|---|---|---|
| `third_party/rpi-rgb-led-matrix/include/led-matrix-c.h` | [hzeller/rpi-rgb-led-matrix](https://github.com/hzeller/rpi-rgb-led-matrix) at `4e326c1b`, unmodified ([provenance](./third_party/rpi-rgb-led-matrix/README.md)) | GPL-2.0-or-later ([`COPYING`](./third_party/rpi-rgb-led-matrix/COPYING)) |

## Fetched at build or run time

None of these is checked in or redistributed by this repository.

| What | Fetched by | Licence |
|---|---|---|
| rpi-rgb-led-matrix library (`lib/librgbmatrix.a`), upstream HEAD at setup time | `scripts/pi-setup.sh`, on the Pi; linked into the cabinet build | GPL-2.0-or-later |
| kokoro-js 1.2.1, @huggingface/transformers 3.8.1 | `npm ci` in `tools/speech`, pinned by its `package-lock.json` | Apache-2.0 |
| Their transitive npm packages | the same `npm ci` | MIT, BSD-3-Clause, Apache-2.0, ISC, BlueOak-1.0.0, MIT OR CC0-1.0; see below |
| Kokoro-82M weights and tokenizer (onnx-community/Kokoro-82M-v1.0-ONNX, revision and SHA-256s in `tools/speech/render.mjs`) | `tools/speech/render.mjs`, into `~/.cache/arkade-speech` | Apache-2.0 (hexgrad/Kokoro-82M) |

Two packages in the speech stack carry terms beyond their neighbours':

- **phonemizer 1.2.1** declares Apache-2.0 but embeds a build of eSpeak NG,
  which upstream licenses GPL-3.0-or-later.
- **sharp's optional `@img/sharp-libvips-*` binaries** are
  LGPL-3.0-or-later (the `@img/sharp-win32-*` and `sharp-wasm32` builds
  combine that with Apache-2.0 and MIT). npm installs only the one for the
  host platform.

The renderer itself (`tools/speech/*.go`, `render.mjs`) is ours, under the
repository licence.

## Go modules linked into the binaries

From `go list -deps ./cmd/...` on the host and for linux/arm64 with
`-tags led`.

| Module | Version | Licence | Notes |
|---|---|---|---|
| github.com/tetratelabs/wazero | v1.10.1 | Apache-2.0 | ships a NOTICE |
| github.com/ebitengine/oto/v3 | v3.3.3 | Apache-2.0 | |
| github.com/ebitengine/purego | v0.8.0 | Apache-2.0 | darwin builds only |
| periph.io/x/conn/v3 | v3.7.1 | Apache-2.0 | |
| periph.io/x/host/v3 | v3.8.2 | Apache-2.0 | |
| github.com/gopxl/beep/v2 | v2.1.1 | MIT | |
| github.com/godbus/dbus/v5 | v5.2.2 | BSD-2-Clause | linux builds |
| github.com/sstallion/go-hid | v0.15.0 | BSD-2-Clause | bundles hidapi's C sources, licensed at the licensee's choice under GPL-3.0, BSD or the original HIDAPI licence |
| github.com/pkg/errors | v0.9.1 | BSD-2-Clause | |
| golang.org/x/sys | v0.29.0 | BSD-3-Clause | with a PATENTS grant |
| golang.org/x/term | v0.28.0 | BSD-3-Clause | with a PATENTS grant |

`go.sum` also lists modules that only the dependencies' own tests use
(go-spew, go-difflib, testify, yaml.v3, clockwork, writerseeker); none is
linked.
