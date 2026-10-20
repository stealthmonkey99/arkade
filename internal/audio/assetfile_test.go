// SPDX-License-Identifier: GPL-2.0-or-later

package audio

// A .arkadesound.wav is a WAV with an extra "arkd" RIFF chunk between "fmt "
// and "data". These tests pin that beep's wav.Decode walks the chunk list
// instead of assuming a 44-byte header, which would play the envelope as noise.

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// withEnvelope returns wav with a sound envelope inserted, the way
// sfxlib.EncodeAsset writes one.
func withEnvelope(t *testing.T, raw []byte, rate, channels, bits int) []byte {
	t.Helper()
	env, err := assetfile.NewSoundEnvelope(assetfile.SoundMeta{
		Name:      "blip",
		Generator: assetfile.Generator{Tool: "audio-test", Version: "1.0.0"},
		Recipe:    &assetfile.SoundRecipe{Compose: []assetfile.SoundNode{{Sfx: "blip"}}},
	}, assetfile.Render{SampleRate: rate, Channels: channels, Bits: bits})
	if err != nil {
		t.Fatalf("envelope: %v", err)
	}
	out, err := assetfile.WriteBytes("blip.arkadesound.wav", env, raw)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(out) <= len(raw) {
		t.Fatalf("the envelope added %d bytes", len(out)-len(raw))
	}
	return out
}

func TestDecodeIgnoresTheArkadeEnvelope(t *testing.T) {
	plain := encodeWav(t, MixRate, 1000, 0.5)
	f, err := assetfile.ReadWAVFormat(plain)
	if err != nil {
		t.Fatalf("read the fixture's own header: %v", err)
	}
	asset := withEnvelope(t, plain, f.SampleRate, f.Channels, f.Bits)

	want, err := decodeWAV(bytes.NewReader(plain))
	if err != nil {
		t.Fatalf("decode the plain WAV: %v", err)
	}
	got, err := decodeWAV(bytes.NewReader(asset))
	if err != nil {
		t.Fatalf("decode the asset file: %v", err)
	}
	if got.format != want.format {
		t.Errorf("format = %+v, want %+v", got.format, want.format)
	}
	if got.buf.Len() != want.buf.Len() {
		t.Fatalf("decoded %d frames, want %d", got.buf.Len(), want.buf.Len())
	}
	gotS, wantS := got.buf.Streamer(0, got.buf.Len()), want.buf.Streamer(0, want.buf.Len())
	gotBuf, wantBuf := make([][2]float64, 256), make([][2]float64, 256)
	for i := 0; ; i++ {
		gn, gok := gotS.Stream(gotBuf)
		wn, wok := wantS.Stream(wantBuf)
		if gn != wn || gok != wok {
			t.Fatalf("block %d: asset streamed %d/%v, plain %d/%v", i, gn, gok, wn, wok)
		}
		if gn == 0 {
			break
		}
		for j := 0; j < gn; j++ {
			if gotBuf[j] != wantBuf[j] {
				t.Fatalf("block %d frame %d: asset %v, plain %v", i, j, gotBuf[j], wantBuf[j])
			}
		}
		if !gok {
			break
		}
	}
}

func TestMixerPlaysASoundAssetFile(t *testing.T) {
	m := newCore(DirResolver(t.TempDir()), nil)
	plain := encodeWav(t, MixRate, 1000, 0.5)
	f, err := assetfile.ReadWAVFormat(plain)
	if err != nil {
		t.Fatal(err)
	}
	const path = "assets/blip.arkadesound.wav"
	if err := m.LoadBytes("games/snake", path, withEnvelope(t, plain, f.SampleRate, f.Channels, f.Bits)); err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	m.Play("games/snake", "", path, 0)
	if got := rms(m.readPCM(64)); got < 0.4 || got > 0.6 {
		t.Errorf("an asset file played at rms %f; the same audio as a plain WAV plays at ~0.5", got)
	}
}

// A writer that put the envelope after "data" would still decode here but
// break a streaming reader that stops at the end of the audio.
func TestEnvelopeSitsBetweenFmtAndData(t *testing.T) {
	plain := encodeWav(t, MixRate, 8, 0.5)
	f, err := assetfile.ReadWAVFormat(plain)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	data := withEnvelope(t, plain, f.SampleRate, f.Channels, f.Bits)
	for off := 12; off+8 <= len(data); {
		id := string(data[off : off+4])
		size := int(binary.LittleEndian.Uint32(data[off+4 : off+8]))
		order = append(order, id)
		off += 8 + size + size%2
	}
	fmtAt, arkdAt, dataAt := -1, -1, -1
	for i, id := range order {
		switch id {
		case "fmt ":
			fmtAt = i
		case "arkd":
			arkdAt = i
		case "data":
			dataAt = i
		}
	}
	if fmtAt < 0 || arkdAt < 0 || dataAt < 0 || !(fmtAt < arkdAt && arkdAt < dataAt) {
		t.Errorf("chunk order = %v, want \"fmt \" then \"arkd\" then \"data\"", order)
	}
}
