// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeHeader(t *testing.T) {
	samples := make([]float64, 10)
	const sampleRate = 44100
	buf := Encode(samples, sampleRate)

	wantLen := wavHeaderLen + 2*len(samples)
	if len(buf) != wantLen {
		t.Fatalf("len(buf) = %d, want %d", len(buf), wantLen)
	}
	if s := string(buf[0:4]); s != "RIFF" {
		t.Errorf("bytes[0:4] = %q, want RIFF", s)
	}
	if s := string(buf[8:12]); s != "WAVE" {
		t.Errorf("bytes[8:12] = %q, want WAVE", s)
	}
	if s := string(buf[12:16]); s != "fmt " {
		t.Errorf("bytes[12:16] = %q, want %q", s, "fmt ")
	}
	if s := string(buf[36:40]); s != "data" {
		t.Errorf("bytes[36:40] = %q, want data", s)
	}

	dataLen := 2 * len(samples)
	if got := binary.LittleEndian.Uint32(buf[4:8]); got != uint32(36+dataLen) {
		t.Errorf("RIFF chunk size = %d, want %d", got, 36+dataLen)
	}
	if got := binary.LittleEndian.Uint32(buf[16:20]); got != 16 {
		t.Errorf("fmt chunk size = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint16(buf[20:22]); got != 1 {
		t.Errorf("audio format = %d, want 1 (PCM)", got)
	}
	if got := binary.LittleEndian.Uint16(buf[22:24]); got != 1 {
		t.Errorf("num channels = %d, want 1 (mono)", got)
	}
	if got := binary.LittleEndian.Uint32(buf[24:28]); got != sampleRate {
		t.Errorf("sample rate = %d, want %d", got, sampleRate)
	}
	if got := binary.LittleEndian.Uint32(buf[28:32]); got != sampleRate*2 {
		t.Errorf("byte rate = %d, want %d", got, sampleRate*2)
	}
	if got := binary.LittleEndian.Uint16(buf[32:34]); got != 2 {
		t.Errorf("block align = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint16(buf[34:36]); got != 16 {
		t.Errorf("bits per sample = %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint32(buf[40:44]); got != uint32(dataLen) {
		t.Errorf("data chunk size = %d, want %d", got, dataLen)
	}
}

func TestEncodeSampleValues(t *testing.T) {
	in := []float64{0, 1, -1, 0.5, -0.5, 2, -2}
	buf := Encode(in, 22050)
	data := buf[wavHeaderLen:]

	half, negHalf := 0.5, -0.5
	want := []int16{
		0,
		32767,
		int16(-1 * 32767),
		int16(half * 32767),
		int16(negHalf * 32767),
		32767,             // clamped from 2
		int16(-1 * 32767), // clamped from -2
	}
	for i, w := range want {
		got := int16(binary.LittleEndian.Uint16(data[2*i : 2*i+2]))
		if got != w {
			t.Errorf("sample %d: got %d, want %d", i, got, w)
		}
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	samples := []float64{0, 0.25, -0.25, 0.9, -0.9}
	dir := t.TempDir()
	path := filepath.Join(dir, "test.wav")

	if err := WriteFile(path, samples, 44100); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := Encode(samples, 44100)
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, len(want) = %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestEncodeEmpty(t *testing.T) {
	buf := Encode(nil, 44100)
	if len(buf) != wavHeaderLen {
		t.Fatalf("len(buf) = %d, want %d", len(buf), wavHeaderLen)
	}
	if got := binary.LittleEndian.Uint32(buf[40:44]); got != 0 {
		t.Errorf("data chunk size = %d, want 0", got)
	}
}
