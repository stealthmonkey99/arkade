<img src="manual-icon.png" width="128" height="128" align="left" alt="Template Demo icon">

# Template Demo

**1 player · demo** · SDK starter cartridge: a bouncing block, a button-light chase, and a persisted counter

<br clear="all">

## How to play

There is nothing to win. An orange block bounces around the panel on its
own while the cabinet's six button lights chase each other in a ring,
one at a time. A counter in the bottom-left corner goes up every time
you press the green button.

The counter is remembered: quit, come back later, and it picks up where
you left it.

## Controls

| Input | Does |
|---|---|
| 🟢 A | Add one to the counter |
| 🔴 B | Quit |

[Keyboard equivalents](https://github.com/stealthmonkey99/arkade-cartridges/blob/main/docs/MANUALS.md#shared-controls-background)

## Rules

- The block moves one step every 0.1 seconds and turns around at each edge.
- The button lights advance one position every 0.1 seconds, six in a ring.
- The counter is never reset by the game.

---
*Manual for version 0.1.0 · [game library](https://github.com/stealthmonkey99/arkade-cartridges/blob/main/docs/MANUALS.md)*
