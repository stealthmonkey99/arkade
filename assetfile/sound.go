// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// SoundRecipe is a sound envelope's recipe: a composition tree and, when the
// nodes are mixed rather than played alone, the mix settings. A recipe with
// no Mix holds exactly one node placed at the start at unity gain, and the
// render is that node's output sample for sample. This package describes
// recipes and never renders them.
type SoundRecipe struct {
	Compose []SoundNode `json:"compose"`
	Mix     *SoundMix   `json:"mix,omitempty"`
}

// SoundNode is one source on the composition tree. Exactly one of Sfx and
// Loop is set and holds the source's name; a loop is a seamless cycle that
// may be tiled. Params is the generator's own document and stays opaque
// here.
type SoundNode struct {
	// Sfx names a one-shot source (empty when this is a loop node).
	Sfx string `json:"sfx,omitempty"`
	// Loop names a seamless-cycle source (empty when this is an sfx node).
	Loop string `json:"loop,omitempty"`
	// Params is the named generator's parameters, verbatim.
	Params json.RawMessage `json:"params,omitempty"`
	// GainDb scales the node in decibels; 0 is unity.
	GainDb float64 `json:"gainDb,omitempty"`
	// AtMs is the node's start time in milliseconds from the sound's
	// start; 0 is the beginning.
	AtMs float64 `json:"atMs,omitempty"`
	// Repeat lays the source down this many times back to back; 0 and 1
	// both mean once.
	Repeat int `json:"repeat,omitempty"`
	// Fx treats this placement of the source, before it is tiled and
	// mixed; nil plays the source as rendered. Only a mixed recipe may
	// carry it.
	Fx *SoundFx `json:"fx,omitempty"`
}

// SoundFx is the treatment one compose node gives its source, applied in
// the order the fields are listed: slice, reverse, stretch, pitch,
// high-pass, low-pass, fades. Every zero value is "leave it alone", so
// the zero SoundFx is no treatment at all.
type SoundFx struct {
	// StartMs and EndMs slice the source before anything else, in the
	// source's own milliseconds. EndMs 0 is the source's end.
	StartMs float64 `json:"startMs,omitempty"`
	EndMs   float64 `json:"endMs,omitempty"`
	// Reverse plays the slice backwards.
	Reverse bool `json:"reverse,omitempty"`
	// Stretch multiplies the slice's length: 2 is twice as long. 0 and 1
	// both mean unchanged. The pitch stays put unless Varispeed is set,
	// in which case the slice is played slower or faster as tape is and
	// the pitch moves with the speed.
	Stretch   float64 `json:"stretch,omitempty"`
	Varispeed bool    `json:"varispeed,omitempty"`
	// PitchSemitones moves the pitch without changing the length.
	PitchSemitones float64 `json:"pitchSemitones,omitempty"`
	// HighpassHz and LowpassHz are corners at 12 dB/octave; 0 is off.
	HighpassHz float64 `json:"highpassHz,omitempty"`
	LowpassHz  float64 `json:"lowpassHz,omitempty"`
	// FadeInMs and FadeOutMs ramp the treated clip's ends linearly.
	FadeInMs  float64 `json:"fadeInMs,omitempty"`
	FadeOutMs float64 `json:"fadeOutMs,omitempty"`
	// GainDb is this clip's own level, applied last and on top of the
	// node's gainDb, which an editor may own for a whole lane.
	GainDb float64 `json:"gainDb,omitempty"`
}

// SoundMix is the mixing stage's settings. Its presence makes a recipe a mix
// of placed, summed nodes rather than a single sound.
type SoundMix struct {
	// Peak is the normalization target the summed mix is scaled to;
	// 0 leaves the mix as it came out.
	Peak float64 `json:"peak"`
	// JoinMs is the crossfade length at a looping node's junctions.
	JoinMs float64 `json:"joinMs"`
	// Seconds forces the mix's total length; 0 means as long as the
	// material.
	Seconds float64 `json:"seconds,omitempty"`
}

// Recipe limits. A bundle is untrusted input and repeat and atMs size the
// renderer's allocations.
const (
	// MaxSoundNodes bounds a recipe's composition tree.
	MaxSoundNodes = 128
	// MaxSoundRepeat bounds one node's tiling.
	MaxSoundRepeat = 64
	// MaxSoundAtMs bounds a node's placement to ten minutes.
	MaxSoundAtMs = 600_000
	// MinSoundGainDb and MaxSoundGainDb bound a node's gain.
	MinSoundGainDb = -96
	MaxSoundGainDb = 24
	// MaxSoundNameLen bounds a source name.
	MaxSoundNameLen = 64
	// MinSoundStretch and MaxSoundStretch bound fx.stretch, which is an
	// allocation: the treated clip's length is the slice's times this.
	MinSoundStretch = 0.25
	MaxSoundStretch = 4
	// MaxSoundPitch bounds fx.pitchSemitones either way.
	MaxSoundPitch = 24
	// MaxSoundFilterHz bounds both fx corners.
	MaxSoundFilterHz = 24000
)

// Name is the node's source name, whichever key carries it.
func (n SoundNode) Name() string {
	if n.Loop != "" {
		return n.Loop
	}
	return n.Sfx
}

// Looping reports whether the node is a loop rather than a one-shot.
func (n SoundNode) Looping() bool { return n.Loop != "" }

// Gain is the node's GainDb as a linear factor. Renderers must use it so
// they agree with the file about loudness.
func (n SoundNode) Gain() float64 {
	if n.GainDb == 0 {
		return 1
	}
	return math.Pow(10, n.GainDb/20)
}

// Repeats is Repeat with 0 and 1 both meaning once.
func (n SoundNode) Repeats() int {
	if n.Repeat < 1 {
		return 1
	}
	return n.Repeat
}

// SoundRecipe parses env's recipe. It returns (nil, nil) when the recipe is
// absent, null or empty, as for a sampled sound nobody synthesized.
func (e *Envelope) SoundRecipe() (*SoundRecipe, error) {
	raw := bytes.TrimSpace(e.Recipe)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("{}")) {
		return nil, nil
	}
	var r SoundRecipe
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("assetfile: sound recipe: %w", err)
	}
	return &r, nil
}

// SetSoundRecipe encodes r into env's Recipe field, or clears it when r is
// nil.
func (e *Envelope) SetSoundRecipe(r *SoundRecipe) error {
	if r == nil {
		e.Recipe = nil
		return nil
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	e.Recipe = raw
	return nil
}

// ValidateSoundRecipe checks a recipe's structure without inspecting any
// node's params.
func ValidateSoundRecipe(r *SoundRecipe) error {
	if r == nil {
		return nil
	}
	if len(r.Compose) == 0 {
		return errors.New("assetfile: sound recipe has no compose nodes")
	}
	if len(r.Compose) > MaxSoundNodes {
		return fmt.Errorf("assetfile: sound recipe has %d nodes, over the limit of %d", len(r.Compose), MaxSoundNodes)
	}
	for i, n := range r.Compose {
		if (n.Sfx == "") == (n.Loop == "") {
			return fmt.Errorf("assetfile: sound recipe node %d must name exactly one of \"sfx\" or \"loop\"", i)
		}
		name := n.Name()
		if len(name) > MaxSoundNameLen {
			return fmt.Errorf("assetfile: sound recipe node %d names %q, longer than %d characters", i, name, MaxSoundNameLen)
		}
		for _, c := range name {
			if c < 0x20 || c == 0x7f {
				return fmt.Errorf("assetfile: sound recipe node %d names %q, which holds a control character", i, name)
			}
		}
		if n.Repeat < 0 || n.Repeat > MaxSoundRepeat {
			return fmt.Errorf("assetfile: sound recipe node %d repeats %d times, outside 0..%d", i, n.Repeat, MaxSoundRepeat)
		}
		if math.IsNaN(n.AtMs) || n.AtMs < 0 || n.AtMs > MaxSoundAtMs {
			return fmt.Errorf("assetfile: sound recipe node %d starts at %v ms, outside 0..%d", i, n.AtMs, MaxSoundAtMs)
		}
		if math.IsNaN(n.GainDb) || n.GainDb < MinSoundGainDb || n.GainDb > MaxSoundGainDb {
			return fmt.Errorf("assetfile: sound recipe node %d has gain %v dB, outside %d..%d",
				i, n.GainDb, MinSoundGainDb, MaxSoundGainDb)
		}
		if err := validateSoundFx(n.Fx); err != nil {
			return fmt.Errorf("assetfile: sound recipe node %d: %w", i, err)
		}
	}
	if r.Mix == nil {
		if len(r.Compose) != 1 {
			return fmt.Errorf("assetfile: sound recipe has %d nodes but no \"mix\" block; only a single node renders without one", len(r.Compose))
		}
		n := r.Compose[0]
		if n.AtMs != 0 || n.GainDb != 0 || n.Repeat > 1 || n.Fx != nil {
			return errors.New("assetfile: a sound recipe with no \"mix\" block must place its one node at the start, at unity gain, once, untreated")
		}
		return nil
	}
	m := r.Mix
	if math.IsNaN(m.Peak) || m.Peak < 0 || m.Peak > 1 {
		return fmt.Errorf("assetfile: sound mix peak %v is outside 0..1", m.Peak)
	}
	if math.IsNaN(m.JoinMs) || m.JoinMs < 0 || m.JoinMs > 1000 {
		return fmt.Errorf("assetfile: sound mix joinMs %v is outside 0..1000", m.JoinMs)
	}
	if math.IsNaN(m.Seconds) || m.Seconds < 0 || m.Seconds > MaxSoundAtMs/1000 {
		return fmt.Errorf("assetfile: sound mix seconds %v is outside 0..%d", m.Seconds, MaxSoundAtMs/1000)
	}
	return nil
}

// SoundMeta is everything a sound envelope needs beyond the audio itself.
type SoundMeta struct {
	Name        string
	Description string
	Tags        []string
	Generator   Generator
	// Recipe is nil for a sound nobody synthesized.
	Recipe *SoundRecipe
}

// NewSoundEnvelope builds a validated sound envelope from meta and the
// format the samples were rendered to.
func NewSoundEnvelope(meta SoundMeta, render Render) (*Envelope, error) {
	if meta.Name == "" {
		return nil, errors.New("assetfile: a sound envelope needs a name")
	}
	if meta.Generator.Tool == "" || meta.Generator.Version == "" {
		return nil, errors.New("assetfile: a sound envelope needs a pinned generator (tool and version)")
	}
	env := &Envelope{
		Format:       FormatVersion,
		Kind:         KindSound,
		Name:         meta.Name,
		Description:  meta.Description,
		Tags:         meta.Tags,
		Generator:    meta.Generator,
		SoundPayload: SoundPayload{Render: &render},
	}
	if err := env.SetSoundRecipe(meta.Recipe); err != nil {
		return nil, err
	}
	if err := validated(env, KindSound, 0, 0); err != nil {
		return nil, err
	}
	return env, nil
}

// WAVFormat is a WAV file's own account of itself from its "fmt " and
// "data" chunks.
type WAVFormat struct {
	SampleRate int
	Channels   int
	Bits       int
	// DataBytes is the "data" chunk's length; Frames is the sample frame
	// count it holds.
	DataBytes int
	Frames    int
}

// ReadWAVFormat parses data's "fmt " and "data" chunks and refuses a WAV
// missing either.
func ReadWAVFormat(data []byte) (WAVFormat, error) {
	form, chunks, err := splitRIFF(data)
	if err != nil {
		return WAVFormat{}, err
	}
	if string(form[:]) != "WAVE" {
		return WAVFormat{}, fmt.Errorf("assetfile: RIFF form is %q, want WAVE", form[:])
	}
	var out WAVFormat
	haveFmt, haveData := false, false
	for _, c := range chunks {
		switch string(c.id[:]) {
		case "fmt ":
			if len(c.data) < 16 {
				return WAVFormat{}, fmt.Errorf("assetfile: WAV \"fmt \" chunk is %d bytes, want at least 16", len(c.data))
			}
			out.Channels = int(binary.LittleEndian.Uint16(c.data[2:4]))
			out.SampleRate = int(binary.LittleEndian.Uint32(c.data[4:8]))
			out.Bits = int(binary.LittleEndian.Uint16(c.data[14:16]))
			haveFmt = true
		case "data":
			out.DataBytes = len(c.data)
			haveData = true
		}
	}
	if !haveFmt {
		return WAVFormat{}, errors.New("assetfile: WAV has no \"fmt \" chunk")
	}
	if !haveData {
		return WAVFormat{}, errors.New("assetfile: WAV has no \"data\" chunk")
	}
	if block := out.Channels * out.Bits / 8; block > 0 {
		out.Frames = out.DataBytes / block
	}
	return out, nil
}

// Sound is a decoded sound file: its validated envelope and the WAV's own
// format. It holds no samples.
type Sound struct {
	Envelope *Envelope
	Format   WAVFormat
}

// DecodeSound reads a sound file's validated envelope and checks that the
// WAV header agrees with the envelope's render block.
func DecodeSound(name string, data []byte) (*Sound, error) {
	env, err := ReadBytes(name, data)
	if err != nil {
		return nil, err
	}
	if env.Kind != KindSound {
		return nil, fmt.Errorf("assetfile: %s is a %q envelope, not a sound", name, env.Kind)
	}
	f, err := ReadWAVFormat(data)
	if err != nil {
		return nil, fmt.Errorf("assetfile: %s: %w", name, err)
	}
	r := env.Render
	switch {
	case f.SampleRate != r.SampleRate:
		return nil, fmt.Errorf("assetfile: %s: the WAV header says %d Hz, the envelope's render block says %d",
			name, f.SampleRate, r.SampleRate)
	case f.Channels != r.Channels:
		return nil, fmt.Errorf("assetfile: %s: the WAV header says %d channel(s), the envelope's render block says %d",
			name, f.Channels, r.Channels)
	case f.Bits != r.Bits:
		return nil, fmt.Errorf("assetfile: %s: the WAV header says %d-bit, the envelope's render block says %d",
			name, f.Bits, r.Bits)
	}
	return &Sound{Envelope: env, Format: f}, nil
}

func validateSoundFx(fx *SoundFx) error {
	if fx == nil {
		return nil
	}
	bad := func(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }
	switch {
	case bad(fx.StartMs) || fx.StartMs < 0 || fx.StartMs > MaxSoundAtMs:
		return fmt.Errorf("fx.startMs %v outside 0..%d", fx.StartMs, MaxSoundAtMs)
	case bad(fx.EndMs) || fx.EndMs < 0 || fx.EndMs > MaxSoundAtMs:
		return fmt.Errorf("fx.endMs %v outside 0..%d", fx.EndMs, MaxSoundAtMs)
	case fx.EndMs != 0 && fx.EndMs <= fx.StartMs:
		return fmt.Errorf("fx.endMs %v is not after fx.startMs %v", fx.EndMs, fx.StartMs)
	case bad(fx.Stretch) || (fx.Stretch != 0 && (fx.Stretch < MinSoundStretch || fx.Stretch > MaxSoundStretch)):
		return fmt.Errorf("fx.stretch %v outside %v..%v", fx.Stretch, MinSoundStretch, MaxSoundStretch)
	case bad(fx.PitchSemitones) || math.Abs(fx.PitchSemitones) > MaxSoundPitch:
		return fmt.Errorf("fx.pitchSemitones %v outside ±%d", fx.PitchSemitones, MaxSoundPitch)
	case bad(fx.HighpassHz) || fx.HighpassHz < 0 || fx.HighpassHz > MaxSoundFilterHz:
		return fmt.Errorf("fx.highpassHz %v outside 0..%d", fx.HighpassHz, MaxSoundFilterHz)
	case bad(fx.LowpassHz) || fx.LowpassHz < 0 || fx.LowpassHz > MaxSoundFilterHz:
		return fmt.Errorf("fx.lowpassHz %v outside 0..%d", fx.LowpassHz, MaxSoundFilterHz)
	case bad(fx.FadeInMs) || fx.FadeInMs < 0 || fx.FadeInMs > MaxSoundAtMs:
		return fmt.Errorf("fx.fadeInMs %v outside 0..%d", fx.FadeInMs, MaxSoundAtMs)
	case bad(fx.FadeOutMs) || fx.FadeOutMs < 0 || fx.FadeOutMs > MaxSoundAtMs:
		return fmt.Errorf("fx.fadeOutMs %v outside 0..%d", fx.FadeOutMs, MaxSoundAtMs)
	case bad(fx.GainDb) || fx.GainDb < MinSoundGainDb || fx.GainDb > MaxSoundGainDb:
		return fmt.Errorf("fx.gainDb %v outside %d..%d", fx.GainDb, MinSoundGainDb, MaxSoundGainDb)
	}
	return nil
}
