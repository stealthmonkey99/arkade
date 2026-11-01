# API reference

One page per SDK package, generated from its Go doc comments: every
exported name with its exact contract: units, ranges, which phase may
call it, and what happens on bad input. These are the pages to look a
symbol up on. For a tour of what to reach for and why, read the
[cartlib guide](cartlib.md) or the [ui guide](ui.md) first. `go doc`
and pkg.go.dev show the same text.

## The packages

| Package | What it is | Guide |
| --- | --- | --- |
| [cartlib](api/cartlib.md) | The cartridge SDK: the `Game` interface, `Setup`, drawing, input, sound, storage, scores, peripherals | [cartlib guide](cartlib.md) |
| [cartlib/hosttest](api/cartlib-hosttest.md) | An in-process fake cabinet for a cartridge's own `go test` | [Testing & debugging](patterns-testing.md) |
| [ui](api/ui.md) | The widget toolkit shared by the cabinet and every cartridge | [ui guide](ui.md) |
| [ui/input](api/ui-input.md) | The input vocabulary: the decoded input record, keys, seats, slots, directions | [ui guide](ui.md) |
| [fsm](api/fsm.md) | The screen-state machine | [ui guide](ui.md#screen-state-sdkfsm) |
| [levels](api/levels.md) | The cartridge-side shape of authored levels | [Tutorial part 3](tutorial-levels.md) |
| [levels/packfile](api/levels-packfile.md) | The level pack format and its generator, host-side only | [Levels reference](reference-levels.md) |
