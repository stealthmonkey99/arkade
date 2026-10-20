// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
)

// wavHeaderLen is the size of the canonical RIFF/WAVE/fmt/data header.
const wavHeaderLen = 44

// Encode renders samples, clamped to [-1,1], as a 16-bit mono PCM WAV with
// the canonical 44-byte header.
func Encode(samples []float64, sampleRate int) []byte {
	data := make([]byte, 2*len(samples))
	for i, v := range samples {
		if v > 1 {
			v = 1
		}
		if v < -1 {
			v = -1
		}
		binary.LittleEndian.PutUint16(data[2*i:], uint16(int16(v*32767)))
	}

	buf := make([]byte, wavHeaderLen+len(data))
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+len(data)))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], 1)
	binary.LittleEndian.PutUint32(buf[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(buf[32:], 2)
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(len(data)))
	copy(buf[wavHeaderLen:], data)
	return buf
}

// WriteFile writes Encode's bytes to path with mode 0o644.
func WriteFile(path string, samples []float64, sampleRate int) error {
	return os.WriteFile(path, Encode(samples, sampleRate), 0o644)
}

// Decoded is a WAV read back as mono samples in [-1,1], downmixed by
// averaging, with the source's own format.
type Decoded struct {
	Samples    []float64
	SampleRate int
	Channels   int
	Bits       int
}

// Duration returns the sound's length in seconds.
func (d Decoded) Duration() float64 {
	if d.SampleRate <= 0 {
		return 0
	}
	return float64(len(d.Samples)) / float64(d.SampleRate)
}

// Peak returns the largest absolute sample.
func (d Decoded) Peak() float64 {
	peak := 0.0
	for _, s := range d.Samples {
		if a := math.Abs(s); a > peak {
			peak = a
		}
	}
	return peak
}

// ReadFile reads and decodes the WAV file at path.
func ReadFile(path string) (Decoded, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Decoded{}, err
	}
	return Decode(b)
}

// Decode parses a RIFF/WAVE file: PCM 8/16/24/32-bit or IEEE float 32/64,
// any channel count. Extra chunks are skipped and a truncated final chunk
// is read as far as it goes. Any other format is an error.
func Decode(b []byte) (Decoded, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return Decoded{}, errors.New("sfxlib: not a RIFF/WAVE file")
	}

	var (
		format     uint16
		channels   int
		rate       int
		bits       int
		data       []byte
		haveFormat bool
	)
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if size < 0 || body+size > len(b) {
			size = len(b) - body
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return Decoded{}, fmt.Errorf("sfxlib: fmt chunk is %d bytes, want at least 16", size)
			}
			format = binary.LittleEndian.Uint16(b[body : body+2])
			channels = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
			rate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
			bits = int(binary.LittleEndian.Uint16(b[body+14 : body+16]))
			haveFormat = true
		case "data":
			data = b[body : body+size]
		}
		off = body + size + size%2
	}

	if !haveFormat {
		return Decoded{}, errors.New("sfxlib: no fmt chunk")
	}
	if channels <= 0 || rate <= 0 {
		return Decoded{}, fmt.Errorf("sfxlib: fmt chunk declares %d channels at %d Hz", channels, rate)
	}
	if data == nil {
		return Decoded{}, errors.New("sfxlib: no data chunk")
	}

	// WAVE_FORMAT_EXTENSIBLE hides the real tag in a GUID; bit depth tells PCM from float.
	if format == wavFormatExtensible {
		if bits == 32 {
			format = wavFormatFloat
		} else {
			format = wavFormatPCM
		}
	}

	read, bytesPerSample, err := sampleReader(format, bits)
	if err != nil {
		return Decoded{}, err
	}

	frameBytes := bytesPerSample * channels
	frames := len(data) / frameBytes
	out := make([]float64, frames)
	for i := 0; i < frames; i++ {
		sum := 0.0
		base := i * frameBytes
		for ch := 0; ch < channels; ch++ {
			sum += read(data[base+ch*bytesPerSample:])
		}
		out[i] = sum / float64(channels)
	}
	return Decoded{Samples: out, SampleRate: rate, Channels: channels, Bits: bits}, nil
}

const (
	wavFormatPCM        = 1
	wavFormatFloat      = 3
	wavFormatExtensible = 0xFFFE
)

func sampleReader(format uint16, bits int) (func([]byte) float64, int, error) {
	switch {
	case format == wavFormatPCM && bits == 8:
		// 8-bit PCM is unsigned.
		return func(b []byte) float64 { return (float64(b[0]) - 128) / 128 }, 1, nil
	case format == wavFormatPCM && bits == 16:
		return func(b []byte) float64 {
			return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
		}, 2, nil
	case format == wavFormatPCM && bits == 24:
		return func(b []byte) float64 {
			v := int32(b[0]) | int32(b[1])<<8 | int32(int8(b[2]))<<16
			return float64(v) / 8388608
		}, 3, nil
	case format == wavFormatPCM && bits == 32:
		return func(b []byte) float64 {
			return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
		}, 4, nil
	case format == wavFormatFloat && bits == 32:
		return func(b []byte) float64 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		}, 4, nil
	case format == wavFormatFloat && bits == 64:
		return func(b []byte) float64 {
			return math.Float64frombits(binary.LittleEndian.Uint64(b))
		}, 8, nil
	}
	return nil, 0, fmt.Errorf("sfxlib: unsupported WAV format tag %d at %d bits", format, bits)
}
