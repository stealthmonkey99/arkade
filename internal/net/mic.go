// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/stealthmonkey99/arkade/internal/mic"
)

// The microphone routes expose the capture pipeline to the peripherals lab:
// waveform, canceller state and detector verdicts frame by frame. This is the
// only place raw audio leaves the process; it is token-gated and bounded in
// count and duration.

const (
	maxMicStreams      = 2
	defaultMicStreamMs = 5 * 60 * 1000
	maxMicStreamMs     = 30 * 60 * 1000
	micWriteTimeout    = 5 * time.Second
	// micStreamQueue is how many frames a slow reader may fall behind before
	// frames are dropped rather than the capture goroutine blocked.
	micStreamQueue = 64
)

// micStreamer is what Remote.Microphone must also implement for these routes
// to serve anything.
type micStreamer interface {
	Listen(fn func(mic.Block)) (cancel func())
	Stats() mic.Stats
}

func (s *Server) micSource() (micStreamer, bool) {
	m, ok := s.rt.Microphone().(micStreamer)
	return m, ok
}

func (s *Server) handleMicStatus(w http.ResponseWriter, r *http.Request) *rejection {
	w.Header().Set("Content-Type", "application/json")
	m, ok := s.micSource()
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{"available": false})
		return nil
	}
	st := m.Stats()
	_ = json.NewEncoder(w).Encode(struct {
		Available bool `json:"available"`
		Rate      int  `json:"rate"`
		mic.Stats
	}{true, mic.Rate, st})
	return nil
}

// Block wire format: a 32-byte little-endian header, then the raw and
// cancelled samples as int16, n each:
//
//	 0  u32  magic "MIC1" (0x3143494D)
//	 4  u32  seq
//	 8  u16  n (samples per channel)
//	10  u16  flags: bit0 echo-dominated, bit1 aligned, bit2 cancelling, bit3 speech gate open
//	12  i16  ERLE in centi-dB
//	14  i32  delay in samples
//	18  u8   event (0 none, 1 whistle, 2 clap, 3 blow)
//	19  i8   cue (-1 none)
//	20  u16  peak Hz
//	22  u16  tonality × 10000
//	24  u16  rms × 65535
//	26  u16  floor × 65535
//	28  u16  cue score × 1000
//	30  u16  reserved
const (
	micMagic      = 0x3143494D
	micHeaderSize = 32
)

// handleMicStream serves frames as a chunked binary body. Query: ?ms= how long
// the stream may run (default 5 minutes, max 30).
func (s *Server) handleMicStream(w http.ResponseWriter, r *http.Request) *rejection {
	m, ok := s.micSource()
	if !ok {
		return reject(http.StatusNotFound, "this cabinet has no microphone")
	}
	dur := time.Duration(defaultMicStreamMs) * time.Millisecond
	if v := r.URL.Query().Get("ms"); v != "" {
		var ms int
		if _, err := fmt.Sscanf(v, "%d", &ms); err != nil || ms <= 0 || ms > maxMicStreamMs {
			return reject(http.StatusBadRequest, fmt.Sprintf("ms must be 1..%d", maxMicStreamMs))
		}
		dur = time.Duration(ms) * time.Millisecond
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return noStreaming()
	}
	if n := s.micStreams.Add(1); n > maxMicStreams {
		s.micStreams.Add(-1)
		return reject(http.StatusTooManyRequests, fmt.Sprintf("too many microphone streams open (max %d): close one first", maxMicStreams)).
			retry("5")
	}
	defer s.micStreams.Add(-1)

	// The listener runs on the capture goroutine, so it only queues frames;
	// a slow reader loses frames rather than stalling capture.
	queue := make(chan []byte, micStreamQueue)
	dropped := 0
	cancel := m.Listen(func(b mic.Block) {
		select {
		case queue <- encodeMicBlock(b):
		default:
			dropped++
		}
	})
	defer cancel()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	s.ring.Append("info", fmt.Sprintf("http: mic stream opened (%s)", dur.Round(time.Second)))

	rc := http.NewResponseController(w)
	deadline := time.NewTimer(dur)
	defer deadline.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-deadline.C:
			s.ring.Append("info", "http: mic stream closed: duration reached")
			return nil
		case frame := <-queue:
			_ = rc.SetWriteDeadline(time.Now().Add(micWriteTimeout))
			if _, err := w.Write(frame); err != nil {
				return nil
			}
			// Drain the queue before flushing so a burst is one write.
			for len(queue) > 0 {
				if _, err := w.Write(<-queue); err != nil {
					return nil
				}
			}
			flusher.Flush()
		}
	}
}

func encodeMicBlock(b mic.Block) []byte {
	n := len(b.Raw)
	buf := make([]byte, micHeaderSize+4*n)
	binary.LittleEndian.PutUint32(buf[0:], micMagic)
	binary.LittleEndian.PutUint32(buf[4:], uint32(b.Seq))
	binary.LittleEndian.PutUint16(buf[8:], uint16(n))
	var flags uint16
	if b.Echo {
		flags |= 1
	}
	if b.Aligned {
		flags |= 2
	}
	if b.Cancel {
		flags |= 4
	}
	if b.Speech {
		flags |= 8
	}
	binary.LittleEndian.PutUint16(buf[10:], flags)
	binary.LittleEndian.PutUint16(buf[12:], uint16(int16(clampF(b.ERLE*100, -32768, 32767))))
	binary.LittleEndian.PutUint32(buf[14:], uint32(int32(b.Delay)))
	buf[18] = byte(b.Event)
	buf[19] = byte(int8(b.Cue))
	binary.LittleEndian.PutUint16(buf[20:], uint16(clampF(b.PeakHz, 0, 65535)))
	binary.LittleEndian.PutUint16(buf[22:], uint16(clampF(b.Tonality*10000, 0, 65535)))
	binary.LittleEndian.PutUint16(buf[24:], uint16(clampF(b.RMS*65535, 0, 65535)))
	binary.LittleEndian.PutUint16(buf[26:], uint16(clampF(b.Floor*65535, 0, 65535)))
	binary.LittleEndian.PutUint16(buf[28:], uint16(clampF(b.CueScore*1000, 0, 65535)))
	off := micHeaderSize
	for _, s := range b.Raw {
		binary.LittleEndian.PutUint16(buf[off:], uint16(pcm16(s)))
		off += 2
	}
	for _, s := range b.Clean {
		binary.LittleEndian.PutUint16(buf[off:], uint16(pcm16(s)))
		off += 2
	}
	return buf
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func pcm16(s float64) int16 {
	return int16(clampF(s*32767, -32768, 32767))
}
