# Årkade Cartridge ABI: design record

The contract itself (exports, imports, input records, the runtime
manifest, sandbox caps, the two hosts, toolchains) is
[`sdk/docs/abi.md`](../sdk/docs/abi.md), with bundles in
[`packaging.md`](../sdk/docs/packaging.md) and each asset and peripheral
in its `reference-*` page. That set is canonical. This file keeps the
reasons behind the rules.

**Goal:** one `.wasm` cartridge runs unmodified on two hosts. The cabinet
embeds [wazero](https://github.com/tetratelabs/wazero) (pure Go, no cgo)
in `internal/cartridge/wasm`. The browser is itself a wasm runtime, so
the browser host (the arkade.build player, not in this repo) needs no
engine port: the ABI is the whole portability contract.

## Versioning

`cart_abi_version` identifies the calling convention, not the set of
imports. Additive host imports never move it: a wasm module resolves only
the imports it declares, so a cartridge built before an import existed
instantiates unchanged. The number moves only when a cartridge built
against the old number would fail on the new host (an import removed or
redefined, an export or record layout changed). Hosts state the range
they accept (`MinSupportedABIVersion..SupportedABIVersion` on the
cabinet) and reject anything outside it before `cart_init`, with one
"unsupported ABI version" line instead of a confusing failure deep inside
a frame.

There is no `host_draw_text`. Text is rendered guest-side from shared
font data (`cartlib.DrawText`), because a host-side text import could not
produce the same pixels on the cabinet and on a browser canvas, and a
cartridge using it could never pass the parity test.

## Design rules

1. **No WASI in the contract.** Time, randomness, logging, storage and
   I/O arrive through `arkade`-module imports. Toolchains still emit a
   few WASI imports, so the cabinet registers exactly four
   `wasi_snapshot_preview1` functions (`internal/cartridge/wasm/wasi.go`):
   `fd_write` (fd 1 logs at info, fd 2 at warn, any other fd returns
   `EBADF`), `random_get` (the engine RNG), `clock_time_get` (every clock
   id is the engine clock) and `proc_exit` (an inert no-op). Out-of-bounds
   pointers return `EBADF` or `EINVAL`. Any other WASI import is
   deliberately unregistered, so instantiation fails loudly.
2. **Determinism carries over.** Cartridges never see a wall clock or an
   OS RNG. Everything observable in a frame is a pure function of
   `(dt, input records, host RNG draws)`. That is the test for whether a
   new capability may live guest-side, and it is what lets the golden
   harness replay a cartridge and diff it pixel for pixel across both
   hosts. The enforced parity is cabinet host versus browser host on the
   *same* `.wasm`, deliberately not cartridge versus native Go: `cartlib`'s
   RNG cannot reproduce `math/rand`'s `Float64` bit pattern from a raw
   `u64` stream, so a ported game draws a different, equally replayable
   sequence and re-records its goldens once.
3. **Sandbox by construction.** No filesystem, no sockets, no spawn.
   Storage and scores are namespaced by slug with no parameter that could
   name another game's data.
4. **The framebuffer blit is the fast path.** One `host_blit` of the
   whole 64×64 RGBA frame beats 4,096 boundary crossings per frame.
5. **Handles, not pointers.** Sprites and sounds are registered once and
   referred to by opaque `u32` ids the host owns. The host never retains a
   guest pointer past the call.

### Runtime boot

If a module exports `_start` (a TinyGo `-target=wasi` build is a WASI
command), the host calls it exactly once at instantiation, before
`cart_abi_version`. Skipping it leaves TinyGo's `-gc=leaking` allocator
with uninitialized heap-growth bookkeeping, and a cartridge that allocates
steadily eventually corrupts its own memory. It is not a place for game
logic: `main` stays empty and setup stays in `cart_init`. During the boot
call `random_get` returns a fixed pattern (`0xA5 ^ i`) instead of drawing
from the RNG, because TinyGo seeds its map hash there and drawing would
shift every game's stream and every golden; `proc_exit(0)` is silent.
Modules without `_start` (a wasi-sdk reactor exporting `_initialize`) are
not booted.

## Why scores are an import and not just storage

A cartridge could keep a high score under `host_storage_write`, and it
would be invisible: the shell's LCD readout, the attract trophy and the
leaderboard ticker read a game's table by slug from outside the game. So
the ABI exposes the table's operations rather than its bytes. The policy
(top ten, a tie with 10th does not qualify, equal scores rank after
existing equals) lives once in `internal/scores`; handing cartridges raw
bytes would mean each one reimplements it in TinyGo and they drift.

## Player strategy is a cabinet affordance, not a guarantee

A strategy push gates which controller's events reach
`cart_handle_input` and, under `solo`, rotates the panel toward the
seated player. A push alone is inert until `host_lock_player_strategy`,
which is what stops a second controller hijacking a run in progress: push
on the title screen so either controller can start, lock when play
begins. The browser host mirrors gating and rotation (it rotates the
frame and remaps directions itself, since a CSS transform would turn the
picture without the input remap). A two-player game still demuxes on the
record's `player` byte rather than assuming the host filtered anything.

## Raw directions is a runtime override of the manifest, not a replacement

The manifest's `rawDirections` is only the initial state.
`host_set_raw_directions` overrides it until the instance unloads; the
next load starts from the manifest again. "Egocentric gameplay,
screen-spatial menu" is an ordinary combination within one cartridge: a
tank game leaves `rawDirections` at its default so the title menu remaps
normally, calls `cartlib.SetRawDirections(true)` when gameplay begins and
`SetRawDirections(false)` on every path back to the title. The browser
host honours the override in its own remap.

## Input records

The 8-byte layout and the key ordinals are in
[abi.md](../sdk/docs/abi.md#input-records); the canonical ordinal table is
`internal/cartridge/wasm/keys.go`. The rules that make it work:

- **`player` and `physical` are not the same thing.** `player` is the
  pairing slot (who is P1). `physical` is the cabinet port (which LCD and
  lamp bank sit in front of that human). They agree on a default cabinet
  and diverge the moment someone picks up the other controller. Address
  LCDs and lights by `physical`; decide whose piece moves by `player`. A
  plain keyboard carries `0xFF` in `physical` and `index` and reads as
  player 1; a keyboard-simulated controller reports a real seat.
- **The virtual flag is load-bearing.** A real joystick emits both a
  joystick record and a synthetic direction keypress for one push. A
  cartridge acting on both moves twice per push on the cabinet while
  behaving perfectly under a keyboard. Gate on `flags & virtual`.
- **Press-record convention.** Arcade controllers emit a keydown+keypress
  pair per press (`internal/controller/decoder.go`), and both reach the
  cartridge. Keypress (kind 2) is the act-once record; acting on keydown
  too double-fires. Native apps' `QueueKeys` match keypress only, and the
  browser host speaks the same dialect; keydown/keyup carry the hold
  state widgets such as `sdk/ui`'s HoldConfirm track.
- **Rotation-aware remapping is host-side.** The host knows its
  orientation and seat mounts; cartridges see logical directions only.

### Microphone events

Acoustic gestures arrive as keypress records on ordinals 28–38
(`mic.whistle`, `mic.clap`, `mic.blow`, `mic.cue0`…`mic.cue7`); the
pipeline, the model file and training are in
[`reference-mic.md`](../sdk/docs/reference-mic.md).

- **No audio crosses the boundary.** The host decides which gesture
  occurred and discards the samples. That is a privacy property, but
  first a correctness one: a quantised event replays, a waveform does not.
- **Cues come from the cartridge.** The three gestures mean the same to
  every game. A cue means whatever the bundled sound model
  (`assets.mic`, a `<name>.arkademic.json`) says: a matched class fires
  its own `cue` field, installed for the session and dropped at teardown.
- **Opt-in.** Only a cartridge whose bundle manifest declares the `mic`
  capability receives them, and only while it is played, which keeps the
  feature additive.
- **No seat.** One microphone cannot tell which side made the sound:
  `physical` and `index` are `0xFF`, `player` is 1, `source` is keyboard
  and the virtual flag is set. A two-player game routes the gesture to
  whoever holds the turn.
- **Never the only way.** Sound is a second path to an action a button
  already performs, and it costs what the button costs.
- **Clap and blow are disabled**: their detectors fire on joystick clicks
  and the game's own sound effects. Build on the whistle and on cues.

Sound does not wake the cabinet from standby; presence is the motion
sensor's job.

## Manifest

`cart_manifest()` returns the runtime manifest described in
[abi.md](../sdk/docs/abi.md#runtime-manifest). A cartlib cartridge never
writes it by hand: `sdk/tools/manifestgen` generates `manifest_gen.go`
from the bundle's `manifest.json` on every build
([packaging.md](../sdk/docs/packaging.md#one-manifest-two-forms)). `slug`
is the storage namespace and must match the `.rcart` filename stem.

`rawDirections` opts out of the host's seat-mount and orientation remap.
Set it when controls are **egocentric** (relative to the player's avatar:
rotate/thrust, steering), not **screen-spatial** (pointing at an edge of
the panel: a board cursor, a menu). Leaving it `false` on an egocentric
game silently turns a physical direction into a different one on any
seat whose remap is not the identity. It is part of the runtime schema,
so the generated manifest carries it over.

## .rcart bundle format

The tree and caps are in
[packaging.md](../sdk/docs/packaging.md#the-rcart-format). An asset named
`<name>.arkade<kind>.<container>` carries an envelope
([ASSETS.md](ASSETS.md)). `sdk/tools/rcartpack` validates every
`*.arkadesprite.png` it packs and fails the build on a broken one; a
bundle that reaches a cabinet with one anyway logs a warning at mount and
that sheet does not load. Same check (`rcart.sheetWarnings`), two
severities: at pack time the fix is one edit away, at mount time somebody
is standing in front of the machine.

### Path convention for `host_load_sound` inside a `.rcart` bundle

A sound path is bundle-relative, exactly the tar entry name
(`"assets/omnomnom.wav"`, no leading `./`). On the cabinet a mounted
bundle's assets never touch disk: they are registered with
`audio.AssetOverlay` under the slug, so sounds resolve through the same
`audio.Resolver` compiled-in apps use. A bare `.wasm` has no overlay; its
loads fail closed with one warning per distinct missing sound.

## Hosts

The comparison table is in [abi.md](../sdk/docs/abi.md#the-two-hosts).
Cabinet decisions worth knowing:

- wazero runs in compiler mode (JIT on arm64), pure Go, so
  `make build-pi` needs no cross toolchain. A golden recorded on the
  compiler cannot see a compiler fault, so
  `carttest.RequireRuntimesAgree` replays a journey on both the compiler
  and the interpreter.
- `wasm.App` adapts a cartridge onto the engine's `App` contract, so a
  mounted cartridge is indistinguishable from a compiled-in app: shell,
  standby and goldens just work.
- A guest call that traps faults the cartridge: the host logs it, writes
  the flight recorder's crash dump once, blanks the framebuffer, stops its
  sounds and never enters the guest again, not even `cart_teardown`. The
  quit keys still unload it, even when it imports `host_exit`. A failure
  in `cart_init` returns straight to the shell.

Toolchain status is in [abi.md](../sdk/docs/abi.md#toolchains); the
versions to install are in [README.md](../README.md#requirements).

## Testing strategy

- **Conformance**: run a cartridge under wazero in the `testkit` rig
  (manual clock, seed 42, capture renderer) and compare golden frames byte
  for byte (`internal/cartridge/wasm/sdk_template_test.go`; each
  cartridge's `*_golden_test.go` through `carttest`).
- **Fuzz the boundary**: malformed manifests, out-of-range pointers,
  oversized blits. The host rejects without panicking; a bad cartridge
  may never take down the cabinet.
- **Browser host**: the same goldens replayed against its implementation
  and diffed frame by frame.

## Clarifications

Details the reference implementation pins and the browser host must
match byte for byte:

1. `host_blit` takes raw interleaved `R,G,B,A` bytes, row-major, exactly
   `cols*rows*4` long, not packed `0xRRGGBBAA` words, so it is a browser
   `ImageData` buffer with no repacking.
2. Key ordinals are the declaration order in
   `internal/cartridge/wasm/keys.go`; the browser host vendors that table.
3. `cart_update`'s `dt_micros` is the per-tick delta, not total duration.
4. `host_play_sound`'s `0xFFFFFFFF` loops forever; any other value is a
   literal count.
5. The `player` byte is never 0: a seatless event reads as player 1 and
   `source` tells it apart.
6. TinyGo cannot emit multi-result exports (it lowers
   `cart_manifest() -> (ptr, len)` to a hidden out-pointer even with
   `+multivalue`). `tools/wasmfix` runs after every TinyGo build: it
   verifies the body stores two constants, appends a genuine
   `() -> (i32, i32)` function and repoints the export, failing loudly on
   anything else.
7. `cart_render` may precede the first `cart_update`; a cartridge renders
   a valid frame from its post-`cart_init` state.
8. A `-scheduler=none -gc=leaking -panic=trap` TinyGo cartridge imports
   only `proc_exit` and `random_get` from WASI.

## Peripheral host functions

Two 16×2 LCDs, six lit buttons per player and a way to leave
(`host_set_text`, `host_set_button_lights`, `host_set_lights`,
`host_set_lamp_levels`, `host_define_lcd_glyph`, `host_exit`). Signatures,
mask and nibble tables, and the glyph rejection table are in
[abi.md](../sdk/docs/abi.md#semantics-worth-pinning).

- **`player`** is `-1` for both sides, `0`/`1` for one; anything else is
  warn-and-no-op, like every other bounds check.
- **`host_set_text`** caps each line at 64 bytes on the wire while the LCD
  clips at 16 runes, leaving headroom for multi-byte glyphs like `♥`.
- **Button masks**: bit *i* is `hardware.LightButtons[i]` (`joya`, `joyb`,
  `joyc`, `joyd`, `joyaux1`, `joyaux2`), the same pin order every
  `hardware.Suite` caller uses; `buttonOrdinal` in `keys.go` aliases it
  and the browser host vendors the same order. `only` mirrors
  `hardware.Suite.SetButtonLights`.
- **`host_define_lcd_glyph`** lets a cartridge bring its own 5×8 bitmaps
  without an engine rebuild. The eight rows are packed into one `u64`
  because 8 × 8 bits is exactly 64: no guest pointer to bounds-check and
  nothing to allocate, which keeps cartlib's wrapper allocation-free. The
  return is a pure function of the arguments and the cartridge's own
  table, never of whether the host has an LCD (`wasm.GlyphDefiner` is an
  optional host interface; the bridge owns validation), so a cartridge
  cannot branch on its host. Definitions shadow the engine palette for
  that cartridge only and are cleared after `cart_teardown` returns, so a
  farewell `host_set_text` still draws them and nothing leaks into the
  shell or the next cartridge. The eight CGRAM slots are shared by both
  rows and by engine and cartridge glyphs. Bundled glyph files
  (`assets.glyphs`) go into the same table at mount
  ([`reference-glyph.md`](../sdk/docs/reference-glyph.md)).
- **`host_exit`** requests an unload through the path a native app's quit
  uses (`engine.AppContext.Exit`), which only sets a transition applied
  between ticks. Calling it from inside any guest call is therefore safe:
  nothing is torn down while cartridge code is on the stack. The browser
  host mirrors it with a callback run after the current step.
- **Quit-key ownership**: declaring the `host_exit` import is the opt-in
  to owning the quit keys. The cabinet detects it at compile time
  (`Cartridge.ImportsExit`) and routes `q`/`escape`/`joyb` to the
  cartridge ahead of the engine's quit-to-shell map, so it can run a
  confirm-exit flow. A cartridge without the import keeps the default, so
  none can become un-exitable.

**Backward compatibility**: these are new imports, not changes to
existing ones. Both runtimes resolve only the imports a module declares,
so a cartridge that never declares them runs exactly as before.

## Asset, channel and lamp imports

Three additive groups: sprites read from the mounted bundle, the way a
console reads its ROM; named input channels, so the ABI stops naming
devices; and lamp choreography plus per-lamp brightness. The file formats
are [ASSETS.md](ASSETS.md); the author pages are
[`reference-sprite.md`](../sdk/docs/reference-sprite.md),
[`reference-hid.md`](../sdk/docs/reference-hid.md) and
[`reference-lights.md`](../sdk/docs/reference-lights.md).

### Sprites from the bundle

- **Zero handles are silent.** Sheet and cel handles are separate 1-based
  per-cartridge spaces. Zero is a no-op everywhere and never logged, so a
  cartridge that stored a failed load and draws it every frame cannot
  flood an operator's log. Loading a path twice returns the same handle.
- **A refusal registers nothing**: every cel is checked before the first
  is kept, and `host_sheet_cel` is then a lookup.
- **`host_cel_read` is all or nothing.** A short buffer copies nothing
  and returns 0, because a half-written cel would draw as garbage while
  reading as success. The cabinet decodes through `assetfile.CelRGBA`,
  so a bundle-loaded sprite and a compiled-in one are the same pixels.
- **Unknown `host_draw_cel` flag bits are ignored, not refused**, so a
  future flag does not turn today's cartridges into warnings.
- **Interlaced PNGs are refused on both hosts.** The browser host's
  decoder cannot read Adam7, and a sheet that works on one host and not
  the other is what the two-host contract exists to prevent.
- **Caps** (`internal/cartridge/wasm/sheets.go`): a 512-byte path and a
  64-byte cel name, plus the sheet, cel and pixel caps in
  [reference-sprite.md](../sdk/docs/reference-sprite.md#what-the-hosts-enforce).
  The 64-pixel cel axis is `ui.Mask`'s `uint64` row width.
- **Guest side.** `Setup.LoadSheet` and `Sheet.Cel` build a `*ui.Sprite`
  at `Init`, allocating each cel's buffers once and building its mask
  with `ui.OpaqueMask` (A==255 collides), so drawing and collision stay
  in the guest and golden-testable. cartlib does not wrap `host_draw_cel`
  or the other host-side draws: its full-frame `host_blit` would
  overwrite them. `sdk/tutorial`'s unchanged goldens are the proof that a
  bundle-loaded sprite and a compiled-in one are the same sprite.

### Input channels

- **Why the ABI stops naming devices.** A record says what happened (a
  key went down, the stick moved), and that vocabulary has no word for a
  spinner, a trackball or an analog trigger. A channel says what is, in
  four integer kinds: `button`, `axis`, `delta`, `blob`. Integers only,
  because a float would make a replay depend on the host's floating
  point.
- **The panel is channels too**, derived by each host from the very
  records on their way to `cart_handle_input`, after remapping and
  gating. So records still flow byte for byte, a cartridge that binds
  nothing is unchanged, there is no second remap to drift, and a replay
  rebuilds the panel channels for free.
- **Binding happens in `cart_init`.** A bind after the first
  `cart_update` returns -1; sealing the table fixes the per-tick sample
  set, which lets the flight recorder file values against stable handles.
  -1 is the only failure signal and reading it is never logged.
- **Sampled once per tick, before `cart_update`.** That is what makes a
  delta mean "since the last update" and two reads in one frame agree.
  `host_input_blob` is all or nothing, like `host_cel_read`.
- **Outputs are reserved.** `host_output_bind` validates and always
  fails; `host_output_set` is a no-op. The surface exists so that rumble
  or pad LEDs later move no signature.
- **Profiles are data.** A `.arkadehid.json` maps report fields to
  channels and is never code, so it can travel in a bundle. The engine's
  built-in profiles are those files too, compiled through the same path,
  so the format cannot quietly lack something a built-in needs.
- **The recorder learns channels additively** (`KindChannelBind`,
  `KindChannel`, `KindChannelBlob`, no format version move). Only
  non-panel channels are recorded. The bind table lets a session
  recorded with a spinner attached replay on a laptop without one.
- **The browser host is thinner on purpose.** The Gamepad API never hands
  a page a device's input report, so it offers the panel identically,
  validates a bundled profile with the same validator and messages, and
  cannot bind its non-standard channels; it says so at mount and at the
  failed bind.

Caps: [reference-hid.md](../sdk/docs/reference-hid.md#caps).

### Lamp sequences

- **Why the host holds the timer.** `host_set_button_lights` is right for
  lamp state. A flourish ("flash twice, then dark") is several states and
  timers, which by hand means a frame counter in `cart_update` forever.
  So the bundle carries a `.arkadelights.json` and the cartridge says
  play.
- **Clocked by the tick.** Playback advances once per tick, before
  `cart_update`, by that tick's `dt_micros`: never a wall clock and
  deliberately not `host_now_micros`, so it costs no extra host call,
  cannot disagree with the guest's own `Update`, and works on a scripted
  test clock. A replay lights the same lamps on the same frames.
- **`assets.lights` is the play call's namespace.** A sequence the
  manifest does not name cannot be played, which turns a mistyped path
  into a pack-time error. No capability gates it: it writes the same
  lamps `host_set_button_lights` writes.
- **`seat` narrows; it never moves a cue.** A mask is per-seat data, so
  re-seating a `p1` cue onto `p2` would invent choreography nobody wrote.
- **No forced pingpong**: a pingpong is a property of how frames were
  drawn.
- **The explicit call wins.** `host_set_button_lights`,
  `host_set_lights` and a touching `host_set_lamp_levels` stop any
  sequence on the seats they address, so the two writers never fight at
  tick resolution. `host_lights_stop` leaves the lamps where the last
  frame put them, because stopping a cue is not a request to go dark.
- **Levels are intent.** A lamp runs at 0..15 and the on/off calls write
  15 and 0. Fades are linear in level space and host-clocked like
  playback; anything that later touches a lamp restarts from where it
  had reached. The cabinet's lamp-brightness setting scales the level
  after the call, invisibly to the cartridge. Physically the cabinet
  bit-angle-modulates the MCP23017 pins and latches without modulation
  when every lamp is 0 or 15.
- **The browser host implements this group completely**: a lamp is an
  output it can draw, and the walk runs on the same tick deltas, so a cue
  can be developed anywhere.

Packing, caps and the cartlib wrappers (`PlayLights`, `StopLights`,
`SetLampLevels`, `FadeLamps`):
[reference-lights.md](../sdk/docs/reference-lights.md).
