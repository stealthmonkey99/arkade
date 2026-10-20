// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"
	"errors"
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// appState is where one launch stands. Only appRunning enters the guest.
type appState uint8

const (
	appRunning appState = iota
	// appBounced: the launch failed and the App has asked to exit.
	appBounced
	// appFaulted: a guest call failed mid-session; the App is dark and
	// quiet until the engine unloads it.
	appFaulted
)

// App runs one launch of a cartridge as an engine.App. The engine treats an
// Init error as fatal, so App never returns one: a launch failure is logged,
// filed as a fault and turned into an exit back to the shell, and a fault
// mid-session darkens the cartridge instead of re-entering it every frame.
type App struct {
	ctx   *engine.AppContext
	host  *EngineHost
	cart  *Cartridge // nil when construction failed
	queue input.Queue

	state     appState
	launchErr error // a construction failure, reported at Init
}

var (
	_ engine.App          = (*App)(nil)
	_ engine.RawInputSink = (*App)(nil)
)

// NewApp builds the EngineHost and Cartridge for one launch under ctx and
// hands the App to the engine's CartAppHook before anything calls Init.
// assets is the bundle, nil for a bare .wasm. opts.Recorder is replaced by
// ctx's flight recorder.
func NewApp(ctx *engine.AppContext, wasmBytes []byte, assets map[string][]byte, opts Options) *App {
	host := NewEngineHost(ctx, PlayIdentity(ctx), ModePlay, opts.Width, opts.Height, assets)
	opts.Recorder = engineRecorder{ctx.RT}
	cart, err := NewCartridge(context.Background(), wasmBytes, host, opts)
	if err != nil {
		return &App{ctx: ctx, host: host, launchErr: fmt.Errorf("instantiate: %w", err)}
	}
	a := &App{ctx: ctx, host: host, cart: cart}
	ctx.RT.NotifyCartApp(a)
	return a
}

type engineRecorder struct{ rt *engine.Runtime }

func (r engineRecorder) RecordChannelBind(handle int32, kind ChannelKind, blobLen int) {
	r.rt.RecordCartridgeChannelBind(handle, uint8(kind), blobLen)
}

func (r engineRecorder) RecordChannel(handle int32, kind ChannelKind, value int32) {
	r.rt.RecordCartridgeChannel(handle, uint8(kind), value)
}

func (r engineRecorder) RecordChannelBlob(handle int32, blob []byte) {
	r.rt.RecordCartridgeChannelBlob(handle, blob)
}

// SetChannelReplay is Options.Replay for the CartAppHook, which reaches the
// App after construction. It reports false, installing nothing, once Init
// has run or when the launch already failed.
func (a *App) SetChannelReplay(replay *ChannelReplay) bool {
	if a.cart == nil || a.cart.initialized {
		return false
	}
	a.cart.bridge.channels.replay = replay
	return true
}

// Init runs Cartridge.Init and subscribes to every input kind. A cartridge
// that imports host_exit also gets a non-bubbling interceptor for the
// engine's quit keys, so they reach its confirm-exit flow instead of
// unloading it.
func (a *App) Init() error {
	if a.launchErr != nil {
		a.bounce(a.launchErr)
		return nil
	}
	if err := a.cart.Init(context.Background()); err != nil {
		a.bounce(err)
		return nil
	}
	keydown, keyup, keypress, joy := input.KindKeydown, input.KindKeyup, input.KindKeypress, input.KindJoystickChange
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{
			{Kind: &keydown}, {Kind: &keyup}, {Kind: &keypress}, {Kind: &joy},
		},
		Queue: &a.queue,
		// The cartridge observes input; the chrome's own shortcuts still fire.
		Bubbles: true,
	}})
	if a.cart.ImportsExit() {
		// Stacked above the catch-all; the default quit map matches keypress only.
		a.ctx.AddKeyMap([]*input.Mapping{{
			Conditions: []input.Condition{
				input.Key(input.KeyQuit),
				input.Key(input.KeyEscape),
				input.Key(input.KeyJoyB),
			},
			Queue: &a.queue,
		}})
	}
	return nil
}

// bounce logs a launch failure, files it as a fault and exits to the shell.
// The recording header's slug, version and wasm hash identify a wrong build.
func (a *App) bounce(err error) {
	a.state = appBounced
	a.ctx.Log(fmt.Sprintf("cartridge %q failed to launch, returning to shell: %v", a.ctx.Manifest.Slug, err))
	a.ctx.RT.NoteCartridgeFault("launch", err)
	a.ctx.Exit()
}

// HandleInput forwards the queued input to cart_handle_input. Once faulted it
// only watches for a quit press, since a cartridge that owns the quit keys
// can no longer act on them.
func (a *App) HandleInput(clock.FrameTime) {
	events := a.queue.Drain()
	switch a.state {
	case appRunning:
		if len(events) > 0 {
			a.check("input", a.dispatch(events))
		}
	case appFaulted:
		for _, ev := range events {
			if isQuitPress(ev) {
				a.ctx.Exit()
				return
			}
		}
	}
}

func isQuitPress(ev input.Event) bool {
	if ev.Kind != input.KindKeypress {
		return false
	}
	switch ev.Name {
	case input.KeyQuit, input.KeyEscape, input.KeyJoyB:
		return true
	}
	return false
}

func (a *App) dispatch(events []input.Event) error {
	records := EventRecords(events)
	// Recorded here, after remapping and gating, so a replay feeds exactly what the guest got.
	for _, rec := range records {
		a.ctx.RT.RecordCartridgeInput(PackRecord(rec))
	}
	return a.cart.DispatchInput(context.Background(), records)
}

// DispatchRawRecords delivers packed records straight to cart_handle_input
// for the replay debugger.
func (a *App) DispatchRawRecords(packed []byte) error {
	if a.state != appRunning {
		return nil
	}
	return a.cart.DispatchInput(context.Background(), UnpackRecords(packed))
}

func (a *App) Update(stats clock.FrameTime) {
	if a.state != appRunning {
		return
	}
	a.check("update", a.cart.Update(context.Background(), uint64(stats.ElapsedMs*1000)))
}

// Render calls cart_render. A cartridge always redraws its whole frame, so
// reason is ignored.
func (a *App) Render(_ clock.FrameTime, _ engine.RenderReason) {
	if a.state != appRunning {
		return
	}
	a.check("render", a.cart.Render(context.Background()))
}

// Teardown calls cart_teardown, unless the cartridge faulted, and releases
// the wasm runtime and the display layer whichever phase failed. The engine
// removes the key maps.
func (a *App) Teardown() error {
	defer a.host.Close()
	if a.cart == nil {
		return nil
	}
	ctx := context.Background()
	if err := errors.Join(a.cart.Teardown(ctx), a.cart.Close(ctx)); err != nil {
		a.check("teardown", err)
		return err
	}
	return nil
}

// check is the flight recorder's crash trigger. A guest fault is reported
// once and darkens the App; a host-side error is reported each time.
func (a *App) check(phase string, err error) {
	if err == nil || a.state == appFaulted {
		return
	}
	a.ctx.Log(fmt.Sprintf("cartridge %s: %v", phase, err))
	a.ctx.RT.NoteCartridgeFault(phase, err)
	if a.cart.Fault() != nil {
		a.state = appFaulted
		a.host.goDark()
	}
}
