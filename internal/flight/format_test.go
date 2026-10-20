// SPDX-License-Identifier: GPL-2.0-or-later

package flight

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

// sampleRecording builds a recording with n events of every kind, so a
// round-trip exercises the whole event vocabulary rather than one of it.
func sampleRecording(n int) *Recording {
	rec := &Recording{Header: Header{
		Slug:            "cart/snake",
		Name:            "Snake",
		ManifestVersion: "1.4.2",
		WasmSHA256:      strings.Repeat("ab", 32),
		EngineSeed:      -8_123_456_789,
		TargetFPS:       30,
		Cols:            64,
		Rows:            64,
		StartedAt:       time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
		DumpedAt:        time.Date(2026, 9, 5, 12, 4, 30, 0, time.UTC),
		Reason:          ReasonCrash,
		Note:            "update: cartridge: cart_update: wasm error: unreachable",
		EngineVersion:   "dev",
		Platform:        "linux/arm64",
		Dropped:         7,
		Truncated:       true,
	}}
	for i := 0; i < n; i++ {
		rec.Events = append(rec.Events,
			Event{Frame: uint32(i), Kind: KindFrame, Data: 33_333},
			Event{Frame: uint32(i), Kind: KindInput, Data: 0x0102030405060708},
			Event{Frame: uint32(i), Kind: KindRandom, Data: ^uint64(0) - uint64(i)},
		)
	}
	rec.Header.Frames, rec.Header.Inputs, rec.Header.Randoms = uint32(n), uint32(n), uint32(n)
	return rec
}

func encodeDecode(t *testing.T, rec *Recording) *Recording {
	t.Helper()
	var buf bytes.Buffer
	if err := Encode(&buf, rec); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return got
}

func TestRoundTripByteFidelity(t *testing.T) {
	// Both sides of the gzip threshold.
	for _, n := range []int{1, 4, 2000} {
		rec := sampleRecording(n)
		got := encodeDecode(t, rec)

		if got.Header != rec.Header {
			t.Fatalf("n=%d: header round-trip differs:\n got %+v\nwant %+v", n, got.Header, rec.Header)
		}
		if len(got.Events) != len(rec.Events) {
			t.Fatalf("n=%d: %d events back, want %d", n, len(got.Events), len(rec.Events))
		}
		for i := range rec.Events {
			if got.Events[i] != rec.Events[i] {
				t.Fatalf("n=%d: event %d: got %+v, want %+v", n, i, got.Events[i], rec.Events[i])
			}
		}

		var first, second bytes.Buffer
		if err := Encode(&first, rec); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if err := Encode(&second, got); err != nil {
			t.Fatalf("re-Encode: %v", err)
		}
		if !bytes.Equal(first.Bytes(), second.Bytes()) {
			t.Fatalf("n=%d: re-encoding a decoded recording produced different bytes (%d vs %d)",
				n, first.Len(), second.Len())
		}
	}
}

func TestGzipOnlyAboveThreshold(t *testing.T) {
	small := &Recording{Header: Header{Slug: "cart/x"}, Events: []Event{{Kind: KindFrame, Data: 1}}}
	var buf bytes.Buffer
	if err := Encode(&buf, small); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if flags := buf.Bytes()[len(Magic)+1]; flags&flagGzip != 0 {
		t.Errorf("a %d byte recording was gzipped; below the %d byte threshold it should be stored plain",
			buf.Len(), gzipThreshold)
	}

	buf.Reset()
	if err := Encode(&buf, sampleRecording(500)); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if flags := buf.Bytes()[len(Magic)+1]; flags&flagGzip == 0 {
		t.Error("a 1500-event recording was not gzipped")
	}
	raw := len(Magic) + 2 + 8 + 1500*EventBytes
	if buf.Len() > raw/4 {
		t.Errorf("compressed to %d bytes from ~%d raw; expected better than 4:1 on repetitive frame events",
			buf.Len(), raw)
	}
}

func TestDecodeRejectsJunk(t *testing.T) {
	good := func() []byte {
		var buf bytes.Buffer
		if err := Encode(&buf, sampleRecording(3)); err != nil {
			t.Fatalf("Encode: %v", err)
		}
		return buf.Bytes()
	}

	cases := []struct {
		name    string
		body    []byte
		wantErr string
		notARec bool
	}{
		{"empty", nil, "too short", true},
		{"wrong magic", []byte("NOTAFLIGHTRECORDING"), "bad magic", true},
		{"future format version", func() []byte {
			b := good()
			b[len(Magic)] = FormatVersion + 1
			return b
		}(), "format version", false},
		{"truncated body", good()[:len(Magic)+4], "", false},
		{"gzip flag on plain bytes", func() []byte {
			b := good()
			b[len(Magic)+1] = flagGzip
			return b
		}(), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(bytes.NewReader(tc.body))
			if err == nil {
				t.Fatal("Decode accepted junk")
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
			if tc.notARec != errors.Is(err, ErrNotARecording) {
				t.Errorf("errors.Is(err, ErrNotARecording) = %v, want %v (err = %v)",
					!tc.notARec, tc.notARec, err)
			}
		})
	}
}

func TestDecodeRefusesAnOverclaimedEventCount(t *testing.T) {
	var buf bytes.Buffer
	rec := sampleRecording(1)
	if err := Encode(&buf, rec); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	body := buf.Bytes()
	_, err := Decode(bytes.NewReader(body[:len(body)-8]))
	if err == nil {
		t.Fatal("Decode accepted a truncated event array")
	}
}

func TestRecordingFrameSpan(t *testing.T) {
	empty := &Recording{}
	if empty.FirstFrame() != 0 || empty.LastFrame() != 0 {
		t.Error("an empty recording should report a zero frame span")
	}
	rec := &Recording{Events: []Event{{Frame: 12}, {Frame: 40}, {Frame: 99}}}
	if rec.FirstFrame() != 12 || rec.LastFrame() != 99 {
		t.Errorf("frame span = %d..%d, want 12..99", rec.FirstFrame(), rec.LastFrame())
	}
}
