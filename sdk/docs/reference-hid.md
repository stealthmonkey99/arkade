# HID profile and input channel reference

This documents two halves of one idea. The `hid` kind of the asset
envelope — a `.arkadehid.json` file — says what a physical device's input
report *means*, as data rather than as code. **Input channels** are how a
cartridge reads it: `host_input_bind` at init, then one call per value per
frame. Read [Asset file reference](reference-assets.md) first for the
envelope fields every kind shares (`arkade`, `kind`, `name`, `generator`,
...); for a JSON kind, the document **is** the file.

hidlab is the easy way to produce one of these — it captures a device's
reports in the browser over WebHID, lets you name the channels and watch
them decode live, and writes the file. This page is for reading or
hand-editing the JSON, and for the ABI half, which no lab can write for
you.

## Why channels exist

An input record tells a cartridge what *happened*: this key went down,
the stick moved to up-left. That is the whole vocabulary the cabinet had,
and it is enough for almost every game — but it has no word for a
spinner, a trackball, a pressure mat, or an analog trigger. A device that
reports one of those had that part of its report dropped on the floor,
because there was nowhere to put it.

A **channel** tells a cartridge what *is*: this button is held, this axis
reads `-3200`, this spinner has turned nine counts since the last frame.
Four kinds, integers only — a float would make a replay depend on the
host's floating-point, and every golden frame rests on replays being
exact.

| Kind | Value | Read with | Examples |
| --- | --- | --- | --- |
| `button` | `0` or `1` | `host_input_button` | pad arrows, panel buttons, a device's extra buttons |
| `axis` | `int16` range | `host_input_axis` | analog stick, trigger, pressure pad, wheel angle, light-gun x/y |
| `delta` | `int32`, accumulated by the host since the last `cart_update` and reset by the read | `host_input_delta` | spinner, trackball, encoder |
| `blob` | a fixed run of bytes the profile declares, at most 256 | `host_input_blob` | anything the other three cannot carry |

**The cabinet's own panel is channels too**, and that is what makes this
addition free rather than parallel. `p1.joy.x`, `p1.a`, `p2.start` are
derived by each host from the very input records already on their way to
`cart_handle_input` — so records still flow exactly as before, a cartridge
that binds nothing is unchanged, seat and mount remapping are already
applied, and a flight replay reproduces the panel channels without
recording anything extra.

## The ABI

```text
host_input_bind(name_ptr, name_len, kind u32) -> i32   // handle, or -1
host_input_button(handle i32) -> u32                   // 0 or 1
host_input_axis(handle i32) -> i32                     // int16 range
host_input_delta(handle i32) -> i32                    // since the last cart_update
host_input_blob(handle i32, dst_ptr, dst_cap) -> u32   // bytes copied
host_output_bind(name_ptr, name_len) -> i32            // RESERVED: always -1
host_output_set(handle i32, value i32)                 // RESERVED: no-op
```

`kind` is `0` button, `1` axis, `2` delta, `3` blob.

**Bind at init, read in update.** The table seals at the first sample,
before the first `cart_update`; a bind of a new name after that returns
`-1` with a warning. Sealing is what makes the per-tick sample set fixed,
which is in turn what lets the flight recorder write a channel's value
against a stable handle. Re-binding an already-bound name returns its
existing handle, even after the seal; binding it under a different kind
fails. cartlib exposes binds only through `Setup`, so in practice they
happen inside `Init`.

**`-1` is the only thing to check.** A bind fails when the name is not
offered, when the kind disagrees with what is offered, when the table is
full, or when init is over — and a cartridge is expected to notice and
play differently rather than to insist. `-1` passed to any reader returns
zero and is *never* logged, so a cartridge that stored a failed bind and
reads it every frame does not flood an operator's log.

**Sampled once per tick, before `cart_update`.** Every bound channel is
read exactly once per frame and held. Two reads of the same channel inside
one frame therefore give the same number — the second does not steal the
first's motion — and a delta's "since the last `cart_update`" is true by
construction rather than by convention.

**`host_input_blob` is all or nothing.** A `dst_cap` below the channel's
declared length copies nothing and returns `0`, exactly as
`host_cel_read` does: a half-filled buffer would read as success and
parse as garbage.

**Outputs are reserved.** `host_output_bind` parses and bounds-checks the
name exactly as an input bind does, then always fails; `host_output_set`
is a no-op. The surface exists ahead of a consumer so that the day a host
drives rumble or a pad's LEDs, no signature moves and no cartridge is
rebuilt to find out.

### Caps

Per cartridge, identical on both hosts
(`internal/cartridge/wasm/channels.go`, mirrored in the browser
host):

| Cap | Value | Bounds |
| --- | --- | --- |
| bound channels | 64 | the per-tick sample, and the recorder's per-frame cost |
| blob bytes | 256 | one blob channel's declared length, and its per-tick copy |
| channel name | 96 B | `host_input_bind`'s name |
| bundled profiles | 8 | `.arkadehid.json` files one cartridge may register at mount |

And on a profile file itself (`assetfile`): 128 channels, a 64-byte
channel name, report byte offsets `0..1023`, and `1..4` bytes for an axis
or delta's byte form.

## Standard channel names

Every channel name begins with a seat prefix, `p1.` or `p2.`. There is no
seat-less channel: "which player's spinner" is never a question a host
should have to guess at.

The **panel** offers these leaves at both seats, and they are offered by
every host — including one with nothing plugged in, because they are the
records:

| Leaf | Kind | Is |
| --- | --- | --- |
| `joy.x`, `joy.y` | axis | the stick as a pair of axes. The cabinet's stick is an eight-way switch, so these read exactly `-1`, `0` or `+1`; `y` is negative upward. |
| `joy.north`, `joy.south`, `joy.east`, `joy.west` | button | the same stick, read as four buttons |
| `a`, `b`, `c`, `d`, `aux1`, `aux2` | button | the six arcade buttons in front of a seat — the same six `host_set_button_lights` addresses |
| `l1`, `r1`, `l2`, `r2`, `select`, `start`, `turbo`, `clear`, `home`, `enter` | button | keys a *controller* may carry. Offered because the engine's key vocabulary has them; a plain cabinet panel never sends one, so a cartridge binding one still has to degrade. |

Everything else — `p1.spinner`, `p1.mat` — comes from a device profile,
and exists only while a device offering it is attached.

### Seat and mount remapping

The panel's channels are derived from records that have *already* been
through the engine's seat-mount and display-orientation remap, so
`p1.joy.west` means "toward the left edge of the picture as this player
sees it", the same as the arrow key a record carries. There is no second
remap to keep in sync with the first.

A **profile's own** channels are not rotated, and that is deliberate
rather than missing: a profile does not say which two of its axes form a
pair, so there is no defined rotation to apply. A spinner has no
orientation; a device axis that *should* rotate says so by aliasing onto
`joy.x`/`joy.y` in its `standard` block, which puts it on the panel and
therefore inside the remap.

The seat prefix a profile's channel appears under is the **device's
paired seat plus the channel's own `seat` field**, which is how a
two-mat dance pad in one port offers `p1.*` and `p2.*` at once. Past seat
1 there is nowhere to go on a two-seat cabinet and the channel is not
offered.

## The profile file

```json file=assets/dancepad.arkadehid.json
{
  "arkade": 1,
  "kind": "hid",
  "name": "dancepad",
  "description": "A four-arrow dance pad plus a spinner and a pressure sensor.",
  "generator": { "tool": "hidlab", "version": "1.0.0" },
  "device": { "vendor": 1356, "product": 2508, "name": "Dance pad", "directional": "dpad" },
  "channels": [
    { "name": "pad.up", "kind": "button", "report": { "byte": 3, "bit": 4 } },
    { "name": "spinner", "kind": "delta", "report": { "byte": 5, "size": 2, "signed": true } },
    { "name": "pressure", "kind": "blob", "report": { "byte": 8, "size": 32 } }
  ],
  "standard": { "pad.up": "joy.north" }
}
```

### `device`

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `vendor` | integer `0`–`65535` | no | USB vendor id. With `product`, this is what matches the profile to an attached device. |
| `product` | integer `0`–`65535` | no | USB product id. |
| `name` | string | no | The device's display name, used in log lines and pairing UI. Defaults to the envelope's `name`. |
| `directional` | `"stick"` or `"dpad"` | no | Informational only; nothing in a decode path branches on it. Defaults to `"stick"`. |
| `bus` | `"usb"` or `"bluetooth"` | no | Restricts the profile to one transport; absent matches either. The same pad can report a different byte layout per transport, which vendor and product cannot tell apart (a DualShock 4 has its left stick at byte 1 over USB and byte 3 over Bluetooth), so ship one profile per bus. A profile naming the attached device's bus wins over an unrestricted one. Any other value is refused. |

The whole block is optional. A profile with no `device` describes
channels nothing will ever match, which is only useful as a fixture.

### `channels`

An **ordered** array — order is load-bearing twice over, and both times
it is the order that decides, never the JSON object's key order:

* it is the order a `standard` button takes in the panel's bit mask, and
  therefore the order key events come out of the decoder when two buttons
  change in the same report;
* it is the order a host offers channels in.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `name` | string | yes | Lowercase letters, digits, and `.` `_` `-`, no empty segment, at most 64 bytes. **May not begin with `p1.` or `p2.`** — the host adds the seat prefix, and a name that spelled its own could contradict its `seat`. Unique within the file. |
| `kind` | `button`, `axis`, `delta`, `blob` | yes | Which of the four kinds this channel is. |
| `seat` | `0` or `1` | no | An OFFSET on the device's own paired seat (see [Seat and mount remapping](#seat-and-mount-remapping)). Defaults to `0`. |
| `report` | object | yes | Where the value is in the report, and the arithmetic that follows. Below. |

### `report`: the field grammar

Two forms, chosen by the channel's kind and then by which fields are
present.

**Bit form** — a run of bits inside one byte.

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `byte` | integer `0`–`1023` | required | The report byte. |
| `bit` | integer `0`–`7` | `0` | The starting bit, `0` being least significant. |
| `bits` | integer `1`–`8` | `1` for a button | The field's width. `bit + bits` must not pass 8: a field never straddles two bytes. |

A `button` is **always** this form and always one bit wide; `bits` above
1 is an error, and so are `size`, `signed`, `map`, `bias` and `divide` on
a button.

**Byte form** — a run of whole bytes, little-endian.

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `byte` | integer `0`–`1023` | required | The first byte. |
| `size` | integer | `1` for axis/delta; required for blob | Width in bytes: `1`–`4` for an axis or delta, `1`–`256` for a blob. |
| `signed` | boolean | `false` | Read the bytes as two's complement, sign-extended from the field's own width. Axis and delta byte form only; a bit field is unsigned. |

An `axis` or `delta` uses the bit form when `bits` is present and the
byte form otherwise. Setting both `bits` and `size` is an error: a field
is either a run of bits or a run of bytes. A `blob` is always the byte
form and copies the bytes raw.

**Then three transforms**, in this order, for `axis` and `delta` only:

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `map` | array of integers | none | Replaces the extracted value with `map[value]`. A value outside the array's index range yields `mapDefault`. This is how a **hat switch** — one small integer standing for nine directions — becomes an axis. |
| `mapDefault` | integer | `0` | `map`'s out-of-range result. A garbled report reads as rest, not as a large deflection. |
| `bias` | integer | `0` | Subtracted. An unsigned byte whose rest position is 128 is `{"size": 1, "bias": 128}`. |
| `divide` | integer | `0` (none) | Divides, **rounding halves toward +∞** exactly as JavaScript's `Math.round` does — so `-0.5` is `0`, not `-1`. That is the rule the ported centre-noise filters were written against, so a converted profile has to keep it. |

Every one of the six profiles the engine ships uses at least one of these, which is
why they exist as declarative verbs rather than as a code escape hatch a
bundled profile could never use.

**Short reports decode to nothing at all.** Every field's extent is known,
so a profile has a minimum report length; a report below it moves no
channel and produces no panel event. All or nothing, so a truncated
report can never advance half a device.

### `standard`

An object mapping a channel's name to a panel name. This is the block
that lets a device the engine has never heard of drive a cartridge that
never heard of the device.

Targets, seat-relative (the channel's `seat` decides which seat):

* axes: `joy.x`, `joy.y`, and `dpad.x`, `dpad.y` for a **second**
  directional beside a stick (a DualShock's hat);
* directions: `joy.north`, `joy.south`, `joy.east`, `joy.west`, and the
  same four under `dpad.`;
* buttons: `a`, `b`, `c`, `d`, `aux1`, `aux2`, `l1`, `r1`, `l2`, `r2`,
  `select`, `start`, `turbo`, `clear`, `home`, `enter`.

The shapes have to agree: a button may alias onto a direction or a
button, an axis onto an axis, and a blob onto nothing at all. A button
aliased onto `joy.x` is refused, because it would have to invent a
deflection the profile never states; an axis aliased onto a panel button
is refused, because it would have to invent a threshold.

Two axes may not alias the same panel axis. That is rejected when the
profile compiles at mount (`internal/controller/hidprofile.go`), not by
`rcartpack` or `assetfile.Validate`.

**Two channels may alias onto one panel button**, and that is not a
loophole — it is how a DualShock's square and L3 both reach the cabinet's
C. The panel button is held while any of its sources is.

## What each host can do

The cabinet reads a device's raw HID input report and applies the whole of
the grammar above.

**A browser tab cannot.** The Gamepad API hands a page a *standard
mapping* — sixteen buttons and four axes with fixed meanings — and never
the device's own report, so `{"byte": 5, "bit": 4}` describes bytes that
host will never see. Concretely:

* the **panel** channels work identically, because they are derived from
  input records and the browser produces those from the keyboard, its
  on-screen seats and the Gamepad API's standard mapping;
* a bundled profile is **validated** at mount with the same validator and
  the same warning sentences, so an author debugging one reads the same
  text in either place;
* its channels that have no `standard` alias are **not bindable**: the
  mount logs what it cannot deliver, and the bind returns `-1`.

So `p1.spinner` binds on a cabinet with a spinner attached and returns
`-1` in a browser tab — which is the same thing a cartridge must already
handle for a cabinet with no spinner plugged in. Bind hopefully, check
`Ok()` once, and have a keyboard-shaped fallback.

## Bundled profiles

A cartridge may ship profiles for the hardware it was built around. They
are listed in the manifest and installed for **that cartridge's session
only**:

```json
"assets": {
  "hid": ["assets/dancepad.arkadehid.json"]
},
"capabilities": ["hid"]
```

The `"hid"` capability is required, and a manifest that lists profiles
without it mounts with them ignored (with a log line saying so).
Unlike a glyph — which changes only what the cartridge draws — a profile
changes how the cabinet reads a **physical device**, for whoever else is
at the machine too, so it is asked for by name rather than implied.

Paths are bundle-relative, the same path space `host_load_sheet` and
`host_load_sound` use, and are read in the order listed, **at mount,
before `cart_init`** — so the binds `cart_init` makes can already see the
profile's channels.

A bundled profile for a device the host already knows **wins for that
session**, and an already-attached device picks it up on its next report
rather than waiting for a hot-plug scan. Everything is dropped at
teardown: a profile that outlived its cartridge would have the shell,
attract and the next game reading a departed cartridge's idea of the
hardware.

**`rcartpack` fails the build** when a listed path is missing from the
bundle, does not follow the `<name>.arkadehid.json` naming rule, does not
validate, or when the capability is not declared. At *mount* the same
checks are a warning and a skip: a game whose second profile is malformed
starts with the panel working rather than refusing to launch with a
player standing in front of it.

## Writing the cartridge

`cartlib` wraps the imports. Everything below is allocation-free:
binding hands the host the name string's own bytes, a read is one call
returning a number, and `Blob` copies into a buffer you own.

```go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// Breakout with a spinner if there is one, and with the stick if there
// is not. Both paths ship; the cabinet decides which one runs.
type Game struct {
	wheel cartlib.Channel
	west  cartlib.Channel
	east  cartlib.Channel
	fire  cartlib.Channel

	paddleX int
}

// Init is the ONLY place a bind can happen: the Setup is the binder and
// it dies when Init returns. Keep the handles; check Ok once, here,
// rather than every frame.
func (g *Game) Init(s *cartlib.Setup) {
	g.wheel = s.BindDelta(cartlib.P1 + "spinner")
	g.west = s.BindButton(cartlib.P1 + cartlib.ChanJoyWest)
	g.east = s.BindButton(cartlib.P1 + cartlib.ChanJoyEast)
	g.fire = s.BindButton(cartlib.P1 + cartlib.ChanA)
	if !g.wheel.Ok() {
		cartlib.Log(cartlib.LogInfo, "no spinner here — playing on the stick")
	}
}

func (g *Game) Update(dtMicros uint64) {
	if g.wheel.Ok() {
		// A delta is MOTION since the last frame, already accumulated by
		// the host, and the read consumes it.
		g.paddleX += g.wheel.Delta()
	} else {
		if g.west.Button() {
			g.paddleX -= 2
		}
		if g.east.Button() {
			g.paddleX += 2
		}
	}
	if g.fire.Button() {
		g.launch()
	}
}

func (g *Game) launch() {}

func main() {} // a cartridge's real main calls cartlib.Run
```

A blob wants a buffer that lives as long as the game, because a
cartridge runs under `-gc=leaking` and must not allocate after init:

```go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

type MatGame struct {
	mat cartlib.Channel
	buf [cartlib.MaxBlobBytes]byte // sized once, in the struct
}

func (g *MatGame) Init(s *cartlib.Setup) { g.mat = s.BindBlob(cartlib.P1 + "mat") }

func (g *MatGame) Update(dtMicros uint64) {
	n := g.mat.Blob(g.buf[:])
	for _, pressure := range g.buf[:n] {
		_ = pressure
	}
}

func main() {} // a cartridge's real main calls cartlib.Run
```

## Worked example

`assetfile/testdata/dancepad.arkadehid.json`, which
`assetfile`'s own tests read back field for field:

```json file=dancepad.arkadehid.json
{
  "arkade": 1,
  "kind": "hid",
  "name": "dancepad",
  "description": "A four-arrow dance pad plus a spinner and a pressure sensor.",
  "generator": {
    "tool": "assetfile-fixtures",
    "version": "1.0.0"
  },
  "device": {
    "vendor": 1356,
    "product": 2508,
    "name": "Dance pad"
  },
  "channels": [
    { "name": "pad.up",   "kind": "button", "report": { "byte": 3, "bit": 4 } },
    { "name": "pad.down", "kind": "button", "report": { "byte": 3, "bit": 5 } },
    { "name": "spinner",  "kind": "delta",  "report": { "byte": 5, "size": 2, "signed": true } },
    { "name": "pressure", "kind": "blob",   "report": { "byte": 8, "size": 32 } }
  ],
  "standard": {
    "pad.down": "joy.south",
    "pad.up": "joy.north"
  }
}
```

Reading it: the pad's two arrows are single bits of report byte 3, and
the `standard` block makes them the panel's stick pushed north and south
— so a cartridge that only ever reads `p1.joy.y`, and has never heard of
a pad, plays. The spinner is a signed 16-bit little-endian value in bytes
5 and 6, with no panel equivalent at all: it is reachable only as
`p1.spinner`, and only on a host with raw HID. The pressure strip is 32
raw bytes at offset 8. The file's minimum report length is 40 bytes
(`8 + 32`); anything shorter decodes to nothing.

## A worked conversion: the built-ins

The engine's own device profiles — Speedlink, DragonRise, Buffalo,
Sonicon, and DualShock (one file for USB, one for Bluetooth) — are
`.arkadehid.json` files under `internal/controller/profiles/`, embedded
and compiled through the same path a bundled profile takes. There is no
private code path for them, so every real device the engine supports is
proof the format can express it.

The three transform verbs are exactly what that conversion needed:

* **Sonicon's hat** is one byte standing for eight directions plus
  centre, so its `joy.x` is `{"byte": 2, "map": [0, 3, 2, 1, 0, -1, -2, -3, 0]}`
  and its `joy.y` a second map over the same byte.
* **DualShock's hat** is four bits in byte 5's low nibble, so it is the
  bit form plus a 16-entry map:
  `{"byte": 5, "bit": 0, "bits": 4, "map": [...]}` aliased onto `dpad.x`.
* **Speedlink and DragonRise's sticks** are unsigned bytes centred on 128
  and 127: `{"byte": 1, "bias": 128}`.
* **Buffalo and DualShock's sticks** add the centre-noise filter:
  `{"byte": 0, "bias": 128, "divide": 10}`.

The proof that the conversion is faithful is that
`internal/controller/decoder_test.go` was **not touched**: every report it
feeds now runs through the compiled JSON and produces the identical event
stream.

## Making one

Open hidlab at `arkade.build/dev/tools`.

Plug the device in, capture reports while you press things, and hidlab
shows which bits moved. Name a channel, pick its bytes or bits and its
kind, and the decoded value updates live as you keep pressing — which is
the part that catches an off-by-one in a bit offset before it becomes a
mystery on the cabinet. Alias what should reach the panel in the
`standard` block, then export the `.arkadehid.json` and add the path to
the cartridge's manifest.
