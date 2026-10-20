# Contributing to Årkade

First: thanks for being here. Årkade is a personal project — a real
arcade cabinet in a real living room — that happens to be open source,
not an open-source project with a governance model. That shapes
everything below, so here's the deal up front.

## The fork-first deal

- **Fork freely.** GPL-2.0-or-later means the code is genuinely yours
  to take, remix, and build your own cabinet on. Forking isn't a
  fallback here — it's the front door. Your fork doesn't need
  permission, blessing, or an upstream plan.
- **Open an issue before a large PR.** If you're about to sink real
  time into something you hope lands upstream, talk first. It takes
  five minutes and it's the difference between "merged" and "beautiful
  work that doesn't fit the cabinet."
- **Review is slow and merging is discretionary.** This is one person's
  evenings. PRs may sit for weeks; some good ones won't merge because
  they pull the project somewhere it isn't going. That's not a verdict
  on your work — it's what "personal project" means. A fork is always a
  fine home for your version.
- **Favorite-improvements are welcome.** The sweet spot for upstream
  PRs: you played a game, something bugged you, you fixed it. Bug
  fixes, polish on an existing game, docs corrections, a device profile
  for your gamepad — these are exactly the PRs that merge.

## Hard constraints

Two rules apply cabinet-wide. PRs that break them won't merge, no
matter how nice the rest is.

1. **Button semantics**: **B is back/exit only.
   D is interact**, and the D button light signals eligibility — lit
   exactly when pressing D does something. Hold-B exit must work from
   every state a game can be in. No stray mappings, no creative
   reinterpretations per game. (Visual style is free; *interactions*
   are uniform so the cabinet always feels like one machine.)
2. **Determinism and goldens**: the engine is deterministic — seeded
   RNG, manual clock under test — and the golden-frame/replay harness
   (`internal/testkit`) is the regression net, plus the future
   leaderboard's replay-validation backbone. Concretely:
   - `make test` must pass (plain `go test ./...` misses the SDK
     modules).
   - Intentional visual/behavior changes come with regenerated goldens
     (`make goldens`) — reviewed, not blindly accepted.
   - Gameplay must stay reproducible from a seed + input log: no
     wall-clock reads in game logic, no unseeded randomness, no map
     iteration order leaking into frames.

## Local setup (optional, recommended)

Nothing here is required to contribute, but running the cartridge
readiness scanner (preflight) yourself moves a failure from "twenty
minutes into a review" to "three seconds after you typed it":

```sh
ARKADE_CARTRIDGE_ROOTS=../arkade-cartridges/cartridges go run ./tools/preflight -all -fast   # every cartridge, fast
ARKADE_CARTRIDGE_ROOTS=../arkade-cartridges/cartridges go run ./tools/preflight -v <slug>    # one cartridge, in detail
```

`ARKADE_CARTRIDGE_ROOTS` names the folders that hold cartridges
(comma-separated, relative to this checkout); point it at a sibling
`arkade-cartridges` clone, your own library, or both.
`tools/preflight/doc.go` lists every check, and
[`docs/CI.md`](docs/CI.md) covers what CI runs.

Working on the engine or the docs: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
maps the repo, [`docs/HARDWARE_NOTES.md`](docs/HARDWARE_NOTES.md) lists the
hardware traps the code already handles, and
[`sdk/docs/AUTHORING.md`](sdk/docs/AUTHORING.md) is the style and test
contract for the SDK documentation.

## Writing a game

Games are **cartridges**: standalone TinyGo→wasm modules that run
unmodified on the cabinet and in the browser, no engine rebuild. Start
here:

- [`sdk/docs/getting-started.md`](sdk/docs/getting-started.md) — the
  SDK documentation set, canonical for authoring
- [`docs/GAME_AUTHORING.md`](docs/GAME_AUTHORING.md) — the human
  on-ramp
- [`docs/GAME_AUTHORING_AI.md`](docs/GAME_AUTHORING_AI.md) — the
  numbered rule checklist for pairing with an AI
- [`sdk/README.md`](sdk/README.md) — the SDK modules (start from
  `sdk/template`)
- [`docs/CARTRIDGE.md`](docs/CARTRIDGE.md) — why the wasm ABI is shaped
  the way it is

Crib from Life (simplest) and Snake (full-featured) in the
[arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
repo. Show off what you build — that's what the game idea/showcase issue
template and Discussions are for. New games ship in the cartridges repo,
not the engine repo.

## Licensing of contributions

By submitting a PR you agree your contribution is licensed under
**GPL-2.0-or-later**, same as the project (inbound = outbound). No CLA.
