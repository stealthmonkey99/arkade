// SPDX-License-Identifier: GPL-2.0-or-later

// Command arkade runs the Årkade engine: a 64×64 arcade for an RGB LED
// matrix cabinet, rendered to the terminal for desktop development.
//
//	arkade                      # boot → shell, console renderer
//	arkade --app apps/clock     # boot straight into a native app
//	arkade --headless --ticks 90 --dump-frame out.txt
//	arkade --replay data/flight/…-cart_snake-crash.arkflt
//
// Every cartridge session is recorded to a bounded in-memory ring that
// spills to <data>/flight; --replay re-lives a recording (see
// docs/FLIGHT_RECORDER.md) and --flight=off disables the recorder.
//
// Idle --attract-after hands off to the attract screensaver; a further
// --standby-after with no motion turns the panel off. A second BACK during
// attract goes "hard" dark, which only a button press wakes until
// --hard-standby-decay of no motion. A zero duration disables that step.
//
// Keyboard controls (dev "test controller"):
//
//	arrows      d-pad         k/l/i/o   JOY A/B/D/C buttons
//	enter/space select        1 / 2     drive player 1 / player 2
//	r / R       rotate        b / B     brightness up / down
//	q / esc     back / quit   ctrl-c    hard quit
package main

import (
	"context"
	"flag"
	"fmt"
	stdnet "net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	bluetoothapp "github.com/stealthmonkey99/arkade/apps/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/audio/device"
	"github.com/stealthmonkey99/arkade/internal/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/config"
	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/controller/hiddev"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/hardware/pi"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/mic"
	netsvc "github.com/stealthmonkey99/arkade/internal/net"
	"github.com/stealthmonkey99/arkade/internal/render"
	"github.com/stealthmonkey99/arkade/internal/render/ledpanel"
	"github.com/stealthmonkey99/arkade/internal/storage"

	_ "github.com/stealthmonkey99/arkade/apps/attract"
	_ "github.com/stealthmonkey99/arkade/apps/boot"
	_ "github.com/stealthmonkey99/arkade/apps/clock"
	_ "github.com/stealthmonkey99/arkade/apps/config"
	_ "github.com/stealthmonkey99/arkade/apps/evening"
	_ "github.com/stealthmonkey99/arkade/apps/morning"
	_ "github.com/stealthmonkey99/arkade/apps/shell"
)

// VariantControl is optional in internal/net, so a drifted signature would
// silently drop the dashboard toggle instead of failing the build.
var _ netsvc.VariantControl = (*cartridge.RemoteControl)(nil)

// version is reported by /api/status; the build does not stamp it.
var version = "dev"

// defaultHTTPAddr is loopback-only because the dashboard's read routes are
// unauthenticated; exposing them on the LAN is an explicit --http opt-in.
const defaultHTTPAddr = "127.0.0.1:8080"

func main() {
	var (
		cols       = flag.Int("cols", 64, "display columns")
		rows       = flag.Int("rows", 64, "display rows")
		fps        = flag.Int("fps", 30, "target frames per second")
		app        = flag.String("app", "", "launch directly into app by slug (skip boot)")
		list       = flag.Bool("list", false, "list registered apps and exit")
		headless   = flag.Bool("headless", false, "run without a display (capture renderer)")
		ticks      = flag.Int("ticks", 300, "headless: number of frames to run")
		dumpFrame  = flag.String("dump-frame", "", "headless: write final frame as ASCII to this file ('-' = stdout)")
		seed       = flag.Int64("seed", 0, "RNG seed (0 = time-based)")
		dataDir    = flag.String("data", "data", "storage directory for settings/scores")
		brightness = flag.Int("brightness", 100, "initial brightness percent (0-100, floored at engine.MinPanelBrightness; overridden by a persisted setting once the config app has saved one)")
		assetsDir  = flag.String("assets", "assets", "root for audio assets")
		mute       = flag.Bool("mute", false, "disable audio output")
		noHID      = flag.Bool("no-hid", false, "disable USB HID gamepad support")
		micDevice  = flag.String("mic", "", "ALSA capture device for acoustic input, e.g. plughw:0,0 "+
			"(empty disables the microphone entirely). Only apps declaring the \"mic\" capability "+
			"receive events; the cabinet computes a gesture and discards the audio.")
		micNoCancel      = flag.Bool("mic-no-cancel", false, "microphone: skip echo cancellation (the raw capture reaches the detectors)")
		micClap          = flag.Bool("mic-clap", false, "microphone: enable the clap detector (experimental; its thresholds are untuned)")
		micBlow          = flag.Bool("mic-blow", false, "microphone: enable the blow detector (experimental; needs a microphone the player can breathe on)")
		led              = flag.Bool("led", false, "render to the RGB LED matrix (build-pi builds only)")
		ledMap           = flag.String("led-mapping", "regular", "LED matrix hardware mapping (regular, adafruit-hat, adafruit-hat-pwm)")
		ledSlow          = flag.Int("led-slowdown", 3, "LED matrix GPIO slowdown")
		attractAfter     = flag.Duration("attract-after", 5*time.Minute, "idle time before the attract screensaver takes over (0 disables standby entirely)")
		standbyAfter     = flag.Duration("standby-after", 5*time.Minute, "time in attract with no motion before the panel goes fully dark (0 disables just this step)")
		hardStandbyDecay = flag.Duration("hard-standby-decay", 10*time.Minute, "time with no motion before a hard dark (back pressed while attracting) decays to soft, where motion wakes it again (0 disables hard mode entirely)")
		httpAddr         = flag.String("http", defaultHTTPAddr, "monitoring dashboard listen address; defaults to loopback-only "+
			"since its read routes are unauthenticated by design. Pass ':8080' or a specific "+
			"LAN address to deliberately expose it beyond this machine instead; empty disables the dashboard entirely "+
			"(always disabled under --headless). Prefer an SSH tunnel (`ssh -L 8080:127.0.0.1:8080 <cabinet>`) over LAN "+
			"exposure when remote access is only occasional")
		httpAllowedHosts = flag.String("http-allowed-hosts", "", "extra comma-separated Host header names the dashboard "+
			"accepts beyond \"localhost\" and literal IP addresses — only needed if --http "+
			"exposes the dashboard beyond loopback and it's reached by a name (e.g. \"cabinet.local\") rather than an IP; "+
			"unrecognized Host headers are rejected (421) to block DNS-rebinding attacks")
		cartridges = flag.String("cartridges", "", "comma-separated cartridge scan directories, glob patterns allowed "+
			"(default \"<data>/cartridges\"; USB auto-mount paths like \"/media/*\" are also useful here — see internal/cartridge's package doc)")
		flightDir = flag.String("flight", "", "directory for input flight recordings (default \"<data>/flight\"); "+
			"empty string via --flight=off disables the recorder entirely (see docs/FLIGHT_RECORDER.md)")
		flightKeep = flag.Int("flight-keep", 0, "how many flight recordings to retain before rotating the oldest out (0 = default)")
		replay     = flag.String("replay", "", "replay a flight recording instead of running the cabinet: "+
			"loads the recorded cartridge, verifies its wasm hash and re-lives the session frame by frame")
		replayForce  = flag.Bool("replay-force", false, "replay even when the installed cartridge's wasm hash differs from the recording's")
		replayPaused = flag.Bool("replay-paused", false, "start the replay paused (space resumes, '.' steps one frame, 'q' quits)")
	)
	flag.Parse()

	cartDirs := cartridgeDirs(*cartridges, *dataDir)

	// Replay never opens hardware, the dashboard or the recorder.
	if *replay != "" {
		if err := runReplay(replayOptions{
			path: *replay, force: *replayForce, paused: *replayPaused,
			dataDir: *dataDir, cartridgeDirs: cartDirs,
			cols: *cols, rows: *rows, interactive: true,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "arkade --replay:", err)
			os.Exit(1)
		}
		return
	}

	if *list {
		// Scan so --list includes mounted cartridges.
		cartridge.NewRegistry(cartridge.Options{
			Dirs: cartDirs, Width: *cols, Height: *rows,
			Logger: func(level, msg string) { fmt.Fprintf(os.Stderr, "[%s] %s\n", level, msg) },
		}).Scan()
		for _, m := range engine.Apps() {
			fmt.Printf("%-18s %s — %s\n", m.Slug, m.Name, m.Description)
		}
		return
	}

	if err := run(runOptions{
		cols: *cols, rows: *rows, fps: *fps, app: *app,
		headless: *headless, ticks: *ticks, dumpFrame: *dumpFrame,
		seed: *seed, dataDir: *dataDir, brightness: *brightness,
		assetsDir: *assetsDir, mute: *mute, noHID: *noHID, micDevice: *micDevice, micNoCancel: *micNoCancel, micClap: *micClap, micBlow: *micBlow,
		led: *led, ledMapping: *ledMap, ledSlowdown: *ledSlow,
		attractAfter: *attractAfter, standbyAfter: *standbyAfter,
		hardStandbyDecay: *hardStandbyDecay,
		httpAddr:         *httpAddr, httpAllowedHosts: splitCSV(*httpAllowedHosts),
		cartridgeDirs: cartDirs,
		flightDir:     flightRecordDir(*flightDir, *dataDir), flightKeep: *flightKeep,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "arkade:", err)
		os.Exit(1)
	}
}

// splitCSV splits a comma-separated flag value into trimmed, non-empty
// entries, or nil if there are none.
func splitCSV(flagValue string) []string {
	var out []string
	for _, v := range strings.Split(flagValue, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// cartridgeDirs splits --cartridges into scan directories or glob patterns,
// defaulting to "<data>/cartridges".
func cartridgeDirs(flagValue, dataDir string) []string {
	if dirs := splitCSV(flagValue); dirs != nil {
		return dirs
	}
	return []string{filepath.Join(dataDir, "cartridges")}
}

// flightRecordDir resolves --flight to the recorder's dump directory:
// "<data>/flight" by default, "" (disabled) for --flight=off. Default-on is
// deliberate; a recorder that must be enabled is off when the bug shows up.
func flightRecordDir(flagValue, dataDir string) string {
	switch flagValue {
	case "off", "none":
		return ""
	case "":
		return filepath.Join(dataDir, "flight")
	}
	return flagValue
}

func containsDir(dirs []string, dir string) bool {
	for _, d := range dirs {
		if d == dir {
			return true
		}
	}
	return false
}

// httpAddrIsLoopback reports whether addr is reachable only from this
// machine; an empty host (":8080") means all interfaces.
func httpAddrIsLoopback(addr string) bool {
	host, _, err := stdnet.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := stdnet.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type runOptions struct {
	cols, rows, fps  int
	app              string
	headless         bool
	ticks            int
	dumpFrame        string
	seed             int64
	dataDir          string
	brightness       int
	assetsDir        string
	mute, noHID, led bool
	micDevice        string
	micNoCancel      bool
	micClap, micBlow bool
	ledMapping       string
	ledSlowdown      int
	attractAfter     time.Duration
	standbyAfter     time.Duration
	hardStandbyDecay time.Duration
	httpAddr         string
	httpAllowedHosts []string
	cartridgeDirs    []string
	flightDir        string
	flightKeep       int
}

func run(o runOptions) error {
	loadDotEnv(".env")
	if o.headless {
		return runHeadless(o.cols, o.rows, o.fps, o.app, o.ticks, o.dumpFrame, o.seed, o.dataDir, o.attractAfter, o.standbyAfter, o.hardStandbyDecay, o.cartridgeDirs)
	}

	var rend render.Renderer
	var console *render.Console
	logSink := func(level, msg string) { fmt.Fprintf(os.Stderr, "[%s] %s\n", level, msg) }
	if o.led {
		panel, err := ledpanel.New(ledpanel.Options{
			Cols:            o.cols,
			Rows:            o.rows,
			HardwareMapping: o.ledMapping,
			GPIOSlowdown:    o.ledSlowdown,
		})
		if err != nil {
			return err
		}
		rend = panel
	} else {
		console = render.NewConsole()
		rend = console
		logSink = console.Log
	}

	// Wrapped before any subsystem gets logSink so the dashboard ring sees all of them.
	logRing := netsvc.NewRing(0)
	baseLog := logSink
	logSink = func(level, msg string) {
		baseLog(level, msg)
		logRing.Append(level, msg)
	}

	cab, err := config.Load(storage.New(o.dataDir))
	if err != nil {
		logSink("warn", fmt.Sprintf("settings: %v; running on the defaults", err))
	}

	// Resolves mounted .rcart bundle assets that never reach disk.
	assetOverlay := audio.NewAssetOverlay()
	sound, closeAudio := openAudio(o.assetsDir, cab, o.mute, logSink, assetOverlay)
	defer closeAudio()

	var hw hardware.Suite
	piSuite, piErr := pi.New(logSink, pi.LoadConfig(o.dataDir))
	if piErr == nil {
		hw = piSuite
		defer piSuite.Close()
		logSink("info", fmt.Sprintf("hardware: lamp driver %s, fan driver %s", piSuite.LampDriver(), piSuite.FanDriver()))
		// Before the settings app runs, so the fan follows the operator from boot.
		piSuite.SetFanConfig(float64(cab.FanThresholdF), cab.FanMode)
		startupCard(piSuite, stageEngine)
	}

	// The output tap lets the capture cancel the cabinet's own playback.
	var capture *mic.Capture
	if o.micDevice != "" {
		det := mic.DefaultConfig()
		det.EnableClap, det.EnableBlow = o.micClap, o.micBlow
		capture = mic.NewCapture(mic.Options{
			Device:   o.micDevice,
			Detector: det,
			NoCancel: o.micNoCancel,
			Log:      func(msg string) { logSink("info", msg) },
		})
		if m, ok := sound.(*audio.Mixer); ok {
			m.SetOutputTap(capture.OutputTap())
		} else {
			logSink("info", "mic: no playback to cancel (audio muted or unavailable)")
		}
		switch {
		case piErr == nil:
			piSuite.AttachMic(capture)
		default:
			v := hardware.NewVirtual()
			v.AttachMic(capture)
			hw = v
		}
	}

	// rt is captured by pad callbacks that only fire after the manager starts.
	var rt *engine.Runtime
	var onPowerDown func(reason string)
	var buildControllers func(*input.Handler) *controller.Manager
	if !o.noHID {
		if enum, err := hiddev.New(); err != nil {
			logSink("warn", fmt.Sprintf("HID disabled: %v", err))
		} else {
			// Drops links, never pairings, so idle pads stop draining batteries.
			pairer := bluetooth.New()
			defer func() {
				if n, err := pairer.DisconnectAll(context.Background()); err != nil {
					logSink("warn", fmt.Sprintf("bluetooth: disconnect on shutdown: %v", err))
				} else if n > 0 {
					logSink("info", fmt.Sprintf("bluetooth: disconnected %d device(s) on shutdown", n))
				}
				_ = pairer.Close()
			}()
			defer enum.Close()
			onPowerDown = func(reason string) {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					n, err := pairer.DisconnectAll(ctx)
					switch {
					case err != nil:
						logSink("warn", fmt.Sprintf("bluetooth: disconnect on %s: %v", reason, err))
					case n > 0:
						logSink("info", fmt.Sprintf("bluetooth: disconnected %d device(s) on %s", n, reason))
					}
				}()
			}
			buildControllers = func(h *input.Handler) *controller.Manager {
				manager := controller.NewManager(h, enum, logSink)
				// Before Start, which scans immediately.
				for id, seat := range loadPadSeats(o.dataDir) {
					manager.RememberSeat(id, input.SeatOf(seat))
				}
				manager.OnAttach(func(controller.Attached) { rt.WakeFromHardware() })
				manager.OnSeatClaim(func(device string, seat input.Seat) {
					rt.Hardware.SetText(seat.Index(), device, fmt.Sprintf("= PLAYER %d", seat.Number()))
				})
				return manager
			}
		}
	}
	var micModels engine.MicModels
	if capture != nil {
		micModels = capture
	}

	rt, err = engine.New(engine.Options{
		Renderer:          rend,
		Cols:              o.cols,
		Rows:              o.rows,
		TargetFPS:         o.fps,
		StartApp:          o.app,
		RandSeed:          o.seed,
		DataDir:           o.dataDir,
		Audio:             sound,
		Hardware:          hw,
		Logger:            logSink,
		AttractAfter:      o.attractAfter,
		StandbyAfter:      o.standbyAfter,
		HardDecayAfter:    o.hardStandbyDecay,
		AssetOverlay:      assetOverlay,
		RoomOrientation:   cab.RoomOrientation,
		SeatedOrientation: cab.SeatedOrientation,
		FlightDir:         o.flightDir,
		FlightKeep:        o.flightKeep,
		Version:           version,
		Controllers:       buildControllers,
		Mic:               micModels,
		Bell:              func() { fmt.Fprint(os.Stderr, "\a") },
		OnPowerDown:       onPowerDown,
	})
	if err != nil {
		return err
	}
	if o.app != "" {
		// Skipping boot skips the screen that fades the backlights in.
		rt.SetBacklightLevel(hardware.LampLevelMax)
	}
	if o.flightDir != "" {
		logSink("info", fmt.Sprintf("flight: recording cartridge sessions to %s (replay with `arkade --replay <file>`)", o.flightDir))
	}
	rt.SetPanelBrightness(cab.PanelBrightness(o.brightness))
	rt.SetLampPolicy(cab.Lamps)

	// installDir receives remote installs and is always scanned, even when
	// --cartridges names only USB globs.
	installDir := filepath.Join(o.dataDir, "cartridges")
	scanDirs := o.cartridgeDirs
	if !containsDir(scanDirs, installDir) {
		scanDirs = append(scanDirs, installDir)
	}
	cartReg := cartridge.NewRegistry(cartridge.Options{
		Dirs: scanDirs, Width: o.cols, Height: o.rows, Logger: logSink,
	})
	cartWatcher := cartridge.NewWatcher(cartReg, 0) // 0 = cartridge.DefaultPollInterval
	cartWatcher.OnHotAdd = func(slugs []string) { rt.RequestLaunch(slugs[0]) }
	cartWatcher.Start()
	defer cartWatcher.Stop()
	startupCard(piSuite, stageCartridges)
	// Not in scanDirs: a stored variant is inert until installed.
	variantsDir := filepath.Join(o.dataDir, cartridge.VariantsDirName)
	cartControl, err := cartridge.NewRemoteControl(cartReg, rt, installDir, variantsDir)
	if err != nil {
		return err
	}
	cartControl.SetLogger(logSink)

	// A bind failure only logs. ARKADE_HTTP_TOKEN gates the mutating routes,
	// which fail closed without it.
	if o.httpAddr != "" {
		httpToken := os.Getenv("ARKADE_HTTP_TOKEN")
		if err := netsvc.CheckToken(httpToken); err != nil {
			logSink("error", fmt.Sprintf("http: %v; remote control routes are disabled", err))
			httpToken = ""
		} else if httpToken == "" {
			logSink("warn", "http: ARKADE_HTTP_TOKEN not set; remote cartridge install/launch routes are disabled")
		}
		if !httpAddrIsLoopback(o.httpAddr) {
			logSink("warn", fmt.Sprintf("http: %s is not loopback-only — the dashboard's read routes (unauthenticated by design) are reachable from the LAN", o.httpAddr))
		}
		httpSrv := netsvc.NewServer(o.httpAddr, rt, logRing, rt.Store, version, httpToken, cartControl, o.httpAllowedHosts...)
		if err := httpSrv.Start(); err != nil {
			logSink("warn", fmt.Sprintf("http: %v (dashboard disabled)", err))
		} else {
			logSink("info", fmt.Sprintf("http: dashboard at http://%s", httpSrv.Addr()))
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = httpSrv.Stop(ctx)
			}()
		}
		startupCard(piSuite, stageDashboard)
	}

	keyboard := input.NewKeyboard(rt.Input)
	if err := keyboard.Start(); err != nil {
		return fmt.Errorf("keyboard: %w", err)
	}
	defer keyboard.Stop()

	// Events arrive as input.KeyMic* keypresses, withheld from apps without the "mic" capability.
	if capture != nil {
		capture.SetEvents(rt.Input.Events())
		capture.SetRecalibrate(func() { go func() { _ = rt.PlayStinger() }() })
		micCtx, cancelMic := context.WithCancel(context.Background())
		defer cancelMic()
		go func() { _ = capture.Run(micCtx) }()
		startupCard(piSuite, stageMicrophone)
	}

	if rt.Controllers != nil {
		rt.Controllers.Start()
		defer rt.Controllers.Stop()
	}
	if console != nil {
		// All three modes are shown so the highlight, not the text, carries the state.
		chip := func(label string, active bool, bg int) string {
			if active {
				return fmt.Sprintf("\x1b[1;30;%dm %s \x1b[0m", bg, label)
			}
			return fmt.Sprintf("\x1b[90m %s \x1b[0m", strings.ToLower(label))
		}
		console.InputBadge = func() string {
			sim := keyboard.SimIndex()
			return chip("⌨ KBD", sim < 0, 47) + // white: plain keyboard
				chip("🕹 P1", sim == 0, 46) + // cyan
				chip("🕹 P2", sim == 1, 45) // magenta
		}
	}

	// A clean stop lets deferred closes blank the matrix and show the LCD power-off message.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		rt.Quit()
	}()
	defer signal.Stop(sigs)

	err = rt.Run()
	keyboard.Stop()
	if err == nil {
		fmt.Println("\nThanks for playing!")
	}
	return err
}

// loadDotEnv applies KEY=VALUE lines from a .env file; a missing file is fine.
func loadDotEnv(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	loaded := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		// Only a matching quote pair is stripped, so a value merely ending in " survives.
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		key = strings.TrimSpace(key)
		_ = os.Setenv(key, value)
		loaded++
	}
	if loaded > 0 {
		fmt.Fprintf(os.Stderr, "loaded %d keys from .env\n", loaded)
	}
}

// openAudio builds the mixer over the assets dir at the cabinet's volume,
// falling back to the recording backend when muted or when no output device
// opens.
func openAudio(assetsDir string, cab config.Cabinet, mute bool, log func(level, msg string), overlay *audio.AssetOverlay) (audio.Service, func()) {
	if mute {
		return audio.NewRecorder(), func() {}
	}
	mixer := audio.NewMixer(overlay.Resolver(audio.DirResolver(assetsDir)), audio.DirLister(assetsDir), log)
	out, err := device.Open(mixer)
	if err != nil {
		log("warn", fmt.Sprintf("audio disabled: %v", err))
		return audio.NewRecorder(), func() {}
	}
	mixer.SetVolumePercent(cab.Volume)
	mixer.SetMuted(cab.Muted)
	return mixer, func() { _ = out.Close() }
}

// loadPadSeats reads the Bluetooth app's remembered seats. It runs here
// because a pad switched on after a reboot attaches before that app has
// ever opened, and would otherwise land unseated.
func loadPadSeats(dataDir string) map[string]int {
	var settings struct {
		Seats map[string]int `json:"seats"`
	}
	if err := storage.New(dataDir).Read(bluetoothapp.Slug, "", &settings); err != nil {
		return nil
	}
	return settings.Seats
}

// runHeadless drives the engine against the capture renderer used by the
// golden tests, for CI smoke runs and frame dumps.
func runHeadless(cols, rows, fps int, app string, ticks int, dumpFrame string, seed int64, dataDir string, attractAfter, standbyAfter, hardStandbyDecay time.Duration, cartridgeDirs []string) error {
	logSink := func(level, msg string) { fmt.Fprintf(os.Stderr, "[%s] %s\n", level, msg) }

	// A single scan suffices for a short run; no watcher.
	cartridge.NewRegistry(cartridge.Options{
		Dirs: cartridgeDirs, Width: cols, Height: rows, Logger: logSink,
	}).Scan()

	cap := render.NewCapture(false)
	// Advanced one frame budget per tick, so runs are deterministic.
	clk := clock.NewManualClock()
	cab, _ := config.Load(storage.New(dataDir))
	rt, err := engine.New(engine.Options{
		Renderer:          cap,
		Cols:              cols,
		Rows:              rows,
		TargetFPS:         fps,
		StartApp:          app,
		RandSeed:          seed,
		Clock:             clk,
		Audio:             audio.NewRecorder(),
		DataDir:           dataDir,
		Logger:            logSink,
		AttractAfter:      attractAfter,
		StandbyAfter:      standbyAfter,
		HardDecayAfter:    hardStandbyDecay,
		RoomOrientation:   cab.RoomOrientation,
		SeatedOrientation: cab.SeatedOrientation,
	})
	if err != nil {
		return err
	}
	if err := rt.Init(); err != nil {
		return err
	}
	for i := 0; i < ticks; i++ {
		clk.Advance(rt.FrameBudget())
		if err := rt.Tick(); err != nil {
			return err
		}
	}
	frame := cap.LastFrame()
	if frame == nil {
		return fmt.Errorf("headless run painted no frames")
	}
	fmt.Fprintf(os.Stderr, "headless: %d ticks, %d dropped, final frame %dx%d\n",
		frame.Stats.Ticks, frame.Stats.DroppedFrames, len(frame.Data[0]), len(frame.Data))
	if dumpFrame != "" {
		ascii := render.ASCII(frame.Data)
		if dumpFrame == "-" {
			fmt.Print(ascii)
			return nil
		}
		return os.WriteFile(dumpFrame, []byte(ascii), 0o644)
	}
	return nil
}
