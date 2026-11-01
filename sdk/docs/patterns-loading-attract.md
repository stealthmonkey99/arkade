# Loading screens and attract mode

The two moments a cartridge is on screen without anyone playing it: the
few seconds while it loads, and the screensaver that shows it off to an
empty room.

## Loading screens

No informational text — a loader is a short (2–3 s) animation that
hints the game's core mechanic, with a sound synced to its beats
(~0.5 peak). The title art must sit at exactly the post-load title's
position (pin it with an alignment test). Draw everything into the
`LoadFrame` framebuffer: it is the only way a pixel reaches the screen.

## Attract mode

The screensaver runs a **dedicated instance** that calls only
`cart_init`, `cart_attract`, and `cart_teardown` — no load phase, no
updates. Audio and peripherals are fully live in it, so the platform
convention is structural silence: attract plays zero sounds and
touches no LCDs/lights; any title sting is fired from a path attract
never runs (`LoadFrame`, not `Init`). Add a golden assertion counting
`host_play_sound` calls across init + a long attract run: zero.
