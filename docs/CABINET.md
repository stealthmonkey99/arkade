# Running on a cabinet

What the engine expects of the hardware, how to set up the Pi, and how to put
engine builds and cartridges on it. The traps behind each part are in
[HARDWARE_NOTES.md](HARDWARE_NOTES.md); the optional PWM board has its own
page, [HARDWARE_PWM.md](HARDWARE_PWM.md).

## Parts and wiring

Every peripheral except the panel and the Pi is optional and auto-detected.
On a Pi the I2C suite drives whatever answers; elsewhere a virtual suite
mirrors the same state into the console header.

| Part | What the engine expects | Code |
|---|---|---|
| Raspberry Pi | a 32-bit (armv6/armv7) or 64-bit (arm64) Debian-based install, such as Raspberry Pi OS or DietPi, with I2C bus 1 enabled | `scripts/pi-setup.sh` |
| LED panel | one 64×64 HUB75 RGB matrix, driven by [hzeller's rpi-rgb-led-matrix](https://github.com/hzeller/rpi-rgb-led-matrix) (`--led-mapping regular` by default; `adafruit-hat` and `adafruit-hat-pwm` also accepted) | `internal/render/ledpanel` |
| Buttons and sticks | USB arcade encoders or gamepads with a supported profile, or Bluetooth pads (see [CONTROLS.md](CONTROLS.md)) | `internal/controller` |
| Character LCDs | two 16×2 HD44780 displays on PCF8574 I2C backpacks at 0x27 (P1) and 0x23 (P2) | `internal/hardware/pi/lcd.go` |
| Lamp and fan expander | an MCP23017 at 0x26 driving the button lamps and the fan | `internal/hardware/pi/mux.go` |
| PWM board (optional) | a PCA9685 at 0x40 for smooth lamp and backlight dimming ([HARDWARE_PWM.md](HARDWARE_PWM.md)) | `internal/hardware/pi/pca9685.go` |
| Motion sensor | a PIR or microwave sensor on GPIO5 | `internal/hardware/pi/motion.go` |
| Speakers | any ALSA output device | `internal/audio` |
| Microphone (optional) | an ALSA capture device, passed as `--mic plughw:0,0` | `internal/mic` |

The LCDs, expander and PWM board share I2C bus 1 with the Pi. A controller
remembered from an earlier boot reconnects silently; otherwise the boot
app's pairing screen waits for a button press from the pad that wants the
slot.

## Setting up the Pi

Clone this repo on the Pi and run the setup script from its root:

```sh
cd ~/arkade && bash scripts/pi-setup.sh
```

It is idempotent, so re-run it after a code sync. It installs Go and the cgo
system libraries (`libasound2-dev`, `libudev-dev`, BlueZ), enables the
Bluetooth radio, clones and builds the LED matrix library into
`third_party/rpi-rgb-led-matrix`, builds `arkade` and `arkade-lcdboot` with
`make build-pi`, runs a headless self-test, and installs and enables the
systemd units and udev rules below.

To try the panel by hand before the service takes over:

```sh
make build-pi
sudo ./arkade --led --led-slowdown 3 --led-mapping regular
```

## Running as a service

`scripts/arkade.service` runs the engine as root (the LED library needs GPIO,
and the same process owns hidraw, I2C and ALSA) from `~/arkade`, scanning
`data/cartridges` and any USB stick mounted under `/media`. Its header
documents the install steps and the dashboard token.

```sh
sudo systemctl status arkade
sudo systemctl restart arkade     # after a rebuild
sudo systemctl stop arkade        # for maintenance
journalctl -u arkade -e           # recent logs; panics and errors land here
journalctl -u arkade -f           # follow
```

`Restart=on-failure` restarts a crashed engine after five seconds: a panic
exits 2 and an unhandled error 1. A clean exit (a quit from the shell, or
`systemctl stop`) does not restart.

`scripts/99-arkade-cartridge.rules` mounts any USB stick read-only under
`/media/cartridge-<device>`; the engine picks up `.rcart` and `.wasm` files at
its root within a few seconds of plug-in, and pulling the stick without
ejecting is safe.

The cabinet also serves a loopback-only monitoring dashboard: status, logs,
the game table and score editing. Reach it through an SSH tunnel; the setup,
the token and every route are in [DASHBOARD.md](DASHBOARD.md).

## lcdboot

`arkade-lcdboot` is a small Go binary built by `make build-pi` alongside the
engine, with no cgo, so it works on a Pi that has not built the matrix
library. `scripts/99-arkade-lcdboot.rules` starts `arkade-lcdboot.service` as
soon as `/dev/i2c-1` appears, because the PCF8574 backpacks power up with
their backlights glaring and nothing else runs that early. It darkens the
lamp and backlight channels, then shows a card at 30% backlight: P1 shows
`Årkade` and a progress bar that advances as the network, sound, Bluetooth
and engine units come up, and P2 shows the IP address, SoC temperature and
wifi signal.

`arkade.service` hooks both ends of the handover:

```ini
ExecStartPre=+/usr/bin/systemctl stop arkade-lcdboot.service
ExecStopPost=+%h/arkade/arkade-lcdboot --after-engine
```

The first stops the tool with the panels still lit at 30%, where the engine
picks them up and keeps a startup card of its own while it scans cartridges,
so nothing goes dark between the two processes. The second runs after the
engine exits: on a failure the tool shows a crash card with the exit reason
until the engine restarts. There is deliberately no `Conflicts=` between the
units: it is symmetric, so a crash card starting would stop the engine. Its
log is `journalctl -u arkade-lcdboot -e`.

## Deploying a cartridge

A cartridge needs no engine rebuild. Build its bundle with `make rcart`, then
copy it into the cabinet's `data/cartridges/` under a temporary name and
rename it into place, so the cartridge watcher never reads a half-copied
file. `$ARKADE_HOST` is the cabinet's SSH host and `~/arkade` the deployment
directory the service uses:

```sh
make rcart
scp mygame.rcart "$ARKADE_HOST":arkade/data/cartridges/.mygame.rcart.tmp
ssh "$ARKADE_HOST" 'mv arkade/data/cartridges/.mygame.rcart.tmp arkade/data/cartridges/mygame.rcart && sudo systemctl restart arkade'
```

`make build` rebuilds the `.wasm` but does not repack the `.rcart`; run
`make rcart` after every wasm rebuild, or you deploy a stale bundle that still
registers cleanly. After the restart, check the log:

```sh
journalctl -u arkade --since "30 sec ago" | grep -iE "registered|error|panic|unreachable"
```

Every cartridge logs `cartridge: <slug> registered from <path>`; repeated per-frame
`wasm error` lines mean a crashed instance (see
[`sdk/docs/tutorial-debug.md`](../sdk/docs/tutorial-debug.md)).

Two other routes need no shell on the cabinet: a USB stick (above), or the
dashboard's `POST /api/cartridges` install route, which needs the
`ARKADE_HTTP_TOKEN` ([DASHBOARD.md](DASHBOARD.md)).

## Deploying an engine change

The engine is a cgo build with the LED tag, so it is built on the Pi. Sync the
source to the Pi, leaving out `data/` (scores and settings live there),
`third_party/` (the Pi's own matrix library build) and the binaries; then
build and restart:

```sh
rsync -a --exclude data/ --exclude third_party/ --exclude /arkade --exclude /arkade-lcdboot \
  ./ "$ARKADE_HOST":arkade/
ssh "$ARKADE_HOST" 'cd arkade && make build-pi && sudo systemctl restart arkade'
```

Never sync over `data/`.
