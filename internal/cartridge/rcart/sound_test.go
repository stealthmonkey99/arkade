// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
)

func testWAV(sampleRate, channels, bits, frames int) []byte {
	blockAlign := channels * bits / 8
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 1)
	binary.LittleEndian.PutUint16(fmtChunk[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(fmtChunk[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(fmtChunk[8:12], uint32(sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(fmtChunk[12:14], uint16(blockAlign))
	binary.LittleEndian.PutUint16(fmtChunk[14:16], uint16(bits))
	data := make([]byte, frames*blockAlign)

	var out bytes.Buffer
	out.WriteString("RIFF")
	total := 4 + 8 + len(fmtChunk) + 8 + len(data)
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(total))
	out.Write(u32[:])
	out.WriteString("WAVE")
	out.WriteString("fmt ")
	binary.LittleEndian.PutUint32(u32[:], uint32(len(fmtChunk)))
	out.Write(u32[:])
	out.Write(fmtChunk)
	out.WriteString("data")
	binary.LittleEndian.PutUint32(u32[:], uint32(len(data)))
	out.Write(u32[:])
	out.Write(data)
	return out.Bytes()
}

// soundAsset's envelope may claim a format the WAV does not have.
func soundAsset(t *testing.T, name string, claim assetfile.Render, recipe *assetfile.SoundRecipe) []byte {
	t.Helper()
	env := &assetfile.Envelope{
		Format:       assetfile.FormatVersion,
		Kind:         assetfile.KindSound,
		Name:         "blip",
		Generator:    assetfile.Generator{Tool: "rcart-test", Version: "1.0.0"},
		SoundPayload: assetfile.SoundPayload{Render: &claim},
	}
	if err := env.SetSoundRecipe(recipe); err != nil {
		t.Fatal(err)
	}
	out, err := assetfile.WriteBytes(name, env, testWAV(22050, 1, 16, 32))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSoundWarningsAcceptsAGoodAssetFile(t *testing.T) {
	entries := append(goodEntries(), tarEntry{
		name: "assets/blip.arkadesound.wav",
		data: soundAsset(t, "blip.arkadesound.wav",
			assetfile.Render{SampleRate: 22050, Channels: 1, Bits: 16},
			&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: "blip"}}}),
	})
	b, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, w := range b.Warnings {
		if w.Category == CategorySound {
			t.Errorf("a well-formed sound asset warned: %s", w)
		}
	}
}

func TestSoundWarningsLeavesPlainWavsAlone(t *testing.T) {
	entries := append(goodEntries(), tarEntry{name: "assets/sample.wav", data: []byte("not a RIFF file at all")})
	b, err := Load(bytes.NewReader(buildTar(t, false, entries)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, w := range b.Warnings {
		if w.Category == CategorySound {
			t.Errorf("a plain .wav was validated as an asset file: %s", w)
		}
	}
}

func TestSoundWarningsCatchesABrokenAssetFile(t *testing.T) {
	cases := []struct {
		name  string
		entry tarEntry
		want  string
	}{
		{
			"no envelope at all",
			tarEntry{name: "assets/bare.arkadesound.wav", data: testWAV(22050, 1, 16, 32)},
			"carries no arkade envelope",
		},
		{
			"the header disagrees with the render block",
			tarEntry{name: "assets/wrongrate.arkadesound.wav", data: soundAsset(t, "wrongrate.arkadesound.wav",
				assetfile.Render{SampleRate: 44100, Channels: 1, Bits: 16},
				&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: "blip"}}})},
			"44100",
		},
		{
			"a recipe that could not be rendered",
			tarEntry{name: "assets/badrecipe.arkadesound.wav", data: soundAsset(t, "badrecipe.arkadesound.wav",
				assetfile.Render{SampleRate: 22050, Channels: 1, Bits: 16},
				&assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: "a"}, {Sfx: "b"}}})},
			"no \"mix\" block",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := Load(bytes.NewReader(buildTar(t, false, append(goodEntries(), tc.entry))))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			var got string
			for _, w := range b.Warnings {
				if w.Category == CategorySound {
					got = w.Message
				}
			}
			if got == "" || !strings.Contains(got, tc.want) {
				t.Errorf("warnings = %v, want one mentioning %q", b.Warnings, tc.want)
			}
		})
	}
}
