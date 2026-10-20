# Årkade cartridge SDK template — build shortcuts for authors.
#
# Prerequisite: TinyGo (see sdk/README.md's Quickstart for the install
# command).
#
# ARKADE is the ONE line to edit: the path to an arkade engine checkout,
# whose tools/ and sdk/ this file reaches into. The template ships two
# directories below sdk/ inside one, so ../.. is right where it sits; a
# copy made in a cartridge library beside the checkout
# (my-cartridges/<slug> next to arkade/) sets it to ../../arkade. Everything else below is derived from manifest.json.
ARKADE ?= ../..

# The cartridge's slug names its outputs: <slug>.wasm and <slug>.rcart.
SLUG := $(shell sed -n 's/.*"slug": *"\([^"]*\)".*/\1/p' manifest.json)

# A build is genlevelpack, manifestgen, tinygo, then wasmfix. genlevelpack
# compiles assets/*.arkadelevels into levels_gen.go (sdk/docs/reference-levels.md)
# and does nothing for a cartridge without one; manifestgen
# writes manifest_gen.go, the runtime manifest the wasm reports, from
# manifest.json (sdk/docs/packaging.md); wasmfix rewrites TinyGo's
# cart_manifest into the genuine two-result export both hosts need (see
# sdk/cartlib's package doc). `build` runs the phony `levels` and `manifest`
# targets every time, because make compares timestamps to the second and an
# edit right after a build would otherwise leave a stale generated file. Both
# generators leave an up-to-date file untouched, so a clean tree is still a
# no-op; only tinygo and wasmfix wait for a source to be newer than the .wasm.
# The generated files are committed, like the .wasm, so a clean clone builds
# and tests without the engine's tools.
#
# This directory is its own Go module (see go.mod's doc comment), so
# `go run <a path outside it>` does not resolve. `go -C $(ARKADE)` runs
# the engine's tools from the engine's own module instead, and the files
# here are passed as absolute paths so the change of directory cannot
# lose them.

.PHONY: build levels manifest test rcart clean

build: levels manifest $(SLUG).wasm

levels:
	go -C $(ARKADE) run ./tools/genlevelpack $(CURDIR)

manifest:
	go -C $(ARKADE) run ./sdk/tools/manifestgen $(CURDIR)

$(SLUG).wasm: $(wildcard *.go) go.mod manifest.json | levels manifest
	tinygo build -target=wasi -scheduler=none -gc=leaking -opt=z -no-debug -panic=trap -o $(SLUG).wasm .
	go -C $(ARKADE) run ./tools/wasmfix -in $(abspath $(SLUG).wasm) -out $(abspath $(SLUG).wasm)

# Host Go, no TinyGo: plain logic tests as they are, and code that calls the
# host through sdk/cartlib/hosttest's fake (sdk/README.md, "Testing").
test:
	go test ./...

# Package this cartridge as a distributable .rcart (sdk/docs/packaging.md),
# enforcing the same caps/validation the cabinet loader applies
# (sdk/tools/rcartpack's package doc).
rcart: build manifest.json
	go -C $(ARKADE) run ./sdk/tools/rcartpack -dir $(CURDIR) -out $(abspath $(SLUG).rcart)

clean:
	rm -f $(SLUG).wasm $(SLUG).rcart
