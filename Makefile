# Årkade engine + SDK build shortcuts.
#
# LDFLAGS note: oto's purego bindings trip a macOS dyld check ("missing
# LC_UUID load command") when Go ≤1.23's internal linker is used; external
# linking emits the LC_UUID and costs nothing. Fixed upstream in newer Go —
# drop the flag once the toolchain moves past 1.23.
LDFLAGS := -ldflags=-linkmode=external

.PHONY: build build-pi run test goldens clean fmt-check license-check binary-check rcart \
	sdk-template sdk-tutorial sdk-tutorial-test sdk-levels-test sdk-docs sdk-docs-check

build:
	go build $(LDFLAGS) -o arkade ./cmd/arkade

# On-Pi build with the LED matrix renderer. Prerequisite (once, on the Pi):
#   git clone https://github.com/hzeller/rpi-rgb-led-matrix third_party/rpi-rgb-led-matrix/upstream
#   make -C third_party/rpi-rgb-led-matrix/upstream/lib
#   mkdir -p third_party/rpi-rgb-led-matrix/lib
#   cp third_party/rpi-rgb-led-matrix/upstream/lib/librgbmatrix.a third_party/rpi-rgb-led-matrix/lib/
# (scripts/pi-setup.sh does this.)
# Then: make build-pi && sudo ./arkade --led --led-slowdown 3
#
# arkade-lcdboot rides along: it is what owns the LCDs before the engine
# starts (scripts/arkade-lcdboot.service), and it deliberately has no cgo and
# no matrix library, so it builds on a Pi that has never cloned hzeller's.
build-pi:
	CGO_ENABLED=1 go build -tags led -o arkade ./cmd/arkade
	go build -o arkade-lcdboot ./cmd/arkade-lcdboot

run: build
	./arkade

clean:
	rm -f arkade arkade-lcdboot

# gofmt over every tracked Go file (third_party excluded). CI blocks on it.
fmt-check:
	@files=$$(git ls-files '*.go' | grep -v -E '(^|/)(third_party|vendor)/'); \
	dirty=$$(gofmt -l $$files); \
	if [ -n "$$dirty" ]; then \
		echo "gofmt-dirty file(s):"; echo "$$dirty"; \
		echo "run: gofmt -w <file> and re-commit"; exit 1; \
	fi

# Every in-scope source file carries the SPDX licence line.
license-check:
	scripts/license-check.sh .

# No tracked file is a compiled executable.
binary-check:
	scripts/binary-check.sh .

# What CI's format, license, go and sdk-modules jobs check. The sdk/*
# modules stay out of `./...` because their tinygo-only files declare
# bodyless go:wasmimport functions the host compiler rejects.
test: fmt-check license-check binary-check sdk-docs-check
	go build ./...
	go vet ./...
	go test ./...
	@for d in sdk/*/; do \
		[ -f "$${d}go.mod" ] || continue; \
		echo "go -C $$d vet+test ./..."; \
		go -C "$$d" vet ./... && go -C "$$d" test ./... || exit 1; \
	done

# Regenerate golden frames after intentional visual changes.
goldens:
	UPDATE_GOLDENS=1 go test ./...
	UPDATE_GOLDENS=1 go -C sdk/tutorial test ./...

# Rebuild the template and tutorial cartridges with TinyGo + wasmfix (see
# sdk/README.md's Quickstart). The checked-in .wasm files double as the
# reproducibility checks in internal/cartridge/wasm and sdk/tutorial.
sdk-template:
	$(MAKE) -C sdk/template build

sdk-tutorial:
	$(MAKE) -C sdk/tutorial build

sdk-tutorial-test:
	go -C sdk/tutorial test ./...

sdk-levels-test:
	go -C sdk/levels test ./...

# The API reference, sdk/docs/api/*.md and abi.md's import and export
# tables, generated from the SDK's doc comments (sdk/docs/AUTHORING.md).
sdk-docs:
	go run ./sdk/tools/sdkdoc

sdk-docs-check:
	go run ./sdk/tools/sdkdoc -check

# Package any cartridge project directory into a .rcart, with the checks
# the engine applies at mount time. Usage: make rcart DIR=sdk/template
rcart:
	@if [ -z "$(DIR)" ]; then echo "usage: make rcart DIR=<cartridge project dir>" >&2; exit 2; fi
	go run ./sdk/tools/rcartpack -dir $(DIR)
