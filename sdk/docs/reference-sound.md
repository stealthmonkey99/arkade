# Sound reference

This documents the `sound` kind of the asset envelope introduced on
[Asset file reference](reference-assets.md): the JSON that lives inside a
`.arkadesound.wav`'s `arkd` RIFF chunk, its fields, what a reader refuses
versus what it lets through, and how a cartridge loads one. Read
[Asset file reference](reference-assets.md) first for the envelope fields
every kind shares (`arkade`, `kind`, `name`, `generator`, `recipe`, ...)
and exactly where this JSON sits inside the WAV.

soundlab is still the easy way to produce one of these files — design the
sound, press Save, and the recipe is written for you. This page is for
reading, hand-editing, or generating one from a cartridge's own
`tools/gen-sfx`.

## The file is a WAV first

A `.arkadesound.wav` **is** a WAV. The audio is exactly the 16-bit mono
PCM `sfxlib.Encode` has always written; the envelope rides in one extra
RIFF chunk between `fmt ` and `data`. Every audio player still plays it,
`ffmpeg -i` still reads it, and — the part that matters at run time —
`host_load_sound` loads it by path and hands the bytes to the platform's
decoder **without ever reading the envelope**. Both decoders in use walk
the RIFF chunk list and skip what they do not recognise (the cabinet's
`gopxl/beep`, the browser's `decodeAudioData`), so the extra chunk costs
nothing but its own bytes.

The consequence worth stating plainly: the envelope is for **tools**. It
is what lets soundlab reopen a sound six months later, and what lets
`rcartpack` refuse a broken one at build time. Nothing at run time
depends on it, and a cartridge that never had one behaves identically.

## The `arkd` chunk

Ordinary RIFF framing, no new rule to learn:

```text
offset  bytes  content
0       4      "arkd"                     — chunk id, ASCII, not NUL-terminated
4       4      chunk size N, little-endian u32
8       N      the envelope JSON, ASCII (canonical: see reference-assets.md)
8+N     0/1    a single zero pad byte, present only if N is odd
```

It is written **after `fmt ` and before `data`**, which is where every
header chunk in a WAV sits. Position is not decoration: a chunk after
`data` would be invisible to a reader that stops at the end of the audio.
A write drops any existing `arkd` chunk and inserts the new one in the
same place; `fmt `, `data` and every other chunk cross byte for byte,
`data`'s samples included.

The JSON is the canonical compact ASCII form described under
[Determinism](reference-assets.md#determinism), so writing the same
envelope twice always produces the same bytes.

## Fields

A sound envelope adds two fields beside the common ones: `render`, the
format the samples were rendered **to**, and `recipe`, the composition
that produced them.

### `render`

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `sampleRate` | integer | yes | Frames per second. Must be positive. The fleet uses 44100 for one-shot effects and 22050 for music beds. |
| `channels` | integer | yes | 1 or 2. Everything the fleet ships is mono; the cabinet's mixer is mono downstream anyway. |
| `bits` | integer | yes | 8, 16, 24 or 32. **16 in practice** — `sfxlib.Encode` writes 16-bit PCM and the cabinet's decoder supports 8, 16 and 24 only, so a 32-bit file validates here and will not play. |

`render` is **not** a recipe field. It is a claim about the container,
and it is checked against the container: a file whose `fmt ` chunk says
22050 Hz and whose envelope says 44100 is refused, by `rcartpack` at
pack time and by any tool that opens it. That check is the whole reason
this block exists — an envelope that could disagree with the audio beside
it would make the recipe unreproducible while looking perfectly fine.

### `recipe`

The composition that produced the samples. Optional: a **sampled** sound
— something recorded or lifted rather than synthesized — is a perfectly
good `.arkadesound.wav` with no recipe at all, it simply cannot be
reopened in a designer. An absent `recipe`, `null`, and `{}` all mean the
same thing.

```json
"recipe": {
  "compose": [
    { "loop": "bassline", "params": { }, "repeat": 4 },
    { "sfx": "blip", "params": { "hz": 880, "ms": 60 }, "atMs": 1200, "gainDb": -6 }
  ],
  "mix": { "peak": 0.9, "joinMs": 6 }
}
```

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `compose` | array of nodes | yes | The sources, in the order they were laid down. 1–128 entries. |
| `mix` | object | no | The mixing stage. Its **presence** decides how the recipe renders — see [One node, or a mix](#one-node-or-a-mix). |

#### `compose` nodes

Each node names one source and says how to make it and where it goes:

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `sfx` | string | one of the two | 1–64 characters, no control characters | — | The source's **name**, when it is a one-shot: material whose own envelope opens and closes at silence. |
| `loop` | string | one of the two | 1–64 characters, no control characters | — | The source's **name**, when it is a seamless cycle: material that may be tiled and that wants a crossfade at its junctions. |
| `params` | object | no | opaque | `{}` | The named generator's own parameters — see [Who reads `params`](#who-reads-params). |
| `atMs` | number | no | 0–600000 | `0` | Where the node starts, in milliseconds from the sound's beginning. |
| `gainDb` | number | no | −96–24 | `0` | The node's gain in decibels. `0` is unity, exactly. |
| `repeat` | integer | no | 0–64 | `1` | How many times the source is laid down back to back. `0` and `1` both mean once. |
| `fx` | object | no | see [`fx`](#fx) | absent | A treatment for this placement of the source: slice, reverse, stretch, pitch, filters, fades, gain. Only a recipe with a `mix` block may carry it. |

Exactly one of `sfx` and `loop` is set. Which key carries the name is
what says what **kind** of material it is — one-shot or cycle — and that
distinction changes the render: a looping node is tiled by `repeat` and
crossfaded into its neighbours, a one-shot is written verbatim because a
fade would soften the very transient it exists for.

Decibels rather than a linear factor because a renderer that always
derives its gain from this number cannot disagree with the file about how
loud a section is. `0` dB is exactly 1, so unity survives a round trip
untouched.

#### `fx`

A node's `fx` treats **this placement** of its source, so one sound
can be trimmed, slowed, re-pitched or ducked where it is used without
becoming a second sound. The steps run in the order of the table, on the
source as rendered and before `repeat` tiles it; every field's zero
leaves its step out, so `{}` is no treatment at all. `sfxlib.ApplyFx` is
the renderer.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `startMs` | number | no | 0–600000 | `0` | Slices the source from here, in the source's own milliseconds. |
| `endMs` | number | no | 0–600000, after `startMs` | `0` | Slices the source up to here. `0` is the source's end. |
| `reverse` | boolean | no | — | `false` | Plays the slice backwards. |
| `stretch` | number | no | 0.25–4 | `1` | Multiplies the slice's length: `2` is twice as long. The pitch is held (a WSOLA time-stretch) unless `varispeed` is set. `0` and `1` both mean unchanged. |
| `varispeed` | boolean | no | — | `false` | Makes `stretch` tape-style: a longer clip is played slower and so lower, by the same ratio. |
| `pitchSemitones` | number | no | −24–24 | `0` | Moves the pitch without changing the length. Within about ±4 a voice keeps its character; further out its formants travel with it. |
| `highpassHz` | number | no | 0–24000 | `0` | A high-pass corner at 12 dB/octave. `0` is off. |
| `lowpassHz` | number | no | 0–24000 | `0` | A low-pass corner at 12 dB/octave. `0` is off. |
| `fadeInMs` | number | no | 0–600000 | `0` | A linear fade up from silence over the treated clip's start. |
| `fadeOutMs` | number | no | 0–600000 | `0` | A linear fade down to silence over its end. |
| `gainDb` | number | no | −96–24 | `0` | The clip's own level, applied last and on top of the node's `gainDb`. An editor that gives the node's gain to a whole lane keeps the clip's here. |

A `loop` node may carry `fx`, but slicing, reversing, stretching or
pitching a cycle breaks the seamless join it exists for and moves its
tempo; soundlab offers only gain, filters and fades on one.

#### `mix`

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `peak` | number | yes | 0–1 | — | The level the summed mix is normalized to. `0` leaves the mix as it came out. soundlab writes `0.9`, the headroom every Årkade asset ships with. |
| `joinMs` | number | yes | 0–1000 | — | The crossfade length at a looping node's junctions. soundlab writes `6`. |
| `seconds` | number | no | 0–600 | `0` | Forces the mix's total length. `0` means "as long as the material", which is what every soundlab save writes. Setting it *shorter* folds the overhang back onto the start — a deliberate loop-shortening tool and an easy way to make a mess. |

Both `peak` and `joinMs` are written explicitly rather than left to a
library default, so that a default moving later cannot change what a
saved file renders to.

### One node, or a mix

The `mix` block's presence is the render's mode, stated rather than
inferred:

- **No `mix`** — the recipe holds **exactly one** node, at `atMs` 0, at
  `gainDb` 0, played once, with no `fx`, and the render **is** that node's own output,
  sample for sample. This is what soundlab's SFX and LOOPS tabs save.
- **With `mix`** — every node is placed on one timeline, summed, and
  normalized to `mix.peak`. This is what the COMPOSE tab saves.

The distinction is not tidiness, it is byte-equality. A lone loop pushed
through the mixer would pick up the junction crossfade and a second
normalization, and the file would stop being the loop its generator
produced. So a recipe with two nodes and no `mix` is **refused**: a
reader that invented a mixer would not reproduce the file it is reading.

### Who reads `params`

Whoever `generator.tool` names, and nobody else. `params` is the
generator's own document — `assetfile`, `rcartpack` and both engine hosts
carry it through untouched and never look inside it.

That is what makes the field useful to two very different kinds of
producer:

- **soundlab** puts its knobs in there. An `sfx` node's params are the
  SFX tab's classic panel (waveform / frequency / ADSR / filters / echo /
  seed); a `loop` node's params are the LOOPS tab's grid (tempo, steps,
  scale, cells). The recipe is self-describing, so opening the file
  restores the editor.
- **A cartridge's `tools/gen-sfx`** often has nothing to put there. Its
  voices are hand-composed oscillators and envelopes; nothing short of
  that Go file reproduces them. So the honest recipe is the generator's
  own **name** for the voice, with no params at all —
  `{"sfx": "lost"}` — and `generator` is what says which program to
  re-run. Name-only is the honest form for a voice no grammar describes;
  Snake's cues began this way, before they were converted to `patch` and
  `vocal`.

Both are legitimate uses of the same field. `generator` being pinned is
what keeps them from being confused: a tool that meets a recipe it did
not write **reports it** and offers to re-render rather than guessing at
somebody else's parameters.

There is a third case, and the rest of this section is about it.
`params.kind` can name one of four **published grammars** —
[`patch`](#the-patch-params), a stack of layers, or
[`vocal`](#the-vocal-params), a spoken phrase, both `sfxlib`'s;
[`speech`](#the-speech-params), a line of text read by a neural voice,
`tools/speech`'s; or
[`song`](#the-song-params), a music bed as note data, `musiclib`'s —
and then the params are neither soundlab's panel nor a name for a
function: they are a document anything that links the named library can
read, turn over and re-render, whoever wrote it. Snake's `omnomnom.arkadesound.wav` is the worked
example: its recipe carries the syllables that MAKE it rather than a name
for a function that does, so the file is its own source and soundlab
reopens and re-renders it without Snake's help.

### The `patch` params

`"patch"` is `sfxlib.Patch`: one designed sound written entirely as data
— a stack of **layers** over one time base, each a source shaped by an
envelope and optionally filtered, summed and normalized. Unlike the two
uses above it is neither soundlab's panel nor a program's name; it is a
published grammar, and anything that links `sfxlib` can read it, turn its
knobs and re-render it. soundlab does: it renders a patch node through
`sfxlib` rather than through its own panel, so a cartridge's cue reopens
and replays there, and re-rendering it reproduces the audio sitting
beside the recipe, sample for sample. soundlab is hosted at
[arkade.build/dev/tools](https://arkade.build/dev/tools).

That is what it is for. [Who reads `params`](#who-reads-params) left a
cartridge's `tools/gen-sfx` with one honest recipe, the voice's **name**,
because its cues were hand-composed loops no parameter set described.
A name is not something anybody can turn a knob on. A patch is how a
cartridge ships an **editable** sound without first getting a bespoke
voice merged into `sfxlib` — the same bargain
[cartridge glyphs](reference-glyph.md#a-worked-cartridge-a-falling-block-game) struck: a
general table in the engine for what every game wants, and a game's own
specific thing carried inside that game's own assets. A cue the grammar
cannot reproduce bit for bit stays a hand-written voice with a name-only
recipe, which is exactly what it is; `tools/assetdiff` decides which is
which, per cue, by comparing the audio.

soundlab's panel is still the right choice for a sound you are designing
by ear, and the name-only recipe for a voice only Go can describe. Reach
for a patch when a cartridge's cue is a stack — a body plus a click
transient, band-passed noise under a swept tone, a fundamental with
partials that die at their own rates — and you want the file to carry
what made it.

#### A layer is not a compose node

Both stack sound up. The difference is what they share.

**Layers** are parts of one voice. They share one time base and they
share the patch's one noise stream, and they are summed sample by sample
in **declared order** — the only way a body and the click transient
gated over its first few milliseconds can draw from one generator and
come out the way a hand-written loop writes them. That order is
observable, not tidiness: two noise layers swapped render different
samples, because the second draws whatever the first left behind. A
layer draws from the stream only for the samples it renders — after its
delay, inside its own length, while its gate is open — so a gated
transient does not move the noise under the layers after it.

**Compose nodes** are separate sounds placed on a timeline at their own
`atMs` and `gainDb`, each rendered whole and then mixed. A fanfare's four
notes are four nodes; a single chime with four partials is one node
carrying one patch with one layer.

#### The document

A patch node's `params` is the whole document below. Numbers are not
range-checked — `assetfile` and `rcartpack` never look inside `params`,
and a value outside the ranges stated here renders whatever the
arithmetic says — but every **word** is: whatever reopens the patch
refuses an unknown `wave`, `sweep`, `env.kind` or `filter.kind` rather
than falling back to a default, because silently rendering a different
envelope from the one the file names is the failure this format exists
to prevent. Durations are seconds throughout, unlike the node's own
`atMs`.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `kind` | string | yes | `"patch"` | — | What says the rest of the document is this grammar. A node whose `kind` is anything else is not a patch, and `sfxlib` leaves it alone rather than refusing it. |
| `name` | string | no | — | — | The sound's own name, written in so the document says what it is. Not checked against the node's `sfx`. |
| `durationS` | number | yes | > 0 | — | The sound's length in seconds. Non-positive renders nothing. |
| `peak` | number | no | 0–1 | absent | The level the summed layers are normalized to. Absent or `0` leaves the sum as it came out. The fleet ships `0.9`. |
| `fadeOutS` | number | no | 0–`durationS` | `0` | Ramps the last `fadeOutS` seconds of the **summed** mix linearly to zero, ahead of `peak`'s normalization — see [The tail fade](#the-tail-fade). `0` is no fade. Negative, or longer than `durationS`, is refused. |
| `seed` | integer | no | 0–2⁶⁴−1 | `0` | Seeds the shared noise stream, so the same patch renders the same samples on every machine. A decimal number in the file (`49407` for a generator's `0xC0FF`). |
| `layers` | array of layers | yes | — | — | The parts of the voice, summed in this order. |

#### The tail fade

A finished sound has to end in silence, and almost nothing here does so
on its own. An `expdecay` envelope gets very small and never reaches
zero; a filter rings past the end of its input; a layer whose `gate` or
`durationS` stops it mid-cycle stops on whatever sample it was on. The
last sample of such a mix is a step down to nothing, and the cabinet's
speakers play a step as a click. `fadeOutS` is the cure: the final
`fadeOutS` seconds of the sum are multiplied by a straight line from
`1` down to exactly `0` on the last sample, so the mix ends at silence
whatever its layers were doing. `0.004` — four milliseconds, a couple of
hundred samples — is inaudible as a fade and enough to remove the click;
every cue in a click-prone set should end this way.

It is a property of the **patch**, not of a layer, and there is no
per-layer form on purpose: a layer that fades under one that does not
still steps to silence, so the only place a fade means anything is the
finished sum. It runs before normalization, so `peak` measures the faded
sound. The ramp is in samples — `round(fadeOutS · sampleRate)` of them —
and a value that rounds to no samples is no fade. A `fadeOutS` longer
than `durationS` is refused when read; from Go, `Patch.FadeOutS` past the
end fades the whole patch rather than failing.

#### A layer

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `name` | string | no | — | — | A label for the reader. Nothing branches on it. |
| `durationS` | number | no | > 0 | the rest of the patch | The layer's own length. It is the `1` every fraction below runs to, except the slide's when `slideS` is written. Clipped at the patch's end. |
| `delayS` | number | no | ≥ 0 | `0` | Holds the layer silent for this long, then starts its own clock at zero. This is how one patch places a second hit without becoming two nodes. |
| `wave` | string | no | `sine`, `square`, `triangle`, `saw`, `noise`, `wood` | `sine` | The source. `noise` is white noise from the shared stream, and ignores `hz`, the slide, `level`, `partials`, `noiseMix` and `noiseAM`. `wood` is a sine plus a bar-mode partial at 2.756×, in every sweep mode. `square` takes a `duty`. |
| `duty` | number | no | 0 up to, not including, 1 | `0` | `square` only: the fraction of each cycle spent high — see [Pulse duty](#pulse-duty). `0` is the plain square. `1` or more, or negative, is refused. |
| `hz` | number | no | > 0 | `0` | The fundamental. `0` is silence for every wave but `noise`. |
| `slideToHz` | number | no | ≥ 0 | absent | Where the fundamental heads: at fraction *u* of the glide it is `hz + (slideToHz − hz) · u^slideCurve`, or with `slideRate` an exponential approach — see [The two slide contours](#the-two-slide-contours). **Absent** is no slide: the pitch holds at `hz` whatever the four fields below say. **Written**, it is a destination the sound is made of, and `0` is zero hertz — see [Zero hertz](#zero-hertz). Negative is refused. |
| `slideCurve` | number | no | > 0 | `1` | The power-law slide's exponent. `1` is linear; `≤ 0` reads as `1`. Refused beside `slideRate`. |
| `slideS` | number | no | > 0 | the layer's length | The glide's own length: *u* runs 0→1 over `slideS` seconds and the pitch then holds at `slideToHz` — see [Glide time](#glide-time). `0` is the whole layer. Negative is refused; so is writing it beside `slideRate`. |
| `slideDelayS` | number | no | > 0 | `0` | Holds the pitch at `hz` this long, then begins the glide — see [Glide time](#glide-time). `0` glides from the first sample. Negative is refused. |
| `slideRate` | number | no | > 0 | none | Makes the slide an exponential approach: `slideToHz + (hz − slideToHz) · e^(−t · slideRate)`. Larger is faster. Negative is refused; beside `slideCurve` or `slideS` it is refused too. Without a `slideToHz` there is nothing to approach and the pitch holds. |
| `sweep` | string | no | `pointwise`, `phase`, `phaselead` | `pointwise` | How a moving frequency becomes samples — see [The `sweep` modes](#the-sweep-modes). Written even for a layer that never moves. |
| `level` | number | no | > 0 | `1` | Weights the fundamental against the partials. `0` reads as `1`. |
| `partials` | array | no | — | none | Extra components at fixed ratios to `hz` — see [`partials`](#partials). |
| `noiseMix` | number | no | 0–1 | `0` | Crossfades the finished oscillator stack toward noise from the shared stream: `(1 − m) · stack + m · noise`. Clamped. A layer with any mix draws from the stream, and so moves the layers after it. |
| `env` | object | yes | — | — | The amplitude shape — see [`env`](#env). |
| `gate` | object | no | — | always open | Restricts the layer to a window of its own clock — see [`gate`](#gate). |
| `am` | object | no | — | none | Tremolo — see [`am`](#am). |
| `noiseAM` | object | no | — | none | Per-sample amplitude roughness drawn from the shared stream — see [`noiseAM`](#noiseam). |
| `vibrato` | object | no | — | none | A wobble on the layer's pitch — see [`vibrato`](#vibrato). |
| `filter` | object | no | — | none | One shaping stage — see [`filter`](#filter). |
| `filters` | array of filter objects | no | — | none | A **cascade**: stages run one into the next, in this order — see [A cascade](#a-cascade). For a layer that needs more than one stage; a single stage is written as `filter`. |
| `gain` | number | no | > 0 | `1` | Scales the layer's **source**, ahead of envelope and filter. `0` reads as `1`, so a layer nobody weighted still sounds. See [`gain` and `level`](#gain-and-level). |

A layer renders as `gain · source`, then `· env · am`, with each filter
stage applied before or after that product as its own `beforeEnv` says.
The gate closes the **source**: a sample outside the window is zero and
draws nothing, but the filters run over the whole layer, so a band-passed
transient still rings after its window shuts.

#### `env`

Every kind is evaluated on the layer's own clock, zero at the end of
`delayS`. The fraction *u* is the layer's elapsed time over its own
length, clamped to 0–1.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `kind` | string | yes | `adsr`, `expdecay`, `attackdecay`, `fadeout`, `arch`, `const` | — | The shape. |
| `attackS` | number | no | ≥ 0 | `0` | `adsr`'s linear rise 0→1, or `attackdecay`'s linear onset. One field, because the two kinds are exclusive. |
| `startLevel` | number | no | 0–1 exclusive | `0` | `adsr` and `attackdecay`: the level the attack starts from, so the rise over `attackS` is `startLevel`→1 rather than 0→1 — see [The swell](#the-swell). `0` is the attack from silence. `1` or more, or negative, is refused; so is writing it without an `attackS`, or on a kind that has no attack. |
| `decayS` | number | no | ≥ 0 | `0` | `adsr`: the linear fall 1→`sustainLevel`. |
| `sustainS` | number | no | ≥ 0 | `0` | `adsr`: how long `sustainLevel` holds. |
| `releaseS` | number | no | ≥ 0 | `0` | `adsr`: the linear fall `sustainLevel`→0. |
| `sustainLevel` | number | no | 0–1 | `0` | `adsr`: the held value. |
| `rate` | number | no | > 0 | `0` | `expdecay` and `attackdecay`: the decay's rate. Larger is faster; `0` never decays. |
| `curve` | number | no | > 0 | `1` | `fadeout`'s exponent. `≤ 0` reads as `1`. |

- `adsr` — the four linear stages, over the sum of the four times. That
  sum need not match the layer's length: past its own end the envelope
  reads `0`, which is how a short tick inside a long layer is written.
- `expdecay` — `e^(−t · rate)`: an instant onset and an exponential
  tail, the shape most of the fleet's percussive cues use.
- `attackdecay` — `min(1, t / attackS) · e^(−t · rate)`, with `t`
  counted from the layer's start throughout. For the cues that ramp in
  over a few milliseconds to avoid the click an instant onset makes. It
  is a product rather than two stages on purpose: ramping to `1` and
  only then beginning to decay steps the level down the moment the ramp
  ends, which is the click the attack exists to prevent.
- `fadeout` — `(1 − u)^curve`. `2` is the `(1−u)²` several cues use.
- `arch` — `sin(π · u)`: silence at both ends, peak at the middle. The
  shape of something passing by.
- `const` — `1` for the layer's whole length.

#### The swell

An attack rises from silence, and that is right for a sound that
**begins**: a note struck, a door opened. It is wrong for a sound the
listener joins **in progress** — a gust caught mid-blow, a crowd that
was already murmuring when the scene cut to it, a hiss fading up under
a whistle. Those are audible on their first sample and then grow, and
`startLevel` is where they start: the onset runs from `startLevel` to
`1` over `attackS`, a straight line as before, and everything after the
attack is unchanged. It is the initial level of a multi-stage envelope
generator, the parameter that lets a brass patch re-trigger without
gapping, and it is not a spelling of anything else here. Two layers of
the same tone, one held at the floor and one rising, add up to the
swell only because they are the same waveform; two layers of noise draw
different samples and add up in power, which is a slower, hollower rise
and one more layer than the sound has.

The value scales the source like every envelope here, so a sine
swelling from `0.2` still opens at zero on its first sample and is
`0.2` of its amplitude a quarter-cycle later. A noise layer swelling
from `0.2` steps to a fifth of its level on sample zero; that step is
the sound already going, and it is what the field is for.

#### `partials`

One extra component of the source, at a fixed **ratio** to `hz` rather
than at a pitch of its own: a partial that keeps its place through a
slide is what makes a swept chime still sound like one object. A chime
is a fundamental plus three of these; a struck wooden bar is a
fundamental plus one at `2.756`.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `ratio` | number | one of the two | > 0 | — | Multiplies the layer's frequency, so the partial follows a slide. A partial with neither `ratio` nor `hz` is skipped. |
| `hz` | number | one of the two | > 0 | — | Names the partial's frequency outright, taking precedence over `ratio`. An absolute partial does **not** follow the layer's slide, because it is not in a relationship with it. Use it for a spread that is not a harmonic series — a struck metal object's modes are not multiples of anything, and `520, 780, 1170, 1560, 2210` written as ratios of the first is arithmetic nobody can check. |
| `gain` | number | yes | ≥ 0 | — | The partial's weight against the fundamental's `level`. `0` is silent here — this is not one of the fields that reads `0` as `1`. |
| `wave` | string | no | `sine`, `square`, `triangle`, `saw` | `sine` | The partial's own shape. `noise` and `wood` are refused: a partial is a ratio against the fundamental, so noise has no frequency to take a ratio of, and wood is already a fundamental plus a bar-mode partial. |
| `decay` | number | no | > 0 | none | An exponential decay of the partial's own, on **top** of the layer's envelope — the whole reason a bell's upper partials die before its fundamental does. |

#### `gate`

Restricts a layer to part of its clock, in seconds from the end of its
`delayS`. It exists for the transient: a tap is a tone plus a few
milliseconds of noise, and the noise layer must be silent — and must draw
no randomness — outside its window, or every following sample of the
shared stream moves.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `fromS` | number | no | ≥ 0 | `0` | The window opens here. |
| `toS` | number | no | > 0 | to the end | The window shuts here. `0` means the layer's end. |
| `periodS` | number | no | > 0 | none | With `onS`, repeats the window instead: open for `onS` out of every `periodS`, counted from `fromS`. Both must be positive for either to mean anything. |
| `onS` | number | no | > 0 | none | See `periodS`. |

#### `am`

A sinusoidal tremolo on the layer's amplitude: the factor is
`base + depth · sin(2π · hz · t)`, on the layer's clock, applied with
the envelope.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `hz` | number | yes | > 0 | — | The modulation rate. `≤ 0` disables the block: the factor is `1`. |
| `base` | number | yes | — | — | The factor's centre. `0.8` with a `depth` of `0.2` swings between `0.6` and `1`. |
| `depth` | number | yes | — | — | The swing either side of `base`. A `depth` larger than `base` crosses zero, which flips the layer's polarity twice a cycle. |

#### `noiseAM`

Roughness rather than wobble: the layer's **source** is multiplied by
`base + depth · noise`, a fresh draw from the shared stream every sample,
before the envelope.

It is neither a tremolo nor a noise layer, and the fleet uses it where
neither would do. An explosion whose tonal body is shredded by randomness
is one sound, not a tone alongside a hiss, and a periodic modulator would
read as a wobble rather than as damage.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `base` | number | yes | — | — | The factor's centre. `0.6` with a `depth` of `0.4` ranges over `0.2`–`1.0`. |
| `depth` | number | yes | — | — | How far the noise pushes it either way. |

A layer carrying this block **draws from the shared stream every
sample**, so it moves every layer declared after it — see
[A layer is not a compose node](#a-layer-is-not-a-compose-node).

#### `vibrato`

A wobble on the layer's **pitch**, at `hz`, whose width is written one
of two ways because they are two sounds.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `hz` | number | yes | > 0 | — | The wobble's rate. `≤ 0` disables the block. |
| `depthHz` | number | one of the two | — | — | The swing **in hertz**: the frequency is the layer's own plus `depthHz · wobble(t)`. |
| `depth` | number | one of the two | 0–1 exclusive | — | The swing as a **fraction of the layer's current frequency**: the frequency is the layer's own `× (1 + depth · wobble(t))`. `1` or more, or negative, is refused; so is writing it beside `depthHz`. |
| `wave` | string | no | `sine`, `triangle` | `sine` | The wobble's shape. Anything else is refused. |

`depthHz` keeps its width while a layer slides, which is what makes the
fleet's falling game-over cue read as an instrument rather than as a
siren. `depth` keeps its **interval**: `0.02` is a third of a semitone
on a 200 Hz note and on a 2 kHz one, which is the wobble a singer or a
player makes, where 20 Hz would be a wide wobble on the first and
inaudible on the second. Under a slide a proportional wobble carries its
interval rather than its hertz. Cents, for small widths, are about
`1731 × depth`.

A `sine` wobble lingers at each extreme and turns smoothly; a `triangle`
moves at a constant rate between its extremes and turns sharply. The
first reads as sung, the second as mechanical — a warble, a trill — and
under `pointwise` evaluation, whose instantaneous pitch carries the
wobble's **slope** (see [The `sweep` modes](#the-sweep-modes)), a
triangle is a trill outright: the pitch alternates between two values
instead of swinging between them. That is not a rounding difference —
a hum written with one and reopened with the other is a different cue.

#### `filter`

One shaping stage. A layer carries one as `filter`, or several as
`filters` — see [A cascade](#a-cascade) below. Absent means unfiltered.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `kind` | string | yes | `none`, `lowpass`, `highpass`, `bandpass` | — | `lowpass` and `highpass` are one-pole corners; `bandpass` is the resonant biquad that turns white noise into an explosion the cabinet's small speakers can reproduce. |
| `hz` | number | yes | > 0 | — | The corner, or the band's centre at the start of the layer. |
| `sweepToHz` | number | no | > 0 | none | `bandpass` only: the centre travels here across the layer, straight, `hz + (sweepToHz − hz) · u`. `0` holds the centre. |
| `arc` | boolean | no | — | `false` | `bandpass` with `sweepToHz`: the centre travels to `sweepToHz` and **back**, following `sin(π · u)` — at `sweepToHz` at the midpoint, at `hz` again at the end. |
| `q` | number | no | > 0 | `0` | `bandpass` resonance; higher is narrower. `0` is not a filter: the band-pass passes the source through unchanged. |
| `gain` | number | no | > 0 | `1` | Scales this stage's output, any kind but `none`. `0` reads as `1`. A narrow band-pass throws most of the energy away, and every generator that uses one lifts it back afterwards. In a cascade each stage has its own, applied as that stage finishes. |
| `beforeEnv` | boolean | no | — | `false` | Applies this stage to the raw source, ahead of the envelope, rather than to the enveloped one. In a cascade the flag is per stage, and it is what places the envelope inside the chain. |

`arc` earns a field because of what it pairs with. A car's whoosh rises
and falls around the moment it is level with the listener; an `arch`
envelope peaks at the layer's midpoint, and an arced sweep reaches its
destination on the same sample. A passing car's `whoosh` is one noise layer,
`arch` over a band arcing 300–900–300 Hz, and the two peak together.

`beforeEnv` exists because a linear filter does not commute with a
moving gain. Enveloping noise and then filtering it is a decaying burst;
filtering the noise and then enveloping the resonance is a different
sound, and the fleet's explosions and splashes are the second. The
flag says which was meant.

#### A cascade

`filters` is an ordered list of the stages above, and they run **one
into the next**: the output of the first is the input of the second. A
cascade is a filter none of its stages is alone. Two one-pole lowpasses
followed by a one-pole highpass is a three-pole band — 12 dB per octave
falling above the upper corner, 6 dB per octave below the lower — and it
is what a woody click or a plucked nail is made of; neither a
one-pole stage nor the resonant `bandpass` has that shape, which is why
such transients cannot be written as a layer with one
`filter`. Reach for a cascade when the shape you want is a slope steeper
than one pole gives, or a band without a resonance in it; reach for
`bandpass` when a resonance is the point.

The envelope sits inside the chain wherever `beforeEnv` puts it: the
stages marked `beforeEnv` run first, in the order written, then the
envelope and tremolo, then the remaining stages in the order written. So
a lowpass on the raw noise and a highpass on the enveloped result is one
layer with two stages, the first marked `beforeEnv`. A cascade is
written in **signal order**, and a `beforeEnv` stage after one that is
not is refused rather than quietly moved ahead: the document must show
the order the audio has.

One spelling per length. `sfxlib` writes a single stage as `filter` and
two or more as `filters`, whichever a generator held them in, so a
document re-saved from soundlab has one form and the one-stage cues
already committed re-marshal as they were written. A document naming
both keys reads as `filter` followed by `filters`, in that order.

At the edges: an empty `filters` is no filter. A stage whose `kind` is
`none` passes the signal through untouched — its `gain` is not applied —
exactly as a lone `filter` of kind `none` does. A stage whose `hz` is
`0` is a pass-through for `lowpass` and `highpass` (there is no corner)
and a band-pass with a `q` of `0` passes through too, so a stage that
filters nothing still takes its `gain`. An unknown `kind` anywhere in the
list is refused, with the stage's index in the message. There is no
limit on length, and a chain of forty stages renders forty passes.

#### The two slide contours

A slide is a destination and a shape, and the grammar has two shapes
because they are two sounds.

The **power law** is the default: *u* is the fraction of the glide
elapsed, and the pitch is `hz + (slideToHz − hz) · u^slideCurve`. It
eases throughout and it **lands** — at the end of the glide the pitch is
`slideToHz` exactly. `slideCurve` below `1` does most of the travel
early, above `1` saves it for the end; a rising alarm is a curve of `2`,
a falling zap a curve of `0.5`.

The **exponential approach** is `slideRate`: the pitch at *t* seconds is
`slideToHz + (hz − slideToHz) · e^(−t · slideRate)`. It moves fastest at
the very start and settles toward `slideToHz` without ever quite arriving
— after `1/slideRate` seconds a third of the distance remains, after
`3/slideRate` a twentieth. That is the shape of a system relaxing toward
rest, and it is the pitch envelope of a kick drum, a dropped object, a
laser: the ear hears the fast initial fall as impact and the long settle
as the thing coming to rest. A power law with a small curve looks
similar on paper and does not sound the same, because it keeps easing
all the way to a landing the ear can place; the exponential has no
landing.

Choose the power law when the sound should **arrive** somewhere — a
siren reaching its top, a glide between two notes, anything melodic.
Choose the exponential when the sound should **settle** — anything
struck, dropped or fired. It is a function of time rather than of a
fraction, so `slideCurve` and `slideS` have no meaning beside it and a
document writing either with `slideRate` is refused rather than left
carrying a number that does nothing.

#### Glide time

`slideS` is the glide's own length. Without it the fraction *u* runs over
the whole layer, so a slide always takes exactly as long as the sound.
With it, *u* reaches `1` after `slideS` seconds and the pitch **holds** at
`slideToHz` for the rest of the layer.

That is portamento: a bend into a held note, a siren that reaches its
pitch and stays, a tom whose pitch drops in the first fifty milliseconds
and then rings for half a second at its floor. None of those can be
written any other way, because a layer's slide and its length were one
number. It is the only field that changes what `durationS` means — the
`1` of the slide's fraction, and nothing else; the `fadeout` and `arch`
envelopes and a filter's `sweepToHz` still run over the layer.

`slideDelayS` is the other end of the same idea: the pitch **holds** at
`hz` for that long and the glide begins there, with its own clock —
`slideS`, or the rest of the layer — starting at that moment. Where
`slideS` is a bend into a held note, this is a held note that bends
away: a guitar string pushed after it is struck, a siren steady before
it dips, a sigh that holds its note and then falls. Together they write
hold–glide–hold, which no single slide could. Beside `slideRate` the
exponential approach starts at `slideDelayS` instead of on the first
sample.

At the edges: a `slideS` longer than the layer is a glide cut off before
it arrives, which is a legitimate sound and not an error. A `slideS` one
sample shorter than the layer is a glide whose last rendered sample sits
exactly on `slideToHz`. A `slideDelayS` past the layer's end is a pitch
that never moves. The value is seconds like every other duration here,
and it is what the audio does — never write a `slideToHz` the sound does
not reach, or an `hz` it never plays, to make a render come out; a
document must carry the numbers the sound is made of.

#### Zero hertz

`slideToHz` is either absent or a number, and the two are different
things. **Absent**, the layer never moves: the pitch holds at `hz`, and
a `slideCurve`, `slideS`, `slideDelayS` or `slideRate` written beside
nothing is a number that does nothing rather than a slide. **Written**,
every value is a destination, and `0` is one — zero hertz, silence, the
floor every fall is measured from.

The distinction exists for the exponential approach, whose `slideToHz`
is where the pitch would settle given forever rather than where the
sound ends. A fall that drops the same interval in every unit of time —
650 → 200 Hz across 300 ms, the glide of a dropped object or a
life-lost buzz — is `hz · (end/hz)^(t/T)`, which is
`hz · e^(−t · ln(hz/end)/T)`: an approach toward **zero** at a
`slideRate` of `ln(hz/end)/T`. Its rest point is silence, and no
positive rest point is that sound. Writing `1` in its place, because `0`
once meant "off", lands the cue at 200.7 Hz where 200 was meant — a
number the document does not mean, which the rule above forbids. So the
key carries the difference the way `gate`, `am` and `filter` do: left
out for none, written for a value.

No committed document ever wrote `0` while it meant "off" — a layer
that never moved was always written without the key — so a document
written before this reads exactly as it did and re-saves exactly as it
was. From Go, `sfxlib.Layer` keeps `SlideTo: 0` as the layer that never
moves, because every generator ever written relies on that, and says
zero hertz with `SlideToZero: true`; the document needs no flag, because
JSON can leave a key out.

A power-law slide to `0` lands on DC at the end of its glide — the
integrated modes stop advancing, `pointwise` evaluates `sin(0)` — which
is a pitch falling to nothing, a legitimate sound, and not refused.

#### Pulse duty

`duty` is the width of a `square` layer's pulse: the fraction of each
cycle the wave spends at `+1` before dropping to `−1`. `0.5` is the
hollow square; `0.25` is the bright chip lead; `0.125` is thin and nasal;
`0.3`–`0.45` sit between, and every one of them is a different colour
of the same note, which is why a variable-width pulse is the defining
timbre of chip audio. It applies to every square in the layer — the
fundamental and any `square` partial — and to no other wave. The name
and the range are `musiclib`'s `Instrument.Duty`, so a cue and a bed
that say `0.25` mean the same wave.

`0` is the square exactly as this grammar has always evaluated it, and
on an integrated phase (`phase`, `phaselead`) writing `0.5` is the
identical wave. In `pointwise` mode there is one edge: the plain square
is the **sign of a sine** at absolute time, which on a sample landing
exactly on a cycle boundary can read either way by floating-point
rounding, whereas any written `duty` reads the phase directly and is
exact there. So a `pointwise` layer that writes `0.5` can differ from one
that writes nothing on those samples, and a converted cue keeps whichever
it had.

#### The `sweep` modes

A layer with a `slideToHz` has a frequency `f(t)`, and there are three
ways to turn that into samples. They are different sounds, not spellings
of one.

`pointwise` recomputes the frequency at each sample and evaluates the
oscillator at **absolute** time: `sin(2π · f(t) · t)`. The instantaneous
frequency that actually produces is `f(t) + t · f′(t)`, so a "300→500 Hz"
pointwise sweep does not arrive at 500 Hz — it overshoots, and the
overshoot is most of why the fleet's zaps and hops read as energetic
rather than smooth. It is the default because it is what a generator
writing `Sine(t, f0+(f1-f0)*u)` gets, which is nearly every swept cue in
the fleet.

`phase` integrates: the phase advances by `f / sampleRate` every sample,
so the sound truly arrives at `slideToHz`. This is a real glide, and it
is what soundlab's Voice does. Each partial keeps its own phase. Each
sample is read at the phase reached so far and the phase then advances,
so the first sample sits at phase zero — a sine begins from silence.

`phaselead` is `phase` with the other initial condition: the phase
advances by `f / sampleRate` **before** each sample is read, so the first
sample is already one step in and every sample leads `phase`'s by exactly
one increment. That is a different waveform, not a rounding difference —
one increment is 5° of a cycle at 620 Hz and 44.1 kHz — and it is why the
mode has a name rather than a flag. The two are two describable starting
points. `phase` begins at rest, which is what a sound starting from
silence expects. `phaselead` begins already moving, which is what a loop
written `ph += f/rate; out = sin(ph)` produces — most of the fleet's
hand-written glides are written that way — and it means an instant-onset
envelope, at its loudest on sample zero, multiplies something other than
`sin(0)`. Reach for `phase` on a new sound; reach for `phaselead` to
carry a cue that was written the other way, or when sample zero must not
be silence. Partials lead with the fundamental.

#### `gain` and `level`

Two fields because they sit at two different points in the arithmetic,
and floating-point multiplication does not associate.

`gain` multiplies the **source**, before the envelope: a layer is
`gain · source · env`, in that order, because that is how the fleet's
generators write `0.5*noise*decay`. An interpreter that computed
`0.5*(noise*decay)` would differ in the last bit — the difference between
a conversion that provably did not move the audio and one that merely
sounds like it didn't.

`level` weights the fundamental **inside** the source, against the
partials: the stack is `level · fundamental + gain · partial` for each
partial, which is how a chime is written — `0.55*fundamental +
0.25*octave`, not `0.55*(fundamental + 0.45*octave)`.

#### Worked example: a `squash` effect

A `squash.arkadesound.wav` written by a cartridge's `tools/gen-sfx`. Two layers: band-passed noise, and a square
body sweeping down from 300 Hz. Its `arkd` chunk holds this envelope, on
one line in the canonical compact form:

```json
{
  "arkade": 1,
  "kind": "sound",
  "name": "squash",
  "description": "Flattened by traffic: a low squelchy thud, mid-centred so the cabinet can reproduce it.",
  "generator": { "tool": "mygame/gen-sfx", "version": "2.0.0" },
  "recipe": {
    "compose": [
      {
        "sfx": "squash",
        "params": {
          "kind": "patch",
          "name": "squash",
          "durationS": 0.24,
          "peak": 0.9,
          "seed": 49407,
          "layers": [
            {
              "wave": "noise",
              "sweep": "pointwise",
              "env": { "kind": "expdecay", "rate": 11 },
              "filter": { "kind": "bandpass", "hz": 380, "q": 1.1, "gain": 2.2, "beforeEnv": true }
            },
            {
              "wave": "square",
              "hz": 300,
              "slideToHz": 110,
              "sweep": "pointwise",
              "level": 0.5,
              "env": { "kind": "expdecay", "rate": 14 }
            }
          ]
        }
      }
    ]
  },
  "render": { "sampleRate": 44100, "channels": 1, "bits": 16 }
}
```

The noise layer is filtered **before** its envelope — a resonance at
380 Hz that the decay then shapes — and lifted by 2.2 to put back what a
band that narrow discards. The body's `level` of `0.5` is a source-time
weight with no partials to weigh against, written that way because the
hand-written loop it replaced wrote `0.5*Square(...)`. Both layers run
the whole 240 ms; the noise layer is declared first, and is the only one
that draws from the stream, so `seed` fixes every sample. One node, no
`mix`: the render is this patch's own output, and `tools/assetdiff`
reports it `SAME` against the file the hand-written generator committed.

### The `vocal` params

`"vocal"` is `sfxlib.Vocal`: a **voice**, written as data. A buzzing
glottal source runs through two resonances — the formants — that move,
with a murmur escaping through the nose beside them, and the phrase it
says is a list of **syllables**, each a run of **phones**. It is the
second published grammar, under the same bargain as a patch: the file
carries what made the sound, `sfxlib` reads it back, and soundlab renders
a vocal node as written — a cartridge's cue reopens and replays there,
and a re-render reproduces its audio. A phrase
edits there too: voice-wide knobs, a card per syllable, and a row of
knobs per phone over a drawing of the formant trajectory, under the same
bargain as a patch — carry the numbers back to the cartridge's
`tools/gen-sfx`, which stays the source of truth.

Reach for it when the sound is something a mouth would **say**: an eat
cue, an "ouch", a robot's acknowledgement, a card game's grumble. Every
sound a mouth makes is the same machine with the resonances in different
places at different times, and that is what a patch cannot spell — a
layer's filter is one band that sweeps straight, not two that travel a
path together under a buzz. Everything struck, swept or hissed is a
patch. A voice that needs other material under it is a **vocal node and
a patch node on one timeline**: a vocal is a whole sound and a sibling of
a patch in the `compose` block, not a layer inside one, because mixing
sounds is what the block is already for.

#### A phone is a position, not a letter

The unit of a phrase is not a letter. It is a **position** of the
articulators, held for `ms`: where the two formants sit (`f1Hz` rises as
the jaw opens, `f2Hz` says where the tongue is), how much sound leaves by
the mouth (`open`) and how much by the nose (`nasal`). A consonant like
M or N is the position where `open` is 0 and `nasal` is not; a vowel is
the one where `open` is 1.

What you **hear** between two phones is the glide from one position to
the next, and that is most of what makes the result sound like speech
rather than like a sequence of beeps. So every phone carries `glideMs`:
how long the articulators take to **arrive** at it, counted from the
moment the phone begins, so a position is reached partway into its own
time rather than at its boundary — which is how articulation works. A
tongue leaving the ridge takes about seventy milliseconds to get to a
vowel, and an M is audible rather than merely implied because the lips
start closing before the voice stops. `0` cuts straight to the position,
which sounds like a machine. On a syllable's **first** phone the glide is
the onset: the formants are already in place, there being nowhere else
for them to be, and `open` and `nasal` rise from nothing over it — a
murmur that swells in rather than switches on.

A syllable holds one pitch, `hz`, and the voice does two things to it on
its own: it lifts by `pitchLift` as the mouth opens (a real jaw does that
to a voice), and it sags across the syllable by `pitchSagPerS` (a real
voice runs out of breath — a phrase that holds its pitch sounds sung).
Syllables may overlap; fast speech does.

#### The document

Times are **milliseconds** throughout, like the node's own `atMs` and
unlike a patch's seconds: a phone is a few tens of them, and nobody types
`0.055`. Nothing in a vocal is a word, so unlike a patch there is no
vocabulary to refuse — an unreadable document is an error, and numbers
are not range-checked. A node whose `kind` is anything but `"vocal"` is
not a vocal, and `sfxlib` leaves it alone rather than refusing it.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `kind` | string | yes | `"vocal"` | — | What says the rest of the document is this grammar. |
| `name` | string | no | — | — | The sound's own name, written in so the document says what it is. Not checked against the node's `sfx`. |
| `sourceHz` | number | yes | > 0 | — | The glottal source's own rolloff: how bright the buzz is before any mouth shapes it. Around `1600` reads as a person; higher is a buzzer being filtered. |
| `f1Q` | number | yes | > 0 | — | The first formant's sharpness. Too low is a hum, too high whistles; `3.5` is a mouth. `0` is not a resonance: the formant passes the source through unchanged. |
| `f2Q` | number | yes | > 0 | — | The second formant's sharpness; `5` is a mouth. |
| `f2Gain` | number | yes | ≥ 0 | — | The second formant's level against the first's `1`. Below it, because that is a voice's real spectral tilt; present, because F2 is where a vowel's identity lives. `0.5` is a mouth; `0` is a voice with no vowels. |
| `pitchLift` | number | no | ≥ 0 | `0` | How far the pitch rises with an open jaw, as a fraction of `hz`. Small — `0.08` — and the reason a syllable sounds spoken rather than played. |
| `pitchSagPerS` | number | no | ≥ 0 | `0` | The exponential droop across a syllable, per second: the pitch at *t* is `hz · e^(−t · pitchSagPerS)` before the lift. `0.9` is speech. |
| `murmur` | number | no | ≥ 0 | `0` | The nasal hum's level under everything else. `0` is a voice with no nose. Above `1` reads as a mouth that is full. |
| `nasalHz` | number | when `murmur` > 0 | > 0 | `0` | The murmur's lowpass corner with the mouth shut. The nose is a dull tube; `650`. |
| `nasalDuckHz` | number | no | ≥ 0 | `0` | How far that corner drops as the mouth opens: the tube gets duller once it stops being the only exit. `300`. |
| `peak` | number | no | 0–1 | absent | The level the finished phrase is normalized to. Absent or `0` leaves it as it came out. The fleet ships `0.9`. |
| `syllables` | array of syllables | yes | 1 or more | — | The phrase. An empty one renders nothing. |

#### A syllable

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `atMs` | number | no | ≥ 0 | `0` | Where the syllable starts within the phrase. Syllables may overlap. |
| `hz` | number | yes | > 0 | — | The syllable's base pitch, before the voice's lift and sag. |
| `gain` | number | no | > 0 | `1` | Scales the whole syllable. `0` reads as `1`. |
| `phones` | array of phones | yes | 1 or more | — | The positions, in order. A syllable with no phones renders nothing. |

#### A phone

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `ms` | number | yes | > 0 | — | How long the position is held, **including** its glide. |
| `f1Hz` | number | yes | > 0 | — | The first formant: jaw height. Roughly `300` shut, `640` open. |
| `f2Hz` | number | yes | > 0 | — | The second formant: where the tongue is. About `1700` at the alveolar ridge (an N), `900`–`1000` for a back vowel, `1000` at the lips — which is what spells an M. |
| `open` | number | no | 0–1 | `0` | How much sound leaves by the mouth: `0` shut, `1` wide. |
| `nasal` | number | no | 0–1 | `0` | How much leaves by the nose. An M or N is `open` 0 with this not. |
| `glideMs` | number | no | ≥ 0 | `0` | How long the articulators take to arrive here, from the phone's start. `0` cuts. On a first phone, the onset. |

A syllable renders as one buzz through both formants — `F1 + f2Gain · F2`,
times `open` — plus the murmur, which is the same fundamental escaping
through the nose: a near-sine lowpassed at `nasalHz − nasalDuckHz · open`,
times `nasal`, ducked to 40% with the mouth wide, times `murmur`. The sum
sits under a 12 ms onset fade, a level of `0.6 + 0.4 · open` (a syllable
is never silent with its mouth shut), and a release over the last 60% of
its final phone. Syllables are summed at their `atMs`, and the phrase is
normalized to `peak`.

#### Worked example: Snake's `omnomnom`

`cartridges/snake/assets/omnomnom.arkadesound.wav`, whose recipe is the
only copy of the phrase: "om nom nom nom nom", five syllables 240 ms
apart, pitched 257.6 Hz down to 181.44, each a little quieter than the
last. Its `arkd` chunk holds this envelope, on one line in the canonical
compact form; the first two syllables are shown in full and the last
three, which differ from the second only in `atMs`, `hz` and `gain`, are
elided:

```json
{
  "arkade": 1,
  "kind": "sound",
  "name": "omnomnom",
  "generator": { "tool": "soundlab", "version": "1.2.0" },
  "recipe": {
    "compose": [
      {
        "sfx": "omnomnom",
        "params": {
          "kind": "vocal",
          "name": "omnomnom",
          "sourceHz": 1600, "f1Q": 3.5, "f2Q": 5, "f2Gain": 0.5,
          "pitchLift": 0.08, "pitchSagPerS": 0.9,
          "murmur": 1.02, "nasalHz": 650, "nasalDuckHz": 300,
          "peak": 0.9,
          "syllables": [
            { "hz": 257.6, "gain": 1.05, "phones": [
              { "ms": 8,   "f1Hz": 300, "f2Hz": 1700, "nasal": 1, "glideMs": 5.6 },
              { "ms": 80,  "f1Hz": 640, "f2Hz": 900,  "open": 1, "nasal": 1, "glideMs": 35 },
              { "ms": 30,  "f1Hz": 300, "f2Hz": 1000, "nasal": 1, "glideMs": 30 },
              { "ms": 140, "f1Hz": 300, "f2Hz": 1000, "nasal": 1 }
            ] },
            { "atMs": 240, "hz": 235.2, "gain": 0.95, "phones": [
              { "ms": 55,  "f1Hz": 300, "f2Hz": 1700, "nasal": 1, "glideMs": 38.5 },
              { "ms": 65,  "f1Hz": 640, "f2Hz": 900,  "open": 1, "nasal": 1, "glideMs": 75 },
              { "ms": 30,  "f1Hz": 300, "f2Hz": 1000, "nasal": 1, "glideMs": 30 },
              { "ms": 140, "f1Hz": 300, "f2Hz": 1000, "nasal": 1 }
            ] },
            { "atMs": 480, "hz": 215.04, "gain": 0.9,  "phones": [ "…as the second" ] },
            { "atMs": 720, "hz": 197.12, "gain": 0.85, "phones": [ "…as the second" ] },
            { "atMs": 960, "hz": 181.44, "gain": 0.8,  "phones": [ "…as the second" ] }
          ]
        }
      }
    ]
  },
  "render": { "sampleRate": 44100, "channels": 1, "bits": 16 }
}
```

Read a syllable's four phones as one bite. The first is the **/n/**:
mouth shut, tongue on the ridge (`f2Hz` 1700), only the murmur audible,
swelling in over 70% of the phone. The second is the **vowel**: the jaw
opens (`f1Hz` 640) and the tongue drops to 900 Hz over a 75 ms glide,
with the nose left open because the mouth is full. The third is the
**close**: over 30 ms the mouth shuts and F2 turns toward the lips (1000)
rather than back toward the ridge, which is what makes the syllable end
in an M and not an N. The fourth is the /m/ **held** — same position, no
glide — while the release fades it. The opening syllable is an "om", not
a "nom": an 8 ms /n/ and a wider, faster vowel, because "nom nom nom"
opened with a full /n/ reads as a stutter and the phrase everybody
actually says starts on the vowel. The contour `257.6, 235.2, 215.04,
197.12, 181.44` is `230, 210, 192, 176, 162` scaled by 1.12: not a
geometric fall, because a real phrase sags fastest at the front, and one
ratio audibly straightens it. `peak` is inside the document, so the
recipe reproduces the file: one node, no `mix`, and a soundlab
re-render writes the file back byte for byte: the envelope already names soundlab as its generator, so a
re-render has nothing left to change.

### The `speech` params

`"speech"` is `tools/speech`: a line of **text**, read aloud by
[Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M), an Apache-2.0
neural voice that runs offline on a CPU. Where a vocal builds a mouth
out of numbers, a speech node names a voice and gives it words, so it is
the grammar for sounds that must be understood rather than suggested: a
score being counted, a menu read aloud, a narrator. It is published
under the same bargain as the others: the file carries what made the
sound, anything linking `tools/speech` reads it back, and soundlab
reopens and re-renders it in its SPEECH panel.

Reach for a vocal when the sound is a gesture (an eat cue, a grunt) and
for speech when it is a sentence. Like a vocal, a speech node is a whole
sound and a sibling of a patch in the `compose` block.

#### Rendering is a build step

The cabinet has no speech engine and a cartridge cannot run one, so a
speech node is rendered at **build time** and shipped as its samples like
every other sound. The renderer runs under Node: `npm ci` in
`tools/speech` once, after which the first render fetches the model
(about 330 MB) from a pinned revision, checks every file's SHA-256, and
caches it under `$ARKADE_SPEECH_CACHE` (default `~/.cache/arkade-speech`).
The model speaks at 24 kHz; `tools/speech` resamples to the rate asked
for.

That makes speech the one grammar whose re-render is not pure Go, and
the difference is stated here so nobody discovers it from a diff: on
one machine a render is byte-identical every time, but a different CPU
architecture may round the model's arithmetic differently and move the
low bits. Compare a speech asset re-rendered on another machine by ear
or with `tools/assetdiff -tolerance`, not by hash; `SAME` holds only on
the machine that made it. Tolerance mode passes a pair as `NEAR` when
its largest sample difference and its signal-to-noise ratio stay within
bounds measured to admit arithmetic noise and refuse a changed word or
pace (the tool's doc has the numbers). `tools/speech/rerender` re-speaks
a speech asset from its recipe, ready for that comparison.

#### The document

A node whose `kind` is anything but `"speech"` is not speech, and
`tools/speech` leaves it alone rather than refusing it.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `kind` | string | yes | `"speech"` | — | What says the rest of the document is this grammar. |
| `name` | string | no | — | — | The sound's own name, written in so the document says what it is. |
| `text` | string | yes | — | — | What is said. Punctuation is read as prosody: a trailing comma holds the voice up as though more is coming, a full stop lets it fall. Empty text renders nothing. |
| `voice` | string | no | a Kokoro voice id | `"bm_george"` | Who says it. The id's first letter is the accent (`b` British, `a` American) and its second the voice's sex; `bm_george` is a British man. An id the model does not ship fails the render. |
| `blendVoice` | string | no | a Kokoro voice id | — | A second voice mixed into `voice` by interpolating the two voices' style vectors, which makes a speaker the model does not ship: `bm_george` blended toward `bm_fable` is neither man. The words are still read with `voice`'s accent. Absent is no blend, and the line renders exactly as it would without the field. An id the model does not ship fails the render. |
| `blendWeight` | number | no | 0–1 | `0` | How far toward `blendVoice` the mix sits: `0` is `voice` alone, sample for sample, `1` is `blendVoice`'s style read in `voice`'s accent. Ignored without a `blendVoice`. Values outside the range are clamped. |
| `speed` | number | no | 0.5–2 | `1` | The pace, as a multiple of the voice's own. A clipped game call wants more than `1`: `1.3` counts a hand of cards at about the pace a person does. Values outside the range are clamped. |
| `pitchSemitones` | number | no | −12–12 | `0` | Moves the voice up or down without changing the line's length: a time-stretch by the pitch ratio, then a resample back. Within about ±4 the speaker stays recognisable; further out the voice's own resonances travel with it, which is the chipmunk at the top and the giant at the bottom, sometimes exactly what a game wants. Values outside the range are clamped. |
| `highpassHz` | number | no | ≥ 0 | `0` | A high-pass corner, applied twice for a 12 dB/octave slope. A man's voice keeps much of its energy at its fundamental, around 100 Hz, below what the cabinet's speakers reproduce; left in, that energy counts toward `peak` and the part a player can hear comes out quieter. `150` is the fleet's setting. `0` is off. |
| `peak` | number | no | 0–1 | `0.9` | The level the finished line is normalized to. A generator that sets loudness some other way, by RMS say, writes the peak its own level landed on, unrounded, so the document still re-renders to its samples. |

After speaking, the line is pitch-shifted and high-passed as the fields
say, then trimmed to 20 ms of quiet either side, faded over 5 ms at both
ends so it cannot click, and normalized to `peak`. The trim and the fades
are fixed rather than fields, so two tools rendering the same document
cannot disagree about them.

```json
{ "sfx": "fifteen-two",
  "params": { "kind": "speech", "name": "fifteen-two", "text": "fifteen two",
              "voice": "bm_george", "speed": 1.3, "highpassHz": 150, "peak": 0.9 } }
```

### The `song` params

`"song"` is `musiclib.Song`: a **music bed**, written as note data. A
tempo, a loop length in beats, and parallel **tracks**, each one
chip-style **instrument** — a pulse, triangle or noise voice under an
envelope — playing a list of **notes** at MIDI pitches on a beat grid
that is not a grid at all, since a note starts and lasts however many
fractional beats it is written to. An optional post-mix **echo** gives a
bare chip mix its room. It is the third published grammar and the first
that is not `sfxlib`'s: `musiclib` reads it back, and everything a bed's
generator expressed in Go — every voice in a cartridge's beds,
their swing, their detune, their echo — this document expresses as
data, so a bed reopens where it is edited rather than only where it was
written.

The bargain is the one a patch struck, one level up. A cartridge's
`tools/gen-music` has always been the only thing that could remake its
beds; the recipe's honest form was the bed's **name**. With a song node
the recipe carries the song, `assetdiff` proves the carried song renders
the committed audio sample for sample, and a bed nobody can turn a knob
on becomes one an editor can open.

#### A bed is a `loop` node

A song renders as one seamless cycle — that is the whole of what
`musiclib` guarantees, and it is what `cartlib.PlaySoundForever` counts
on — so a song node names its source under `loop`, not `sfx`. A bed
shipped as itself is **one loop node and no `mix`**: the render is the
song's own output, normalized to the `peak` the document pins, and
nothing else touches it. That is what makes the conversion provable:
a lone node through the mixer would pick up a junction crossfade and a
second normalization, and the file would stop being the loop the
generator produced.

Everything above a bed is the `compose` block's job, not this grammar's.
"Play the verse twice, then the chorus" is two song nodes with `repeat`
and `atMs` under a `mix`; a sting over a bed is a patch node beside a
song node. `musiclib.Composition` — the sequencer soundlab's COMPOSE tab
drives — is exactly that block, rendered; it needs no `params.kind` of
its own because it carries no parameters a node does not already spell.
A song is a **texture**; the timeline it sits on is the recipe's.

#### The document

A song node's `params` is the whole document below. Note positions and
lengths are in **beats** — the unit a composer thinks in, and the one
that survives a tempo change — envelope stages in **milliseconds**,
corners and vibrato rates in **hertz**. Numbers are not range-checked;
what the renderer makes of an odd one is stated field by field, and
`assetfile` never looks inside `params`. The one word, `wave`, is: an
unknown wave is refused rather than read as `pulse`. A node whose `kind`
is anything but `"song"` is not a song, and `musiclib` leaves it alone.

**Every zero is a default.** Every optional number below reads `0` as
the default the column states — never as a value of its own — which is
what lets the writer leave every zero field out and still round-trip
exactly: absent and `0` land in the same place, and the renderer sees the
same song either way. Numbers are written in their shortest
round-tripping decimal form, so a document reads back to the same
`float64` bit for bit, and a swing upbeat written `1.67` in Go is `1.67`
in the file.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `kind` | string | yes | `"song"` | — | What says the rest of the document is this grammar. |
| `name` | string | no | — | — | The bed's own name, written in so the document says what it is. Not checked against the node's `loop`. |
| `bpm` | number | yes | > 0 | — | The tempo. Every beat below is `60 / bpm` seconds; a note written in beats keeps its place when this changes. |
| `beats` | number | no | > 0 | auto | The loop length. Absent or `0` is the last note-end across every track, rounded **up** to a whole beat, so a hand-written bed loops on the bar. Written by every bed in the fleet, because a loop length that depends on the last note is one that moves when a note does. |
| `peak` | number | yes | 0–1 | — | The level the mix is normalized to. `musiclib` writes it **always**, `0.9` unless the song says otherwise, so a library default moving later cannot change what a committed bed renders to. Absent or `0` reads as `0.9`; there is no un-normalized render through this grammar. |
| `tracks` | array of tracks | yes | — | — | The parallel channels, summed, in the order written. An empty list renders silence of the loop's length — or nothing at all when `beats` is auto, there being no last note to measure to. |
| `echo` | object | no | — | none | The post-mix delay send — see [`echo`](#echo). Absent is off. |

#### A track

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `name` | string | no | — | — | A label for the reader — `"lead"`, `"bass"`, `"hats"`. Nothing branches on it. |
| `instrument` | object | yes | — | — | The track's one voice — see [An instrument](#an-instrument). |
| `gain` | number | no | > 0 | `1` | The track's mix level. `0` reads as `1`. |
| `notes` | array of notes | yes | — | — | The events, in the order written; they simply sum, and may overlap. For a pitched voice the order is not audible; for `noise` it is the seed of every note that does not carry its own — see [`seed`](#a-note). |

#### An instrument

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `wave` | string | no | `pulse`, `triangle`, `noise` | `pulse` | The oscillator. `pulse` is the rectangular chip lead; `triangle` the soft bass and flute; `noise` a deterministic LFSR for percussion, re-seeded per note so a render is the same on every machine. Any other word is refused. |
| `duty` | number | no | (0, 1) | `0.5` | `pulse` only: the fraction of a cycle spent high. `0.125` is thin and nasal, `0.25` the bright NES lead, `0.5` a hollow square. `0`, or anything outside the open interval, reads as `0.5`. |
| `attackMs` | number | no | > 0 | `2` | The rise from silence to full. `0` reads as `2`. |
| `decayMs` | number | no | ≥ 0 | `0` | The fall from full to `sustainLevel`. `0` is no decay stage: the note holds at `sustainLevel` from the end of its attack. |
| `sustainLevel` | number | no | (0, 1] | `1` | The level held after the decay. `0` reads as `1`, so a plucky voice writes a small positive value — the fleet's kits use `0.001` for "decay to silence". |
| `releaseMs` | number | no | > 0 | `10` | The fade to silence at the note's end. `0` reads as `10`. The release always finishes **inside** the note's own duration, gating early if it must; with the attack it is squeezed proportionally into a note too short to hold both. This is one of the three loop-seam mechanisms. |
| `noiseTone` | number | no | −1–1 | `0` | `noise` only: a one-pole colour. Positive blends toward high-passed (a hat), negative toward low-passed (a thump), `0` is white. The note's `midi` sets the LFSR clock beside it — 60 is the full sample rate, each octave down halves it and darkens. |
| `gain` | number | no | > 0 | `1` | Scales the voice, ahead of the track's `gain` and the note's `vel`. `0` reads as `1`. |
| `vibratoHz` | number | no | > 0 | none | With `vibratoSemis`, a sinusoidal pitch wobble that restarts at each note. Both must be positive for either to mean anything. |
| `vibratoSemis` | number | no | > 0 | none | The wobble's depth, in semitones either side of the note. |
| `lowpassHz` | number | no | > 0 | none | A one-pole lowpass on the voice, its state reset at each note. `0` is off. Most of the fleet's leads carry one: a raw pulse measures brighter than a chip. |
| `detuneCents` | number | no | > 0 | none | `pulse` and `triangle`: spreads the voice into three equal-weighted oscillators — at pitch, and this many cents above and below — the chorus a single chip voice lacks. `0` is one oscillator. Phases restart at each note. |

An instrument is evaluated per note: phase starts at zero at the note's
onset — the chip-style retrigger, and the third loop-seam mechanism, since
a note beginning at beat 0 rises from silence rather than jumping — and
the envelope is `attack`, `decay` to `sustainLevel`, hold, then `release`
squeezed to close at the note's end. A note's amplitude is
`vel · (instrument.gain · track.gain)`, with exactly that association —
the shape the renderer computes, stated for the reason a patch states
its `gain · source · env`.

A `noise` voice starts its LFSR from a 16-bit state chosen per note. A
note that carries a `seed` starts from that state; one that does not
starts from a state derived from the track's and the note's **position**
in the document. Either way the same hat on the same beat renders
identically on every machine. The difference is under editing: a
position-derived seed re-rolls whenever the note or its track moves to
another index, so reordering is a change to a noise track that has no
seeds written, and not one to a pitched track or to a noise track whose
notes carry their own.

#### A note

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `midi` | integer | yes | — | — | The pitch as a MIDI note number: 60 is C4, 69 is A4 at 440 Hz, and every number in between and beyond is equal-tempered from there. For a `noise` instrument it tunes the LFSR clock instead — see `noiseTone`. Written always; `0` is a real pitch. |
| `start` | number | no | ≥ 0 | `0` | Where the note begins, in beats from the loop's start. Fractional throughout: a swung upbeat is `1.67`, a voice dragged off the grid is `4.02`. |
| `dur` | number | yes | > 0 | — | How long it sounds, in beats. A note that rounds to fewer than one sample renders nothing. |
| `vel` | number | no | (0, 1] | `1` | The velocity. `0` reads as `1`. |
| `seed` | integer | no | 1–65535 | by position | `noise` only: the 16-bit LFSR state the hit starts from, used exactly as written. Absent or `0` derives the state from the note's **position** — its track's index and its own index in `notes` — which is the rule every bed was rendered with before this field existed, so a document without seeds renders as it always has. Outside 0–65535 the document is refused, as an unknown `wave` is: the renderer cannot start from a state it cannot hold. Ignored by `pulse` and `triangle`. |

A note whose tail crosses the loop's end wraps to the loop's start —
writes are modulo the loop length, the second loop-seam mechanism — so a
held chord in the last bar rings over the join rather than being cut at
it.

`seed` exists so that a drum part can be edited. With seeds derived from
position, deleting one hat re-rolls every hat after it and moving a track
re-rolls every hit on it — a fine rule for a generator that writes a bed
once, and a trap for an editor. Writing each note's derived state into it
as `seed` changes the document and not one sample of its render
(`musiclib.Note.NoiseSeed` is the number, and the fleet's beds are the
proof: pinned, every one saves the PCM it committed), and from then on
the seed travels with the note through any edit. Absent means "by
position" rather than a fixed state because the fleet's beds were
rendered by position: any other default would move every drum in every
committed bed. A bed nobody edits never needs the field; soundlab writes
it the first time it touches a noise track, and a `seed` typed or rolled
by hand is simply a different hit.

#### `echo`

The dry mix, delayed by `delayBeats` with geometric `feedback` decay,
added at `mix` level: `wet[i] = Σ feedback^(t−1) · dry[i − t · delay]`
over every tap `t ≥ 1` whose gain is still at least `0.002`. Tap indices
wrap modulo the loop length, so an echo tail that crosses the loop's end
folds into its start and the seamless-loop guarantee holds exactly.

| Field | Type | Required | Range | Default | Meaning |
| --- | --- | --- | --- | --- | --- |
| `delayBeats` | number | yes | > 0 | — | The tap spacing, in beats: `0.5` is an eighth-note slapback in 4/4. `≤ 0` disables the echo. |
| `feedback` | number | no | 0–0.95 | `0` | How much each repeat keeps of the last. Clamped to `0.95`; `0` is a single repeat. |
| `mix` | number | yes | (0, 1] | — | The wet level relative to dry. `≤ 0` disables the echo. |

The whole block is left out when the song has no echo, and written with
every non-zero field when it does — including the pathological
`{"delayBeats": 0.5}` with no `mix`, which is off in the renderer and
still round-trips as written.

#### Worked example

A two-bar sketch in the grammar — the document `musiclib`'s own tests
pin the wire shape against, written out with whitespace. A lead with the
fleet's bright-NES voice plays a swung pair, a hat ticks once, and the
room is a half-beat slapback:

```json
{
  "kind": "song",
  "name": "bed",
  "bpm": 120,
  "beats": 2,
  "peak": 0.9,
  "tracks": [
    {
      "name": "lead",
      "instrument": { "wave": "pulse", "duty": 0.25, "attackMs": 4, "decayMs": 60,
                      "sustainLevel": 0.25, "releaseMs": 20, "gain": 0.9,
                      "vibratoHz": 5, "vibratoSemis": 0.18, "lowpassHz": 3200, "detuneCents": 3 },
      "gain": 0.8,
      "notes": [
        { "midi": 76, "dur": 1, "vel": 0.9 },
        { "midi": 72, "start": 1.67, "dur": 0.33 }
      ]
    },
    { "name": "hat", "instrument": { "wave": "noise", "noiseTone": -0.35 },
      "notes": [ { "midi": 80, "start": 0.5, "dur": 0.25, "vel": 0.4 } ] }
  ],
  "echo": { "delayBeats": 0.5, "feedback": 0.3, "mix": 0.12 }
}
```

Read what is missing as much as what is there. The first note has no
`start`, because it begins at 0; the second has no `vel`, because it is
played at full; the hat's instrument names only its wave and its colour,
because a 2 ms attack, a 10 ms release and a sustain of 1 are what an
instrument is until told otherwise; the hat has no `seed`, so it is
seeded by its position, as a hand-written bed is. Wrapped in a recipe, the bed is
`{"compose": [{"loop": "bed", "params": { …this… }}]}` and no `mix`:
one node, played as itself, at the `peak` the document carries.

A full-length bed is the same document a few hundred notes longer — a
long arrangement can run to 421 notes across six tracks, and about 20 KB
of params — and it is what `tools/assetdiff` proves: every shipped
bed, written back through this grammar,
renders the committed audio **byte for byte**, which is the bar a
conversion in this repository has to clear.

## Structural errors versus repairs

Unlike a sprite's animation hold times, **nothing in a sound envelope is
repaired**. Every field above is either within range or the file is
refused.

The asymmetry is the format's own logic. A silly hold time still leaves a
cel that draws; a `gainDb` of 200 or a second node with no mixer leaves a
recipe that renders something other than the audio sitting beside it,
which makes the record meaningless rather than merely untidy. And there
is no legacy format to be lenient toward: nothing has ever written one of
these files except the tools in this repository.

**Errors:** an unknown `arkade` format version; `kind` not `"sound"` (or
not matching the file's own name); a missing `render`; a non-positive
`sampleRate`; `channels` other than 1 or 2; `bits` other than 8/16/24/32;
a `recipe` that is not readable JSON; a recipe with no `compose` nodes or
more than 128; a node naming both `sfx` and `loop`, or neither; a name
over 64 characters or holding a control character; `repeat`, `atMs` or
`gainDb` out of range; `peak`, `joinMs` or `seconds` out of range; two or
more nodes with no `mix` block; a single node placed, gained or repeated
with no `mix` block. And, from the container rather than the JSON: a
`fmt `/`data` chunk that disagrees with `render`.

A plain `.wav` — no `.arkadesound` in the name — is subject to none of
this. It is an ordinary sound asset, exactly as it has always been, and
`rcartpack` packs it untouched.

## Worked example

`assetfile/testdata/tone.arkadesound.wav` is a six-frame
22050 Hz mono file (`assetfile`'s tests write it and read it back byte
for byte). Its `arkd` chunk holds this envelope:

```json
{
  "arkade": 1,
  "kind": "sound",
  "name": "tone",
  "description": "Fixture tone: a single short blip.",
  "generator": { "tool": "assetfile-fixtures", "version": "1.0.0" },
  "recipe": {
    "compose": [
      { "sfx": "blip", "params": { "hz": 880, "ms": 60 } }
    ]
  },
  "render": { "sampleRate": 22050, "channels": 1, "bits": 16 }
}
```

One node, no `mix`: the render is that generator's own output. The
`params` are `assetfile-fixtures`'s document and mean nothing to anyone
else, which is exactly the point — the fixture is a shape, not a sound
anybody can re-render.

## Loading a sound from a cartridge

Unchanged from a plain `.wav`, and deliberately so. The path is
bundle-relative, the same path space `Setup.LoadSheet` uses:

```go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

var (
	soundTheme uint32
	soundLost  uint32
)

// loadSounds runs ONCE, from Game.Init, with its Setup. A bare .wasm
// build with no bundled assets gets handle 0 back for each, and
// PlaySound(0) is already a safe no-op — so nothing has to branch on a
// missing bundle.
func loadSounds(s *cartlib.Setup) {
	soundTheme = s.LoadSound("assets/theme.wav")           // a plain WAV
	soundLost = s.LoadSound("assets/lost.arkadesound.wav") // an asset file
}

func onLost() {
	cartlib.PlaySound(soundLost, 0) // 0 = play once
}

func main() {} // a cartridge's real main calls cartlib.Run
```

The two lines are the same line. `host_load_sound` takes a path and
returns a handle; the envelope is not read, by the import or by the host.
A cartridge cannot tell an asset file from a plain WAV and has no reason
to want to.

## Generating one

`sfxlib.WriteAssetFile` is the one door — the same one soundlab's save
goes through. A cartridge's `tools/gen-sfx` converts by swapping it in
for `sfxlib.WriteFile` and giving each cue a name:

```go
package main

import (
	"log"
	"path/filepath"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// generator pins THIS program in every envelope it writes. The version is
// the generator's own: bump it when a voice changes, because that is
// exactly when a file written by an older version stops matching what
// re-running this would produce.
var generator = assetfile.Generator{Tool: "widget/gen-sfx", Version: "1.0.0"}

// write emits one cue as an asset file. The file name and the recipe
// node's name both come from `name`, so the two cannot drift apart.
func write(dir, name string, samples []float64) {
	path := filepath.Join(dir, sfxlib.SoundFileName(name)) // "lost" -> "lost.arkadesound.wav"
	err := sfxlib.WriteAssetFile(path, samples, sfxlib.SampleRate44100, assetfile.SoundMeta{
		Name:      name,
		Generator: generator,
		Recipe:    &assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: name}}},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func main() {} // the real main synthesizes, asserts the spectrum, then calls write
```

The audio is byte for byte what `sfxlib.WriteFile` would have written —
`WriteAssetFile` calls the same encoder — so converting a generator
changes the file's name and adds metadata, and does not move a single
sample. It does **not** relieve a generator of the fleet's other rule:
assert the spectrum with `sfxlib.AssertSpectrum` before writing, or a
sound that is inaudible on the cabinet's small speakers ships without
anyone hearing it fail.

### Starting from a template

Most game cues are one of a handful of shapes, and a survey of the
fleet's own generators found its authors writing the same little helper
to build them over and over — four cartridges with their own `tapVoice`,
five with their own `fanfareNote`, three shipping a refusal buzz on the
identical two frequencies. `sfxlib` carries those shapes so a cue can
start from one:

| Function | The sound |
| --- | --- |
| `Tap(hz, decay, durationS)` | A struck, pitched object: a decaying body under a few milliseconds of noise. Contact rather than a beep. |
| `Impact(bandHz, q, noiseDecay, toms, durationS)` | Something breaking: a band-passed noise burst with pitched `Tom` bodies under it. |
| `Sweep(fromHz, toHz, decay, durationS)` | One tone travelling. Upward reads as gained, downward as lost. |
| `Bonk(lowHz, highHz, wave)` | The refusal — two pitches close enough to beat, cut off without a ring. `Bonk(BonkLowHz, BonkHighHz, WaveSquare)` is the fleet's own. |
| `Scrape(hz, q, durationS)` | No pitch at all: noise through a resonance, swelling and fading. |
| `Motif(notes, decay)` + `Run(hz, beat, lastScale, wave)` | The fanfare, the jingle, the two-note chime. `Run` lays a list of pitches out back to back and gives the arrival `MotifChord` and a longer hold. |

Every one returns a plain `sfxlib.Patch`. Nothing is sealed: change a
field, add a layer, drop a layer, or place two of them on a timeline as
separate `compose` nodes. A template is where a cue **starts**, and a
cartridge that ends up somewhere else has used it correctly — which is
also why none of them take an options struct, since the options are the
`Patch`'s own fields and those are already in hand.

```go fragment
lost := sfxlib.Sweep(420, 180, 9, 0.35)
lost.Layers[0].Env = sfxlib.Env{Kind: sfxlib.EnvFadeOut, Curve: 2}
lost.Layers[0].Vibrato = &sfxlib.Vibrato{Hz: 7, DepthHz: 6} // the fleet's sad trombone
```

There is deliberately no template for an engine drone or a character's
voice. A drone's length is derived from whole cycles of its own carrier
so its loop seam is silent, and a template invites somebody to change one
number and break that without hearing it; a character voice is a
hand-tuned stack that reduces to no small set of knobs, and exposing ten
parameters would be a worse interface than the `Patch` literal it
replaced.

## What `rcartpack` enforces

Every `.arkadesound.wav` in a bundle is validated at pack time: the
envelope's format version, its `kind` against the file's own name, the
`render` block, the recipe's structure, and the `fmt `/`data` chunks
against `render`. A broken one **fails the build**, naming the file.

At **mount** the same check is a warning: the cabinet starts the game and
that one cue is silent, rather than a player standing in front of a
machine that refuses to play. One check, two severities, decided by the
caller — the same split
[sprite sheets](reference-sprite.md#what-the-hosts-enforce) live under.

A sound needs no manifest entry: the cartridge asks for it by path (see
[Manifests & packaging](packaging.md)).
