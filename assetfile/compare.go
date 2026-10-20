// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
)

// SameAudio reports whether two WAV files carry the same "fmt " description
// and byte-identical "data" chunks. Envelope chunks and chunk order are not
// compared.
func SameAudio(a, b []byte) error {
	fa, da, err := wavPayload(a)
	if err != nil {
		return fmt.Errorf("assetfile: left: %w", err)
	}
	fb, db, err := wavPayload(b)
	if err != nil {
		return fmt.Errorf("assetfile: right: %w", err)
	}
	switch {
	case fa.SampleRate != fb.SampleRate:
		return fmt.Errorf("assetfile: sample rate %d vs %d", fa.SampleRate, fb.SampleRate)
	case fa.Channels != fb.Channels:
		return fmt.Errorf("assetfile: %d channel(s) vs %d", fa.Channels, fb.Channels)
	case fa.Bits != fb.Bits:
		return fmt.Errorf("assetfile: %d-bit vs %d-bit", fa.Bits, fb.Bits)
	case len(da) != len(db):
		return fmt.Errorf("assetfile: %d sample frames vs %d", len(da)/frameSize(fa), len(db)/frameSize(fb))
	}
	if i := firstDiff(da, db); i >= 0 {
		return fmt.Errorf("assetfile: audio differs from sample frame %d", i/frameSize(fa))
	}
	return nil
}

// SamePixels reports whether two PNG files decode to the same image: equal
// bounds and equal RGBA at every pixel. Encoded bytes are not compared
// because Go's DEFLATE output is not stable across releases.
func SamePixels(a, b []byte) error {
	ia, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		return fmt.Errorf("assetfile: left: %w", err)
	}
	ib, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("assetfile: right: %w", err)
	}
	ra, rb := ia.Bounds(), ib.Bounds()
	if ra.Dx() != rb.Dx() || ra.Dy() != rb.Dy() {
		return fmt.Errorf("assetfile: %dx%d vs %dx%d", ra.Dx(), ra.Dy(), rb.Dx(), rb.Dy())
	}
	for y := 0; y < ra.Dy(); y++ {
		for x := 0; x < ra.Dx(); x++ {
			r1, g1, b1, a1 := ia.At(ra.Min.X+x, ra.Min.Y+y).RGBA()
			r2, g2, b2, a2 := ib.At(rb.Min.X+x, rb.Min.Y+y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				return fmt.Errorf("assetfile: pixels differ at (%d,%d)", x, y)
			}
		}
	}
	return nil
}

// SamePayload compares two files of the kind name implies: sounds by audio,
// sprites by pixels, every other kind by bytes. A plain .wav or .png is
// recognised by its magic bytes.
func SamePayload(name string, a, b []byte) error {
	kind, ok := KindFromFilename(name)
	if !ok {
		switch {
		case bytes.HasPrefix(a, []byte("RIFF")):
			kind = KindSound
		case bytes.HasPrefix(a, []byte("\x89PNG\r\n\x1a\n")):
			kind = KindSprite
		}
	}
	switch kind {
	case KindSound:
		return SameAudio(a, b)
	case KindSprite:
		return SamePixels(a, b)
	}
	if i := firstDiff(a, b); i >= 0 {
		return fmt.Errorf("assetfile: %s differs at byte %d", name, i)
	}
	if len(a) != len(b) {
		return fmt.Errorf("assetfile: %s is %d bytes vs %d", name, len(a), len(b))
	}
	return nil
}

func wavPayload(data []byte) (WAVFormat, []byte, error) {
	f, err := ReadWAVFormat(data)
	if err != nil {
		return WAVFormat{}, nil, err
	}
	_, chunks, err := splitRIFF(data)
	if err != nil {
		return WAVFormat{}, nil, err
	}
	for _, c := range chunks {
		if string(c.id[:]) == "data" {
			return f, c.data, nil
		}
	}
	return WAVFormat{}, nil, errors.New("WAV has no \"data\" chunk")
}

// frameSize is floored at 1 so a malformed header cannot divide by zero.
func frameSize(f WAVFormat) int {
	if n := f.Channels * f.Bits / 8; n > 0 {
		return n
	}
	return 1
}

// firstDiff returns -1 when the shorter input is a prefix of the other.
func firstDiff(a, b []byte) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return -1
}
