# Catcher — the SDK tutorial's finished cartridge.
#
# Mirrors sdk/template/Makefile, which is the file to read for the full
# rationale. The short version:
#
# ARKADE is the one line a copy of this file ever needs edited: the path
# to an arkade engine checkout. Here that is two directories up; a copy
# in a cartridge library beside the checkout sets ../../arkade.
#
# `build` always runs the level generator (the phony `levels` target), which
# compiles assets/waves.arkadelevels into levels_gen.go through
# tools/genlevels' solver, and manifestgen (the phony `manifest` target),
# which writes manifest_gen.go from manifest.json. Both leave an up-to-date
# file untouched. tinygo and then wasmfix, which turns TinyGo's cart_manifest
# into the genuine two-result export both hosts need, run when a source is
# newer than the .wasm.
#
# This directory is its own Go module, so `go run <a path outside it>`
# does not resolve — every recipe that reaches into the engine checkout
# runs there with `go -C $(ARKADE)` and names this directory's files by
# absolute path.
ARKADE ?= ../..

# The cartridge's slug names its outputs: <slug>.wasm and <slug>.rcart.
SLUG := $(shell sed -n 's/.*"slug": *"\([^"]*\)".*/\1/p' manifest.json)

.PHONY: build manifest test rcart assets sheet sfx levels clean

build: levels manifest $(SLUG).wasm

levels:
	go -C $(ARKADE) run ./tools/genlevelpack $(CURDIR)

manifest:
	go -C $(ARKADE) run ./sdk/tools/manifestgen $(CURDIR)

$(SLUG).wasm: $(wildcard *.go) go.mod manifest.json | levels manifest
	tinygo build -target=wasi -scheduler=none -gc=leaking -opt=z -no-debug -panic=trap -o $(SLUG).wasm .
	go -C $(ARKADE) run ./tools/wasmfix -in $(abspath $(SLUG).wasm) -out $(abspath $(SLUG).wasm)

# The cartridge's own suite: the pure-logic unit tests (no TinyGo, no
# wasm) plus the golden/determinism tests, which drive the committed
# catcher.wasm through the carttest harness.
test:
	go test ./...

# Regenerate the sprite sheet and the sounds. Neither runs as part of
# `build`: the packed sheet and assets/*.wav are COMMITTED, so a normal build
# (and every test) uses exactly the bytes that were reviewed, and
# regenerating is a deliberate act with a reviewable diff. The levels, whose
# compiled form is build output like manifest_gen.go, are `build`'s own job.
assets: sheet sfx

# PNGs in, one packed .arkadesprite.png out. The PNGs under
# assets/sprites/ are still the source of truth — edit them in spritelab
# and re-run this — but the cartridge no longer compiles them in: it loads
# the sheet from its own bundle at Init (see sprites.go).
sheet:
	go run ./tools/gen-sheet

# Voice/Song literals in, spectrum-checked WAVs out.
sfx:
	go run ./tools/gen-sfx

# Package manifest.json + catcher.wasm + assets/ into a distributable
# bundle, enforcing the exact validation the cabinet's loader applies at
# mount time — so a bad bundle is rejected here, not days later.
rcart: build manifest.json
	go -C $(ARKADE) run ./sdk/tools/rcartpack -dir $(CURDIR) -out $(abspath $(SLUG).rcart)

clean:
	rm -f $(SLUG).wasm $(SLUG).rcart
