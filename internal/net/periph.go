// SPDX-License-Identifier: GPL-2.0-or-later

package net

// Peripherals-lab routes: an SSE stream of observed input, lamps, motion and
// mic level, and a lamp choreography endpoint. Both are token-gated, and the
// lights route shares the preview rate limiter. Nothing here calls into the
// engine loop; the stream polls concurrency-safe readers on its own goroutine.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

const (
	maxPeriphStreams = 2
	// defaultPeriphHz sees nearly every change of the 30fps engine publish.
	defaultPeriphHz = 20
	// A faster poll than the engine publishes only re-sends.
	minPeriphHz = 1
	maxPeriphHz = 60
	// The stream duration cap keeps a forgotten tab from holding observation
	// armed; a longer watch reconnects.
	defaultPeriphStreamMs = 5 * 60 * 1000
	maxPeriphStreamMs     = 30 * 60 * 1000
	// periphPingEvery keeps proxies and the SSH tunnel from reaping an idle stream.
	periphPingEvery = 15 * time.Second
	// periphWriteTimeout drops a client that is not reading.
	periphWriteTimeout = 5 * time.Second
	// maxPeriphInputsPerPoll bounds the first packet after a long stall.
	maxPeriphInputsPerPoll = 64
)

const maxPeriphLightsBytes = 16 << 10

// maxPeriphLightFadeMs caps a step's fade; larger values are repaired, not refused.
const maxPeriphLightFadeMs = 10000

// handlePeriphStream serves the observation stream as Server-Sent Events.
// Query: ?hz= poll rate (default 20, 1..60), ?ms= duration (default 5 minutes,
// max 30), ?mic=0 to omit the mic channel. Events: "hello" on connect, then
// "input", "lights", "motion", "micinfo" and "mic" as they change, "ping" as
// keepalive, and "bye" with a reason when the server closes.
func (s *Server) handlePeriphStream(w http.ResponseWriter, r *http.Request) *rejection {
	hz, dur, wantMic, err := periphStreamParams(r)
	if err != nil {
		return invalid("periph stream", err)
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return noStreaming()
	}
	if n := s.periphStreams.Add(1); n > maxPeriphStreams {
		s.periphStreams.Add(-1)
		return reject(http.StatusTooManyRequests, fmt.Sprintf("too many peripheral streams open (max %d): close one first", maxPeriphStreams)).
			because("periph stream rejected: %d already open (max %d)", n-1, maxPeriphStreams).retry("5")
	}
	defer s.periphStreams.Add(-1)

	release := s.rt.WatchPeriph()
	defer release()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	// Stops a proxy from buffering the stream.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	st := s.rt.Status()
	sw := &periphWriter{w: w, flusher: flusher, rc: http.NewResponseController(w)}
	if !sw.send("hello", periphHello{
		Version:    s.version,
		PollHz:     hz,
		DurationMs: dur.Milliseconds(),
		Mic:        wantMic,
		Slug:       st.Slug,
		Standby:    st.StandbyState,
		Buttons:    hardware.LightButtons,
		LevelMax:   hardware.LampLevelMax,
	}) {
		return nil
	}
	s.ring.Append("info", fmt.Sprintf("http: periph stream opened (%dHz, %s%s)",
		hz, dur.Round(time.Second), micNote(wantMic)))

	s.pumpPeriphStream(r, sw, hz, dur, wantMic)
	return nil
}

func micNote(on bool) string {
	if on {
		return ", mic"
	}
	return ", no mic"
}

func (s *Server) pumpPeriphStream(r *http.Request, sw *periphWriter, hz int, dur time.Duration, wantMic bool) {
	ticker := time.NewTicker(time.Second / time.Duration(hz))
	defer ticker.Stop()
	deadline := time.NewTimer(dur)
	defer deadline.Stop()

	var (
		cursor     uint64
		lastLamps  [2]hardware.LightMask
		lastLevels [2]hardware.LampLevels
		lastShow   bool
		lastMotion bool
		lastMicSeq uint64
		haveState  bool
		lastPing   = time.Now()
	)

	for {
		select {
		case <-r.Context().Done():
			s.ring.Append("info", "http: periph stream closed by client")
			return
		case <-deadline.C:
			sw.send("bye", map[string]string{"reason": "duration limit reached: reconnect to keep watching"})
			s.ring.Append("info", "http: periph stream closed: duration limit")
			return
		case <-ticker.C:
		}

		// Input goes first because it is the channel that can lose events.
		events, next, missed := s.rt.DrainPeriphInputs(cursor)
		cursor = next
		if len(events) > 0 || missed > 0 {
			if len(events) > maxPeriphInputsPerPoll {
				missed += uint64(len(events) - maxPeriphInputsPerPoll)
				events = events[len(events)-maxPeriphInputsPerPoll:]
			}
			if !sw.send("input", periphInputBatch{Events: events, Missed: missed}) {
				return
			}
		}

		snap := s.rt.PeriphSnapshot()
		st := s.rt.Status()
		if !snap.Watching {
			// Armed, but the engine has not published a frame yet.
			continue
		}

		if !haveState || snap.Lights != lastLamps || snap.Levels != lastLevels || snap.LightShowActive != lastShow {
			lastLamps, lastLevels, lastShow = snap.Lights, snap.Levels, snap.LightShowActive
			if !sw.send("lights", newPeriphLights(snap)) {
				return
			}
		}
		if !haveState || st.MotionActive != lastMotion {
			lastMotion = st.MotionActive
			if !sw.send("motion", periphMotion{Active: st.MotionActive, AgoMs: st.MotionAgoMs}) {
				return
			}
		}
		if !haveState && wantMic {
			if !sw.send("micinfo", snap.MicInfo) {
				return
			}
		}
		if wantMic && snap.MicOK && (!haveState || snap.Mic.Seq != lastMicSeq) {
			lastMicSeq = snap.Mic.Seq
			if !sw.send("mic", periphMic{MicLevel: snap.Mic, Synthetic: snap.MicInfo.Synthetic}) {
				return
			}
		}
		haveState = true

		if now := time.Now(); now.Sub(lastPing) >= periphPingEvery {
			lastPing = now
			if !sw.send("ping", map[string]any{"frame": snap.Frame, "dropped": snap.Dropped}) {
				return
			}
		}
	}
}

// periphHello is the first event on every stream. The capture path is not in
// it because no snapshot exists until a tick after arming; it follows as a
// "micinfo" event.
type periphHello struct {
	Version    string   `json:"version"`
	PollHz     int      `json:"pollHz"`
	DurationMs int64    `json:"durationMs"`
	Mic        bool     `json:"mic"`
	Slug       string   `json:"slug"`
	Standby    string   `json:"standbyState"`
	Buttons    []string `json:"buttons"`
	// LevelMax is hardware.LampLevelMax, the top of the lights level scale.
	LevelMax int `json:"levelMax"`
}

type periphInputBatch struct {
	Events []engine.PeriphInput `json:"events"`
	// Missed counts records lost before this batch, to ring overwrite or the
	// per-poll cap.
	Missed uint64 `json:"missed,omitempty"`
}

type periphLights struct {
	Seats [2][]string `json:"seats"`
	Masks [2]uint8    `json:"masks"`
	// Levels is per-lamp brightness, 0..hardware.LampLevelMax.
	Levels [2][6]uint8 `json:"levels"`
	// Show reports that a choreography owns the lamps. Seats, Masks and
	// Levels then describe the app's state and ShowSeats and ShowMasks what
	// is lit.
	Show      bool        `json:"show"`
	ShowStep  int         `json:"showStep,omitempty"`
	ShowSeats [2][]string `json:"showSeats,omitempty"`
	ShowMasks [2]uint8    `json:"showMasks,omitempty"`
}

func newPeriphLights(snap engine.PeriphSnapshot) periphLights {
	out := periphLights{
		Seats:  [2][]string{snap.Lights[0].Names(), snap.Lights[1].Names()},
		Masks:  [2]uint8{uint8(snap.Lights[0]), uint8(snap.Lights[1])},
		Levels: [2][6]uint8{[6]uint8(snap.Levels[0]), [6]uint8(snap.Levels[1])},
		Show:   snap.LightShowActive,
	}
	if snap.LightShowActive {
		out.ShowStep = snap.LightShowStep
		out.ShowSeats = [2][]string{snap.LightShowMask[0].Names(), snap.LightShowMask[1].Names()}
		out.ShowMasks = [2]uint8{uint8(snap.LightShowMask[0]), uint8(snap.LightShowMask[1])}
	}
	return out
}

// periphMotion carries the same conflated signal as engine.Status.MotionActive:
// a PIR edge on the cabinet, any input at a desk.
type periphMotion struct {
	Active bool     `json:"active"`
	AgoMs  *float64 `json:"agoMs,omitempty"`
}

type periphMic struct {
	hardware.MicLevel
	Synthetic bool `json:"synthetic"`
}

type periphWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	rc      *http.ResponseController
	buf     bytes.Buffer
}

func (p *periphWriter) send(event string, payload any) bool {
	body, err := json.Marshal(payload)
	if err != nil {
		return false // unreachable: every payload here is a plain struct
	}
	p.buf.Reset()
	p.buf.WriteString("event: ")
	p.buf.WriteString(event)
	p.buf.WriteString("\ndata: ")
	p.buf.Write(body)
	p.buf.WriteString("\n\n")
	// Some wrapped ResponseWriters reject deadlines; the write is then unbounded.
	_ = p.rc.SetWriteDeadline(time.Now().Add(periphWriteTimeout))
	if _, err := p.w.Write(p.buf.Bytes()); err != nil {
		return false
	}
	p.flusher.Flush()
	return true
}

func periphStreamParams(r *http.Request) (hz int, dur time.Duration, mic bool, err error) {
	hz = defaultPeriphHz
	if v := r.URL.Query().Get("hz"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < minPeriphHz || n > maxPeriphHz {
			return 0, 0, false, fmt.Errorf("invalid hz=%q: want an integer from %d to %d", v, minPeriphHz, maxPeriphHz)
		}
		hz = n
	}
	ms := defaultPeriphStreamMs
	if v := r.URL.Query().Get("ms"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n <= 0 || n > maxPeriphStreamMs {
			return 0, 0, false, fmt.Errorf("invalid ms=%q: want a positive integer up to %d", v, maxPeriphStreamMs)
		}
		ms = n
	}
	mic = r.URL.Query().Get("mic") != "0"
	return hz, time.Duration(ms) * time.Millisecond, mic, nil
}

type periphLightsRequest struct {
	// Steps holds one to engine.MaxLightShowSteps steps.
	Steps []periphLightStep `json:"steps"`
	// Loop repeats the pattern until the timeout instead of holding the last step.
	Loop bool `json:"loop"`
}

type periphLightStep struct {
	// Seats holds exactly two lists of hardware.LightButtons names, player 1
	// then 2; an empty list is a dark seat.
	Seats [][]string `json:"seats"`
	// HoldMs of 0 takes the engine default.
	HoldMs int `json:"holdMs"`
	// FadeMs is the cross-fade in; 0 is a hard cut. Negative becomes 0 and
	// values past 10 seconds are capped.
	FadeMs int `json:"fadeMs,omitempty"`
}

// handlePeriphLights serves POST /api/periph/lights: a bounded lamp
// choreography, e.g. {"loop":true,"steps":[{"seats":[["joya"],["joyd"]],
// "holdMs":120}]}. ?ms= sets the timeout. The engine restores the app's lamps
// when the show ends.
func (s *Server) handlePeriphLights(w http.ResponseWriter, r *http.Request) *rejection {
	timeout, rej := s.previewGate(r, "lights")
	if rej != nil {
		return rej
	}
	var req periphLightsRequest
	if rej := decodeBody(w, r, "periph lights", maxPeriphLightsBytes, &req); rej != nil {
		return rej
	}
	steps, err := buildLightSteps(req.Steps)
	if err != nil {
		return invalid("periph lights", err)
	}

	s.rt.RequestLightShow(engine.LightShow{Steps: steps, Loop: req.Loop, Timeout: timeout})
	applied := engine.ClampLightShowTimeout(timeout).Milliseconds()
	s.ring.Append("info", fmt.Sprintf("http: periph lights accepted: %d step(s)%s (%dms)",
		len(steps), loopSuffix(req.Loop), applied))
	writeJSON(w, http.StatusAccepted, periphLightsResponse{
		Kind:      "lights",
		TimeoutMs: applied,
		Steps:     len(steps),
		Loop:      req.Loop,
	})
	return nil
}

type periphLightsResponse struct {
	Kind      string `json:"kind"`
	TimeoutMs int64  `json:"timeoutMs"`
	Steps     int    `json:"steps"`
	Loop      bool   `json:"loop"`
}

func loopSuffix(loop bool) string {
	if loop {
		return " looping"
	}
	return ""
}

func buildLightSteps(in []periphLightStep) ([]engine.LightStep, error) {
	switch {
	case len(in) == 0:
		return nil, errors.New(`body must set "steps" to at least one step`)
	case len(in) > engine.MaxLightShowSteps:
		return nil, fmt.Errorf("too many steps: %d (max %d)", len(in), engine.MaxLightShowSteps)
	}
	out := make([]engine.LightStep, 0, len(in))
	for i, st := range in {
		if len(st.Seats) != 2 {
			return nil, fmt.Errorf(`step %d: "seats" has %d entries, want exactly 2 (player 1 then player 2)`, i, len(st.Seats))
		}
		if st.HoldMs < 0 {
			return nil, fmt.Errorf("step %d: negative holdMs", i)
		}
		var step engine.LightStep
		for seat, names := range st.Seats {
			mask, err := hardware.LightMaskFromNames(names)
			if err != nil {
				return nil, fmt.Errorf("step %d seat %d: %w", i, seat, err)
			}
			step.Seats[seat] = mask
		}
		step.Hold = time.Duration(st.HoldMs) * time.Millisecond
		fadeMs := st.FadeMs
		if fadeMs < 0 {
			fadeMs = 0
		} else if fadeMs > maxPeriphLightFadeMs {
			fadeMs = maxPeriphLightFadeMs
		}
		step.Fade = time.Duration(fadeMs) * time.Millisecond
		out = append(out, step)
	}
	return out, nil
}

// handleCancelPeriphLights serves DELETE /api/periph/lights. It is not rate
// limited, and cancelling with nothing playing still succeeds.
func (s *Server) handleCancelPeriphLights(w http.ResponseWriter, r *http.Request) *rejection {
	s.rt.CancelLightShow()
	s.ring.Append("info", "http: periph lights cancel requested")
	writeJSON(w, http.StatusAccepted, map[string]string{"kind": "cancel"})
	return nil
}
