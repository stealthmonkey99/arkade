# The input flight recorder

The cabinet is always recording the cartridge it is running. When something
goes wrong — a trap, a ghost through a wall, a score that shouldn't have been
possible — that recording becomes a file, and one command re-lives the exact
session at your desk.

```sh
arkade --replay data/flight/20260905T204431Z-cart_snake-crash.arkflt
```

This is what determinism is *for*. The cartridge ABI gives a guest exactly
three inputs from the outside world — the `dt_micros` handed to
`cart_update`, the packed input records handed to `cart_handle_input`, and
the values handed back by `host_random_u64` — and nothing else. Write those
three streams down and you own the session. `docs/GAME_AUTHORING_AI.md`'s
["Debugging a wasm crash on the cabinet"](GAME_AUTHORING_AI.md) goes from
archaeology in `journalctl` to a one-command repro.

---

## What is recorded

One pre-allocated ring, per cartridge launch, of fixed-size 16-byte events:

| Kind | Payload | Cadence |
|---|---|---|
| `frame` | the tick's `dt_micros` | one per engine tick |
| `input` | one packed 8-byte ABI input record | one per record the guest receives |
| `random` | one `host_random_u64` result | one per draw |
| `channelBind` | one successful `host_input_bind`: handle, kind, blob length | once per bind, at `cart_init` |
| `channel` | a bound channel's value (handle, kind, `i32`) | when it changed and, for a delta channel, whenever non-zero |
| `blob` | one chunk (up to 6 bytes) of a changed blob channel | one per chunk |

The three channel kinds carry the non-panel input channels. Panel channels are
rebuilt from the `input` records, so they are not recorded separately.

plus a header written once at launch: the slug, the cartridge's manifest
version, the **SHA-256 of the exact `.wasm` bytes**, the engine seed, the
target FPS, the panel size, the build version, the platform and the start
time.

Three things are worth spelling out, because each is a trap that was
deliberately avoided:

**Input is recorded where the guest receives it**, in
`Cartridge.DispatchInput` in `internal/cartridge/wasm` — *after* the seat-mount
remap, *after* standby gating, *after* the player-strategy gate. A recording
made any earlier would replay a different game, because a controller's RIGHT
is not the cartridge's RIGHT (see `internal/engine`'s `remapEvent`).

**Random values are recorded, not just the seed.** The engine RNG is a
*single stream* shared with the shell, attract and the screensaver, so a
cartridge's position in it depends on everything that ran before it. Replaying
the seed alone would put the cartridge somewhere else in that stream. The
replayer serves the recorded draws back instead.

**Only cartridge sessions are recorded.** A native app has no wasm to pin and
no ABI input records; recording one would produce a file nothing could
replay. With the shell up, `POST /api/flight/dump` answers `409`.

### The bound

The ring is bounded by **memory, not time** — "minutes" is a property of how
busy a session is, and the memory is what actually has to be bounded on a Pi.

| | |
|---|---|
| Default capacity | 131072 events (`engine.FlightRingEvents`) |
| Memory | **2 MiB**, allocated once at launch |
| Floor at 30fps | ≈ **72 minutes** of pure ticking (frame events alone) |
| Realistic play | ≈ **40–55 minutes** for a busy two-stick game; longer for a turn-based one |

Frame events dominate: one per tick, versus a handful of input records a
second and however much randomness the game draws. A single arcade session is
minutes; this is sized so a whole evening on one cartridge still fits.

When the ring *does* wrap, the header records how many events were overwritten
and sets `truncated`. **A truncated recording is not replayable** — it starts
mid-session, from a state the cartridge never reached that way. The replayer
says so, loudly, before it starts.

### Cost

Zero allocation in the steady state: the ring is allocated once and then only
overwritten. One mutex acquisition and one array store per event.
`TestRecorderDoesNotAllocate` (`internal/engine/flight_test.go`) pins
that with `testing.AllocsPerRun`.

The recorder is **observe-only**, the same posture as
the on-hardware asset preview: it never injects input, never touches the
standby machine, never loads or unloads anything, and never draws from the
engine RNG on its own account. A golden-tested cartridge produces identical
frames whether or not it is running.

---

## The three triggers

### 1. A crash (automatic)

TinyGo's `-panic=trap` turns every runtime panic into a bare `unreachable`,
which wazero surfaces to the host as an ordinary error from
`cart_update`/`cart_render`/`cart_handle_input`. That error path is the hook:
the first fault in a session reaches `Runtime.NoteCartridgeFault`
(`internal/engine/flight.go`), which calls `dumpAndLog` to write a dump with
`reason: "crash"` and the trap message as its note.

Only the first. A trapping guest re-traps every single frame — the log spam is
the symptom you already know — and thirty dumps a second would rotate the
useful evidence off the disk in under a second.

A cartridge that fails to *launch* (corrupt module, wrong ABI version, a
`cart_init` that trapped) also dumps. The recording holds almost nothing, but
its header holds the slug, the version and the wasm hash — which is exactly
what "the stick in the cabinet isn't the build you think it is" looks like.

### 2. `POST /api/flight/dump` (manual)

The "that was weird" button, over the same tunnel as everything else
([DASHBOARD.md](DASHBOARD.md)):

```sh
ssh -N -L 8080:127.0.0.1:8080 "$ARKADE_HOST" &
export ARKADE_HTTP_TOKEN=$(ssh "$ARKADE_HOST" 'systemctl show arkade -p Environment' | sed 's/.*ARKADE_HTTP_TOKEN=//')

curl -sX POST -H "Authorization: Bearer $ARKADE_HTTP_TOKEN" \
  "http://localhost:8080/api/flight/dump?note=ghost+walked+through+a+wall"
```

```json
{
  "path": "data/flight/20260905T204431Z-cart_snake-manual.arkflt",
  "slug": "cart/snake", "reason": "manual",
  "events": 9214, "frames": 9012, "inputs": 190, "randoms": 12,
  "droppedEvents": 0, "truncated": false, "bytes": 21873
}
```

Same posture as the `/api/preview/*` family and for the same reasons: bearer
token, fail-closed with no `ARKADE_HTTP_TOKEN` configured (`503`),
Host-header validated (`421` for the cabinet's `.local` hostname — use `localhost` or an IP),
audited to the log ring accepted or rejected.

| | |
|---|---|
| `?note=` | free text stored in the header; ≤ 200 characters, no control characters |
| Rate limit | burst **3**, then **one every two seconds**; `429` with `Retry-After: 2` |
| `409 Conflict` | no cartridge session is being recorded |

Two deliberate differences from the preview routes:

- **Its own rate-limit budget.** A tool spraying previews must not be able to
  lock out the one dump somebody is taking of the crash they just saw.
- **`200`, not `202`.** A dump is not queued for the engine's next tick; it
  reads the ring under the recorder's own lock and writes the file on the
  request's goroutine. By the time you get the response the file exists — and
  you get its path, which is the only thing you actually wanted.

### 3. The debug combo on the pads (physical)

Hold 🟣 **Aux1** and push **UP UP DOWN DOWN LEFT RIGHT LEFT RIGHT** — the
fleet-wide debug combo (`sdk/cartlib/cheat.go`). No laptop, no tunnel, no
token.

**Why the host detects it a second time.** The combo is entirely guest-side:
`cartlib.CheatCode` is compiled into the cartridge and the host never learns
that it fired. There is no `host_` import for "the cheat happened", and adding
one would be an ABI addition that every cartridge would need rebuilding
against before it did anything.

It does not need one. `CheatCode.Feed` is a pure function of the record
stream — *"there is no timer and no dtMicros anywhere in here"* — and the
recorder already has that exact stream at the exact moment the guest gets it.
So `internal/engine/flightcombo.go` runs its own copy of the detector over the
records it is already recording, including all four rotations (the cabinet
remaps every stick by its seat mount, so the literal sequence is not what a
player physically pushing the code produces). Nothing is added to the ABI,
nothing is rebuilt, and a cartridge that never adopted the combo still gets
the trigger.

The cost is a second implementation of a rule that lives in `cartlib`, and it
is bounded: the two are independent, so drift makes the host detector fire at
a slightly different moment or not at all — a missed debug dump, never a wrong
frame on the panel. The ABI ordinals it hardcodes are pinned by
`TestFlightComboOrdinalsMatchTheABI`.

**A cartridge that uses the combo as a gameplay shortcut** (a debug shortcut that
jumps the board or skips a level) will also produce a dump. That is intended — entering a
debug code is about as clear a "something interesting is happening here"
signal as a pad can send — and it is capped at one per cartridge session.

---

## Where dumps live

`<data>/flight/`, named `<timestamp>-<slug>-<reason>.arkflt`. The **last 8**
are kept; older ones are deleted after every write, oldest first. A second
dump inside the same second gets a `.N` sequence before the extension
(`<timestamp>-<slug>-<reason>.2.arkflt`, then `.3`, and so on) rather than
silently overwriting — two dumps in the same second is exactly the "manual
dump, then the crash it was chasing" case. The listing, and so the rotation,
sorts by timestamp and then by sequence within the second, which is the order
the dumps were taken; plain lexical filename order is not relied on.

The write goes through a temp file and a rename, so a dump taken mid-crash can
never leave a half-written file that looks like a recording.

```
--flight <dir>       # override the directory (default <data>/flight)
--flight=off         # disable the recorder entirely: no ring, no dumps
--flight-keep <n>    # retain n dumps instead of 8
```

Default-on is deliberate. A recorder you have to remember to enable is a
recorder that is off on the one evening the bug shows up.

---

## The file format

`internal/flight`. A JSON head for the handful of facts a human reads first,
and a flat binary body for the up-to-millions of fixed-size events. The whole
payload is gzipped above 1 KiB, which takes a full 2 MiB ring to well under a
tenth of that — that is what makes "attach it to the issue" true rather than
aspirational.

```
offset 0   "ARKFLT"            magic
offset 6   u8                  format version (1)
offset 7   u8                  flags; bit 0 = payload gzipped
offset 8   payload:
             u32  headerLen
             …    header JSON
             u32  eventCount
             …    eventCount × 16 bytes:
                    u32 frame      (tick index within the session)
                    u8  kind       (0 frame, 1 input, 2 random,
                                    3 channelBind, 4 channel, 5 blob)
                    u8  ×3         padding, zero
                    u64 data       (dt_micros | packed record | rng value |
                                    packed channel bind | packed channel value |
                                    packed blob chunk)
```

All integers little-endian. The `frame` index is **per session**: the engine
resets its tick counter on every app load, so frame 1 is always the first tick
after the cartridge was loaded and frame 0 is `cart_init`'s own work.

The header's `frames`, `inputs` and `randoms` count the retained events of
each of the first three kinds; `channels` counts the retained events of kinds
3 to 5 together and is omitted when there are none. Readers skip kinds they do
not know.

The **format version** moves only for a breaking change to that layout, never
for a new header field — the header is JSON precisely so additive changes need
no version move.

Decoding treats the file as untrusted (it is, after all, a file somebody
emailed you): every length is capped before anything is allocated for it, the
event array is read in chunks so an over-claimed count cannot make a truncated
file allocate 64 MiB, and a gzip bomb is bounded by a decompressed-size limit.

---

## Replaying

```sh
arkade --replay <file>              # load, verify, re-live the session
arkade --replay <file> --replay-paused
arkade --replay <file> --replay-force
```

`--replay` is a whole other program: it opens no cabinet hardware, starts no
dashboard and records nothing. It resolves the recorded slug through the
*ordinary* cartridge registry (same scan dirs, same `--cartridges` flag), so a
cartridge that boots normally replays normally.

**Controls** (read straight off the terminal in raw mode — an engine keypress
would be dispatched into the cartridge being replayed and corrupt the very
session under test):

| Key | |
|---|---|
| `space` | pause / resume |
| `.` or `n` | step one frame (and stay paused) |
| `q`, Ctrl-C | quit |

The frame counter rides in the console stats line, in the slot the input-mode
chips normally occupy: `REPLAY 412/9012 (f412)`.

The replay is **silent**. The recording carries no audio state, the mixer's
scheduling is wall-clock, and a half-reproduced soundtrack over a frame-exact
replay would only mislead.

### Hash pinning, and `--replay-force`

A recording replays against the exact `.wasm` that produced it, or it replays
against nothing. Same slug, different bytes, is a different game wearing the
same name, and a replay of it would *look* like a reproduction while being a
fiction. So a mismatch is refused, with both hashes and both manifest
versions printed:

```
wasm mismatch for cart/snake:
  recorded: 3f9a…  (manifest version "1.4.2")
  installed: c1b7… (manifest version "1.4.3")
  …
  Pass --replay-force to replay anyway.
```

`--replay-force` exists for one legitimate workflow: you have rebuilt the
cartridge to add logging and want the same inputs against the new build. That
is real, and the refusal must not stand in its way — it just must not be the
default, and it must say what it is doing.

---

## Limits — read these before trusting a replay

**Wasm byte-equality is platform-specific.** Rebuilding a cartridge from
identical source on a different machine, or a different TinyGo, can produce
different bytes and therefore a different hash. A hash mismatch after a
rebuild is *not necessarily* a different game — but you cannot tell from here,
which is why the refusal is the default. **The fix is to fetch the `.wasm`
from the cabinet, not to rebuild it.** Recordings should travel with, or pin,
the exact binary; the header records the recording platform for this reason.

**A truncated recording does not reproduce the session.** If the ring wrapped
(`truncated: true`), the replay starts from a state the cartridge never
reached that way. Treat it as a *sample of the tail*, not a repro.

**Standby ends a recording.** When the screensaver parks a running cartridge,
recording stops and does not resume — a recording that skipped the suspension
would replay as a different game. The ring is kept, so a crash after the wake
still dumps everything up to the suspension, marked `suspendedAtFrame`; the
replay of it ends early rather than diverging silently.

**Only the guest is reproduced.** Scores, storage, recent names and the
cabinet's persisted settings are *not* in the recording. A cartridge that
branches on `host_score_read` or `host_storage_read` will diverge if your
local `data/` differs from the cabinet's. Copy `data/cart/<slug>` across if a
bug depends on it.

**The peripherals are outputs, not inputs.** LCD text, button lights and
sounds are things the guest *does*; nothing about them feeds back into the
sim, so they are not recorded and a replay simply re-performs them.

**Native apps are not recorded at all** — the shell, attract, boot, config.
Only cartridges.

**Wall-clock timestamps are decoration.** `startedAt`/`dumpedAt` are for the
human reading the header. Nothing in a replay depends on them: the guest's only
clock is `dt_micros`, which lives in the frame events.

---

## Limitations

- The pad trigger fires on the same input records the guest receives; it has
  been exercised with synthetic joystick records and a fixture cartridge.
- The ring bound in minutes is arithmetic from the event rate. The
  `droppedEvents` field of a dump shows whether a given session wrapped.
- A dump copies up to 2 MiB under the recorder's lock on the engine goroutine
  for the crash and combo triggers, then gzips and writes outside it. The
  write is storage-bound and off the tick path.
