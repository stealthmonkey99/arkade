// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/render"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

// DefaultFPS is the default game-loop rate.
const DefaultFPS = 30

// Options configures a Runtime.
type Options struct {
	Renderer  render.Renderer
	Cols      int
	Rows      int
	TargetFPS int
	Clock     clock.Clock
	Audio     audio.Service
	DataDir   string
	// Hardware is the peripheral suite; nil means hardware.Virtual.
	Hardware hardware.Suite
	// StartApp is the slug to boot into instead of the boot app.
	StartApp string
	// RandSeed seeds the engine RNG; 0 seeds from the clock.
	RandSeed int64
	// Logger receives log lines. nil logs to a Console renderer's tail, or
	// discards.
	Logger func(level, msg string)
	// AttractAfter is the idle time before the running app is suspended and
	// attract takes over. 0 disables standby entirely.
	AttractAfter time.Duration
	// StandbyAfter is how long attract may run with no presence before the
	// panel goes dark. 0 lets attract run forever.
	StandbyAfter time.Duration
	// HardDecayAfter is how long a hard dark (BACK pressed during attract)
	// must see no presence before motion can wake it again. 0 disables hard
	// dark; BACK in attract still goes dark.
	HardDecayAfter time.Duration
	// AssetOverlay resolves a mounted cartridge's bundled assets through
	// Audio's Resolver. nil is fine; every AssetOverlay method is nil-safe.
	AssetOverlay *audio.AssetOverlay
	// FlightDir enables the input flight recorder and names its dump
	// directory. Empty disables it at no cost.
	FlightDir string
	// FlightRingEvents is the recorder's ring capacity in events and
	// FlightKeep how many dumps it retains; 0 picks the default for each.
	FlightRingEvents int
	FlightKeep       int
	// Version identifies the build in a flight recording's header.
	Version string
	// RoomOrientation is the panel's top edge while idle (attract, dark) and
	// SeatedOrientation its top edge during active play. Controller
	// directions are remapped to match whichever is live. Both default to
	// display.Upright.
	RoomOrientation   display.Orientation
	SeatedOrientation display.Orientation
	// Controllers builds the HID manager over the engine's input handler.
	// New builds it; the caller starts it. nil leaves Runtime.Controllers nil.
	Controllers func(*input.Handler) *controller.Manager
	// Mic is the microphone classifier; nil on a cabinet without one.
	Mic MicModels
	// Bell rings the key-reject signal; nil is silent.
	Bell func()
	// OnPowerDown is called with a short reason when the cabinet goes dark
	// and when it shuts down, so the host can power down what the engine
	// does not own, such as a wireless pad. Engine goroutine; do not block.
	// Repeated darks with no wake between them call it once.
	OnPowerDown func(reason string)
}

// Runtime is the engine: services plus the game loop. The exported services
// are for app and test access; the loop is single-goroutine, and anything an
// HTTP handler may touch is on Remote.
type Runtime struct {
	Renderer render.Renderer
	Display  *display.Display
	Input    *input.Handler
	Audio    audio.Service
	Store    *storage.Store
	Hardware hardware.Suite
	Clock    clock.Clock
	// AssetOverlay is Options.AssetOverlay; the cartridge host registers
	// each launch's bundle on it. nil-safe.
	AssetOverlay *audio.AssetOverlay
	// Controllers is the HID manager; nil when HID isn't wired.
	Controllers *controller.Manager
	// Mic is Options.Mic, nil without a microphone. A cartridge's bundled
	// sound models are installed here for its session.
	Mic MicModels
	// Rand is the engine RNG; games must use it rather than math/rand's
	// globals so seeded runs are reproducible.
	Rand *rand.Rand

	targetFPS    int
	startApp     string
	logger       func(level, msg string)
	randSeed     int64 // never 0; flight dumps record it
	buildVersion string
	bootTime     time.Time // unlike the frame clock's zero, never reset by a load
	bell         func()

	cur       App
	curCtx    *AppContext
	suspended *suspendedApp
	hwShadow  *shadowHardware

	frames     frameClock
	halt       haltSignal
	transition transition
	requests   requests
	notes      loadNotes

	standby           *standby
	hardwareWake      atomic.Bool
	attractHold       attractHold
	darkFade          darkFade
	powerDown         powerDownNotice
	powerOff          powerOffSeq
	bootPairingWaited bool

	savers     screensaverPicker
	labPreview *labPreviewState
	lightShow  *lightShowState
	periph     periphObserver
	recent     recentNames
	flight     *flightRecorder
	replay     replayHooks

	panel              panelLevel
	orientation        orientationPolicy
	strategies         strategyStack
	lastControllerSeat input.Seat

	mem        memSample
	status     atomic.Pointer[Status]
	loadingBar *element.LoadingBar
}

// New builds a Runtime with the given options.
func New(opts Options) (*Runtime, error) {
	if opts.Renderer == nil {
		return nil, fmt.Errorf("engine: Renderer is required")
	}
	cols, rows := opts.Cols, opts.Rows
	if cols == 0 {
		cols = display.DefaultWidth
	}
	if rows == 0 {
		rows = display.DefaultHeight
	}
	fps := opts.TargetFPS
	if fps == 0 {
		fps = DefaultFPS
	}
	clk := opts.Clock
	if clk == nil {
		clk = clock.SystemClock{}
	}
	aud := opts.Audio
	if aud == nil {
		aud = audio.NewRecorder()
	}
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = "data"
	}
	seed := opts.RandSeed
	if seed == 0 {
		seed = clk.Now().UnixNano()
	}
	hw := opts.Hardware
	if hw == nil {
		hw = hardware.NewVirtual()
	}
	shadow := newShadowHardware(hw)
	rt := &Runtime{
		Rand:         rand.New(rand.NewSource(seed)),
		Renderer:     opts.Renderer,
		Display:      display.New(cols, rows, opts.Renderer),
		Input:        input.NewHandler(),
		Audio:        aud,
		Store:        storage.New(dataDir),
		Hardware:     shadow,
		hwShadow:     shadow,
		Clock:        clk,
		AssetOverlay: opts.AssetOverlay,
		targetFPS:    fps,
		startApp:     opts.StartApp,
		logger:       opts.Logger,
		randSeed:     seed,
		buildVersion: opts.Version,
		bootTime:     clk.Now(),

		orientation: orientationPolicy{room: opts.RoomOrientation, seated: opts.SeatedOrientation},

		Mic:       opts.Mic,
		bell:      opts.Bell,
		powerDown: powerDownNotice{fn: opts.OnPowerDown},
	}
	if rt.bell == nil {
		rt.bell = func() {}
	}
	if opts.Controllers != nil {
		rt.Controllers = opts.Controllers(rt.Input)
	}
	rt.requests.init()
	rt.panel.pct = int(math.Round(opts.Renderer.Brightness() * 100))
	rt.standby = newStandby(opts.AttractAfter, opts.StandbyAfter, opts.HardDecayAfter, clk.Now())
	rt.applyBaseOrientation()
	if rt.logger == nil {
		if console, ok := opts.Renderer.(*render.Console); ok {
			rt.logger = console.Log
		} else {
			rt.logger = func(string, string) {}
		}
	}
	if console, ok := opts.Renderer.(*render.Console); ok {
		console.HardwareLine = rt.Hardware.StatusLines
		console.PerfStats = rt.PerfStats
	}
	rt.flight = newFlightRecorder(opts.FlightDir, opts.FlightRingEvents, opts.FlightKeep, rt.logger)
	// Any input counts as motion for presence detection.
	if spy, ok := rt.Hardware.(hardware.InputSpy); ok {
		rt.Input.AddSpy(spy.InputSignal)
	}
	return rt, nil
}

// Log writes to the engine log sink.
func (rt *Runtime) Log(level, msg string) { rt.logger(level, msg) }

// haltSignal is the loop's stop request, raised from any goroutine and read
// at the top of each pass; an abort carries the error Run returns.
type haltSignal struct {
	stop atomic.Bool
	mu   sync.Mutex
	err  error
}

func (h *haltSignal) request() { h.stop.Store(true) }

func (h *haltSignal) abort(err error) {
	h.mu.Lock()
	h.err = err
	h.mu.Unlock()
	h.stop.Store(true)
}

func (h *haltSignal) requested() bool { return h.stop.Load() }

func (h *haltSignal) cause() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

// Quit stops the loop abruptly after the current tick; safe from any
// goroutine. A shutdown a person asked for goes through PowerOff instead.
func (rt *Runtime) Quit() { rt.halt.request() }

// QuitRequested reports whether Quit or Abort has been called.
func (rt *Runtime) QuitRequested() bool { return rt.halt.requested() }

// Abort stops the loop like Quit but makes Run return err. A non-zero exit
// is what lets systemd's Restart=on-failure bring the cabinet back. Safe
// from any goroutine.
func (rt *Runtime) Abort(err error) { rt.halt.abort(err) }

func (rt *Runtime) rejectKey(ev input.Event) {
	name := string(ev.Name)
	if ev.Kind == input.KindJoystickChange {
		name = string(ev.Position)
	}
	rt.Log("warn", fmt.Sprintf("unhandled %q event: <%s> @%s:%v", ev.Kind, name, ev.Source.Type, ev.Source.Slot))
}

// RingBell signals a rejected key.
func (rt *Runtime) RingBell() { rt.bell() }

// CurrentSlug returns the running app's slug, or "" when nothing is loaded.
func (rt *Runtime) CurrentSlug() string {
	if rt.curCtx == nil {
		return ""
	}
	return rt.curCtx.Manifest.Slug
}

// suspendedApp is a parked app with the clock and peripheral state needed to
// resume it unnoticed. Its display layers stay in the compositor, hidden by
// attract's opaque background, so element state survives.
type suspendedApp struct {
	app   App
	ctx   *AppContext
	clock parkedClock
	hw    hardwareState
}

// RequestLaunch asks the engine to launch a hot-mounted cartridge. Safe from
// any goroutine; applied next tick, latest request wins. Dropped unless the
// shell is the foreground app.
func (rt *Runtime) RequestLaunch(slug string) { rt.requests.hotMountLaunch.Set(slug) }

// RawHardware returns the real hardware suite behind Hardware's
// write-mirroring wrapper.
func (rt *Runtime) RawHardware() hardware.Suite { return rt.hwShadow.inner }

// SuspendedSlug returns the slug of the app parked behind attract, or "".
func (rt *Runtime) SuspendedSlug() string {
	if rt.suspended == nil {
		return ""
	}
	return rt.suspended.ctx.Manifest.Slug
}

// suspendCurrent parks the running app: its mappings come off the stack and
// its sounds stop; instance, layers and strategies stay. The display resets
// upright and resumeSuspended's strategy re-Init restores the rotation.
func (rt *Runtime) suspendCurrent(now time.Time) {
	if rt.cur == nil {
		return
	}
	// Recording stops but the ring is kept, marked as ending early; it does
	// not resume after the wake.
	rt.flight.suspend(uint32(rt.frames.ticks))
	rt.curCtx.detachKeyMaps()
	rt.Audio.Stop(rt.curCtx.Manifest.Slug, "")
	rt.Renderer.VirtualRotate(display.Upright)
	rt.suspended = &suspendedApp{
		app: rt.cur, ctx: rt.curCtx,
		clock: rt.frames.park(now),
		hw:    rt.hwShadow.snapshot(),
	}
	rt.Log("info", fmt.Sprintf("standby: suspending %s", rt.curCtx.Manifest.Slug))
	rt.cur, rt.curCtx = nil, nil
}

// resumeSuspended unloads the current standby app and restores the parked
// one, shifting its time base past the gap so DurationMs is seamless.
// Reports whether anything was parked.
func (rt *Runtime) resumeSuspended(now time.Time) bool {
	s := rt.suspended
	if s == nil {
		return false
	}
	rt.suspended = nil
	rt.unloadCurrent()
	rt.cur, rt.curCtx = s.app, s.ctx
	s.ctx.reattachKeyMaps()
	rt.frames.resume(s.clock, now)
	if st := rt.strategies.top(); st != nil {
		st.Init()
	}
	rt.hwShadow.restore(s.hw)
	if r, ok := rt.cur.(Resumer); ok {
		r.Resumed()
	}
	rt.Log("info", fmt.Sprintf("standby: resuming %s", rt.curCtx.Manifest.Slug))
	return true
}

// FrameBudget is the per-tick time slice.
func (rt *Runtime) FrameBudget() time.Duration {
	return time.Second / time.Duration(rt.targetFPS)
}

// Stats builds the FrameTime for the current instant.
func (rt *Runtime) Stats() clock.FrameTime {
	return rt.frames.stats(rt.Clock.Now(), rt.FrameBudget())
}

// systemMappings wires quit, rotate and brightness keys. Registered first so
// app maps pushed later take precedence.
func (rt *Runtime) systemMappings() {
	ctrl := true
	sysSet := []*input.Mapping{{
		Conditions: []input.Condition{
			{Name: ptr(input.KeyName("c")), Ctrl: &ctrl},
			{Name: ptr(input.KeyName("d")), Ctrl: &ctrl},
			input.Key(input.KeyQuit),
			input.Key(input.KeyEscape),
		},
		Trigger: func(input.Event) { rt.Quit() },
	}}
	rt.Input.AddMappings(sysSet)

	fwSet := []*input.Mapping{
		{Conditions: []input.Condition{input.KeyShift(input.KeyRotate, false)}, Trigger: func(input.Event) { rt.Renderer.RotateRight() }},
		{Conditions: []input.Condition{input.KeyShift(input.KeyRotate, true)}, Trigger: func(input.Event) { rt.Renderer.RotateLeft() }},
		{Conditions: []input.Condition{input.KeyShift(input.KeyBrightness, false)}, Trigger: func(input.Event) { rt.Renderer.DimUp() }},
		{Conditions: []input.Condition{input.KeyShift(input.KeyBrightness, true)}, Trigger: func(input.Event) { rt.Renderer.DimDown() }},
		{Conditions: []input.Condition{input.KeyShift(input.KeyRight, true)}, Trigger: func(input.Event) { rt.Renderer.RotateRight() }},
		{Conditions: []input.Condition{input.KeyShift(input.KeyLeft, true)}, Trigger: func(input.Event) { rt.Renderer.RotateLeft() }},
		{Conditions: []input.Condition{
			input.KeyShift(input.KeyUp, true),
			input.KeyShift(input.KeyDown, true),
		}, Trigger: func(input.Event) { rt.Renderer.Rotate180() }},
	}
	rt.Input.AddMappings(fwSet)
}

func ptr[T any](v T) *T { return &v }

func (rt *Runtime) loadApp(m Manifest) error {
	rt.unloadCurrent()
	// Boot never shows the bar; the splash and the LCD startup card are the
	// progress.
	quiet := rt.notes.takeQuiet() || m.Type == TypeBoot
	if !quiet {
		rt.showLoading(true)
	}
	rt.frames.reset(rt.Clock.Now())

	// Before construction: cart_init already draws host_random_u64, and the
	// tick counter has just reset to zero.
	rt.startFlightSession(m)

	ctx := &AppContext{Manifest: m, RT: rt}
	ctx.openedForSettings = rt.notes.takeSettingsOpen(m.Slug)
	app := m.New(ctx)
	rt.cur = app
	rt.curCtx = ctx

	// Decode up front so the first note doesn't stall on a WAV decode.
	rt.Audio.PreloadOwner(m.Slug)

	if m.Type == TypeBoot {
		rt.bootPairingWaited = false // a reset re-runs pairing, so re-ask the question
	}

	if m.Type != TypeBoot {
		ctx.AddKeyMap([]*input.Mapping{{
			Conditions: []input.Condition{
				input.Key(input.KeyQuit),
				input.Key(input.KeyEscape),
				input.Key(input.KeyJoyB),
			},
			Trigger: func(input.Event) { rt.requestUnload(false) },
		}})
	}

	if err := app.Init(); err != nil {
		return fmt.Errorf("app %q init: %w", m.Slug, err)
	}
	if !quiet {
		rt.showLoading(false)
	}

	stats := rt.Stats()
	rt.renderApp(stats, scene.ReasonInit)
	rt.paint(stats)
	return nil
}

func (rt *Runtime) unloadCurrent() {
	// Before the nil check: a screensaver's panel dim is scoped to its load,
	// and every route out of one passes through here.
	rt.clearPanelDim()
	if rt.cur == nil {
		return
	}
	// A ring holding two sessions replays as neither.
	rt.flight.stop()
	if sa, ok := rt.cur.(SceneApp); ok && sa.Scenes() != nil {
		sa.Scenes().Teardown()
	}
	if err := rt.cur.Teardown(); err != nil {
		rt.Log("error", fmt.Sprintf("teardown %s: %v", rt.curCtx.Manifest.Slug, err))
	}
	rt.curCtx.detachKeyMaps()
	for rt.curCtx.pushedStrategies > 0 {
		rt.curCtx.pushedStrategies--
		rt.PopPlayerStrategy()
	}
	// Apps that rotate without a strategy, like boot during pairing, would
	// otherwise leak their rotation into the next load.
	rt.Renderer.VirtualRotate(display.Upright)
	rt.Audio.Stop(rt.curCtx.Manifest.Slug, "")
	rt.Display.RemoveLayers(rt.curCtx.Manifest.Slug, "")
	rt.cur = nil
	rt.curCtx = nil
}

func (rt *Runtime) showLoading(visible bool) {
	if rt.loadingBar == nil {
		return
	}
	rt.loadingBar.SetVisible(visible)
	rt.paint(rt.Stats())
}

// wantsMic reports whether the loaded app declared the mic capability. Apps
// that don't never see a mic event, which keeps their goldens unchanged.
func (rt *Runtime) wantsMic() bool {
	if rt.curCtx == nil {
		return false
	}
	return slices.Contains(rt.curCtx.Manifest.Capabilities, CapabilityMic)
}

// LastControllerSeat returns the seat of the most recent controller event,
// or input.NoSeat before any. Read at Init, it is the seat that launched the
// app.
func (rt *Runtime) LastControllerSeat() input.Seat { return rt.lastControllerSeat }

// SetRawDirectionsOverride overrides the current app's
// Manifest.RawDirections for the rest of its instance's life, surviving
// suspend; the engine side of host_set_raw_directions. No-op with no app
// loaded.
func (rt *Runtime) SetRawDirectionsOverride(enabled bool) {
	if rt.curCtx != nil {
		rt.curCtx.rawDirOverride = &enabled
	}
}

// remapEvent applies cabinet-position remapping to controller events unless
// the app wants raw directions.
func (rt *Runtime) remapEvent(ev input.Event) input.Event {
	if ev.Source.Type != input.SourceController || rt.curCtx.rawDirections() {
		return ev
	}
	pos := input.PositionUnknown
	switch ev.Source.Seat {
	case input.SeatP1:
		pos = input.PositionLeft
	case input.SeatP2:
		pos = input.PositionRight
	}
	ev.Name = input.MapKeyDirection(pos, rt.orientation.input, ev.Name)
	if ev.Kind == input.KindJoystickChange {
		ev.Position = input.MapJoyDirection(pos, rt.orientation.input, ev.Position)
	}
	return ev
}

// Init prepares the renderer, system layers and key mappings. Split from Run
// so tests can drive ticks manually.
func (rt *Runtime) Init() error {
	if err := rt.Renderer.Init(); err != nil {
		return err
	}
	rt.systemMappings()

	rt.PushPlayerStrategy(NewAnyPlayer(rt))

	rt.Display.AddLayer("system", "scrim", display.LayerOptions{Fill: &display.Transparent})
	bar, err := element.NewLoadingBar(element.LoadingBarOptions{
		Options:  element.Options{OffsetX: 22, OffsetY: 30, Hidden: true},
		Infinite: true,
		Max:      ptr(1000.0),
	})
	if err != nil {
		return err
	}
	rt.loadingBar = bar
	rt.Display.AddLayer("system", "loading-bar", display.LayerOptions{Elements: []display.Element{bar}})

	rt.frames.reset(rt.Clock.Now())

	var first Manifest
	switch {
	case rt.startApp != "":
		m, err := Lookup(rt.startApp)
		if err != nil {
			return err
		}
		first = m
	default:
		if boot, ok := lookupType(TypeBoot); ok {
			first = boot
		} else if shell, ok := lookupType(TypeShell); ok {
			first = shell
		} else {
			return fmt.Errorf("engine: no boot or shell app registered")
		}
	}
	return rt.loadApp(first)
}

// Tick runs one frame: input, update, render, paint. Run calls it in a paced
// loop; tests call it directly.
func (rt *Runtime) Tick() error {
	rt.frames.begin(rt.Clock.Now())
	defer rt.publishStatus()
	defer rt.publishPeriph()

	// Before applyTransition, so a requested load applies this tick.
	now := rt.frames.tickStart
	rt.drainRequests(now)

	if stop, err := rt.applyTransition(); stop || err != nil {
		return err
	}

	// A power-off owns the cabinet; input during it is discarded, never a
	// wake.
	if rt.poweringOff() {
		rt.stepLampFades(uint64(rt.Stats().ElapsedMs * 1000))
		rt.Input.Drain()
		rt.stepPowerOff(now)
		rt.frames.end()
		return nil
	}

	rt.pollMotion(now)
	rt.takeHardwareWake(now)

	// Reflects last tick's standby transitions; a one-frame lag is
	// imperceptible.
	rt.applyBaseOrientation()

	// Before anything paints, so a step or expiry lands this tick.
	rt.stepLabPreview(now)
	rt.stepLightShow(now)

	if rt.cur == nil && rt.standby.state != StateDark {
		// A lab preview loads no app but must still paint; the console's
		// hardware header only refreshes on a paint call.
		if rt.LabPreviewActive() {
			rt.paint(rt.Stats())
		}
		return nil
	}

	rt.samplePerf()

	rt.orientation.input = rendererInputOrientation(rt.Renderer)

	stats := rt.Stats()

	// The dt_micros cart_update will see; recording it rather than the frame
	// budget makes a replay keep the cabinet's real pace, drops included.
	// Lamp fades ride the same delta so a replay reproduces them.
	dtMicros := uint64(stats.ElapsedMs * 1000)
	rt.noteFlightFrame(dtMicros)

	rt.stepLampFades(dtMicros)
	rt.stepWakeBloom(dtMicros)

	rt.dispatchInput(now)

	// Before the idle step, so a completed hold isn't pre-empted by dark.
	rt.stepAttractHold(now)

	// Before the idle step, so a screensaver still due suppresses this
	// tick's dark.
	rt.stepScreensaver(now)

	rt.stepStandby(now)

	if rt.cur == nil { // a mapping, or a standby transition, unloaded the app
		rt.paintWithoutApp(now, stats)
		return nil
	}
	rt.cur.HandleInput(stats)
	if rt.cur == nil { // an input handler suspended or unloaded the app (e.g. shell SLEEP)
		rt.paintWithoutApp(now, stats)
		return nil
	}

	if sa, ok := rt.cur.(SceneApp); ok && sa.Scenes() != nil {
		sa.Scenes().Update(stats)
	}
	rt.cur.Update(stats)

	rt.renderApp(stats, scene.ReasonTick)
	rt.paint(stats)

	rt.frames.end()
	return nil
}

// dispatchInput drains the tick's input. Outside ACTIVE the standby router
// decides what each event does; in ACTIVE the player strategy gates it and
// the app's mappings receive it.
func (rt *Runtime) dispatchInput(now time.Time) {
	for _, ev := range rt.Input.Drain() {
		if input.IsMicKey(ev.Name) && (rt.standby.state != StateActive || !rt.wantsMic()) {
			continue
		}
		if ev.Source.Type == input.SourceController {
			rt.lastControllerSeat = ev.Source.Seat
		}
		// Strategy gating applies only while ACTIVE, so a suspended game's
		// locked strategy can't stop the other seat waking the cabinet.
		if rt.standby.state == StateActive && !rt.gateInput(ev) {
			continue
		}
		if rt.standby.state != StateActive {
			switch rt.routeStandbyInput(now, ev) {
			case routeDrop:
				continue
			case routeStop:
				return
			}
			// Post-remap and before the wake, so the waking press is observed
			// too; then the rest of the press's events are dropped so they
			// can't act on the app that just came back.
			rt.notePeriphInput(rt.remapEvent(ev))
			rt.leaveStandby(now, resumeParked)
			return
		}
		remapped := rt.remapEvent(ev)
		rt.notePeriphInput(remapped)
		rt.standby.noteInput(now)
		if !rt.Input.Dispatch(remapped) {
			rt.rejectKey(ev)
		}
	}
}

// paintWithoutApp paints the tick that has no app loaded: the power-off
// sequence, a lab preview's frame, or the goodnight fade.
func (rt *Runtime) paintWithoutApp(now time.Time, stats clock.FrameTime) {
	switch {
	case rt.stepPowerOff(now):
	case rt.LabPreviewActive():
		rt.paint(stats)
	default:
		rt.stepDarkFade(now)
	}
}

// paint sends a lab preview frame instead of the composited display when one
// owns the panel. The swap is at the last step, so the compositor carries on
// untouched and the app reappears the instant the preview ends.
func (rt *Runtime) paint(stats clock.FrameTime) {
	if rt.labPreview.hasFrames() {
		rt.Renderer.Paint(rt.labPreview.frame(), stats)
		return
	}
	rt.Display.Paint(stats)
}

func blackFrame(w, h int) display.Matrix {
	return display.NewMatrix(w, h, display.Black)
}

func (rt *Runtime) renderApp(stats clock.FrameTime, reason RenderReason) {
	if rt.cur == nil {
		return
	}
	if sa, ok := rt.cur.(SceneApp); ok && sa.Scenes() != nil {
		sa.Scenes().Render(stats, reason)
	}
	rt.cur.Render(stats, reason)
}

// Run executes Init then the paced game loop until Quit.
func (rt *Runtime) Run() error {
	if err := rt.Init(); err != nil {
		return err
	}
	defer func() {
		rt.unloadCurrent()
		// A suspended app still owns resources, so it gets the same full
		// unload.
		if s := rt.suspended; s != nil {
			rt.suspended = nil
			rt.cur, rt.curCtx = s.app, s.ctx
			rt.unloadCurrent()
		}
		_ = rt.Renderer.Close()
	}()

	budget := rt.FrameBudget()
	next := rt.Clock.Now().Add(budget)
	for !rt.halt.requested() {
		if err := rt.Tick(); err != nil {
			return err
		}
		if sleep := next.Sub(rt.Clock.Now()); sleep > 0 {
			rt.Clock.Sleep(sleep)
			next = next.Add(budget)
		} else {
			for next.Sub(rt.Clock.Now()) <= 0 {
				next = next.Add(budget)
				rt.frames.dropped++
			}
		}
	}
	return rt.halt.cause()
}
