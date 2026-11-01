# Art and sound

How a cartridge's pictures and noises are made so that they survive the
trip to a panel and a pair of small speakers, and stay identical on both
hosts.

## Sprite art as PNGs

Gameplay art lives as hand-editable PNGs, shipped in the bundle and
lifted with `Setup.LoadSheet` in `Init` into a guest-side `ui.Sprite`,
so goldens and parity hold. The alpha rule, `ui.OpaqueMask`'s: only
`A == 255` pixels enter the collision mask, so soft edges are automatically forgiving and a
"solid-looking but benign" region is authored at A=254. Keep animated
effects code-drawn on top; keep tiling/parallax placement in code with
the repeating unit as the PNG. The full pipeline, spritelab to
`Sheet.Cel`, is in [the tutorial](tutorial-assets.md#draw-the-fruit).

## Sound design

The cabinet's small speakers reproduce nothing below ~150 Hz —
synthesize fundamentals in the 250–1500 Hz band and make the asset
*generator* assert its spectrum (the shared synth
`tools/sfxlib` has oscillators, ADSR, noise, and Goertzel
band assertions built in). Perceptual rumble = amplitude-modulating a
mid-band carrier at 30–60 Hz. Add a ~90 ms retrigger cooldown wherever
physics can machine-gun a collision sound. Game-over deserves its own
sting, sequenced structurally after the death beat.
