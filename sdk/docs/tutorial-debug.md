# Tutorial part 4: play, record, replay, ship

[Part 1](tutorial.md) built the game, [part 2](tutorial-assets.md) gave
it art and a voice, and [part 3](tutorial-levels.md) gave it an authored
opening. This part is everything between "it works on my
machine" and "it is safe to put on a cabinet": a fast iteration loop, a
way to replay a session that went wrong, tests that pin what the game
looks like and prove it is deterministic, and one command that grades
the whole cartridge before you call it done.

## Iterate from the terminal

The loop from [Getting started](getting-started.md#4-play-it) is two
commands. In the cartridge, build and pack:

```sh
make rcart      # from my-cartridges/catcher
```

and in the engine checkout, play it:

```sh
./arkade --cartridges ../my-cartridges/catcher
```

`q` quits; rebuild and run again. `make rcart` builds first, so what you
play is exactly what ships — `.rcart` and all, which means your sounds
and assets are live. A failed build stops at the compiler's error and
leaves the previous `.rcart` where it was. The browser player at
[arkade.build](https://arkade.build) plays the same `.rcart` by drag
and drop.

This is where you actually play it. Catch things. Miss deliberately.
Mash the dash while it is dark and check the light matches. Let the
score qualify and enter initials. Hold B in the middle of a run and
again over the initials screen.

## When something goes wrong: the flight recorder

Some bugs only happen once. The flight recorder exists for those: the
engine writes down the three things a cartridge receives from the
outside world — the `dt_micros` handed to `cart_update`, the input
records handed to `cart_handle_input`, and every value returned by
`host_random_u64` — and replaying those three streams re-lives the
session exactly.

**It is already on.** There is no `--record` flag; every cartridge
session is recorded into a ring buffer, and the flags only configure or
disable it:

```text
--flight <dir>      directory for input flight recordings (default "<data>/flight");
                    empty string via --flight=off disables the recorder entirely
--flight-keep <n>   how many flight recordings to retain before rotating the oldest out
```

The ring holds 131,072 events — about 2 MiB, allocated once at launch,
zero allocation in steady state — which is roughly forty minutes of busy
play. It is only written to disk when something asks for it, and three
things can:

1. **A crash.** TinyGo's `-panic=trap` turns a guest panic into a bare
   `unreachable`, and the host dumps automatically on the first fault of
   a session.
2. **The debug combo, on the pads.** Hold **Aux1** and push
   **UP UP DOWN DOWN LEFT RIGHT LEFT RIGHT**. The host detects this
   itself, in its own copy of the detector, over the record stream it is
   already recording — the combo is guest-side and has no host call
   behind it.
3. **The dashboard**, without touching the pad at all. It listens on
   loopback only, and its mutating routes (this one included) are
   disabled unless `ARKADE_HTTP_TOKEN` is set before `./arkade` starts —
   set your own before launch and reuse it:

```sh
export ARKADE_HTTP_TOKEN=$(openssl rand -hex 24)   # set before launching ./arkade
./arkade &

curl -sX POST -H "Authorization: Bearer $ARKADE_HTTP_TOKEN" \
  "http://localhost:8080/api/flight/dump?note=fruit+fell+through+the+basket"
```

```text
{"path":"data/flight/<time>-cart_catcher-manual.arkflt",
 "slug":"cart/catcher","reason":"manual","events":9214,"frames":9012,
 "inputs":190,"randoms":12,"bytes":21873}
```

Note `"randoms":12` — twelve random spawns, twelve draws. That is the
property `catcher.go` was built for. (The authored waves that opened the
session drew none at all, which is why the number is so much smaller than
the fruit count.)

### Replaying it

```sh
./arkade --replay data/flight/<time>-cart_catcher-manual.arkflt
```

It prints the header before anything runs:

```text
replay: cart/catcher "Catcher" version "1.2.0"
replay: recorded <time> on darwin/arm64 (dev), dumped <time> because: manual
replay: note: fruit fell through the basket
replay: 9214 events (9012 frames, 190 input records, 12 rng draws), seed 4815162342, 30fps
```

and then re-lives the session frame by frame. `--replay-paused` starts
it stopped: space resumes, `.` steps one frame, `q` quits. Those keys
are read from the terminal directly rather than through the engine, so
stepping never dispatches a keypress into the cartridge you are
inspecting. The frame counter rides in the console stats line —
`REPLAY 412/9012 (f412)`.

Three things about it are worth knowing before you rely on it:

- **The recording pins the exact `.wasm` bytes** by SHA-256 and refuses
  to replay against different ones. A rebuild of the *same source* can
  differ byte for byte across machines and toolchain versions, so the
  fix for a mismatch is usually to fetch the `.wasm` from the cabinet
  rather than to rebuild it. `--replay-force` overrides, and lies to you
  about as often as you would expect.
- **Only the guest is reproduced.** Scores, storage, recent names and
  settings are not recorded, so a cartridge that branches on
  `ScoreQualifies` — like this one — can diverge unless you copy
  `data/cart/catcher` across with the recording.
- **Random VALUES are recorded, not the seed.** The engine RNG is one
  stream shared with the shell and the attract loop, so a cartridge's
  position in it depends on everything that ran before. This is the
  concrete reason for the rule about `cartlib.RandomU64` being your only
  source of chance: a cartridge with a second one cannot be replayed at
  all.

## Golden tests

Replay is for the session you already lost. Golden tests are for the
regression you have not made yet. The bar is two things:
**determinism** — two identical runs produce identical pixels — and at
least one **correctness anchor**, so the suite proves the game still
does the right thing rather than merely still doing whatever it did
last time.

They live in the cartridge directory as `package main`, beside the
rules and screen tests, so all three can share helpers. They drive the
committed `.wasm` through the engine's `carttest` harness, which is
host-side code like the generators: the engine `require` that part 2
added to `go.mod` already covers it. After adding the file, run
`go mod tidy` once — the harness reaches dependencies (the wasm runtime)
the generators did not.

Here is the skeleton. `sdk/tutorial/catcher_golden_test.go` is the full
suite — ABI conformance, the peripherals, the attract-silence check and
the shared name-entry contract — and copying it into your cartridge
(`cp ../../arkade/sdk/tutorial/catcher_golden_test.go .`) is the quickest
way to have one; everything load-bearing is below.

```go
package main

import (
	"context"
	"os"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
)

const (
	catcherWasmPath   = "catcher.wasm"
	catcherTickMicros = 33_333 // the cabinet's 30 fps cadence

	catcherSeat   uint8 = 0 // Physical: whose LCD and lights these are
	catcherPlayer uint8 = 1 // Player: the pairing slot

	// The catcher's starting left edge, (64-7)/2, and the column the
	// endless finale's first scripted fruit falls down.
	catcherStartColumn uint64 = 28
)

func loadCatcherWasm(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(catcherWasmPath)
	if err != nil {
		t.Fatalf("read the checked-in %s (run `make build`): %v", catcherWasmPath, err)
	}
	return data
}

// newCatcherHost is the scripted host every test starts from. FakeHost
// clamps at the last RandScript value once the script runs out, so two
// numbers describe a whole endless run: one fruit into the basket, then
// column 0 forever.
func newCatcherHost() *carttest.FakeHost {
	host := carttest.NewFakeHost()
	host.RandScript = []uint64{catcherStartColumn, 0}
	return host
}

func newCatcherCartridge(t *testing.T, host carttest.Host) *carttest.Cartridge {
	t.Helper()
	cart, err := carttest.NewCartridge(context.Background(), loadCatcherWasm(t), host, carttest.Options{})
	if err != nil {
		t.Fatalf("NewCartridge: %v", err)
	}
	t.Cleanup(func() { _ = cart.Close(context.Background()) })
	if err := cart.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	carttest.AdvancePastSplash(t, cart) // one fat tick consumes the load phase
	return cart
}

func catcherTick(t *testing.T, cart *carttest.Cartridge, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if err := cart.Update(ctx, catcherTickMicros); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if err := cart.Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
}

// catcherPress sends the keydown+keypress+keyup triple a real cabinet
// button emits. Use the realistic shape: CLI-shaped single keypresses
// can mask real-pad bugs, and ui.HoldConfirm in
// particular cannot arm without seeing the keyup.
func catcherPress(t *testing.T, cart *carttest.Cartridge, key uint8) {
	t.Helper()
	ctx := context.Background()
	for _, kind := range []uint8{carttest.RecordKindKeydown, carttest.RecordKindKeypress, carttest.RecordKindKeyup} {
		rec := carttest.InputRecord{
			Kind: kind, Key: key,
			Player: catcherPlayer, Physical: catcherSeat,
			Source: carttest.RecordSourceController, Index: 0,
		}
		if err := cart.DispatchInput(ctx, []carttest.InputRecord{rec}); err != nil {
			t.Fatalf("DispatchInput(key=%d kind=%d): %v", key, kind, err)
		}
	}
}

// catcherJourney is the shared spine: the same scripted run, played the
// same way, every time.
func catcherJourney(t *testing.T, dump bool) (*carttest.Cartridge, *carttest.FakeHost) {
	t.Helper()
	host := newCatcherHost()
	cart := newCatcherCartridge(t, host)

	catcherTick(t, cart, 3)
	if dump {
		carttest.GoldenFrame(t, "catcher-title", carttest.MatrixFrame(t, host.LastBlit(t)))
	}

	catcherPress(t, cart, carttest.KeyJoyA) // title -> playing

	catcherTick(t, cart, 12) // the first wave is still announcing itself
	if dump {
		carttest.GoldenFrame(t, "catcher-wave", carttest.MatrixFrame(t, host.LastBlit(t)))
	}

	catcherTick(t, cart, 78) // 90 in total: fruit in flight
	if dump {
		carttest.GoldenFrame(t, "catcher-playing", carttest.MatrixFrame(t, host.LastBlit(t)))
	}
	return cart, host
}

// TestCatcherGoldenFrames records what the game looks like.
func TestCatcherGoldenFrames(t *testing.T) {
	catcherJourney(t, true)
}

// TestCatcherDeterminism is the ABI's core promise, asserted: two fresh
// instantiations driven identically produce identical pixels.
func TestCatcherDeterminism(t *testing.T) {
	_, a := catcherJourney(t, false)
	_, b := catcherJourney(t, false)
	carttest.AssertBlitsIdentical(t, a, b)
}

// TestCatcherScoresACatchAndEndsOnThreeMisses is the correctness
// anchor. Without one of these, a golden suite only proves the binary
// still draws whatever it drew last time.
func TestCatcherScoresACatchAndEndsOnThreeMisses(t *testing.T) {
	cart, host := catcherJourney(t, false)

	for i := 0; i < 3000; i++ {
		catcherTick(t, cart, 1)
		host.Mu.Lock()
		line1 := carttest.LastLCDLine1(host, int(catcherSeat))
		host.Mu.Unlock()
		if line1 == "GAME OVER" {
			host.Mu.Lock()
			line2 := host.LCDTexts[len(host.LCDTexts)-1].Line2
			host.Mu.Unlock()
			if line2 != "SCORE 10" {
				t.Fatalf("game-over LCD line 2 = %q, want %q — the script catches exactly one fruit", line2, "SCORE 10")
			}
			return
		}
	}
	t.Fatal("the scripted run never reached game over")
}

// TestCatcherWasmMatchesSource closes the one gap the goldens cannot
// see: they were recorded FROM the committed binary, so a source edit
// that never got its rebuild keeps passing them.
func TestCatcherWasmMatchesSource(t *testing.T) {
	carttest.RequireWasmMatchesSource(t, ".", "catcher.wasm", "make build")
}
```

The journey rests on two different kinds of determinism, and it is worth
seeing which one is doing the work.

A run opens with the authored waves from [part 3](tutorial-levels.md), and
those draw **no random numbers at all** — the schedule *is* the script.
`FIRST DROPS` puts its first fruit over the catcher's starting column and
the four after it well away from it, so a player who never touches the
stick catches exactly one and then misses three. That is the shortest
journey that reaches a **qualifying** name entry, which is what the
platform's name-entry contract needs, and it is a property of a grid somebody
drew rather than of anything the harness set up.

The two-element `RandScript` describes the *other* half of the game, the
endless finale on the far side of the waves. It works because `catcher.go`
draws exactly one number per random spawn and none at any other time, so
`{28, 0}` says: *the first random fruit falls into the catcher's starting
column, and every one after it falls at column 0*. Neither half of the
game needs a hundred lines of scripted joystick pushes.

Record the frames the first time, then read them:

```sh
UPDATE_GOLDENS=1 go test ./...      # from ../my-cartridges/catcher
go test ./...                       # now they are assertions
```

The snapshots land in `testdata/golden/*.txt` as ASCII art with a colour
legend. **Read them.** They are the only part of a golden suite that a
human can review, and a diff you skimmed is a golden you did not write:

```text
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
aabaabbbaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaccaaccaacca
abbaababaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaccaaccaacca
...
aeeeeeeeeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
--- legend ---
a = #080c1a/ff
b = #ffd65c/ff   ← the score
c = #e8424a/ff   ← three life pips
e = #5c96ff/ff   ← the dash meter, full
```

Two more tests belong in every suite, and neither is optional:

**`carttest.RequireWasmMatchesSource`** rebuilds the source with the
exact platform flags, applies the same `wasmfix` pass, and compares against
the committed binary. This closes the one gap the goldens structurally
cannot see: they were recorded *from* that binary, so a source edit that
never got its rebuild keeps passing them while the new code simply never
runs. That is not hypothetical — two cartridges sat drifted on main for
a day. The byte-exact half of the check only runs on the machine the
shipped binaries are built on, because two OS builds of the same
TinyGo release do not emit identical wasm; the helper's doc comment has
the whole story.

**`carttest.NameEntryB`** is the shared regression harness for
the rule [part 1](tutorial.md) built the confirm flow around: a fumbled B
over an unsubmitted placement must open the hold overlay, not discard
the run. You supply a function that parks a fresh cartridge on a
qualifying entry and it asserts the rest — so the contract is checked
from one place and a cartridge that regresses says so in the same words
as its neighbours.

```text
carttest.NameEntryB{
	Reach:    catcherReachNameEntry,
	Tick:     catcherTick,
	Player:   catcherPlayer,
	Physical: catcherSeat,
}.Run(t)
```

Wire the suite into the build so it actually runs. For your own
library there is no registration step: `go test ./...` from the
cartridge directory runs it, same as any other Go module, and
preflight's `tests` check runs the same command.

## Preflight

One command grades the whole cartridge against the platform's rules:

```sh
ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/preflight catcher
```

```text
catcher 0.2.0 — Catcher
  skip config          no preflight.json — scanned with the defaults
  pass golden-rules    10 TinyGo-compiled files clean
  pass wav-audibility  5 WAV(s) clear the 150 Hz / 50% floor
  pass sprite-alpha    4 PNG(s) (2 under assets/sprites, where alpha is the hitbox), alpha channels clean
  pass manifest        schema-1 valid, 0.2.0, manual current
  pass levels          generated level files current (tools/genlevels -check)
  pass bundle          packs to a loadable .rcart: catcher, 10 asset(s), 58 KiB wasm
  pass lcd             2 SetText call(s) fit 16 columns and the CGRAM budget
  pass tests           `go -C ../my-cartridges/catcher test ./...` passes
  verdict: PASS

1 cartridges: 1 pass, 0 warn, 0 fail
```

Nine checks. `config` reads the cartridge's optional `preflight.json`
— a per-cartridge exemption file for findings that are right about the
code and wrong about the intent, one required reason per suppression,
always printed on the report card (even suppressed) rather than made
invisible; none here, so it's skipped rather than scored. `golden-rules`
is static analysis over the sources TinyGo actually compiles, looking
for `func init()`, `time`, `math/rand`, goroutines, channels,
`fmt`/`reflect`/`encoding/json`, and libm on per-frame paths.
`wav-audibility` re-measures every shipped WAV against the speaker
floor and is informative only — it warns about a thin-sounding clip but
never fails the run, because speakers vary and only a human ear settles
it. `sprite-alpha` verifies the PNGs' alpha reads as an intended
collision mask. `levels` fails a `levels_gen.go` that is stale, hand-edited
or missing against the `assets/` packs, naming the fix (`make build`).
`bundle` packs a `.rcart` and loads it back. `lcd`
proves every line fits sixteen columns and stays inside the display's
eight custom-glyph budget. `tests` shells out to your suite, and
`-fast` skips it.

Getting to a clean run is the useful part, and your first run will not
be clean: the `MANUAL.md` you kept from the template is still the
template's. A cartridge whose manual names another game, and whose
`manifest.json` was edited after its last build, fails like this:

```text
  FAIL manifest        2 violation(s), 0 to look at
       - ../my-cartridges/catcher/MANUAL.md: no '# Catcher' heading matching the manifest title
       - ../my-cartridges/catcher/manifest_gen.go: manifest_gen.go is stale against manifest.json — regenerate with `go run ./sdk/tools/manifestgen ../my-cartridges/catcher` and rebuild the wasm
```

Both are real. **Every cartridge ships a player manual** —
`MANUAL.md` at the cartridge root, with a `# <title>` heading matching
the manifest, a `manual-icon.png` header image
(`ARKADE_CARTRIDGE_ROOTS=../my-cartridges go run ./tools/manualicon`
upscales the 8×8 `assets/icon.arkadesprite.png`, or `assets/icon.png`
when there is none, to 128×128), and
a `*Manual for version <v>*` footer quoting the *current* manifest
version. That footer is the mechanism: a gameplay change that bumps
the version forces a manual pass in the same commit. Editing the
footer alone defeats the whole thing. `sdk/tutorial/MANUAL.md` is this
game's: copy it over yours, set its footer to your manifest's version,
and rerun `manualicon`.

And **`manifest_gen.go` must be current**. The shell lists a game from
`manifest.json` and the running cartridge reports itself from that
generated file; `make build` regenerates it before every compile, so the
check only fires on a hand edit or a build that was skipped after a
`manifest.json` change. Version numbers live in two places — the
manifest and the manual footer — and both move together.

`-all` scans every cartridge, `-v` prints each finding's rule citation, and
`-json` is for CI. For your own library there's no Makefile step to
wire up — the command above, by hand, is the whole interface, and
running it is the last thing you do before calling a cartridge
finished.

## Pack it and play it

```sh
make rcart      # in ../my-cartridges/catcher
```

`rcartpack` applies **the exact validation the cabinet's loader applies
at mount time**, so a bundle that is too big or has a bad manifest is
rejected here with the identical error rather than failing days later on
hardware. [Manifests & packaging](packaging.md) has the format and the
[caps](packaging.md#caps).

Drag the `.rcart` onto the hosted player at
[arkade.build](https://arkade.build) exactly like a bare `.wasm`, now
with working sound. On a cabinet, copy it
in and restart the service — [Getting started](getting-started.md#7-put-it-on-a-cabinet)
has the atomic-rename recipe, and a USB stick with the bundle at its
root works the same way.

## Where next

You have built one whole game and met most of the system doing it. What
is left is mostly depth:

- **[Authoring patterns](patterns.md)** end to end — every lesson on
  that page is backed by a real cartridge, and reading it is much
  cheaper than re-earning it.
- **[Seating & rotation](seating.md)** before you write anything with a
  second player. The rules there have sharp edges and none of them are
  guessable.
- **[The ui reference](ui.md)** for the widgets Catcher never needed —
  [menus](ui.md#menu), layers and compositors.
- **[The cartlib reference](cartlib.md)** for
  [storage](cartlib.md#storage), which this game deliberately does not
  use, and the rest of the peripheral surface.
- **Steal from real games.** In the `arkade-cartridges` repo, `life` is
  the simplest complete cartridge and `snake` shows everything — menus,
  two players, scores, sound, an attract mode, and a test spread with an
  input mash and PNG dump hooks for iterating on pixel art.

The finished cartridge from this tutorial is at `sdk/tutorial/`, with
its own `README.md` describing how it is wired.
