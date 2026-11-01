# Testing, storage, and debugging

What keeps a finished cartridge finished: the tests that pin it, the
storage rules that fail without a word, and how to read a crash on a
cabinet that only says `unreachable`.

## Testing strategy

- **Golden test minimum**: determinism (two identical runs) + one
  correctness anchor, driving the checked-in `.wasm`. Keep the seeded
  input mash and the alloc soak *in* the test file — both classes of
  bug they catch (trap crashes, leak/corruption) resurface.
- **Host-Go render tests**: because draw code targets `ui.Canvas`, a
  plain `go test` can render any board state into a `ui.Layer` and
  assert pixels — no TinyGo, no wasm. Use it for worst-case layouts
  scripting a journey can't reach.
- **PNG dump hooks** gated on an env var inside the golden test are
  the pixel-art iteration loop; dump dirs go in the scratchpad, never
  the cartridge directory.
- **Use realistic record shapes** (keydown+keypress+keyup triples,
  correct `Physical` bytes, `FlagVirtual` only where true) — CLI-
  shaped test input has masked multiple real-pad bugs.
- Reproduce before fixing; prove regressions fail-then-pass against
  the pre-fix build.

## Storage discipline

Keys must match `[A-Za-z0-9_.-]+` — a slash means every write is
refused. `cartlib.StorageWrite` reports it by returning false, and
nothing else will: the call never reaches the host, and no log on the
cabinet says why. Give each cartridge one regression that (a) asserts
`cartlib.StorageKeyOK` for each real key, or `StorageWrite`'s result,
and (b) proves a write→read round trip on a fresh instance
(`sdk/cartlib/hosttest` keeps storage across a test). Version numbers live in two places (manifest.json and
the MANUAL.md footer) — bump both and rebuild, since the compiled-in
copy is generated from manifest.json; preflight's `manifest` check
catches the misses.

## Debugging a wasm crash

TinyGo `-panic=trap` turns every runtime panic into a bare
`unreachable`. On the cabinet: frozen visuals + still-playing audio +
per-frame `wasm error: unreachable` journal spam = dead instance; the
engine logs a wasm stack trace on the first trap (function names
survive `-no-debug`). Audit slice indexes and divides on `Update`
paths — input record bytes are host data; never index by
`Player`/`Physical` unchecked (`Seat.Index` and `Slot.Index` return -1
when there is no single seat or slot).
