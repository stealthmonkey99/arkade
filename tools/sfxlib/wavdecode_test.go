// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodeRoundTripsEncode(t *testing.T) {
	want := []float64{0, 0.5, -0.5, 0.999, -0.999}
	got, err := Decode(Encode(want, 22050))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.SampleRate != 22050 || got.Channels != 1 || got.Bits != 16 {
		t.Errorf("format: %d Hz, %d ch, %d bits; want 22050/1/16", got.SampleRate, got.Channels, got.Bits)
	}
	if len(got.Samples) != len(want) {
		t.Fatalf("decoded %d samples, want %d", len(got.Samples), len(want))
	}
	for i, w := range want {
		// Encode scales by 32767 and truncates; Decode divides by 32768.
		if math.Abs(got.Samples[i]-w) > 2.0/32767 {
			t.Errorf("sample %d = %v, want %v", i, got.Samples[i], w)
		}
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	for name, raw := range map[string][]byte{
		"too short":  []byte("RIFF"),
		"not a RIFF": []byte("this is not a RIFF file at all"),
		"no chunks":  append([]byte("RIFF"), append([]byte{0, 0, 0, 0}, []byte("WAVE")...)...),
		"a-law":      riffWAV(6, 1, 8, []byte{0}, nil),
	} {
		if _, err := Decode(raw); err == nil {
			t.Errorf("%s: decoded without error", name)
		}
	}
}

func TestDecodeEveryFormat(t *testing.T) {
	le16 := func(v int16) []byte { return binary.LittleEndian.AppendUint16(nil, uint16(v)) }
	cases := []struct {
		name     string
		format   uint16
		channels int
		bits     int
		data     []byte
		want     []float64
	}{
		{"pcm8", 1, 1, 8, []byte{128, 192, 64}, []float64{0, 0.5, -0.5}},
		{"pcm16 stereo downmix", 1, 2, 16, append(le16(16384), le16(-16384)...), []float64{0}},
		{"pcm24", 1, 1, 24, []byte{0, 0, 0x40, 0, 0, 0xc0}, []float64{0.5, -0.5}},
		{"pcm32", 1, 1, 32, binary.LittleEndian.AppendUint32(nil, 1<<30), []float64{0.5}},
		{"float32", 3, 1, 32, binary.LittleEndian.AppendUint32(nil, math.Float32bits(-0.25)), []float64{-0.25}},
		{"float64", 3, 1, 64, binary.LittleEndian.AppendUint64(nil, math.Float64bits(0.75)), []float64{0.75}},
		{"extensible pcm16", 0xFFFE, 1, 16, le16(-32768), []float64{-1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode(riffWAV(tc.format, tc.channels, tc.bits, tc.data, []byte("LIST")))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Samples) != len(tc.want) {
				t.Fatalf("samples = %v, want %v", got.Samples, tc.want)
			}
			for i := range tc.want {
				if math.Abs(got.Samples[i]-tc.want[i]) > 1e-6 {
					t.Errorf("sample %d = %v, want %v", i, got.Samples[i], tc.want[i])
				}
			}
		})
	}
}

func riffWAV(format uint16, channels, bits int, data []byte, extraID []byte) []byte {
	fmtBody := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtBody[0:], format)
	binary.LittleEndian.PutUint16(fmtBody[2:], uint16(channels))
	binary.LittleEndian.PutUint32(fmtBody[4:], 8000)
	binary.LittleEndian.PutUint16(fmtBody[14:], uint16(bits))
	chunk := func(id string, body []byte) []byte {
		out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
		out = append(out, body...)
		if len(body)%2 == 1 {
			out = append(out, 0)
		}
		return out
	}
	body := append([]byte("WAVE"), chunk("fmt ", fmtBody)...)
	if extraID != nil {
		body = append(body, chunk(string(extraID), []byte{1, 2, 3})...)
	}
	body = append(body, chunk("data", data)...)
	return append(append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...), body...)
}
