// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

// The .wasm fixtures are checked in beside their .wat so go test never needs
// wabt; regenerate after editing a .wat.

//go:generate wat2wasm testdata/good.wat -o testdata/good.wasm
//go:generate wat2wasm testdata/abimismatch.wat -o testdata/abimismatch.wasm
//go:generate wat2wasm testdata/hostile.wat -o testdata/hostile.wasm
//go:generate wat2wasm testdata/e2e.wat -o testdata/e2e.wasm
//go:generate wat2wasm testdata/flight.wat -o testdata/flight.wasm
//go:generate wat2wasm testdata/glyphs.wat -o testdata/glyphs.wasm
//go:generate wat2wasm testdata/channels.wat -o testdata/channels.wasm
//go:generate wat2wasm testdata/lights.wat -o testdata/lights.wasm
//go:generate wat2wasm testdata/trap.wat -o testdata/trap.wasm
