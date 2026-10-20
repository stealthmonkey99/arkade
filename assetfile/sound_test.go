// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func pcmWAV(sampleRate, channels, bits, frames int) []byte {
	blockAlign := channels * bits / 8
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 1) // PCM
	binary.LittleEndian.PutUint16(fmtChunk[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(fmtChunk[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(fmtChunk[8:12], uint32(sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(fmtChunk[12:14], uint16(blockAlign))
	binary.LittleEndian.PutUint16(fmtChunk[14:16], uint16(bits))
	return joinRIFF([4]byte{'W', 'A', 'V', 'E'}, []riffChunk{
		{id: [4]byte{'f', 'm', 't', ' '}, data: fmtChunk},
		{id: [4]byte{'d', 'a', 't', 'a'}, data: make([]byte, frames*blockAlign)},
	})
}

func TestReadWAVFormat(t *testing.T) {
	f, err := ReadWAVFormat(pcmWAV(44100, 2, 16, 100))
	if err != nil {
		t.Fatal(err)
	}
	if f.SampleRate != 44100 || f.Channels != 2 || f.Bits != 16 {
		t.Errorf("format = %+v, want 44100/2/16", f)
	}
	if f.Frames != 100 || f.DataBytes != 400 {
		t.Errorf("frames = %d (%d bytes), want 100 (400 bytes)", f.Frames, f.DataBytes)
	}

	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"not RIFF", []byte("nope"), "not a RIFF"},
		{"no fmt", joinRIFF([4]byte{'W', 'A', 'V', 'E'}, []riffChunk{{id: [4]byte{'d', 'a', 't', 'a'}}}), "no \"fmt \""},
		{"no data", joinRIFF([4]byte{'W', 'A', 'V', 'E'}, []riffChunk{{id: [4]byte{'f', 'm', 't', ' '}, data: make([]byte, 16)}}), "no \"data\""},
		{"short fmt", joinRIFF([4]byte{'W', 'A', 'V', 'E'}, []riffChunk{
			{id: [4]byte{'f', 'm', 't', ' '}, data: make([]byte, 8)},
			{id: [4]byte{'d', 'a', 't', 'a'}},
		}), "at least 16"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadWAVFormat(tc.data); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestDecodeSoundCrossChecksTheHeader(t *testing.T) {
	const name = "blip.arkadesound.wav"
	base := pcmWAV(22050, 1, 16, 64)

	ok, err := NewSoundEnvelope(SoundMeta{
		Name:      "blip",
		Generator: Generator{Tool: "t", Version: "1.0.0"},
		Recipe:    &SoundRecipe{Compose: []SoundNode{{Sfx: "blip"}}},
	}, Render{SampleRate: 22050, Channels: 1, Bits: 16})
	if err != nil {
		t.Fatal(err)
	}
	good, err := WriteBytes(name, ok, base)
	if err != nil {
		t.Fatal(err)
	}
	sound, err := DecodeSound(name, good)
	if err != nil {
		t.Fatalf("a file whose header and envelope agree: %v", err)
	}
	if sound.Format.Frames != 64 {
		t.Errorf("frames = %d, want 64", sound.Format.Frames)
	}

	for _, tc := range []struct {
		name   string
		render Render
		want   string
	}{
		{"rate", Render{SampleRate: 44100, Channels: 1, Bits: 16}, "44100"},
		{"channels", Render{SampleRate: 22050, Channels: 2, Bits: 16}, "channel"},
		{"bits", Render{SampleRate: 22050, Channels: 1, Bits: 8}, "render block says 8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, err := NewSoundEnvelope(SoundMeta{
				Name: "blip", Generator: Generator{Tool: "t", Version: "1.0.0"},
			}, tc.render)
			if err != nil {
				t.Fatal(err)
			}
			data, err := WriteBytes(name, env, base)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeSound(name, data); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateSoundRecipeStructure(t *testing.T) {
	mix := func() *SoundMix { return &SoundMix{Peak: 0.9, JoinMs: 6} }
	cases := []struct {
		name   string
		recipe *SoundRecipe
		want   string
	}{
		{"nil is fine", nil, ""},
		{"one solo node", &SoundRecipe{Compose: []SoundNode{{Sfx: "blip"}}}, ""},
		{"one loop node", &SoundRecipe{Compose: []SoundNode{{Loop: "bass"}}}, ""},
		{"a mix", &SoundRecipe{
			Compose: []SoundNode{{Loop: "bass", Repeat: 4}, {Sfx: "hit", AtMs: 1200, GainDb: -6}},
			Mix:     mix(),
		}, ""},
		{"empty", &SoundRecipe{}, "no compose nodes"},
		{"both keys", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Loop: "b"}}}, "exactly one"},
		{"neither key", &SoundRecipe{Compose: []SoundNode{{}}}, "exactly one"},
		{"control character in a name", &SoundRecipe{Compose: []SoundNode{{Sfx: "a\x01b"}}}, "control character"},
		{"absurd repeat", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Repeat: 1000}}, Mix: mix()}, "repeats 1000"},
		{"negative time", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", AtMs: -1}}, Mix: mix()}, "starts at -1"},
		{"absurd gain", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", GainDb: 200}}, Mix: mix()}, "gain 200"},
		{"peak out of range", &SoundRecipe{Compose: []SoundNode{{Sfx: "a"}}, Mix: &SoundMix{Peak: 4}}, "peak 4"},
		{"two nodes, no mix", &SoundRecipe{Compose: []SoundNode{{Sfx: "a"}, {Sfx: "b"}}}, "no \"mix\" block"},
		{"placed node, no mix", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", AtMs: 10}}}, "at the start"},
		{"gained node, no mix", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", GainDb: -3}}}, "at unity gain"},
		{"treated node, no mix", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Fx: &SoundFx{Reverse: true}}}}, "untreated"},
		{"a treated node", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Fx: &SoundFx{
			StartMs: 20, EndMs: 400, Reverse: true, Stretch: 1.5, PitchSemitones: -3,
			HighpassHz: 150, LowpassHz: 6000, FadeInMs: 10, FadeOutMs: 40}}}, Mix: mix()}, ""},
		{"slice ends before it starts", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Fx: &SoundFx{StartMs: 400, EndMs: 100}}}, Mix: mix()}, "not after"},
		{"absurd stretch", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Fx: &SoundFx{Stretch: 50}}}, Mix: mix()}, "fx.stretch 50"},
		{"absurd pitch", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Fx: &SoundFx{PitchSemitones: -60}}}, Mix: mix()}, "fx.pitchSemitones -60"},
		{"negative corner", &SoundRecipe{Compose: []SoundNode{{Sfx: "a", Fx: &SoundFx{LowpassHz: -1}}}, Mix: mix()}, "fx.lowpassHz -1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSoundRecipe(tc.recipe)
			if tc.want == "" {
				if err != nil {
					t.Errorf("ValidateSoundRecipe: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestSoundRecipeParamsStayOpaque(t *testing.T) {
	raw := json.RawMessage(`{"hz":880,"ms":60,"whatever":[1,2,{"deep":true}]}`)
	env := &Envelope{
		Format: FormatVersion, Kind: KindSound,
		Name: "t", Generator: Generator{Tool: "t", Version: "1"},
		SoundPayload: SoundPayload{Render: &Render{SampleRate: 22050, Channels: 1, Bits: 16}},
	}
	if err := env.SetSoundRecipe(&SoundRecipe{Compose: []SoundNode{{Sfx: "blip", Params: raw}}}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(env, KindSound, 0, 0); err != nil {
		t.Fatalf("params this package does not understand must still validate: %v", err)
	}
	back, err := env.SoundRecipe()
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(back.Compose[0].Params, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(got, want) {
		t.Errorf("params came back as %v, want %v", got, want)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestSoundRecipeAbsentAndEmpty(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("{}")} {
		env := &Envelope{Recipe: raw}
		r, err := env.SoundRecipe()
		if err != nil || r != nil {
			t.Errorf("SoundRecipe(%s) = (%v, %v), want (nil, nil)", raw, r, err)
		}
	}
}

func TestSoundNodeAccessors(t *testing.T) {
	loop := SoundNode{Loop: "bass", Repeat: 0}
	if !loop.Looping() || loop.Name() != "bass" || loop.Repeats() != 1 {
		t.Errorf("loop node = %+v, want a looping \"bass\" played once", loop)
	}
	if g := (SoundNode{}).Gain(); g != 1 {
		t.Errorf("0 dB = %v, want exactly 1 — unity has to survive the round trip", g)
	}
	if g := (SoundNode{GainDb: -6}).Gain(); g < 0.49 || g > 0.51 {
		t.Errorf("-6 dB = %v, want ~0.5", g)
	}
}

func TestNewSoundEnvelopeRefusesAnUnpinnedGenerator(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta SoundMeta
		want string
	}{
		{"no name", SoundMeta{Generator: Generator{Tool: "t", Version: "1"}}, "needs a name"},
		{"no tool", SoundMeta{Name: "t", Generator: Generator{Version: "1"}}, "pinned generator"},
		{"no version", SoundMeta{Name: "t", Generator: Generator{Tool: "t"}}, "pinned generator"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewSoundEnvelope(tc.meta, Render{SampleRate: 22050, Channels: 1, Bits: 16})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
