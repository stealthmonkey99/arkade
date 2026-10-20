# Architecture

How the engine repo is laid out, and the pieces a contributor meets first:
audio, headless runs and the golden-frame tests. Every package has a doc
comment, so `go doc ./internal/<name>` is the next step from any line below.

## Layout

```text
cmd/                 binaries
  arkade               the engine: console, headless and (build-pi) LED renderers
  arkade-lcdboot       owns the LCDs before the engine starts and after it crashes
  arkadepreview        pushes a sprite, WAV or LCD glyph at a running cabinet (go build ./cmd/arkadepreview)
  micmodel             builds and inspects sound models from a directory of recordings
  micprobe             runs the microphone pipeline alone and prints what the detectors hear
apps/                native, non-game apps
  attract              the default screensaver: high scores over an ambient animation
  bluetooth            wireless pad pairing and seat assignment
  boot                 splash and controller pairing
  clock                analog, digital and calendar faces
  config               the cabinet settings menu
  evening, morning     scheduled screensavers (docs/SCREENSAVERS.md)
  screensaver          what the scheduled screensavers share
  shell                the game browser
internal/
  engine               game loop, app lifecycle, registry, standby
  cartridge            discovers .rcart/.wasm cartridges and registers them as apps
  cartridge/wasm       the wazero host that runs a cartridge (the ABI's engine side)
  display              pixels, layers and the compositor
  render               console (ANSI), capture (data-only) and LED panel renderers
  element              retained-mode UI elements, the cabinet side of sdk/ui
  scene                scene state machine and transitions
  fsm                  a tiny string-keyed state machine for native apps
  input                events and mapping-stack dispatch; the keyboard controller
  controller           HID gamepads: profiles, decoding, hot-plug
  bluetooth            BlueZ pairing over D-Bus
  audio                the sound service: mixer, native device, recorder
  mic, dsp             microphone capture, echo cancellation and recognition
  hardware             cabinet peripherals: pi (real I2C/GPIO) and Virtual
  lcdrune              which runes a cartridge may define LCD glyphs on
  flight               the input flight recorder's file format (docs/FLIGHT_RECORDER.md)
  net                  the dashboard and HTTP API (docs/DASHBOARD.md)
  config               the cabinet's settings.json schema and migrations
  storage              atomic JSON persistence under data/
  scores               per-game top-10 high-score tables
  weather              Open-Meteo client and IP location lookup
  brand                the Årkade wordmark artwork
  clock                frame timing and an injectable time source
  testkit, goldens     the golden-frame test rig and file format
assetfile/           the asset envelope (docs/ASSETS.md) shared by tools and hosts
carttest/            host-side test harness for wasm cartridges
sdk/                 cartridge authoring SDK (sdk/README.md): cartlib, ui, fsm, levels,
                     template, tutorial, tools/rcartpack, tools/manifestgen, docs
tools/               build-time tools: wasmfix, genlevelpack, preflight, sfxlib,
                     musiclib, speech, assetdiff, pngchunk, fleet, manualicon
scripts/             Pi setup, systemd units, udev rules, licence check
third_party/         rpi-rgb-led-matrix (header; the library is cloned on the Pi)
docs/                this documentation
```

Every game is a cartridge: a wasm module loaded at runtime, built from its own
Go module with TinyGo. `apps/` holds only the cabinet's own non-game apps,
which are compiled into the engine.

## Audio

Native playback goes through a beep mixer feeding oto (CoreAudio on macOS,
ALSA on Linux). WAVs decode once into RAM; one-shot effects and looping music
mix at 44.1 kHz with a 20 ms device buffer. Cartridges carry their own sound
files inside the `.rcart` bundle; engine-side WAVs resolve against `--assets`
(default `assets/`). Volume and mute live in the config app's settings and
apply live. `--mute`, a missing audio device or headless mode swaps in the
recording backend, the audio equivalent of the capture renderer, which tests
assert against.

## Headless mode

The `render.Capture` renderer records frames as data instead of drawing them.
The CLI exposes it for smoke runs and frame dumps:

```sh
./arkade --headless --ticks 300 --app apps/clock --dump-frame -
```

Headless runs use a manual clock (one frame budget per tick); pass `--seed`
for a fixed RNG, and the same invocation produces the same frames. `make
smoke` boots to the shell for 150 frames this way.

## Testing

```sh
make test       # format, licence and binary checks, the engine suite, every SDK module
make goldens    # regenerate golden frames after an intentional visual change
```

`go test ./...` at the root covers only the engine module; the SDK packages
are separate modules (see `sdk/README.md`), which is why `make test` is the
command to run. The other SDK modules have their own targets
(`make sdk-tutorial-test`, `sdk-levels-test`).

The golden-frame harness (`internal/testkit`) boots the real engine against
the capture renderer with a manual clock and seeded RNG, scripts input, and
compares composited frames pixel for pixel against ASCII-art files in
`testdata/golden/`:

```go
rig := testkit.NewRig(t, testkit.Options{StartApp: "apps/clock"})
rig.TickFor("600ms")       // ride out the intro fade-in
rig.Golden("digital-face") // testdata/golden/digital-face.txt
rig.Press(input.KeyEnter)
rig.Tick(10)
rig.Golden("analog-face")
```

Every frame flows through the same rotation and brightness pipeline the LED
matrix uses, so a golden failure means the pixels really changed. Goldens are
the oracle: a refactor that moves one is a behaviour change. Regenerate them
only for a change you intended, and review the diff of every regenerated file.
Cartridges use the same file format through `carttest`.

CI runs the same checks; [CI.md](CI.md) lists the jobs.
