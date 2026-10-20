// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// ReadWAV decodes a 16-bit PCM WAV (mono, or stereo downmixed) into samples
// scaled to -1..1, returning its sample rate. The one format the lab writes
// and arecord produces; anything else is refused rather than guessed at.
func ReadWAV(r io.Reader) (samples []float64, rate int, err error) {
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, 0, err
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, 0, errors.New("not a WAV file")
	}
	channels := 0
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return nil, 0, errors.New("no data chunk")
		}
		id, size := string(ch[0:4]), int(binary.LittleEndian.Uint32(ch[4:8]))
		switch id {
		case "fmt ":
			b := make([]byte, size+size%2)
			if _, err := io.ReadFull(r, b); err != nil {
				return nil, 0, err
			}
			if f := binary.LittleEndian.Uint16(b[0:2]); f != 1 {
				return nil, 0, fmt.Errorf("want PCM, got format %d", f)
			}
			channels = int(binary.LittleEndian.Uint16(b[2:4]))
			if channels < 1 || channels > 2 {
				return nil, 0, fmt.Errorf("want mono or stereo, got %d channels", channels)
			}
			if bits := binary.LittleEndian.Uint16(b[14:16]); bits != 16 {
				return nil, 0, fmt.Errorf("want 16-bit, got %d-bit", bits)
			}
			rate = int(binary.LittleEndian.Uint32(b[4:8]))
		case "data":
			if rate == 0 {
				return nil, 0, errors.New("data chunk before fmt")
			}
			b := make([]byte, size)
			n, err := io.ReadFull(r, b)
			if err != nil && err != io.ErrUnexpectedEOF {
				return nil, 0, err
			}
			b = b[:n&^1]
			frames := len(b) / 2 / channels
			samples = make([]float64, frames)
			for i := range samples {
				s := 0.0
				for c := 0; c < channels; c++ {
					s += float64(int16(binary.LittleEndian.Uint16(b[(i*channels+c)*2:])))
				}
				samples[i] = s / float64(channels) / 32768
			}
			return samples, rate, nil
		default:
			if _, err := io.CopyN(io.Discard, r, int64(size+size%2)); err != nil {
				return nil, 0, err
			}
		}
	}
}

// ReadWAVFile is ReadWAV over a path.
func ReadWAVFile(path string) ([]float64, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	s, rate, err := ReadWAV(f)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}
	return s, rate, nil
}

// WriteWAV writes samples (-1..1) as 16-bit mono PCM at rate.
func WriteWAV(w io.Writer, samples []float64, rate int) error {
	n := len(samples) * 2
	if _, err := w.Write(wavHeader(rate, n)); err != nil {
		return err
	}
	buf := make([]byte, n)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(clip16(s)))
	}
	_, err := w.Write(buf)
	return err
}

// WriteWAVFile is WriteWAV to a path, created or truncated.
func WriteWAVFile(path string, samples []float64, rate int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := WriteWAV(f, samples, rate); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func clip16(s float64) int16 {
	if s >= 1 {
		return 32767
	}
	if s <= -1 {
		return -32768
	}
	return int16(s * 32767)
}

func wavHeader(rate, dataBytes int) []byte {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+dataBytes))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 1) // mono
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(dataBytes))
	return h
}
