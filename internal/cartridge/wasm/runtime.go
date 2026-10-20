// SPDX-License-Identifier: GPL-2.0-or-later

// Package wasm runs Årkade cartridges on wazero: the "arkade" host module
// bridge, a minimal WASI stub, and an App adapter to engine.App.
//
// The bridge, not the Host, owns everything a guest could observe: argument
// validation, the sandbox caps and the registries behind handles (sheets,
// glyphs, input channels, lamp sequences). The cabinet and the browser host
// implement that one specification, so a guest cannot tell which it runs on;
// a Host supplies only where bytes come from and where output goes.
//
// Nothing here reads the wall clock or the OS RNG. A cartridge's time and
// randomness, WASI's included, come only from its Host.
package wasm

import (
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// SupportedABIVersion is the newest cart_abi_version this host implements.
// It names the calling convention, not the import list, so adding a host
// import never moves it.
const SupportedABIVersion = 1

// MinSupportedABIVersion is the oldest cart_abi_version this host still
// mounts.
const MinSupportedABIVersion = 1

// ErrABIMismatch wraps any cart_abi_version result outside
// MinSupportedABIVersion..SupportedABIVersion.
var ErrABIMismatch = fmt.Errorf("cartridge: unsupported ABI version")

const maxAllocBytes = 1 << 16

const maxInputRecords = 1024

type frameDims struct{ cols, rows int }

var ErrNotInitialized = errors.New("cartridge: not initialized")

// ErrFaulted is returned by every cart_* call after one has failed. The
// guest is not entered again; Fault reports the failure that stopped it.
var ErrFaulted = errors.New("cartridge: faulted")

// Fault is the failed guest call that stopped a Cartridge: a trap (a TinyGo
// -panic=trap build traps on every runtime panic) or any other error the
// guest returned through wazero.
type Fault struct {
	Export string // the cart_* export that failed
	Err    error
}

func (f *Fault) Error() string { return f.Export + ": " + f.Err.Error() }

func (f *Fault) Unwrap() error { return f.Err }

type Options struct {
	// Width and Height are cart_init's (cols, rows) and the host_blit frame
	// size; zero means display.DefaultWidth and DefaultHeight.
	Width, Height int

	// The Bundle lists are the manifest's assets paths, read through the
	// Host's BundleAsset and installed for this session before cart_init.
	// Nil when there is no bundle.

	// BundleGlyphs are registered in the cartridge's LCD glyph table.
	BundleGlyphs []string
	// BundleHID are installed on the Host's ChannelSource, if it is one.
	BundleHID []string
	// BundleMic are installed on the Host's MicModelSink, if it is one; a
	// matched class arrives as a mic.cueN keypress.
	BundleMic []string
	// BundleLights are validated at mount and are the only sequences
	// host_lights_play can name.
	BundleLights []string

	// Recorder receives the non-panel channel stream for the flight
	// recorder; nil when nothing records.
	Recorder ChannelRecorder
	// Replay serves a recording's channel stream in place of the Host's, for
	// the replay debugger; nil in normal operation. The panel's own channels
	// are never served from it: they are rebuilt from the replayed records.
	Replay *ChannelReplay

	// Interpreter runs the guest on wazero's interpreter instead of its
	// compiler, for differential tests; the cabinet never sets it.
	Interpreter bool
}

// Cartridge is one instantiated cartridge module wired to a Host, with no
// engine dependency. It is not goroutine-safe.
type Cartridge struct {
	runtime wazero.Runtime
	module  api.Module
	host    Host
	bridge  *bridge
	assets  []sessionAsset

	cartAbiVersion  api.Function
	cartManifest    api.Function
	cartInit        api.Function
	cartHandleInput api.Function
	cartUpdate      api.Function
	cartRender      api.Function
	cartTeardown    api.Function
	cartAlloc       api.Function

	cartAttract api.Function // optional; nil when not exported
	importsExit bool

	initialized bool
	fault       *Fault
	closed      bool
}

// call enters the guest unless an earlier call faulted, and latches the
// first failure.
func (c *Cartridge) call(ctx context.Context, fn api.Function, params ...uint64) ([]uint64, error) {
	if c.fault != nil {
		return nil, ErrFaulted
	}
	res, err := fn.Call(ctx, params...)
	if err != nil {
		c.fault = &Fault{Export: fn.Definition().ExportNames()[0], Err: err}
		return nil, err
	}
	return res, nil
}

func (c *Cartridge) ready() error {
	if c.fault != nil {
		return ErrFaulted
	}
	if !c.initialized {
		return ErrNotInitialized
	}
	return nil
}

// Fault returns the guest call that stopped the cartridge, or nil while it
// is healthy. Once set, every cart_* call returns ErrFaulted without
// entering the guest, and Teardown skips cart_teardown.
func (c *Cartridge) Fault() *Fault { return c.fault }

// ExportsAttract reports whether the module bytes export cart_attract,
// compiling without instantiating. A compile failure reports false.
func ExportsAttract(ctx context.Context, wasmBytes []byte) bool {
	rt := wazero.NewRuntime(ctx)
	defer func() { _ = rt.Close(ctx) }()
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return false
	}
	_, ok := compiled.ExportedFunctions()["cart_attract"]
	return ok
}

// NewCartridge compiles and instantiates a cartridge module, runs _start
// once if exported, resolves the required cart_* exports and loads the
// listed bundle assets. Init runs cart_init.
func NewCartridge(ctx context.Context, wasmBytes []byte, host Host, opts Options) (*Cartridge, error) {
	if host == nil {
		return nil, fmt.Errorf("cartridge: nil Host")
	}
	w, h := opts.Width, opts.Height
	if w == 0 {
		w = display.DefaultWidth
	}
	if h == 0 {
		h = display.DefaultHeight
	}

	cfg := wazero.NewRuntimeConfigCompiler()
	if opts.Interpreter {
		cfg = wazero.NewRuntimeConfigInterpreter()
	}
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	br := newBridge(host, frameDims{cols: w, rows: h}, opts)
	if _, err := instantiateHostModule(ctx, rt, "arkade", br.arkadeImports()); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("cartridge: register host module: %w", err)
	}
	boot := &bootPhase{}
	if _, err := instantiateHostModule(ctx, rt, "wasi_snapshot_preview1", wasiImports(host, boot)); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("cartridge: register wasi stub: %w", err)
	}

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("cartridge: compile module: %w", err)
	}
	modCfg := wazero.NewModuleConfig().WithStartFunctions() // _start is called below, under bootPhase
	mod, err := rt.InstantiateModule(ctx, compiled, modCfg)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("cartridge: instantiate module: %w", err)
	}

	// TinyGo's _start initializes the leaking allocator's heap bookkeeping.
	// Skipped, short sessions work but heap growth later corrupts memory.
	// random_get serves a fixed pattern meanwhile so the RNG stream is untouched.
	if start := mod.ExportedFunction("_start"); start != nil {
		boot.booting = true
		_, startErr := start.Call(ctx)
		boot.booting = false
		if startErr != nil {
			_ = mod.Close(ctx)
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("cartridge: _start (runtime boot): %w", startErr)
		}
	}

	c := &Cartridge{runtime: rt, module: mod, host: host, bridge: br}
	for _, def := range compiled.ImportedFunctions() {
		if m, name, ok := def.Import(); ok && m == "arkade" && name == "host_exit" {
			c.importsExit = true
			break
		}
	}

	required := []struct {
		name string
		slot *api.Function
	}{
		{"cart_abi_version", &c.cartAbiVersion},
		{"cart_manifest", &c.cartManifest},
		{"cart_init", &c.cartInit},
		{"cart_handle_input", &c.cartHandleInput},
		{"cart_update", &c.cartUpdate},
		{"cart_render", &c.cartRender},
		{"cart_teardown", &c.cartTeardown},
		{"cart_alloc", &c.cartAlloc},
	}
	for _, r := range required {
		fn := mod.ExportedFunction(r.name)
		if fn == nil {
			_ = mod.Close(ctx)
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("cartridge: missing required export %q", r.name)
		}
		*r.slot = fn
	}

	c.cartAttract = mod.ExportedFunction("cart_attract")

	// Loaded last, so a module that fails to mount leaves nothing installed.
	c.assets = []sessionAsset{
		{load: func() { br.glyphs.loadBundle(opts.BundleGlyphs) }, drop: br.glyphs.dropBundle},
		{load: func() { br.channels.loadBundle(opts.BundleHID) }, drop: br.channels.dropBundle},
		{load: func() { br.mic.loadBundle(opts.BundleMic) }, drop: br.mic.dropBundle},
		{load: func() { br.lights.loadBundle(opts.BundleLights) }, drop: br.lights.teardown},
	}
	for _, a := range c.assets {
		a.load()
	}
	return c, nil
}

// ImportsExit reports whether the module declares host_exit, its opt-in to
// owning the quit keys. Without it the engine's default quit stays, so no
// cartridge becomes un-exitable.
func (c *Cartridge) ImportsExit() bool { return c.importsExit }

// SupportsAttract reports whether the cartridge exports cart_attract, its
// opt-in to drawing its own attract slot.
func (c *Cartridge) SupportsAttract() bool { return c.cartAttract != nil }

// Attract calls cart_attract(dt_micros), or does nothing when it is not
// exported.
func (c *Cartridge) Attract(ctx context.Context, dtMicros uint64) error {
	if c.cartAttract == nil {
		return nil
	}
	if err := c.ready(); err != nil {
		return err
	}
	// An attract instance never sees cart_update, so lamps advance here too.
	c.bridge.lights.advance(dtMicros)
	_, err := c.call(ctx, c.cartAttract, dtMicros)
	return err
}

func (c *Cartridge) ABIVersion(ctx context.Context) (uint32, error) {
	res, err := c.call(ctx, c.cartAbiVersion)
	if err != nil {
		return 0, err
	}
	if len(res) != 1 {
		return 0, fmt.Errorf("cartridge: cart_abi_version returned %d values, want 1", len(res))
	}
	return api.DecodeU32(res[0]), nil
}

func (c *Cartridge) Manifest(ctx context.Context) ([]byte, error) {
	res, err := c.call(ctx, c.cartManifest)
	if err != nil {
		return nil, err
	}
	if len(res) != 2 {
		return nil, fmt.Errorf("cartridge: cart_manifest returned %d values, want 2", len(res))
	}
	ptr, length := api.DecodeU32(res[0]), api.DecodeU32(res[1])
	buf, ok := readBytes(c.module, ptr, length, maxManifestBytes)
	if !ok {
		return nil, fmt.Errorf("cartridge: cart_manifest returned an invalid (ptr=%#x, len=%d)", ptr, length)
	}
	return buf, nil
}

const maxManifestBytes = 1 << 16

// Init validates the cartridge's ABI version (returning an error wrapping
// ErrABIMismatch if it falls outside
// MinSupportedABIVersion..SupportedABIVersion, without calling cart_init)
// and then runs cart_init(cols, rows).
func (c *Cartridge) Init(ctx context.Context) error {
	v, err := c.ABIVersion(ctx)
	if err != nil {
		return fmt.Errorf("cartridge: cart_abi_version: %w", err)
	}
	if v < MinSupportedABIVersion || v > SupportedABIVersion {
		return fmt.Errorf("%w: cartridge reports %d, host supports %d..%d",
			ErrABIMismatch, v, MinSupportedABIVersion, SupportedABIVersion)
	}
	dims := c.bridge.dims
	if _, err := c.call(ctx, c.cartInit, api.EncodeU32(uint32(dims.cols)), api.EncodeU32(uint32(dims.rows))); err != nil {
		return fmt.Errorf("cartridge: cart_init: %w", err)
	}
	c.initialized = true
	return nil
}

// DispatchInput writes records into a cart_alloc buffer and calls
// cart_handle_input. A no-op if records is empty.
func (c *Cartridge) DispatchInput(ctx context.Context, records []InputRecord) error {
	if err := c.ready(); err != nil {
		return err
	}
	// Panel channels derive from exactly the records the guest receives, so
	// they cannot disagree and a replay rebuilds them for free.
	for _, rec := range records {
		c.bridge.channels.panel.apply(rec)
	}
	if len(records) == 0 {
		return nil
	}
	if len(records) > maxInputRecords {
		records = records[:maxInputRecords]
	}
	buf := PackRecords(records)
	if len(buf) > maxAllocBytes {
		return fmt.Errorf("cartridge: %d input bytes exceeds the %d cap", len(buf), maxAllocBytes)
	}
	res, err := c.call(ctx, c.cartAlloc, api.EncodeU32(uint32(len(buf))))
	if err != nil {
		return fmt.Errorf("cartridge: cart_alloc: %w", err)
	}
	if len(res) != 1 {
		return fmt.Errorf("cartridge: cart_alloc returned %d values, want 1", len(res))
	}
	ptr := api.DecodeU32(res[0])
	if !c.module.Memory().Write(ptr, buf) {
		return fmt.Errorf("cartridge: cart_alloc returned ptr %#x, which doesn't fit %d bytes of guest memory", ptr, len(buf))
	}
	if _, err := c.call(ctx, c.cartHandleInput, api.EncodeU32(ptr), api.EncodeU32(uint32(len(buf)))); err != nil {
		return fmt.Errorf("cartridge: cart_handle_input: %w", err)
	}
	return nil
}

// Update advances lamp sequences and samples every bound input channel, then
// calls cart_update(dt_micros). Sampling once before cart_update is the ABI
// contract.
func (c *Cartridge) Update(ctx context.Context, dtMicros uint64) error {
	if err := c.ready(); err != nil {
		return err
	}
	c.bridge.lights.advance(dtMicros)
	c.bridge.channels.sample()
	if _, err := c.call(ctx, c.cartUpdate, dtMicros); err != nil {
		return fmt.Errorf("cartridge: cart_update: %w", err)
	}
	return nil
}

func (c *Cartridge) Render(ctx context.Context) error {
	if err := c.ready(); err != nil {
		return err
	}
	if _, err := c.call(ctx, c.cartRender); err != nil {
		return fmt.Errorf("cartridge: cart_render: %w", err)
	}
	return nil
}

// Teardown calls cart_teardown when Init succeeded and nothing has faulted,
// then drops every listed asset even if it failed. It does not close the
// runtime.
func (c *Cartridge) Teardown(ctx context.Context) error {
	var err error
	if c.initialized && c.fault == nil {
		_, err = c.call(ctx, c.cartTeardown)
	}
	for _, a := range c.assets {
		a.drop()
	}
	if err != nil {
		return fmt.Errorf("cartridge: cart_teardown: %w", err)
	}
	return nil
}

// Close releases the wasm module and its runtime. Safe to call more than once.
func (c *Cartridge) Close(ctx context.Context) error {
	if c.closed {
		return nil
	}
	c.closed = true
	err := c.module.Close(ctx)
	if rtErr := c.runtime.Close(ctx); err == nil {
		err = rtErr
	}
	return err
}
