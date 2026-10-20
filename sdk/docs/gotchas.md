# Gotchas

Every trap on this page compiles cleanly and then misbehaves: on the
cabinet after an hour, on one host and not the other, or silently. Each
is one line, with the symbol or check that documents or enforces it;
follow the link for the reasoning. Read it once before your first
cartridge and again before you call one finished.

## TinyGo and the reactor

A cartridge is built `-scheduler=none -gc=leaking` and run as a wasm
reactor, which turns ordinary Go habits into bugs.

- **`func main` runs once during `_start` and must stay empty; `func init`
  is refused** — register with `var _ = cartlib.Run(game)` at package level
  and do computed setup in `Game.Init`
  ([registration](cartlib.md#registration-and-the-reactor-model);
  preflight's `golden-rules` check fails an `init` or a non-empty `main`).
- **A package-level initializer that calls a function silently never
  runs** in the wasm build, while every host-Go test passes — keep
  package vars to literals, and assign callbacks such as
  `CheatCode.OnStep` in `Init` (preflight `golden-rules`).
- **Nothing allocated after `Init` is ever freed** (`-gc=leaking`), so a
  per-frame allocation leaks for the life of the cabinet process —
  `carttest.RequireNoAllocsAfterInit` reads TinyGo's own `-print-allocs`
  report and is the oracle; `hosttest.Host.AllocsPerRun` is the fast
  first check.
- **A slice or map literal inside a function allocates each time it
  runs** — build `[]ui.Hint{...}`, `ui.NewMenu([]string{...})` and the
  like once, at package level or in `Init` ([button hints](ui.md#button-hints)).
- **A `string(buf[:n])` conversion or a `+` concatenation allocates** —
  format into a package-level `[N]byte` and pass it to
  `cartlib.SetTextBytes`; `cartlib.SetText` with a constant is free.
  `RequireNoAllocsAfterInit` cannot see these conversions, so a green run
  does not clear them.
- **A local array whose slice reaches a host call is moved to the heap
  by TinyGo**, and so is any value over about 1 KB built in a function —
  make scratch buffers (`ScoreRead`'s, `StorageRead`'s) package-level, and
  reset big structs field by field.
- **The allocating SDK calls are Init-only by type**: loading sounds and
  sheets, binding channels and defining glyphs are methods of the
  `*cartlib.Setup` only `Init` receives, and `Sheet.Cel` dies with it;
  `cartlib.DecodeInputRecords` allocates too, so use
  `DecodeInputRecordsInto` (`cartlib.Setup`).
- **No goroutines, channels, `time` or `sync`** — `-scheduler=none`
  fails the build or traps (preflight `golden-rules`).
- **Any runtime panic is a wasm trap, and a trapped cartridge is never
  entered again** — an index out of range or nil dereference ends the
  instance for good, so bounds-check anything input or storage can drive
  (`wasm.ErrFaulted`).
- **`int` is 32 bits on wasm32** — widen to `int64` or `uint64` before
  multiplying microsecond counts or accumulating scores.
- **`fmt`, `reflect` and `encoding/json` cost binary size and misbehave
  under reflection** — draw numbers with `DrawUint` or
  `ui.Font3x5.DrawUint` and encode storage as fixed binary (preflight
  warns).

## Determinism

Everything a frame shows must be a pure function of `dtMicros`, the
input records and the `RandomU64` draws ([determinism](abi.md#determinism)).

- **Time comes only from `Update`'s `dtMicros`**, accumulated; it is the
  per-tick delta, not a total, and not always 33,333 —
  `cartlib.NowMicros` is the same engine clock, for absolute timestamps
  only (`Game.Update`).
- **Randomness comes only from `cartlib.RandomU64`** — `math/rand` and
  `crypto/rand` break replay and goldens (preflight `golden-rules`).
- **libm transcendentals (`math.Sin`, `Cos`, `Pow`) can differ between
  hosts** — keep them off per-frame observable paths; plain `float64`
  arithmetic is bit-exact (preflight reports the call sites).
- **`Render` can run before the first `Update`**, and the framebuffer
  keeps the previous frame's pixels — draw a whole valid frame from the
  state `Init` left, starting with `fb.Clear` (`Game.Render`,
  `cartlib.Framebuffer`).
- **A title or splash frame must be opaque**, or the previous screen
  shows through it — `ui.Splash` fills its background for you (`ui.Splash`).

## Calls that fail silently

The ABI never traps on a bad call: it does nothing, and often says
nothing. Check the return value where there is one.

- **`StorageWrite` drops an illegal key or an oversize value whole** and
  returns false; a key with a `/` in it is the classic, and a host-side
  failure after a true return is not reported at all (`cartlib.StorageWrite`,
  `cartlib.StorageKeyOK`).
- **`StorageRead` truncates a value longer than its buffer** and returns
  0 for an absent or illegal key (`cartlib.StorageRead`).
- **A sound that failed to load is handle 0, and playing it is a
  no-op** — which is every sound in a bare `.wasm` with no bundle
  (`cartlib.Setup.LoadSound`).
- **A sheet that failed to load is the zero `Sheet`, whose `Cel`
  returns nil**, as does a cel name the sheet lacks
  (`cartlib.Setup.LoadSheet`).
- **A channel nothing on this cabinet offers binds to the zero
  `Channel`, which reads zero forever** — check `Ok()` once and play
  differently (`cartlib.Channel`); `BindOutput` fails on every current
  host.
- **`Setup.DefineLCDGlyph` returns false** for ASCII, emoji, wide runes
  and a 33rd distinct rune — branch on it ([your own
  glyphs](cartlib.md#your-own-glyphs)).
- **`PlayLights` returns false** for a path the manifest does not list
  under `assets.lights` ([lamp sequences](reference-lights.md)).
- **Once the cheat combo completes, every score write refuses**:
  `ScoreSubmit` returns 0 and `ScoreQualifies` false for the rest of the
  instance (`cartlib.CheatTainted`).
- **`cartlib.DrawText` draws lowercase and most punctuation as blank
  cells, and `ui.Font3x5` draws nothing for a glyph it lacks** — check
  every literal (`cartlib.DrawText`, [fonts](ui.md#fonts)).
- **`ui.Font3x5` steps one cell per byte, not per rune**, so a multi-byte
  rune such as the degree sign draws as two cells — spell units in ASCII
  (`ui.Font3x5`).
- **A sheet cel is at most 64 pixels on a side**, the width of a
  `ui.Mask` row; a larger sheet fails to load as the zero `Sheet`
  (`cartlib.Setup.LoadSheet`).
- **A lamp level above `LampLevelMax` wraps rather than clamps** (16
  reads as 0), and any lamp write that touches a seat stops its running
  `PlayLights` sequence, except one with an empty touch mask
  (`cartlib.PackLampLevels`, `cartlib.SetLampLevels`).
- **`cartlib.PlaySoundForever` loops until stopped** — keep the handle and
  stop it on every exit path (`cartlib.StopSound`).
- **A `.rcart` is not rebuilt with the `.wasm`** — `make rcart` after
  every `make build`, or the cabinet runs the old game and looks healthy
  doing it ([rcartpack](packaging.md#rcartpack)).
- **Never hand-roll a `.rcart` with `tar`** — macOS bsdtar writes PAX
  headers and AppleDouble entries the browser host's reader rejects; use
  `rcartpack`, which builds with `rcart.BuildDirTar` and refuses `._` and
  `.DS_Store` files ([rcartpack](packaging.md#rcartpack)).
- **`manifest_gen.go` is generated** — an edit to it is overwritten by
  the next build; change `manifest.json` (`Game.Manifest`, preflight
  `manifest`).

## The hardware

- **Each LCD line shows 16 characters, and an LCD holds eight custom
  glyphs at once across both its rows**, the engine's and your own
  together; a ninth draws as `?` (`cartlib.Setup.DefineLCDGlyph`,
  preflight `lcd`).
- **A glyph row is 5 bits with bit 4 leftmost, top row first**; higher
  bits are ignored and redefining an ASCII rune is refused
  (`cartlib.Setup.DefineLCDGlyph`, [your own glyphs](cartlib.md#your-own-glyphs)).
- **An LCD cannot redraw legibly at frame rate** — keep updates to about
  4 Hz and push on events, not every tick (`cartlib.SetText`).
- **A seat is not a slot**: `Physical` (`cartlib.Seat`) says which LCD
  and lamps face the player, `Player` (`cartlib.Slot`) says whose piece
  moves, and they diverge when someone picks up the other controller —
  the types keep them apart, and `SeatOf`/`SlotOf` convert an index
  ([seat vs player](glossary.md#seat-vs-player)).
- **Every peripheral call ignores `SeatNone`**, which is what a keyboard
  or microphone record carries — harmless to pass through, but nothing
  lights (`cartlib.SetText`).
- **A player strategy never gates a keyboard record**, on either host —
  a two-player game still demuxes on each record's `Player`
  (`cartlib.PushPlayerStrategy`).
- **Nothing restores LCD text or lamps for you** — every path into a
  screen, including the way out from under an overlay, re-asserts them
  ([buttons and lights](patterns.md#buttons-and-lights)).

## Input

- **Act on keypresses only** — the cabinet sends a keydown and a
  keypress for every press, so acting on both double-fires
  (`InputRecord.IsPress`).
- **Take directions from `PressDir`** — a real stick sends a joystick
  record and an arrow keypress per push (`InputRecord.PressDir`).
- **Track held state with `ui.HeldDirs`** — a terminal keyboard never
  sends a keyup ([HeldDirs](ui.md#helddirs)).
- **The quit keys exit unless you take them** — cartlib calls `Exit` on
  q, escape and the red button before `HandleInput` sees the batch; a
  game with its own confirm implements `cartlib.QuitOwner`.
