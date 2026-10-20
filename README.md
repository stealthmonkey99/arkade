<p align="center">
  <img src="docs/arkade.svg" alt="Årkade" width="240">
</p>

# Årkade

A Go arcade engine for a 64×64 RGB LED matrix cabinet on a Raspberry Pi,
built around two hard requirements: a sustained **30 FPS** on the Pi and
**native audio** with no external player process. Games are WebAssembly
cartridges: the same `.wasm` runs on the cabinet and in the browser at
[arkade.build](https://arkade.build), loaded at runtime with no engine rebuild.

## The pieces

- **This repo**: the engine, the native apps (shell, clock, settings,
  screensavers), the cartridge SDK and the build tools.
- **[arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)**:
  the games, each with its own player manual.
- **The labs** (sprite, sound, level, HID and more) are hosted at
  [arkade.build/dev/tools](https://arkade.build/dev/tools).

The cabinet itself is a Pi, the LED panel, two character LCDs, lit arcade
buttons, a motion sensor and an optional microphone; [docs/CABINET.md](docs/CABINET.md)
lists the parts.

## Requirements

- **Go 1.23.4 or newer** for the engine, as `go.mod` says; the Pi builds it
  with 1.23.4.
- On Linux, cgo and the ALSA and udev headers: `libasound2-dev` and
  `libudev-dev`.
- To build cartridges: **TinyGo 0.42.0**, which needs **Go 1.25 to 1.27**. Host-side
  cartridge tests run on the engine's Go.
- On macOS, build with `make build`, not plain `go build`: with Go 1.23's
  internal linker, dyld rejects the binary ("missing LC_UUID load command"),
  and the Makefile links externally.

## Quick start

```sh
make run                       # build, boot, land in the shell
```

The shell starts empty until it has cartridges. Pack one from a sibling
checkout of arkade-cartridges and drop it in the default cartridge directory,
`data/cartridges/` (or point `--cartridges` at any directory):

```sh
make rcart DIR=../arkade-cartridges/cartridges/life
mkdir -p data/cartridges
cp ../arkade-cartridges/cartridges/life/life.rcart data/cartridges/
./arkade
```

```sh
./arkade --app apps/clock      # jump straight into an app
./arkade --list                # everything registered
```

The console renderer draws the framebuffer with half-block characters in
24-bit colour, two pixels per terminal cell; make the terminal at least 65
columns by about 40 rows.

## Controls

| Key | Function |
|---|---|
| arrows | d-pad |
| `i` `o` / `k` `l` | JOY A, B / JOY C, D (the cabinet's two button rows) |
| `1` / `2` / `0` | drive player 1 / player 2 / plain keyboard |
| enter / space | select / start |
| `q` / esc | quit game → shell; quit shell → exit |

USB gamepads and arcade encoders with a supported profile work on plug-in,
and Bluetooth pads pair from the shell. [docs/CONTROLS.md](docs/CONTROLS.md)
has the full keyboard map, the device profiles and seat claiming.

## Testing

```sh
make test       # gofmt check, the engine suite, sdk/fsm and the tutorial cartridge
make goldens    # regenerate golden frames after an intentional visual change
```

Golden-frame tests boot the real engine against a data-only renderer with a
manual clock and seeded RNG, and compare frames pixel for pixel. Goldens are
the oracle: review every regenerated file, and never regenerate one to make a
refactor pass. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#testing) and
[docs/CI.md](docs/CI.md).

## Commands

| Target | What it does |
|---|---|
| `make build` | build `./arkade` for this machine |
| `make build-pi` | on the Pi: build with the LED renderer, plus `arkade-lcdboot` |
| `make run` | build and run |
| `make test` | format, licence and binary checks, then the engine suite and every SDK module's |
| `make goldens` | regenerate golden frames |
| `make fmt-check` | gofmt over every tracked Go file |
| `make license-check` | every source file carries its SPDX line |
| `make binary-check` | no native executable or library is tracked |
| `make rcart DIR=<dir>` | pack a cartridge project into `<dir>/<slug>.rcart` |
| `make sdk-template`, `make sdk-tutorial` | build the template and tutorial cartridges (TinyGo) |
| `make sdk-tutorial-test`, `make sdk-levels-test` | one SDK module's tests |
| `make clean` | remove the built binaries |

| Flag | Use |
|---|---|
| `--app <slug>` | boot straight into one app or cartridge |
| `--headless --ticks N --dump-frame -` | run without a display and print the last frame |
| `--seed N` | fixed RNG seed, for reproducible runs |
| `--cartridges <dirs>` | comma-separated cartridge directories (default `data/cartridges`) |
| `--mute` | no audio output |
| `--replay <file>` | replay an input flight recording ([docs/FLIGHT_RECORDER.md](docs/FLIGHT_RECORDER.md)) |

`./arkade -h` lists the rest.

## Running on a cabinet

On the Pi, from a clone of this repo:

```sh
bash scripts/pi-setup.sh       # toolchain, LED library, build, systemd units
sudo systemctl status arkade
```

The script clones the LED matrix library into the gitignored
`third_party/rpi-rgb-led-matrix/upstream/`, builds it, and copies
`librgbmatrix.a` to `third_party/rpi-rgb-led-matrix/lib/`.

[docs/CABINET.md](docs/CABINET.md) covers the parts, the service, the boot
LCDs and deploying cartridges and engine builds; [docs/DASHBOARD.md](docs/DASHBOARD.md)
covers the monitoring page.

## Writing a game

Start at **[sdk/docs/getting-started.md](sdk/docs/getting-started.md)**: the
SDK documentation set, compile-checked and also published on the site. Then:

- [docs/GAME_AUTHORING.md](docs/GAME_AUTHORING.md), the human on-ramp and
  the reasoning behind the rules;
- [docs/GAME_AUTHORING_AI.md](docs/GAME_AUTHORING_AI.md), the numbered rule
  checklist for pairing with an AI;
- [docs/CARTRIDGE.md](docs/CARTRIDGE.md), why the ABI is shaped the way it is.

Crib from Life (the simplest) and Snake (everything) in
[arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges).

## Documentation

| Doc | What it covers |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | repo layout, audio, headless mode, testing |
| [docs/CONTROLS.md](docs/CONTROLS.md) | keyboard map, USB and Bluetooth controllers |
| [docs/SCREENSAVERS.md](docs/SCREENSAVERS.md) | standby, scheduled screensavers, morning and evening modes |
| [docs/CABINET.md](docs/CABINET.md) | parts, Pi setup, the service, lcdboot, deploying |
| [docs/DASHBOARD.md](docs/DASHBOARD.md) | the cabinet's monitoring page and HTTP API |
| [docs/FLIGHT_RECORDER.md](docs/FLIGHT_RECORDER.md) | input recordings and `--replay` |
| [docs/HARDWARE_NOTES.md](docs/HARDWARE_NOTES.md) | hardware and host traps, each tied to the code that handles it |
| [docs/HARDWARE_PWM.md](docs/HARDWARE_PWM.md) | the optional PCA9685 dimming board |
| [docs/CI.md](docs/CI.md) | what CI runs |
| [docs/GAME_AUTHORING.md](docs/GAME_AUTHORING.md) | on-ramp for cartridge authors |
| [docs/GAME_AUTHORING_AI.md](docs/GAME_AUTHORING_AI.md) | the cartridge rule checklist |
| [docs/CARTRIDGE.md](docs/CARTRIDGE.md) | the ABI's design rationale |
| [docs/ASSETS.md](docs/ASSETS.md) | the asset formats' design rationale |
| [docs/build/](docs/build/README.md) | cabinet build photos |
| [sdk/README.md](sdk/README.md) | the SDK modules and how to depend on them |
| [sdk/docs/README.md](sdk/docs/README.md) | index of the SDK documentation set |

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md).

## Licensing

The source is GPL-2.0-or-later, and so is everything else in the repo unless
a file says otherwise; see [`COPYING`](COPYING). Distributed binaries go out
under GPLv3, because they link Apache-2.0 modules (wazero, oto, periph), and
ship with wazero's NOTICE; [`THIRD_PARTY.md`](THIRD_PARTY.md) lists every
dependency and its licence.
