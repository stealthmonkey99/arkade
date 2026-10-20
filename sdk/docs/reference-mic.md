# Microphone and sound model reference

This documents two halves of one idea. The `mic` kind of the asset
envelope — a `.arkademic.json` file — is a **sound model**: a few
recordings of each word or noise a cartridge wants to recognise, as data
rather than as code. The **microphone keys** are how a cartridge reads
it: a matched sound arrives as an ordinary keypress, in the same batch as
the buttons. Read [Asset file reference](reference-assets.md) first for
the envelope fields every kind shares (`arkade`, `kind`, `name`,
`generator`, ...); for a JSON kind, the document **is** the file.

periphlab's Listen room is the easy way to produce one — it records clips
on the cabinet itself, trains a model from them and writes it into the
cartridge's `assets/` — and `micmodel` is the same trainer on the command
line. This page is for reading or checking the JSON, and for the input
half, which no lab can write for you.

The microphone is **cabinet-only** and **off unless an operator asked for
it** (`--mic`). A browser tab has none, and neither does a cabinet whose
owner never passed the flag. Everything below has to degrade to nothing.

## What a cartridge receives

Eleven key ordinals, appended to the ordinary key table
([ABI reference](abi.md)), delivered as **`KindKeypress`** records:

| Ordinal | Key | `cartlib` | Fires on |
| --- | --- | --- | --- |
| 28 | `mic.whistle` | `KeyMicWhistle` | a sustained, in-band, tonal whistle |
| 29 | `mic.clap` | `KeyMicClap` | a broadband transient |
| 30 | `mic.blow` | `KeyMicBlow` | sustained low-frequency breath |
| 31–38 | `mic.cue0`…`mic.cue7` | `KeyMicCue0`…`KeyMicCue7` | a class of **this cartridge's own** sound model |

**Clap and blow ship disabled.** On the cabinet the clap detector fired
on joystick clicks and on the game's own sound effects, several times a
second; the DSP passes its synthetic tests but the thresholds are
guesses that need a labelled recording of a real room. They are in the
table because the ordinals are spent, not because a cartridge will see
one today. Build on the whistle and on cues.

The record itself:

| Field | Value | Why |
| --- | --- | --- |
| `Kind` | `KindKeypress` | Edge-triggered, one event per sound. A two-second whistle is one record, not a hundred. |
| `Key` | one of the eleven above | |
| `Player` | always `1` | A mono microphone has no seat, and `1` is the zero-value slot, not a claim. |
| `Source` | `SourceKeyboard` | The honest answer for a source with no controller and no port; it is also what makes a mic event pass the host's player-strategy gate. |
| `Physical` | `SeatNone` (`0xFF`) | No panel. |
| `Index` | `IndexNone` (`0xFF`) | No controller slot. |
| `Flags` | `FlagVirtual` set | Follows from the keyboard source. A cartridge that drops every virtual record would drop every whistle with them. |

## The rules

**An alternative, never a replacement.** A quiet room, a sore throat, a
player who simply cannot whistle, and a cabinet whose operator never
passed `--mic` must all still be able to play the whole game. Every
action a sound reaches must have a button that reaches it too, and the
scores are meant to be comparable between cabinets.

**A spoken action still costs what the button costs.** If tossing a
treat spends a treat, so does saying "treat". A cue that skips the price
is not an accessibility path, it is a cheat code that only some cabinets
have.

**Route it to whoever holds the turn.** The record is seatless, so a
cartridge's own turn filter — the one that drops a record whose slot is
not the active player's — would pass every mic event to P1 and silently
drop it for P2. Bypass the filter for mic keys and hand the event to the
player whose turn it is. That is the whole bug: it works in 1P, it works
on P1's turn, and it dies on the second player's turn at the cabinet long
after it shipped.

**No audio crosses the boundary.** The host matches and discards the
samples; a cartridge is told "someone whistled" and nothing else. That is
a privacy property, but it is first a correctness one: a cartridge must
stay a pure function of `dtMicros`, `RandomU64` and input records or
golden frames and replay validation both stop working. A quantised event
replays; a waveform does not. Raw audio leaves the cabinet's process in
exactly one place — a token-gated `/api/mic/stream` the peripherals lab
reads, never a cartridge.

**The capability gates everything.** A cartridge sees a mic record only
if its bundle manifest declares `"mic"`, and only while it is actually
being played — never in attract, never in the shell. Every cartridge
built before the cabinet had a microphone keeps byte-identical behaviour,
which is what makes the feature additive rather than a fleet-wide change.

**Sound does not wake the cabinet.** Standby is the motion sensor's job:
it fires for someone who walks up without speaking and stays quiet for a
television in the next room. A whistle into a dark cabinet does nothing.

```go
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

// The recall whistle and the spoken cue that does the same thing, beside
// the button that has always done it.
type Game struct {
	activeSlot cartlib.Slot
	treats     int
}

func micKey(k uint8) bool {
	return k == cartlib.KeyMicWhistle || (k >= cartlib.KeyMicCue0 && k <= cartlib.KeyMicCue7)
}

func (g *Game) HandleInput(records []cartlib.InputRecord) {
	for _, r := range records {
		if !r.IsPress() {
			continue
		}
		// A mic record is seatless, so it skips the turn filter every
		// button press goes through — and only that filter.
		if !micKey(r.Key) && r.Player != g.activeSlot {
			continue
		}
		switch r.Key {
		case cartlib.KeyJoyD, cartlib.KeyMicWhistle, cartlib.KeyMicCue0:
			g.recall()
		case cartlib.KeyJoyC, cartlib.KeyMicCue1:
			// Spoken or pressed, it spends a treat.
			g.tossTreat()
		}
	}
}

func (g *Game) recall() {}

func (g *Game) tossTreat() {
	if g.treats > 0 {
		g.treats--
	}
}

func main() {} // a cartridge's real main calls cartlib.Run
```

## The sound model file

```json file=assets/commands.arkademic.json
{
  "arkade": 1,
  "kind": "mic",
  "name": "commands",
  "description": "Come, sit, treat.",
  "generator": { "tool": "micmodel", "version": "1.0.0" },
  "features": { "sampleRateHz": 14700, "frameLen": 512, "hopLen": 147,
                "melBands": 26, "coeffs": 13, "minHz": 100, "maxHz": 6000 },
  "segment":  { "minMs": 150, "maxMs": 1200, "hangoverMs": 200,
                "snr": 4.0, "minRms": 0.02 },
  "classes": [
    { "name": "come", "cue": 0, "threshold": 0.42,
      "templates": [ [[/* 13 floats per frame */], [...]], [...] ] }
  ]
}
```

**Nothing in it is hand-written.** The coefficients are measurements of
real recordings, and a threshold is a distance the trainer measured
between them. This section is for checking a file, or for understanding
what a number in a training report means — not for typing one.

### `features`

The front end the host must run to compare a captured sound against this
model's templates. It is part of the **file** rather than the host's own
choice because a template is only meaningful under the front end that
produced it: a model trained at 26 bands and matched at 40 compares two
different things and matches nothing.

| Field | Type | Meaning |
| --- | --- | --- |
| `sampleRateHz` | integer, must be `14700` | The analysis rate the templates were measured at: the cabinet decimates its 44.1 kHz capture by three and runs everything at the result. The validator refuses any other value, so a file that packs also mounts. |
| `frameLen` | power of two, `128`–`2048` | Samples per analysis frame. 512 at 14.7 kHz is 34.8 ms. |
| `hopLen` | integer `1`–`frameLen` | Samples between frames. 147 is 10 ms. |
| `melBands` | integer `1`–`64` | Mel filter bank size. |
| `coeffs` | integer `1`–`melBands`, at most `20` | Cepstral coefficients kept per frame, and therefore the width of every template frame. Speech work uses 12–13; past about 20 they describe the microphone rather than the sound. |
| `minHz`, `maxHz` | numbers, `0 <= minHz < maxHz <= sampleRateHz/2` | The band the mel bank spans. |

### `segment`

The endpointer: when the host decides a sound has started and stopped.
Energy above the gate opens a segment, a hangover of quiet closes it, and
what is between is what gets matched. Three hops before the gate opened
are kept as well, so an onset's first consonant is not clipped off.

| Field | Type | Meaning |
| --- | --- | --- |
| `minMs` | integer > 0 | Shorter than this and the segment is discarded, not matched. |
| `maxMs` | integer > `minMs` | A segment that reaches this length is closed where it stands. |
| `hangoverMs` | integer >= 0 | Quiet needed to close a segment. Its silence is trimmed off the tail. |
| `snr` | number >= 1 | How far above the tracked noise floor a frame has to sit to count as sound. |
| `minRms` | number `0`–`1` | The absolute floor, full-scale, under the adaptive one. |

### `classes`

One entry per thing the model can recognise. At most **8**, because a
class's `cue` indexes the eight cue keys and a ninth would have nowhere
to fire.

| Field | Type | Meaning |
| --- | --- | --- |
| `name` | string | For whoever reads the file and the training report. Lowercase words joined by `.`, `_` or `-`; unique within the file. Never reaches the cartridge. |
| `cue` | integer `0`–`7` | **Which key this class fires**: `cue: 3` is `mic.cue3`. Unique within the file. |
| `threshold` | number > 0 | The DTW distance under which a captured segment counts as this class. Measured by the trainer; see below. |
| `templates` | `[template][frame][coefficient]` | The recordings themselves, as cepstral-mean-subtracted MFCC frames. At most **16** templates, each **2–400** frames, each frame exactly `coeffs` long. |

**A cue means whatever this file says it means.** The three gestures are
the cabinet's own vocabulary and mean the same thing to every game; cue 0
is "come" in one cartridge and a kazoo note in the next, which is why the
keys are numbered rather than named.

### Caps

| Cap | Value | Bounds |
| --- | --- | --- |
| classes per model | 8 | one per cue key |
| templates per class | 16 | the work of matching one captured sound |
| frames per template | 400 | about four seconds at a 10 ms hop — longer than any segment a sane `segment` block admits |
| coefficients per frame | 20 | past this they describe the microphone |
| mel bands | 64 | the coefficients are derived from these and can never exceed them |
| models per cartridge | 4 | every installed model is matched against every captured sound; a cartridge that wants more than four wants more classes in one model |

## How the host runs it

Per captured sound, once the segmenter has closed a segment:

1. **Segment.** The energy gate above opens and closes on the *cancelled*
   capture; a segment made mostly of frames the canceller judged to be
   the cabinet's own playback is dropped rather than matched.
2. **Featurise.** MFCC frames under the model's own `features`, then
   cepstral mean subtraction over the segment — which is what removes the
   microphone's colour and the room's gain, since both are multiplicative
   in the spectrum and so additive in the cepstrum.
3. **Match.** Dynamic time warping against every template of every class,
   with a Sakoe–Chiba band of 20 % of the longer sequence, so the same
   word said a fifth faster or slower still aligns. A class's distance is
   its nearest template's.
4. **Decide.** Within a model, the **lowest distance that is under its
   own class's threshold** wins; a segment under nobody's threshold fires
   nothing at all. With several models installed, the first one (in
   manifest order) to fire wins.
5. **Deliver.** The winning class's `cue` field picks the key, and it
   arrives as the keypress described above.

Everything in that list is a pure function of the samples, which is what
lets a clip recorded on the cabinet be scored identically at a desk —
and is why a `micmodel` score is a promise about the cabinet rather than
an estimate of one.

### Producing one

Record clips, lay them out by class, train:

```text
clips/
  come/    come-01.wav come-02.wav …      # one utterance per file
  treat/   treat-01.wav …
  _noise/  room.wav game-audio.wav …      # what it must NOT fire on
```

Any underscore-prefixed directory is negatives. WAVs are 16-bit mono at
14700 Hz (the analysis rate) or 44100 Hz (the capture rate, decimated
through the same filter the cabinet captures with); any other rate is
refused by name.

```sh
micmodel train -samples clips -out assets/commands.arkademic.json -classes come,treat
```

`-classes` is the **cue order**: the first name listed fires `mic.cue0`.
Without it every non-underscore directory is used, sorted. `-templates`
caps how many clips per class are kept (default 5, hard cap 16): the most
typical clip first, then whichever clip is farthest from everything
already chosen, so the templates span the class's variation rather than
crowding its centre.

**The trainer sets the thresholds**, from the distances the cabinet
itself will compute: halfway between the positives' 90th percentile —
measured leave-one-out, as the class would score a recording it had never
seen — and the negatives' 10th, then held below the nearest negative. The
report is what says whether that landed: per class, clips kept, templates
chosen, threshold, positives accepted, false alarms against every other
class and the noise, and the nearest negative distance, plus a confusion
matrix and **every file the trainer refused**. That last list is not an
appendix — a class that looks strong because half its clips were dropped
is exactly what it exists to catch.

periphlab's Listen room does all of the above on the cabinet: record a
clip from the live stream (cancelled, for a model that must work while
the game plays sound), label it, train, and it writes the validated
envelope straight into the chosen cartridge's `assets/`.

### Listing it

```json
"assets": {
  "mic": ["assets/commands.arkademic.json"]
},
"capabilities": ["mic"]
```

The `"mic"` capability is required, and a manifest that lists models
without it mounts with them ignored and a log line saying so. Paths are
bundle-relative, read in the order listed, at mount; the set is installed
for **that cartridge's session only** and dropped at teardown, because a
model that outlived its cartridge would leave the cabinet listening for a
departed game's words.

**`rcartpack` fails the build** when a listed path is missing from the
bundle, does not follow the `<name>.arkademic.json` naming rule, does not
validate, or when the capability is not declared. At *mount* each of
those is a warning and a skip instead: a game whose second model is
malformed starts with its buttons working rather than refusing to launch
with a player standing in front of it.

## The pipeline the events come out of

Worth knowing because its limits are visible from inside a game.

The cabinet captures mono at **44.1 kHz** through `arecord` and decimates
by 3 to **14.7 kHz**, where every later stage works — a 7 kHz ceiling is
all a whistle, a word or a clap has to say, and it lets the mixer's own
44.1 kHz output tap decimate onto the same grid with no resampling in the
echo path.

**Echo cancellation exists because a synthesised effect is a purer tone
than a person.** The speakers sit inches from the capture device, and a
cabinet game's own recall whistle scored 1.00 on the tonality test where
a real person scores 0.94 — no threshold separates them. But the host knows
exactly what it is playing, which is the one thing a threshold never has.
So the mixer's output is tapped as an echo reference, aligned against the
capture by correlating the two signals' **envelope slopes** (a tone's
waveform correlates equally well at every whole period of lag; an onset
happens once) and refined by a waveform correlation, then predicted by a
**768-tap NLMS filter** with **192 taps of pre-roll** ahead of the
estimated delay and subtracted. A **spectral gate** runs beside it,
marking the bins where the speaker is loud *right now* — the filter only
learns the echo path at frequencies the game has already played.

Three limits follow, and all three are the game's business:

- **Alignment needs an onset**, so the boot splash plays a stinger — a
  soft rise over a quiet sweep from 150 Hz to 7 kHz — that the pipeline
  aligns on and learns the speaker path from across the whole band before
  any game sound. The cabinet replays it if the capture ever restarts. Until
  an alignment exists the rule is blunt: while the mixer is producing
  anything, the detectors are off. `/api/mic/status` reports whether the
  last stinger was heard.
- **A sound at a frequency the game has never played** leans entirely on
  the spectral gate rather than the filter. The gate is deliberately
  generous — it over-marks rather than under-marks.
- **A human whistle over loud game audio in the same band is
  suppressed**, on purpose. When the frame's energy mostly sits in bins
  the speaker owns, the detector is told to stand down; a player who
  whistles over the cabinet's own recall whistle is competing with it.

## On the cabinet

| Flag | Meaning |
| --- | --- |
| `--mic plughw:0,0` | The ALSA capture device. **Empty is the default**, and empty means the microphone does not exist: no capture process, no events, nothing to declare. |
| `--mic-no-cancel` | Skip echo cancellation — the raw capture reaches the detectors. For measuring what cancellation is worth, and for capturing the cabinet's own playback as training material. |
| `--mic-clap`, `--mic-blow` | Enable the clap and blow detectors, which otherwise emit nothing. Experimental: the thresholds are untuned, a clap detector also hears joystick thumps, and blowing needs a microphone the player can breathe on. |

The cabinet's systemd unit carries `--mic plughw:0,0`. With audio muted
or unavailable there is no mixer to tap, the cabinet logs `mic: no
playback to cancel`, and everything still runs — uncancelled.

## The bench tools

```text
micprobe -device plughw:0,0 -meter          # live, with a level meter
micprobe -dump room.wav -secs 20            # live, keeping the capture
micprobe -file room.wav                     # replay a recording
micprobe -file word.wav -model x.arkademic.json
```

`micprobe` runs the capture pipeline with nothing attached and prints
what the detectors decide. Every detector threshold is a flag
(`-tonality`, `-min-rms`, `-snr`, `-min-hz`, `-max-hz`, `-hold`,
`-refractory`, and `-clap`/`-blow` to enable the two disabled ones), so a
number can be moved against a recording instead of against a person
standing at the cabinet. Live mode has no playback reference and so
cancels nothing; the Listen room is where cancellation is watched.

```text
micmodel train -samples DIR -out NAME.arkademic.json [-name NAME] [-classes a,b] [-templates 5]
micmodel eval  -model NAME.arkademic.json -samples DIR
micmodel run   -model NAME.arkademic.json clip.wav…
micmodel info  -model NAME.arkademic.json
```

`eval` scores a **held-out** set through the live match path and the
model's own thresholds — the honest number, since the trainer chose those
thresholds knowing the training set. `run` replays clips in the same
256-sample blocks the cabinet hands the classifier, so it segments where
a live capture would. `info` prints a model's front end, segmenter and
class table without opening the JSON.

## Levels and bands (host side)

Beside the keys the pipeline publishes two coarse readings a second time a
level window closes — 50 ms at the 14.7 kHz analysis rate, twenty frames a
second. **Neither reaches a cartridge**, by the same rule as everything else
here: what crosses the ABI is quantized input events, never a measurement.
They are for the host — the peripherals lab's meter, and the cabinet's own
evening screensaver.

| Extension | Reports | |
| --- | --- | --- |
| `hardware.MicSource` | `MicLevel{Seq, RMS, Peak, Clipped}` | The window's level, 0..1 against full scale, of the **raw** capture — the room as heard, cancellation and all. `Seq` counts frames from the source's start, so a reader can tell a fresh frame from one it has already weighed. |
| `hardware.MicBander` | `MicBands() ([8]float64, bool)` | Eight band **powers**, at the `hardware.MicBandCentersHz` centres: 80, 140, 240, 420, 740, 1300, 2280 and 4000 Hz. A band is the summed power of every DFT bin between the geometric means of its neighbours, so the eight tile 60 Hz to 5.3 kHz with no gaps and nothing in the room falls between them. Normalised so a full-scale sine anywhere inside a band reads ~1.0; a band full of broadband energy can exceed it. Powers, not amplitudes — a decibel difference is `10*log10` of the ratio. The analysis is a 1024-point FFT (70 ms) run twice a frame, and the frame carries each band's **peak** over the window, so a knock shorter than a frame is not averaged away. |

Both are optional extensions a `hardware.Suite` may implement, both must be
non-blocking (the engine reads them on the tick), and both report `ok=false`
on a cabinet with no capture path. `hardware.Virtual` forwards whatever mic
is attached to it; the engine's hardware shadow forwards the suite's.

## Testing without a cabinet

A desk has no `arecord` and no speakers inches from a microphone, so
none of the live path is reachable there:

- **The test audio backend has no output tap.** Anything driving the
  engine with the recording backend (or with `--mute`) gives the
  canceller no reference at all.
- **The desk harness's microphone is synthetic.** `hardware.VirtualMic` —
  a quiet room with a clap every seven seconds and a whistle every eleven
  — is what the peripherals stream's meter shows when there is no real
  capture. It is levels and a synthetic spectrum only: it says so
  (`MicInfo.Synthetic`) and it **never emits a key**, so nothing downstream
  of it can fire a cartridge. Never tune a threshold against one.
- **Replay is the real bench.** `micprobe -file` and `micmodel run` put a
  recording through the identical code the cabinet runs.

For the cartridge's own half, build the record and call your input
handler directly — it is eight plain bytes, and the shape is fixed:

```go fragment
cartlib.InputRecord{
	Kind:     cartlib.KindKeypress,
	Key:      cartlib.KeyMicWhistle,
	Player:   1,
	Source:   cartlib.SourceKeyboard,
	Physical: cartlib.SeatNone,
	Index:    cartlib.IndexNone,
	Flags:    cartlib.FlagVirtual,
}
```

Worth writing at least four tests, which is a good set to keep: the
sound does what the button does; it reaches **player two's turn** (the
one that catches a missing turn-filter bypass); a button press from the
wrong seat is **still** filtered (the bypass is scoped to the
microphone, not a hole in the arbiter); and a spoken action that costs a
resource still spends it.
