// SPDX-License-Identifier: GPL-2.0-or-later

package net

// Lab preview routes: token-gated, rate-limited handlers that push a WAV, LED
// frames or LCD text from the asset lab at the cabinet. Bodies are untrusted,
// so each is read through readBody's cap and decoded with trivial parsers only.
// Handlers reach the engine solely through RequestLabPreview.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
)

const (
	// maxPreviewSoundBytes is about twelve seconds of 44.1kHz 16-bit stereo,
	// longer than the preview timeout can play.
	maxPreviewSoundBytes = 2 << 20
	// maxPreviewFrameBytes holds maxPreviewFrames 64x64 frames (16,384 base64
	// bytes each) plus the JSON around them.
	maxPreviewFrameBytes = 2 << 20
	maxPreviewLCDBytes   = 16 << 10
)

const (
	maxPreviewFrames = 64
	// maxPreviewLineRunes is four screens of a 16-column panel; longer lines
	// marquee.
	maxPreviewLineRunes = 64
	// maxPreviewGlyphs is the HD44780 CGRAM slot count; more would silently
	// evict.
	maxPreviewGlyphs = 8
	// maxGlyphRow is the largest 5-pixel CGRAM row; the chip ignores higher
	// bits.
	maxGlyphRow = 0x1f
	glyphRows   = 8
)

// These routes drive physical hardware, so the ceiling is set by how fast a
// person can look at previews.
const (
	// previewBurst fits a frame, an LCD caption and a sound pushed twice.
	previewBurst           = 6
	previewRefillPerSecond = 3
)

// tokenBucket rate-limits a family of routes. The preview routes share one
// bucket and the flight-dump route has its own, so previews cannot lock out
// a dump. The clock is wall time, injectable for tests.
type tokenBucket struct {
	mu     sync.Mutex
	burst  float64
	refill float64 // tokens per second
	tokens float64
	last   time.Time
	now    func() time.Time
}

func newTokenBucket(burst, refillPerSecond float64) *tokenBucket {
	return &tokenBucket{burst: burst, refill: refillPerSecond, tokens: burst, now: time.Now}
}

func (l *tokenBucket) allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !l.last.IsZero() {
		l.tokens = min(l.burst, l.tokens+now.Sub(l.last).Seconds()*l.refill)
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

func (s *Server) previewGate(r *http.Request, kind string) (time.Duration, *rejection) {
	if !s.previewLimit.allow() {
		return 0, reject(http.StatusTooManyRequests, "too many preview requests: slow down").
			because("preview %s rejected: rate limited", kind).retry("1")
	}
	timeout, err := previewTimeout(r)
	if err != nil {
		return 0, invalid("preview "+kind, err)
	}
	return timeout, nil
}

// previewTimeout reads ?ms=. Absent or 0 means the engine default; the engine
// clamps the rest.
func previewTimeout(r *http.Request) (time.Duration, error) {
	v := r.URL.Query().Get("ms")
	if v == "" {
		return 0, nil
	}
	ms, err := strconv.Atoi(v)
	if err != nil || ms < 0 {
		return 0, fmt.Errorf("invalid ms=%q: want a non-negative integer number of milliseconds", v)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// previewResponse is the body of every accepted preview. TimeoutMs is the
// post-clamp value the engine will use.
type previewResponse struct {
	Kind      string  `json:"kind"`
	TimeoutMs int64   `json:"timeoutMs"`
	Frames    int     `json:"frames,omitempty"`
	Bytes     int     `json:"bytes,omitempty"`
	Gain      float64 `json:"gain,omitempty"`
	Glyphs    int     `json:"glyphs,omitempty"`
}

// accepted answers 202 because the preview is only queued for the next tick.
func (s *Server) accepted(w http.ResponseWriter, resp previewResponse, detail string) {
	s.ring.Append("info", fmt.Sprintf("http: preview %s accepted: %s (%dms)", resp.Kind, detail, resp.TimeoutMs))
	writeJSON(w, http.StatusAccepted, resp)
}

type previewFramesRequest struct {
	Frames []previewFrameSpec `json:"frames"`
}

type previewFrameSpec struct {
	// RGB is base64 of exactly cols*rows*3 row-major bytes, no alpha.
	RGB string `json:"rgb"`
	// DelayMs of 0 takes the engine default.
	DelayMs int `json:"delayMs"`
}

// handleFramePreview serves POST /api/preview/frame. A non-JSON body is one
// raw cols*rows*3 RGB frame; application/json carries {"frames":[{"rgb":
// "<base64>","delayMs":100}]}. ?loop=1 repeats a sequence until the timeout.
func (s *Server) handleFramePreview(w http.ResponseWriter, r *http.Request) *rejection {
	timeout, rej := s.previewGate(r, "frame")
	if rej != nil {
		return rej
	}
	body, rej := readBody(w, r, "preview frame", maxPreviewFrameBytes)
	if rej != nil {
		return rej
	}

	cols, rows := s.displaySize()
	var frames []engine.LabPreviewFrame
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		frames, err = decodeFrameSequence(body, cols, rows)
	} else {
		var one engine.LabPreviewFrame
		one, err = decodeRawFrame(body, cols, rows)
		frames = []engine.LabPreviewFrame{one}
	}
	if err != nil {
		return invalid("preview frame", err)
	}

	p := engine.LabPreview{
		Frames:  frames,
		Loop:    r.URL.Query().Get("loop") == "1",
		Timeout: timeout,
	}
	s.rt.RequestLabPreview(p)
	s.accepted(w, previewResponse{
		Kind:      "frame",
		TimeoutMs: engine.ClampLabPreviewTimeout(timeout).Milliseconds(),
		Frames:    len(frames),
	}, fmt.Sprintf("%d frame(s) of %dx%d", len(frames), cols, rows))
	return nil
}

func decodeRawFrame(body []byte, cols, rows int) (engine.LabPreviewFrame, error) {
	m, err := rgbToMatrix(body, cols, rows)
	if err != nil {
		return engine.LabPreviewFrame{}, err
	}
	return engine.LabPreviewFrame{Data: m}, nil
}

func decodeFrameSequence(body []byte, cols, rows int) ([]engine.LabPreviewFrame, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var req previewFramesRequest
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid request body: %v", err)
	}
	switch {
	case len(req.Frames) == 0:
		return nil, errors.New(`body must set "frames" to at least one frame`)
	case len(req.Frames) > maxPreviewFrames:
		return nil, fmt.Errorf("too many frames: %d (max %d)", len(req.Frames), maxPreviewFrames)
	}
	out := make([]engine.LabPreviewFrame, 0, len(req.Frames))
	for i, f := range req.Frames {
		raw, err := base64.StdEncoding.DecodeString(f.RGB)
		if err != nil {
			return nil, fmt.Errorf("frame %d: rgb is not valid base64: %v", i, err)
		}
		m, err := rgbToMatrix(raw, cols, rows)
		if err != nil {
			return nil, fmt.Errorf("frame %d: %w", i, err)
		}
		if f.DelayMs < 0 {
			return nil, fmt.Errorf("frame %d: negative delayMs", i)
		}
		out = append(out, engine.LabPreviewFrame{
			Data:  m,
			Delay: time.Duration(f.DelayMs) * time.Millisecond,
		})
	}
	return out, nil
}

// rgbToMatrix expands cols*rows RGB triples into a matrix. The length must be
// exact so a wrong panel size or stray alpha channel is reported.
func rgbToMatrix(raw []byte, cols, rows int) (display.Matrix, error) {
	want := cols * rows * 3
	if len(raw) != want {
		return nil, fmt.Errorf("frame is %d bytes, want exactly %d (%dx%d RGB, 3 bytes per pixel)",
			len(raw), want, cols, rows)
	}
	m := make(display.Matrix, rows)
	i := 0
	for y := 0; y < rows; y++ {
		row := make([]display.Pixel, cols)
		for x := 0; x < cols; x++ {
			row[x] = display.RGB(raw[i], raw[i+1], raw[i+2])
			i += 3
		}
		m[y] = row
	}
	return m, nil
}

// handleSoundPreview serves POST /api/preview/sound: the body is a WAV played
// once over the running app. ?gain= is a 0..1 linear attenuation. The bytes
// are not parsed here; the audio backend's decoder is the only one.
func (s *Server) handleSoundPreview(w http.ResponseWriter, r *http.Request) *rejection {
	timeout, rej := s.previewGate(r, "sound")
	if rej != nil {
		return rej
	}
	gain, err := previewGain(r)
	if err != nil {
		return invalid("preview sound", err)
	}
	body, rej := readBody(w, r, "preview sound", maxPreviewSoundBytes)
	if rej != nil {
		return rej
	}
	if len(body) == 0 {
		return reject(http.StatusBadRequest, "empty body: POST the WAV file's bytes").because("preview sound rejected: empty body")
	}

	s.rt.RequestLabPreview(engine.LabPreview{
		Sound:   &engine.LabPreviewSound{WAV: body, Gain: gain},
		Timeout: timeout,
	})
	applied := gain
	if applied == 0 {
		applied = engine.DefaultLabPreviewGain
	}
	s.accepted(w, previewResponse{
		Kind:      "sound",
		TimeoutMs: engine.ClampLabPreviewTimeout(timeout).Milliseconds(),
		Bytes:     len(body),
		Gain:      applied,
	}, fmt.Sprintf("%d byte wav @%.2f gain", len(body), applied))
	return nil
}

// previewGain reads ?gain=. Absent means the engine default; out-of-range is
// refused rather than clamped.
func previewGain(r *http.Request) (float64, error) {
	v := r.URL.Query().Get("gain")
	if v == "" {
		return 0, nil
	}
	g, err := strconv.ParseFloat(v, 64)
	if err != nil || g < 0 || g > 1 {
		return 0, fmt.Errorf("invalid gain=%q: want a number from 0 to 1", v)
	}
	return g, nil
}

type previewLCDRequest struct {
	// Line1 and Line2 write both seats' panels.
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
	// Seats writes the panels individually: exactly two entries, player 1
	// then 2. Exclusive with Line1/Line2.
	Seats []previewLCDSeat `json:"seats"`
	// Glyphs are 5x8 CGRAM bitmaps keyed by the rune each draws: eight row
	// values, top to bottom, 0..31.
	Glyphs map[string][]int `json:"glyphs"`
}

type previewLCDSeat struct {
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
}

// handleLCDPreview serves POST /api/preview/lcd. The engine restores the
// app's panel contents when the preview expires.
func (s *Server) handleLCDPreview(w http.ResponseWriter, r *http.Request) *rejection {
	timeout, rej := s.previewGate(r, "lcd")
	if rej != nil {
		return rej
	}
	var req previewLCDRequest
	if rej := decodeBody(w, r, "preview lcd", maxPreviewLCDBytes, &req); rej != nil {
		return rej
	}
	lcd, err := buildPreviewLCD(req)
	if err != nil {
		return invalid("preview lcd", err)
	}

	s.rt.RequestLabPreview(engine.LabPreview{LCD: lcd, Timeout: timeout})
	s.accepted(w, previewResponse{
		Kind:      "lcd",
		TimeoutMs: engine.ClampLabPreviewTimeout(timeout).Milliseconds(),
		Glyphs:    len(lcd.Glyphs),
	}, fmt.Sprintf("%q / %q + %d glyph(s)", lcd.Lines[0][0], lcd.Lines[0][1], len(lcd.Glyphs)))
	return nil
}

func buildPreviewLCD(req previewLCDRequest) (*engine.LabPreviewLCD, error) {
	out := &engine.LabPreviewLCD{}
	switch {
	case len(req.Seats) > 0 && (req.Line1 != "" || req.Line2 != ""):
		return nil, errors.New(`set either "line1"/"line2" (both panels) or "seats" (one entry per panel), not both`)
	case len(req.Seats) > 0:
		if len(req.Seats) != 2 {
			return nil, fmt.Errorf(`"seats" has %d entries, want exactly 2 (player 1 then player 2)`, len(req.Seats))
		}
		for i, seat := range req.Seats {
			l1, err := checkPreviewLine(seat.Line1)
			if err != nil {
				return nil, fmt.Errorf("seat %d line1: %w", i, err)
			}
			l2, err := checkPreviewLine(seat.Line2)
			if err != nil {
				return nil, fmt.Errorf("seat %d line2: %w", i, err)
			}
			out.Lines[i] = [2]string{l1, l2}
		}
	default:
		l1, err := checkPreviewLine(req.Line1)
		if err != nil {
			return nil, fmt.Errorf("line1: %w", err)
		}
		l2, err := checkPreviewLine(req.Line2)
		if err != nil {
			return nil, fmt.Errorf("line2: %w", err)
		}
		out.Lines[0] = [2]string{l1, l2}
		out.Lines[1] = [2]string{l1, l2}
	}

	glyphs, err := buildPreviewGlyphs(req.Glyphs)
	if err != nil {
		return nil, err
	}
	out.Glyphs = glyphs
	return out, nil
}

// checkPreviewLine allows any Unicode, since glyph previews send runes the
// panel lacks, but refuses invalid UTF-8 and control characters, which the
// HD44780 would take as commands.
func checkPreviewLine(line string) (string, error) {
	if !utf8.ValidString(line) {
		return "", errors.New("not valid UTF-8")
	}
	if n := utf8.RuneCountInString(line); n > maxPreviewLineRunes {
		return "", fmt.Errorf("%d characters is longer than the %d character limit", n, maxPreviewLineRunes)
	}
	for _, c := range line {
		if unicode.IsControl(c) {
			return "", fmt.Errorf("contains control character %q", c)
		}
	}
	return line, nil
}

func buildPreviewGlyphs(in map[string][]int) (map[rune]hardware.GlyphRows, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > maxPreviewGlyphs {
		return nil, fmt.Errorf("too many glyphs: %d (the HD44780 holds %d custom characters at once)",
			len(in), maxPreviewGlyphs)
	}
	out := make(map[rune]hardware.GlyphRows, len(in))
	for key, rows := range in {
		r, size := utf8.DecodeRuneInString(key)
		if r == utf8.RuneError || size != len(key) {
			return nil, fmt.Errorf("glyph key %q must be exactly one character", key)
		}
		if len(rows) != glyphRows {
			return nil, fmt.Errorf("glyph %q has %d rows, want exactly %d", key, len(rows), glyphRows)
		}
		var bitmap hardware.GlyphRows
		for i, v := range rows {
			if v < 0 || v > maxGlyphRow {
				return nil, fmt.Errorf("glyph %q row %d is %d, want 0..%d (5 pixels wide)", key, i, v, maxGlyphRow)
			}
			bitmap[i] = byte(v)
		}
		out[r] = bitmap
	}
	return out, nil
}

// handleCancelPreview serves DELETE /api/preview. It is not rate limited, and
// cancelling with nothing previewing still succeeds.
func (s *Server) handleCancelPreview(w http.ResponseWriter, r *http.Request) *rejection {
	s.rt.CancelLabPreview()
	s.ring.Append("info", "http: preview cancel requested")
	writeJSON(w, http.StatusAccepted, map[string]string{"kind": "cancel"})
	return nil
}

func (s *Server) displaySize() (cols, rows int) {
	st := s.rt.Status()
	if st.DisplayCols > 0 && st.DisplayRows > 0 {
		return st.DisplayCols, st.DisplayRows
	}
	return display.DefaultWidth, display.DefaultHeight
}
