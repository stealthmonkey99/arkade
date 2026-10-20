// SPDX-License-Identifier: GPL-2.0-or-later

package audio

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/gopxl/beep/v2"
)

// pcmReader adapts a beep.Streamer to the io.Reader oto pulls from. Reads hold
// the mixer mutex so playback can't race Play/Stop/SetVolume.
type pcmReader struct {
	mu  *sync.Mutex
	src beep.Streamer
	buf [512][2]float64
	// tap returns the current output tap, or nil. Called outside the mixer lock.
	tap func() func(frames [][2]float64)
}

const bytesPerFrame = 8 // 2 channels x 4-byte float32

func (r *pcmReader) Read(p []byte) (int, error) {
	frames := len(p) / bytesPerFrame
	if frames == 0 {
		return 0, nil
	}
	if frames > len(r.buf) {
		frames = len(r.buf)
	}

	r.mu.Lock()
	n, _ := r.src.Stream(r.buf[:frames])
	r.mu.Unlock()

	if n > 0 && r.tap != nil {
		if fn := r.tap(); fn != nil {
			fn(r.buf[:n])
		}
	}

	if n == 0 {
		// Only reachable with a non-mixer source; io.Reader must not return 0, nil.
		for i := 0; i < bytesPerFrame; i++ {
			p[i] = 0
		}
		return bytesPerFrame, nil
	}

	for i := 0; i < n; i++ {
		l := clampSample(r.buf[i][0])
		rr := clampSample(r.buf[i][1])
		binary.LittleEndian.PutUint32(p[i*bytesPerFrame:], math.Float32bits(float32(l)))
		binary.LittleEndian.PutUint32(p[i*bytesPerFrame+4:], math.Float32bits(float32(rr)))
	}
	return n * bytesPerFrame, nil
}

func clampSample(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}
