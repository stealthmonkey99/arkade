module github.com/stealthmonkey99/arkade

go 1.23.4

require (
	github.com/ebitengine/oto/v3 v3.3.3
	github.com/godbus/dbus/v5 v5.2.2
	github.com/gopxl/beep/v2 v2.1.1
	github.com/sstallion/go-hid v0.15.0
	github.com/stealthmonkey99/arkade/sdk/levels v0.1.0
	github.com/stealthmonkey99/arkade/sdk/ui v0.1.0
	github.com/tetratelabs/wazero v1.10.1
	golang.org/x/term v0.28.0
	periph.io/x/conn/v3 v3.7.1
	periph.io/x/host/v3 v3.8.2
)

// The shared UI toolkit lives in the cartridge SDK so the same widget code
// serves the cabinet and a cartridge; the engine consumes it from the local
// working copy rather than a published version.
replace github.com/stealthmonkey99/arkade/sdk/ui => ./sdk/ui

// The level pack format lives in the cartridge SDK for the same reason: the
// runtime half compiles into cartridges, and tools/genlevelpack
// (in this module) is the generator that feeds it.
replace github.com/stealthmonkey99/arkade/sdk/levels => ./sdk/levels

require (
	github.com/ebitengine/purego v0.8.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/sys v0.29.0 // indirect
)
