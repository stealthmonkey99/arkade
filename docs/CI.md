# CI

One workflow, `.github/workflows/ci.yml`, runs on every push to `main`, every
pull request, and on demand. A newer run on the same ref cancels the older one.

| Job | What it checks |
|---|---|
| `format` | `make fmt-check`: `gofmt -l` is empty over every tracked Go file (`third_party/` excluded) |
| `license` | `make license-check` (every in-scope source file carries the SPDX line), then `make binary-check` (`scripts/binary-check.sh`: no tracked ELF, Mach-O or PE file) |
| `go` | `go build ./...`, `go vet ./...`, `go test ./...` for the engine module, golden frame tests included |
| `sdk-modules` | `go vet` and `go test` in every `sdk/*/` directory with its own `go.mod` |
| `cartridges` | rebuilds the SDK template and the tutorial's Catcher with the pinned TinyGo ([requirements](../README.md#requirements)), then re-tests them |

The engine's import graph is cgo on Linux, so the Go jobs install
`libasound2-dev` (oto) and `libudev-dev` (go-hid). The Makefile's
`-linkmode=external` flag is a macOS workaround and is not used on the runners.

## Running it locally

```sh
make test          # fmt-check, license-check, binary-check, the engine suite, every SDK module
make license-check
make binary-check
go vet ./...
```

## wasm and source drift

Every cartridge commits its `.wasm`. The golden tests drive that binary, so a
source edit that never got rebuilt keeps passing against the old code. The
`cartridges` job guards against it in this order:

| Step | Command | Question |
|---|---|---|
| 1 | `-run WasmMatchesSource` | does a fresh build match the committed `.wasm` byte for byte? |
| 2 | `-skip WasmMatchesSource` | does the committed binary still behave? |
| - | `make sdk-template sdk-tutorial` | rebuild, overwriting the committed `.wasm` in the working tree |
| 3 | `-skip WasmMatchesSource` | does a binary built from this commit's source behave? |

Byte equality is a property of the build host, not of the TinyGo version: the
same release produces different bytes on darwin/arm64 and linux/amd64. Step 1
therefore asserts only on the host that built the committed binaries
(`carttest.RequireWasmMatchesSource`) and skips with an explanation anywhere
else, including the runners. Step 3 is the portable drift check. Run
`make test` on the build host before committing a cartridge change; CI is a
backstop.
