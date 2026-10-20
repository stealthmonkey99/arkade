# Lamp sequence reference

This documents the `lights` kind of the asset envelope introduced on
[Asset file reference](reference-assets.md): a `.arkadelights.json` file
is one piece of **lamp choreography** — frames of per-seat button-lamp
masks with hold times and a loop mode. A cartridge lists these in its
manifest, and the host walks the frames on the tick, so a flourish costs
a play call instead of a timer, a frame index and a switch statement
living in `Update` for the rest of the game's life. Read
[Asset file reference](reference-assets.md) first for the envelope fields
every kind shares (`arkade`, `kind`, `name`, `generator`, `recipe`, ...);
for a JSON kind, the document **is** the file.

periphlab is the easy way to produce one — it draws the six lamps per
seat as a panel, gives every frame its own hold, plays the sequence on
the page, pushes it down the dashboard relay so it plays on a real
cabinet's buttons, and writes the file straight into the cartridge's
`assets/lights/`. This page is for reading or hand-editing the JSON.

## Fields

A lamp sequence adds two fields beside the common ones.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `frames` | array of frame objects | yes | The sequence, in play order. At least one, at most **256**. |
| `loop` | string | no | `"cycle"` (default), `"once"` or `"pingpong"`. See [Loop modes](#loop-modes). Anything else is repaired to `"cycle"`. |
| `name` | string | yes | The envelope's shared `name`, and for this kind it is the *cue's* name rather than the file's — `"win"`, `"your-turn"`. periphlab derives the filename from it. |
| `description` | string | no | One line saying what the cue **means**, not what it looks like. "Two flashes of the whole panel: the run is over" earns its place; "flashing lights" does not. |

`recipe` has no meaning for this kind and is normally absent: the frames
ARE the artefact, there is nothing to re-render them from.

### A frame

```json
{ "lamps": { "p1": 63, "p2": 0 }, "holdMs": 90, "fadeMs": 150 }
```

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `lamps.p1` | integer `0`–`63` | at least one of the two | Seat 1's whole lamp state while this frame shows. See [Mask bits](#mask-bits). |
| `lamps.p2` | integer `0`–`63` | at least one of the two | Seat 2's, likewise. |
| `holdMs` | integer | no | How long this frame is held, in milliseconds. `10`–`10000`; anything outside that is repaired (see [Error versus repair](#error-versus-repair)). Absent reads as `0`, which is below the floor, so a frame with no `holdMs` gets the `120` ms default. |
| `fadeMs` | integer | no | How long the lamps take to cross-fade *into* this frame's masks when it is entered, in milliseconds. Absent or `0` is a hard cut. `holdMs` counts from the same moment the fade starts, so the fade runs inside the hold — a `fadeMs` longer than `holdMs` is cut short by the next frame. |

A frame is a **state**, not an edit: the mask it names for a seat is that
seat's *whole* lamp state while the frame is showing, which is why the
host writes it with `SetButtonLights(seat, mask, only=true)`. Every lamp
the mask does not light is a lamp it turns off. A frame carrying a
`fadeMs` writes the same whole state, but as a cross-fade rather than a
cut — see [Playback semantics](#playback-semantics).

An **absent** seat key is not the same as a mask of `0`.

- **Absent** means the frame says nothing about that seat: its lamps stay
  exactly as they were, whether that is what an earlier frame of this
  sequence set or what the game itself last wrote. This is what lets a
  win pulse for the player who won leave the other seat's panel alone.
- **`0`** means explicitly dark.

A frame naming *neither* seat is refused. A frame that changes nothing is
a longer hold on the frame before it, and it should be written as one.

### Mask bits

Six lamps per seat — the arcade buttons in front of it, the same six
`host_set_button_lights` addresses — packed one per bit:

| bit | value | name |
| --- | --- | --- |
| 0 | 1 | `joya` |
| 1 | 2 | `joyb` |
| 2 | 4 | `joyc` |
| 3 | 8 | `joyd` |
| 4 | 16 | `joyaux1` |
| 5 | 32 | `joyaux2` |

So `63` is every lamp, `0` is dark, `8` is the D button alone. That is
`internal/hardware.LightButtons`' own order — the MCP23017 pin offset
within a seat's port — and the same order the wire mask
`host_set_button_lights` takes is numbered in, so a mask written by hand
here and a mask written by hand in a cartridge mean the same thing.
`assetfile.LightButtonNames` is the list, checked against
`hardware.LightButtons` by a test so the two cannot drift.

A mask outside `0`–`63` is an **error**: it names a lamp the cabinet does
not have, so the file describes hardware that is not there.

### Loop modes

| `loop` | Play order for a 4-frame sequence | Ends |
| --- | --- | --- |
| `cycle` | 1 2 3 4 1 2 3 4 … | never; runs until stopped or replaced |
| `once` | 1 2 3 4 | stops, resting on frame 4's lamps |
| `pingpong` | 1 2 3 4 3 2 1 2 3 4 … | never |

`pingpong` does **not** repeat the turnaround frames: a three-frame
bounce is 1 2 3 2, so frame 3 is held once and not twice. That is the
same rule a sprite animation's pingpong follows.

`once` **rests on its last frame**, exactly as a `once` sprite animation
rests on its last cel. If a cue should end with the panel dark, say so —
make the last frame `{"lamps": {"p1": 0}, "holdMs": …}`. If it should end
with one lamp lit (handing the player a button to press), say that
instead.

## Error versus repair

The split is the sprite kind's, not the HID profile's.

**Refused** — the file will not load, on either host, and `rcartpack`
will not build a bundle containing it:

- a `frames` list that is empty or over the 256 cap
- a mask outside `0`–`63`, or negative
- a frame naming neither seat
- an envelope `arkade` version this build does not know, or a `kind` that
  does not match the filename

**Repaired**, silently and identically by both hosts:

- `holdMs` below `10` (including a missing one, which reads as `0`) →
  `120`
- `holdMs` above `10000` → `10000`
- `fadeMs` below `0` → `0`
- `fadeMs` above `10000` → `10000`
- a `loop` value that is none of the three → `"cycle"`

A mask is *wiring* — it names physical lamps — and a nearly-right one
lights the wrong buttons, so it is refused. A hold time, a fade time and
a loop mode are presentation: a 2 ms hold is a human meaning "as fast as
you can" and a misspelled loop mode is a typo, and none of them is worth
refusing to light a lamp over. periphlab shows the repaired values before
you save, so what the file will carry is what the editor displays.

## Listing them in the manifest

```json
"assets": {
  "lights": ["assets/lights/win.arkadelights.json"]
}
```

Unlike `assets.glyphs` and `assets.hid`, this list is not only a
mount-time load list — **it is `host_lights_play`'s namespace**. A
sequence the manifest does not name cannot be played at all; the call
returns 0 and warns. That is deliberate: it bounds the work a host does
at mount, and it turns a mistyped path into a `rcartpack` error rather
than a lamp that quietly never lights.

No capability is required. A lamp sequence writes the same six lamps per
seat `host_set_button_lights` writes, on the
cartridge's own cabinet, for as long as the cartridge is loaded — there
is nothing here a cartridge could not already do by hand. (A HID profile
needs one because it changes how the cabinet reads *physical hardware*,
for whoever else is at the machine.)

Sequences are loaded and validated at mount, before `cart_init`, so
`Init` can already play one. A broken file is **one warning and a skip**:
the game starts with dark buttons rather than refusing to launch.
`rcartpack` and `tools/preflight` apply the identical check and turn it
fatal, where the fix is one edit away and nobody is waiting to play.

Caps, all per cartridge: **16** sequences, **256** frames each, a
**512-byte** path. They live in
`internal/cartridge/wasm/lights.go` and `assetfile/lights.go`, mirrored
in the browser host.

## Playing one

```text
host_lights_play(path_ptr, path_len, seat i32, loop u32) -> u32
host_lights_stop(seat i32)
```

From a cartridge, through cartlib:

```go fragment
cartlib.PlayLights("assets/lights/win.arkadelights.json", seat, cartlib.LightsLoopFile)
cartlib.StopLights(cartlib.SeatBoth)
```

`PlayLights` reports whether anything is now playing. `false` means no
such sequence, `cartlib.SeatNone` (which never reaches the host), or a
sequence with no frames for the seat asked for — the host logs the first
and the last. It is
allocation-free: the host reads the path's own bytes in place, so the
string constant every real call site uses costs nothing.

**`seat`** is a `cartlib.Seat`: `SeatP1` or `SeatP2` for one seat, or
`cartlib.SeatBoth` for "both seats, as the file says". Because a frame
only writes the seats it names, a one-seat cue played at `SeatBoth`
lights one seat and leaves
the other alone. Naming a seat the file never mentions **fails** rather
than silently doing nothing.

`seat` narrows; it never **moves** a cue. A sequence whose frames only
name `p1`, played at `SeatP2`, does not light seat 2 — a mask is
per-seat data, and re-seating it would invent choreography the author
never wrote. A cue meant for either seat has to say both.

**`loop`** overrides the file's own mode: `LightsLoopFile` (0) uses it,
`LightsLoopOnce` (1) forces a single pass, `LightsLoopCycle` (2) forces
endless repetition. That is what lets one file be both a one-shot
flourish and a standing attract-mode idle. There is deliberately no
"force pingpong": a pingpong is a property of how the frames were drawn
(they read the same forwards and backwards), so forcing it onto a file
that was not authored for it would produce a cue nobody designed.

### Playback semantics

**Frame 0 lands on the play call itself.** A cue started inside
`cart_update` is already on the panel for the frame that asked for it.

**Everything after it is the tick's.** The host advances playback once
per tick, immediately before `cart_update`, by the tick's own
`dt_micros` — the same number the guest is about to receive, which is the
engine clock's delta. Never a wall clock, never a timer thread. That is
what makes a lamp cue as reproducible as the rest of a cartridge: a
sequence started on frame 100 of a replay lights exactly the same lamps
on exactly the same frames, every time.

**A lamp write happens only when the frame changes.** A 90 ms frame at 30
fps costs one `SetButtonLights`, not three. A hard-cut frame (no
`fadeMs`, or `fadeMs: 0`) is still exactly that `SetButtonLights(seat,
mask, only=true)` call. A frame with a `fadeMs` writes instead as one
`host_set_lamp_levels` call touching all six lamps (`touch = 63`),
target level `15` where the mask lights a lamp and `0` where it does
not, ramped over `fadeMs` — the same host machinery
[Levels and fades from code](#levels-and-fades-from-code) describes for
a cartridge's own calls, so a sequence's fade and a hand-written one
behave identically.

**The explicit call wins.** `SetButtonLights` (or `SetLights`) on a seat
**stops** whatever sequence is playing there, in the same call. Without
that rule the two writers would fight at tick resolution and the loser
would be whichever ran second, which is not a rule anybody can hold in
their head. With it, a cartridge can always take its lamps back by doing
the obvious thing, and never has to remember a `StopLights` first.

**`StopLights` is not "go dark".** It stops the walk and leaves the lamps
exactly where the last frame put them. A cartridge that wants dark says
so with `SetButtonLights(seat, 0, true)` — which stops the sequence
anyway, so the two happen in one call.

**Teardown drops the playback and the sequences**, and leaves the lamps
alone. The shell writes its own the moment it comes back, and a
cartridge's plain `SetButtonLights` is not undone at teardown either.

### In the browser

Fully live, and this is the one asset kind where the browser has no
caveat at all. A lamp is an output, the player page draws six of them per
seat, and the walk runs on the same tick deltas — so a sequence plays in
a browser tab exactly as it plays on the cabinet, frame for frame, with
the same validator and the same warning sentences. A cue can therefore be
developed anywhere, and a headless test asserts the same call sequence on
either host.

## Levels and fades from code

A lamp is not only on or off: each of the six per seat runs at a level
`0`–`15` (`cartlib.LampLevelMax`), and `SetButtonLights`/`SetLights` are
the degenerate case — they write `15` and `0`.
`cartlib.SetLampLevels(seat, levels, touch, fadeMs)` writes all six
explicitly; `cartlib.FadeLamps(seat, mask, level, fadeMs)` is the
one-line form for the common case, "bring these lamps to this level
over this long."

```go fragment
cartlib.FadeLamps(seat, cartlib.MaskJoyD, cartlib.LampLevelMax, 0)
// 300ms later
cartlib.FadeLamps(seat, cartlib.MaskJoyD, 0, 600)
```

That is a tone-button flourish: a pressed button snaps to full the
instant it registers (`fadeMs` 0), and 300 ms later fades back down to
dark over 600 ms.

`touch` (`mask` for `FadeLamps`) names only the lamps a call addresses;
every other lamp keeps its level and any fade already running on it.
`touch == 0` is a complete no-op — alone among the lamp calls, it does
not even stop a running sequence.

**The explicit call wins**, exactly as for `SetButtonLights`:
`SetLampLevels`/`FadeLamps` stop any sequence playing on the seat(s)
they touch (unless `touch`/`mask` is empty, above), and anything that
later touches a lamp — `SetButtonLights`, `SetLights`, another levels
call, or a lamp-sequence frame landing on it — cancels whatever fade was
running on that lamp and starts fresh from wherever it had reached.

**A level is intent, not the wire value the hardware ends up at.** The
SYSTEM shelf's BRIGHT tile cycles a cabinet's lamp policy — LAMPS AUTO
(default: lamps scale with the LED panel's own brightness), LAMPS ON,
LAMPS OFF — and that scaling is applied to the effective level *after*
the cartridge's call, invisibly to the cartridge. A cartridge always
writes the level its cue means; it never needs to know or branch on
which policy the cabinet is running. The browser player has no such
setting: a lamp draws at `level/15` opacity of its lit look.

Physically, the cabinet drives this by bit-angle-modulating the
MCP23017 pins at 100 Hz — four writes per frame, shared by every lamp —
and latches once with no modulation when every lamp on a seat is at `0`
or `15`, the same wire the on/off calls have always used.

## Worked example

This is `assetfile/testdata/pulse.arkadelights.json`, the package's own
fixture — a win pulse for one seat: two flashes of the whole panel, then
the D button alone, once.

```json file=pulse.arkadelights.json
{
  "arkade": 1,
  "kind": "lights",
  "name": "pulse",
  "description": "Win pulse: two flashes of every lamp, then the D button alone.",
  "tags": [
    "fixture"
  ],
  "generator": {
    "tool": "assetfile-fixtures",
    "version": "1.0.0"
  },
  "frames": [
    {
      "lamps": {
        "p1": 63
      },
      "holdMs": 90
    },
    {
      "lamps": {
        "p1": 0
      },
      "holdMs": 90
    },
    {
      "lamps": {
        "p1": 63
      },
      "holdMs": 90
    },
    {
      "lamps": {
        "p1": 8
      },
      "holdMs": 200
    }
  ],
  "loop": "once"
}
```

Read it back: 63 is all six lamps, 0 is dark, 8 is bit 3 — `joyd`, the
blue button. It never mentions `p2`, so playing it at `SeatBoth` lights
seat 1 and leaves seat 2's panel exactly as the game left it, and playing
it at `SeatP2` fails with a warning rather than doing nothing. It
ends holding one lamp for 200 ms, which is the shape of a cue that hands
the player a button to press next.

## A worked cartridge: Catcher

`sdk/tutorial` is the SDK tutorial's finished game, and its game-over
beat is the smallest real use of this kind. It ships
`assets/lights/gameover.arkadelights.json` — four frames, both seats
named, ending dark — lists it in its manifest, and plays it at game
over:

```go fragment
const lightsGameOver = "assets/lights/gameover.arkadelights.json"

func playGameOverLights(seat cartlib.Seat) {
	if seat == cartlib.SeatNone {
		return
	}
	cartlib.PlayLights(lightsGameOver, seat, cartlib.LightsLoopFile)
}
```

The cue's last frame is dark, the same state one
`SetButtonLights(seat, 0, true)` would leave, so the flourish adds to
the moment without changing what follows it.
[tutorial-assets.md](tutorial-assets.md) walks through building it.

## Editing one

Open periphlab at `arkade.build/dev/tools` and pick the Lights room.

periphlab's Lights room is the editor: a frame timeline over the twelve
lamps, the six buttons per seat drawn as a cluster, a hold and a fade
per frame, the loop mode, add/duplicate/remove/reorder, **Play here** on
the page's own panel, and **Preview on cabinet** — which pushes the
sequence down the dashboard relay so it plays on real 12 V buttons
before it is ever in a bundle, fade included. Whether a 120 ms frame
reads as rhythm through the MCP23017's write latency, or a fade reads as
smooth rather than steppy, is a look-at-it question, and that button is
how you look at it.

Save writes the file into the current cartridge's `assets/lights/` and
prints the `assets.lights` line to add to the manifest. On the hosted
face there is no cartridge and no local process, so Download and Open a
file are the two halves that still work.
