// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func wav(t *testing.T, rate int, samples []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("RIFF")
	b.Write([]byte{0, 0, 0, 0})
	b.WriteString("WAVEfmt ")
	b.Write([]byte{16, 0, 0, 0})
	fmtChunk := []byte{
		1, 0, 1, 0,
		byte(rate), byte(rate >> 8), byte(rate >> 16), byte(rate >> 24),
		0, 0, 0, 0, 2, 0, 16, 0,
	}
	b.Write(fmtChunk)
	b.WriteString("data")
	n := len(samples)
	b.Write([]byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)})
	b.Write(samples)
	raw := b.Bytes()
	size := len(raw) - 8
	raw[4], raw[5], raw[6], raw[7] = byte(size), byte(size>>8), byte(size>>16), byte(size>>24)
	return raw
}

func TestSameAudioIgnoresTheEnvelope(t *testing.T) {
	plain := wav(t, 44100, []byte{1, 0, 2, 0, 3, 0, 4, 0})
	env, err := NewSoundEnvelope(SoundMeta{
		Name:      "probe",
		Generator: Generator{Tool: "compare_test", Version: "1.0.0"},
	}, Render{SampleRate: 44100, Channels: 1, Bits: 16})
	if err != nil {
		t.Fatal(err)
	}
	converted, err := WriteBytes("probe.arkadesound.wav", env, plain)
	if err != nil {
		t.Fatal(err)
	}
	if len(converted) <= len(plain) {
		t.Fatal("the conversion added no envelope, so this proves nothing")
	}
	if err := SameAudio(plain, converted); err != nil {
		t.Errorf("adding an envelope changed the audio: %v", err)
	}
	if err := SamePayload("probe.arkadesound.wav", plain, converted); err != nil {
		t.Errorf("SamePayload disagreed with SameAudio: %v", err)
	}
}

func TestSameAudioNoticesRealChanges(t *testing.T) {
	base := wav(t, 44100, []byte{1, 0, 2, 0, 3, 0, 4, 0})
	for _, tc := range []struct {
		name  string
		other []byte
	}{
		{"one sample moved", wav(t, 44100, []byte{1, 0, 2, 0, 9, 0, 4, 0})},
		{"a different length", wav(t, 44100, []byte{1, 0, 2, 0, 3, 0})},
		{"a different rate", wav(t, 22050, []byte{1, 0, 2, 0, 3, 0, 4, 0})},
	} {
		if err := SameAudio(base, tc.other); err == nil {
			t.Errorf("%s: reported identical", tc.name)
		}
	}
}

func pngOf(t *testing.T, img image.Image, level png.CompressionLevel) []byte {
	t.Helper()
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: level}
	if err := enc.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSamePixelsIgnoresTheEncoder(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: 40, A: 255})
		}
	}
	best := pngOf(t, img, png.BestCompression)
	none := pngOf(t, img, png.NoCompression)
	if bytes.Equal(best, none) {
		t.Fatal("the two encodings matched, so this proves nothing")
	}
	if err := SamePixels(best, none); err != nil {
		t.Errorf("re-encoding changed the pixels: %v", err)
	}
	if err := SamePayload("icon.png", best, none); err != nil {
		t.Errorf("SamePayload compared a PNG by bytes: %v", err)
	}
}

func TestSamePixelsNoticesRealChanges(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	b := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	b.SetNRGBA(2, 1, color.NRGBA{R: 255, A: 255})
	if err := SamePixels(pngOf(t, a, png.BestCompression), pngOf(t, b, png.BestCompression)); err == nil {
		t.Error("a changed pixel was reported identical")
	}
	wide := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	if err := SamePixels(pngOf(t, a, png.BestCompression), pngOf(t, wide, png.BestCompression)); err == nil {
		t.Error("a changed size was reported identical")
	}
}

func TestSamePayloadFallsBackToBytes(t *testing.T) {
	a := []byte(`{"arkade":1,"kind":"glyph"}`)
	if err := SamePayload("heart.arkadeglyph.json", a, a); err != nil {
		t.Errorf("identical bytes reported different: %v", err)
	}
	if err := SamePayload("heart.arkadeglyph.json", a, []byte(`{"arkade":1,"kind":"hid"}`)); err == nil {
		t.Error("differing bytes reported identical")
	}
	if err := SamePayload("walls.arkadelevels", []byte("a\nb\n"), []byte("a\nb\nc\n")); err == nil {
		t.Error("a longer file reported identical")
	}
}
