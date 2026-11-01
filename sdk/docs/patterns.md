# Authoring patterns

The rules on these pages hold across every cartridge — distilled to one
paragraph per lesson, each learned from a real cartridge rather than a
hypothetical. Three reference games are worth keeping open while you
read: `cartridges/life` (minimal) and `cartridges/snake` (full-featured)
from the [arkade-cartridges](https://github.com/stealthmonkey99/arkade-cartridges)
repo, and Catcher at `sdk/tutorial` (sprites, sound, levels and
goldens in a small codebase).

## The pages

| Page | The lessons |
| --- | --- |
| [Designing for the hardware](patterns-hardware.md) | The 64×64 panel, what B and D mean, input truths, eligibility lighting, the LCDs |
| [Art and sound](patterns-art-sound.md) | Sprite art as PNGs and the collision alpha rule; sound that survives small speakers |
| [Loading screens and attract mode](patterns-loading-attract.md) | The loader animation, and the screensaver's structural silence |
| [Seating, rotation, and who gets to play](seating.md) | Where each player sits, which way the board faces, and which strategy call to make |
| [Testing, storage, and debugging](patterns-testing.md) | The golden-test minimum, storage keys that fail silently, reading a wasm trap |
| [Gotchas](gotchas.md) | The platform traps that compile fine and fail later, one line each |

## Before you call it done

Re-run the golden and host-render tests (see [Testing
strategy](patterns-testing.md#testing-strategy)), and re-check the storage-key and
version-bump discipline in [Storage discipline](patterns-testing.md#storage-discipline).
Read [Gotchas](gotchas.md) once more, and walk the seating page's own
[checklist](seating.md#before-you-call-it-done).
If the game changes gameplay later, bump the version and re-pass the
manual.
