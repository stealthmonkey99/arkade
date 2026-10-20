// SPDX-License-Identifier: GPL-2.0-or-later

// Package cartlib is the Årkade cartridge SDK for TinyGo: a Go wrapper
// around the docs/CARTRIDGE.md ABI (see [AbiVersion]) that supplies every
// cart_* export a cartridge needs and delegates them to one registered
// [Game]. sdk/template is a complete minimal cartridge to copy.
//
// # Registering a Game
//
// A cartridge is a package main that implements [Game] and registers one
// instance with [Run] in a package-level initializer:
//
//	var game = &myGame{}
//	var _ = cartlib.Run(game)
//
//	func main() {} // runs once during _start; keep it empty
//
// The hosts run a cartridge as a WASI reactor: _start runs package
// initializers and main exactly once at instantiation, before any cart_*
// export, and preflight refuses a non-empty main or an init function.
// Guest code therefore executes in two places only: package-level initializers that
// TinyGo's compile-time interpreter can fold into static data (literals,
// composite literals, the Run call above), and the cart_* exports the host
// calls, starting with cart_init ([Game.Init]), which is where computed
// one-time setup belongs.
//
// # Build tags
//
// The host-import declarations and the cart_* exports build only under the
// tinygo tag. Under the plain Go toolchain every host call goes instead to
// the [HostABI] a test installs with [UseHost], and panics when none is, so
// a cartridge's `go test` runs pure game logic as it is and drives the whole
// Game, Init through Render, against the in-process fake in
// sdk/cartlib/hosttest. [Game], [Framebuffer], the font and input decoding
// work in both builds.
//
// # Manifest and wasmfix
//
// TinyGo lowers the two-result cart_manifest export through a hidden output
// pointer, and the tools/wasmfix pass that every SDK build runs after
// `tinygo build` can rewrite it only when [Game.Manifest] returns the same
// static slice on every call. See [Game.Manifest].
//
// # No allocation after Init
//
// A cartridge is built with -gc=leaking, which never frees, so anything
// allocated per frame leaks for the life of the cabinet process. Every
// cartlib call usable after Init is allocation-free; the ones that are not
// (sound and sheet loading, channel binding, glyph definition) are methods
// of the [Setup] Init receives, which is dead once Init returns. Time comes only from
// dtMicros or [NowMicros], and randomness only from [RandomU64]; goroutines
// are not available.
package cartlib
