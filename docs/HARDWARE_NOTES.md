# Hardware and host notes

Traps for whoever sets up, repairs or extends the cabinet. Cartridge authors
want [gotchas](../sdk/docs/gotchas.md) instead. Each line is a fact, what to
do about it, and the code or test that holds it. The PCA9685 board has its
own page: [Hardware PWM](HARDWARE_PWM.md).

## LCDs (HD44780 behind a PCF8574 backpack)

- The chip has 8 custom-glyph slots shared by both rows; a ninth draws as
  `?`. The pager evicts least-recently-used and `createChar` leaves the
  address counter in CGRAM until the next line write
  (`internal/hardware/pi/glyphs.go`, `internal/hardware/pi/lcd.go`).
- The boot cards define 9 custom runes against the 8 slots, which works only
  because the pager never needs more than 8 live at once
  (`cmd/arkade-lcdboot`).
- The mask ROM draws ASCII, so custom glyphs live on runes at U+0080 and
  above only; `MaxCartGlyphs` (32) caps a cartridge's table, not the chip
  (`internal/lcdrune`).
- A glyph row is 5 bits with bit 4 leftmost; the chip ignores higher bits
  (`internal/hardware/pi/glyphs.go`).
- The chip runs control characters in text as commands, so LCD text is
  refused if it holds one (`internal/net/preview.go`).
- A full redraw is about 136 bus transactions (roughly 40 ms), so push
  on events, not every frame.
- The backpack carries the backlight as one bit in every nibble write, so a
  dimmed level rounds to on or off at the backpack; real dimming needs the
  PCA9685.
- A backlight write must never land between a nibble's enable-high and
  enable-low; only the screen worker issues one
  (`internal/hardware/pi/lcd.go`).
- The panel driver centres lines under 16 columns and scrolls longer ones,
  so boot cards are padded to exactly 16 (`cmd/arkade-lcdboot`).

## I2C chips

- The lamp expander is an MCP23017 at 0x26 in its power-on `BANK=0` layout;
  writing OLATA then OLATB auto-increments, so one 3-byte write sets all 16
  latches (about 411 us median at 100 kHz) (`internal/hardware/pi/mux.go`).
- Lamps are bit-angle modulated at 100 Hz in slots 1, 2, 4, 8, because
  sigma-delta flickered on the cabinet; a lit lamp never rounds to dark
  (`internal/hardware/pi/lamps.go`).
- On the PCA9685, `PRE_SCALE` is writable only while `SLEEP` is set, and
  `ALLCALL` stays off so 0x70 is free to probe a second board
  (`internal/hardware/pi/pca9685.go`).
- The PCA9685 keeps its PWM after the process exits; drive channels dark
  before quitting if a lamp must go out (`internal/hardware/pi/pca9685.go`).
- Fan relay channels are only ever full-on or full-off; an intermediate
  duty chatters the coil (`internal/hardware/pi/fans.go`).
- The PCF8574 powers up with every pin high and an inverted PCA backlight
  channel resets to its lit end, so darken backlights before either panel
  wakes (`pi.Suite.initPWM`, `cmd/arkade-lcdboot`; pinned by
  `TestPCABacklightWriteOrder`).
- The motion sensor is on GPIO5 with a 5 s falling-edge debounce evaluated
  on reads (`internal/hardware/pi/motion.go`).

## Controllers (hidapi)

- `hid_close` frees the handle, so a second close is a double free in glibc:
  `Close` is idempotent, must not run during a cgo read, and the read loops
  leave the close to `Manager.Stop` (`internal/controller/hiddev`,
  `internal/controller`).
- Read with a timeout and treat a timeout, `EINTR` and `EAGAIN` as "keep
  reading"; an idle pad sends reports only on change, so silence is not a
  disconnect (`internal/controller/hiddev`).
- go-hid converts device strings with `wcstombs` and panics on `EILSEQ`, and
  a Go binary starts in the C locale; the package selects a UTF-8 `LC_CTYPE`
  before any hidapi call, falling through `""`, `C.UTF-8`, `en_US.UTF-8`
  because macOS rejects `C.UTF-8` and the cabinet boots with no locale
  environment (`internal/controller/hiddev/locale.go`).
- A DualShock 4 over Bluetooth prefixes its reports with extra header bytes,
  so profiles are bus-specific and a profile naming another bus must never
  match (`internal/controller`).

## Bluetooth (BlueZ)

- `dtparam=krnbt=on` in the boot config binds the radio, and any
  `blacklist` of `hci_uart`, `hidp`, `rfcomm`, `btbcm`, `bnep` or
  `bluetooth` in `modprobe.d` makes it vanish on the next boot while a
  hand-run `modprobe` still works (`scripts/pi-setup.sh` clears both).
- A link can show Connected while the kernel never builds the HID device
  because the `uhid` module is missing; the pad then does nothing and no
  error appears (`internal/bluetooth`).
- Pairing needs a NoInputNoOutput agent exported on the bus.
  `RegisterAgent` returns `AlreadyExists` if an earlier connection never
  unregistered, so use a private connection and close it; `AlreadyExists`
  on pair and `InProgress` on discovery are not errors
  (`internal/bluetooth/bluez_linux.go`).
- Use `dbus.SystemBusPrivate`, not the shared bus; closing the shared
  connection breaks other holders such as the power-down hook.
- A call to a stopped `bluetoothd` blocks for dbus's 25 s service-start
  timeout; ordinary calls are bounded to 5 s (`internal/bluetooth/bluez_linux.go`).
- A device with no advertised name gets its dashed MAC as `Alias`, so
  "has a real name" compares the address ignoring case and separators
  (`internal/bluetooth/sort.go`).

## Process and host

- `systemctl is-active` exits non-zero when any unit is down, so only empty
  output is a failure; `ExecStopPost` must start the unit with `--no-block`
  because it runs inside `arkade.service`'s own stop job
  (`cmd/arkade-lcdboot`).
- `/proc/net/wireless` level fields carry a trailing dot, and 0 means no
  link rather than a strong signal (`cmd/arkade-lcdboot/sysinfo.go`).
- The LED matrix library keeps root so the controllers, I2C and audio stay
  reachable: `drop_privileges` is set to -1, since the C wrapper copies only
  a non-zero value (`internal/render/ledpanel`).
- The Pi setup supports 32-bit (armv7) and 64-bit (arm64) installs, so the
  score editor refuses values above `int32` rather than risk a decode failure
  on the 32-bit one (`internal/net/games.go`).
- The Pi detection helpers read `/proc/cpuinfo`; `pi.New` fails off a Pi
  (`internal/hardware/pi`).
- Raw microphone audio never reaches a running cartridge, only levels, or a
  recorded session could not be replayed (`internal/hardware/periph.go`).
