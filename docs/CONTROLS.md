# Controls

Årkade takes input from three places: the keyboard (for desktop development),
USB HID gamepads and arcade encoders, and Bluetooth pads. All three feed the
same event stream, so an app or cartridge never knows which one a press came
from.

## Keyboard (dev test controller)

The keyboard stands in for an arcade panel. It is one *virtual controller*
that you move between player slots, not two pads: you drive P1 or P2, never
both at once. It boots already driving **P1**, so there is nothing to press
to get started.

```text
        d-pad                                       buttons
                                            ┌───┐ ┌───┐
        ┌───┐                               │ i │ │ o │   JOY A   JOY B
        │ ↑ │                               └───┘ └───┘
  ┌───┐ ├───┤ ┌───┐                         ┌───┐ ┌───┐
  │ ← │ │ ↓ │ │ → │                         │ k │ │ l │   JOY C   JOY D
  └───┘ └───┘ └───┘                         └───┘ └───┘

        which slot the keyboard is driving  (starts on P1)
        ┌───┐        ┌───┐        ┌───┐
        │ 0 │        │ 1 │        │ 2 │
        └───┘        └───┘        └───┘
      plain kb     player 1     player 2
```

Pressing `1` / `2` stamps every subsequent event as coming from virtual
controller `virtual-0` / `virtual-1`, the same identity the boot pairing flow
and per-player game logic key off. `0` drops back to **plain keyboard mode**:
events carry no player at all, and `i` `o` `k` `l` go back to being literal
letters instead of JOY buttons.

The two button rows mirror the cabinet, where A/B sit above C/D, so your
fingers learn the panel's geometry rather than its mirror image.

| Key           | Function                             |
|---------------|--------------------------------------|
| enter / space | select / start                       |
| `r` / `R`     | rotate display right / left          |
| `b` / `B`     | brightness up / down                 |
| `q` / esc     | quit game → shell; quit shell → exit |
| ctrl-c        | hard quit                            |

Two things the diagram can't show. The digit keys are **swallowed**: they set
the slot and emit nothing, and `3`–`9` are consumed silently, so no digit ever
reaches an app. And the keyboard has no mapping for the aux buttons
(`joyaux1`–`joyaux4` exist but nothing on the keyboard emits them), so
anything bound to aux is reachable only from real hardware. See
`internal/input/keyboard.go`.

## USB HID gamepads

Speedlink, DragonRise, Buffalo, Sonicon and DualShock 4 controllers are
supported natively. Plug one in at any time (hot-plug is rechecked every two
seconds); the first becomes player 1 and the second player 2. Directional
input is cabinet-position aware, to suit sticks mounted on the cabinet's
sides. `--no-hid` disables HID support.

Each supported device is a data file,
`internal/controller/profiles/<name>.arkadehid.json`: the VID/PID match, the
report layout and the button table. The format is documented in
[`sdk/docs/reference-hid.md`](../sdk/docs/reference-hid.md). To write a profile
for a new device, capture its reports in hidlab at
[arkade.build/dev/tools](https://arkade.build/dev/tools); the config app's
TEST INPUTS screen shows exactly what each connected controller sends.

## Bluetooth pads

A Bluetooth pad pairs from the shell's **BLUETOOTH** tile on the SYSTEM shelf
(`apps/bluetooth`), which also assigns seats. Pairing goes through BlueZ
(`internal/bluetooth`); a paired pad appears as one more hidraw device and
is decoded by a profile from the same directory (profiles are bus-specific,
so a DualShock 4 has one for USB and one for Bluetooth). Seats are remembered by the pad's own
identity, so they survive the pad sleeping.

In a game, **SELECT + R1** claims P1 and **SELECT + L1** claims P2, so two
players can swap seats without leaving the game
(`internal/controller/seatchord.go`).

The Pi-side setup (the `krnbt` boot parameter, the `uhid` module, the
modprobe blacklist trap) is in [`HARDWARE_NOTES.md`](HARDWARE_NOTES.md#bluetooth-bluez).
