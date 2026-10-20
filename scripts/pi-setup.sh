#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-or-later

# One-shot bootstrap for running Årkade on a Raspberry Pi. Idempotent — safe
# to re-run after a code sync. Run from the repo root ON THE PI:
#
#   cd ~/arkade && bash scripts/pi-setup.sh
#
# Installs the Go toolchain + cgo system libs, builds hzeller's LED matrix
# library, compiles arkade (led tag) and arkade-lcdboot, runs a headless
# self-test on a built-in app, and installs the systemd units and udev rules.
set -euo pipefail

GO_VERSION=1.23.4
LEDLIB_DIR=third_party/rpi-rgb-led-matrix

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

# Root images (DietPi) have no sudo; use it only when we're not already root.
if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi

case "$(uname -m)" in
	aarch64 | arm64) GO_TARBALL="go${GO_VERSION}.linux-arm64.tar.gz" ;;
	armv7l | armv6l) GO_TARBALL="go${GO_VERSION}.linux-armv6l.tar.gz" ;;
	*)
		echo "unsupported architecture: $(uname -m)" >&2
		exit 1
		;;
esac

# go-hid (hidraw backend) needs libudev; oto needs ALSA; the LED matrix C++
# library needs a compiler; the microphone captures through arecord
# (alsa-utils).
log "Installing system libraries"
$SUDO apt-get update -qq
$SUDO apt-get install -y build-essential libudev-dev libasound2-dev alsa-utils pkg-config i2c-tools git curl \
	bluez pi-bluetooth

# A paired pad is just another /dev/hidraw* node. krnbt=on binds the radio in
# the device tree; hciuart cannot with enable_uart=0, so it is disabled below.
log "Enabling the Bluetooth radio"
BOOTCFG=/boot/firmware/config.txt
[ -f "$BOOTCFG" ] || BOOTCFG=/boot/config.txt
if grep -q '^dtparam=krnbt=on' "$BOOTCFG"; then
	log "krnbt already enabled in $BOOTCFG"
else
	$SUDO cp -a "$BOOTCFG" "$BOOTCFG.bak-$(date +%Y%m%d-%H%M%S)"
	printf '\n# Bluetooth: bind the on-board radio to the PL011 UART at boot.\ndtparam=krnbt=on\n' | $SUDO tee -a "$BOOTCFG" >/dev/null
	log "Added dtparam=krnbt=on to $BOOTCFG (takes effect on the next boot)"
fi
# A blacklist (DietPi ships one) defeats modules-load.d below while a hand-run
# modprobe still works, so the radio would vanish silently on the next boot.
BT_MODULES='hci_uart|hidp|rfcomm|btbcm|bnep|bluetooth'
for f in /etc/modprobe.d/*.conf; do
	[ -e "$f" ] || continue
	grep -qE "^[[:space:]]*blacklist[[:space:]]+($BT_MODULES)[[:space:]]*$" "$f" || continue
	# % as the delimiter: BT_MODULES's own pipes would end the s command.
	$SUDO sed -i -E "s%^([[:space:]]*blacklist[[:space:]]+($BT_MODULES)[[:space:]]*)$%# \1%" "$f"
	log "Cleared a Bluetooth blacklist in $f"
done
# Nothing autoloads hci_uart for the DT node. Without uhid and hidp a pad pairs
# but never reads; the only sign is ioctl_connadd(): Protocol not supported.
printf 'hci_uart\nuhid\nhidp\n' | $SUDO tee /etc/modules-load.d/arkade-bluetooth.conf >/dev/null
$SUDO systemctl disable hciuart >/dev/null 2>&1 || true
$SUDO systemctl enable bluetooth >/dev/null 2>&1 || true

ver_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]; }
have_go=0
if command -v go >/dev/null 2>&1; then
	cur=$(go version | awk '{print $3}' | sed 's/go//')
	ver_ge "$cur" "$GO_VERSION" && have_go=1
fi
if [ "$have_go" -eq 0 ]; then
	log "Installing Go ${GO_VERSION}"
	curl -fsSL "https://go.dev/dl/${GO_TARBALL}" -o /tmp/go.tgz
	$SUDO rm -rf /usr/local/go
	$SUDO tar -C /usr/local -xzf /tmp/go.tgz
	export PATH=/usr/local/go/bin:$PATH
	grep -q '/usr/local/go/bin' ~/.profile 2>/dev/null ||
		echo 'export PATH=$PATH:/usr/local/go/bin' >>~/.profile
fi
log "Using $(go version)"

# The tracked header stays; upstream is cloned into a gitignored upstream/
# and only the static library is copied to lib/, where ledpanel's cgo
# flags look for it.
if [ ! -f "$LEDLIB_DIR/lib/librgbmatrix.a" ]; then
	log "Building rpi-rgb-led-matrix"
	rm -rf "$LEDLIB_DIR/upstream"
	git clone --depth 1 https://github.com/hzeller/rpi-rgb-led-matrix "$LEDLIB_DIR/upstream"
	make -C "$LEDLIB_DIR/upstream/lib"
	mkdir -p "$LEDLIB_DIR/lib"
	cp "$LEDLIB_DIR/upstream/lib/librgbmatrix.a" "$LEDLIB_DIR/lib/"
else
	log "rpi-rgb-led-matrix already built"
fi

log "Building arkade (cgo + led tag) and arkade-lcdboot"
make build-pi

log "Headless self-test"
selftest=$(mktemp -d)
./arkade --headless --ticks 30 --app apps/clock --data "$selftest" >/dev/null
rm -rf "$selftest"

# arkade.service's ReadWritePaths fails the unit if data/ does not exist yet.
mkdir -p data/cartridges

# Enabled, not started: test the panel first.
log "Installing systemd service"
$SUDO cp scripts/arkade.service /etc/systemd/system/arkade.service
$SUDO cp scripts/arkade-lcdboot.service /etc/systemd/system/arkade-lcdboot.service
$SUDO cp scripts/99-arkade-lcdboot.rules /etc/udev/rules.d/99-arkade-lcdboot.rules
$SUDO udevadm control --reload
$SUDO systemctl daemon-reload
$SUDO systemctl enable arkade
$SUDO systemctl enable arkade-lcdboot

$SUDO cp scripts/99-arkade-cartridge.rules /etc/udev/rules.d/
$SUDO udevadm control --reload

log "Configuring persistent journalctl logs"
$SUDO mkdir -p /var/log/journal
$SUDO systemd-tmpfiles --create --prefix /var/log/journal

log "Setup complete."

cat <<'EOF'

Next steps (run on the Pi):

  # Test the panel before enabling auto-boot (audio muted for the first run):
  sudo ./arkade --led --mute --app apps/clock

  # If the panel is an Adafruit HAT/Bonnet instead of direct GPIO wiring:
  sudo ./arkade --led --mute --led-mapping adafruit-hat --app apps/clock

  # Flicker? Try a different GPIO slowdown (2–4):
  sudo ./arkade --led --mute --led-slowdown 4 --app apps/clock

  # Games are cartridges: pack one with `make rcart DIR=<cartridge dir>`
  # (e.g. a checkout of arkade-cartridges' cartridges/snake) and put the
  # .rcart in data/cartridges/, where the shelf finds it.

  # Once the panel looks right, start the service (auto-starts on every
  # boot from now on, and auto-restarts after a crash):
  sudo systemctl start arkade

  # Check logs:
  journalctl -u arkade -e

  # The Bluetooth radio needs one reboot (krnbt binds it in the device
  # tree at boot). After that, hci0 should be up:
  sudo reboot
  hciconfig -a          # expect: hci0 ... UP RUNNING

  # Pair a wireless pad (hold Share+PS on a DualShock until it flashes):
  bluetoothctl scan on  # note the address, then, in bluetoothctl:
  #   pair <addr> / trust <addr> / connect <addr>
  # A paired pad shows up as another /dev/hidraw* and the engine picks it
  # up on its next hot-plug scan, like any wired one.

EOF
