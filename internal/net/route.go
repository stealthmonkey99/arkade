// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// handler serves one route. It writes its own success response and returns
// nil, or returns the rejection for the route wrapper to answer and audit.
type handler func(w http.ResponseWriter, r *http.Request) *rejection

// route is one mux entry. gated routes pass requireToken before serve runs.
type route struct {
	pattern string
	gated   bool
	serve   handler
}

// readOnly is an unauthenticated route. Only GET may be unauthenticated, so
// any other method panics at construction.
func readOnly(pattern string, h handler) route {
	if !strings.HasPrefix(pattern, http.MethodGet+" ") {
		panic("net: unauthenticated route must be GET: " + pattern)
	}
	return route{pattern: pattern, serve: h}
}

// mutating is a route that changes the cabinet; it requires the bearer token.
func mutating(pattern string, h handler) route {
	return route{pattern: pattern, gated: true, serve: h}
}

// streaming is a GET that exposes live input or audio, so it is gated like a
// mutating route.
func streaming(pattern string, h handler) route {
	return route{pattern: pattern, gated: true, serve: h}
}

func (s *Server) routes() []route {
	return []route{
		readOnly("GET /api/status", s.handleStatus),
		readOnly("GET /api/logs", s.handleLogs),
		readOnly("GET /api/scores/{app...}", s.handleScoresOne),
		readOnly("GET /api/scores", s.handleScoresAll),
		readOnly("GET /api/games", s.handleGames),
		readOnly("GET /api/screensavers", s.handleScreensavers),
		readOnly("GET /api/mic/status", s.handleMicStatus),
		readOnly("GET /{$}", s.handleDashboard),

		mutating("PUT /api/scores/{app...}", s.handleScoresWrite),
		mutating("PUT /api/screensavers/{slug...}", s.handleScreensaverWrite),
		mutating("POST /api/screensavers/{slug...}", s.handleScreensaverPost),
		mutating("POST /api/cartridges", s.handleInstallCartridge),
		mutating("POST /api/launch/{slug...}", s.handleLaunch),
		mutating("POST /api/attract/{slug...}", s.handleAttract),
		// {variant} comes first so the trailing wildcard can swallow a slug's slash.
		mutating("POST /api/variants/{variant}/{slug...}", s.handleSetVariant),
		mutating("POST /api/preview/frame", s.handleFramePreview),
		mutating("POST /api/preview/sound", s.handleSoundPreview),
		mutating("POST /api/preview/lcd", s.handleLCDPreview),
		mutating("DELETE /api/preview", s.handleCancelPreview),
		mutating("POST /api/flight/dump", s.handleFlightDump),
		mutating("POST /api/periph/lights", s.handlePeriphLights),
		mutating("DELETE /api/periph/lights", s.handleCancelPeriphLights),

		streaming("GET /api/periph/stream", s.handlePeriphStream),
		streaming("GET /api/mic/stream", s.handleMicStream),
	}
}

func (s *Server) mux() *http.ServeMux {
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.HandleFunc(rt.pattern, s.wrap(rt))
	}
	return mux
}

func (s *Server) wrap(rt route) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rt.gated {
			if rej := s.requireToken(r); rej != nil {
				s.refuse(w, rej)
				return
			}
		}
		if rej := rt.serve(w, r); rej != nil {
			s.refuse(w, rej)
		}
	}
}

// rejection is a refused request: the status and text the caller sees, and
// the line the audit ring keeps.
type rejection struct {
	status int
	text   string
	level  string
	audit  string
	// kind coalesces the audit line through rejectLog, for floods of
	// unauthenticated traffic.
	kind       string
	retryAfter string
}

// reject writes no audit line until because or failed adds one.
func reject(status int, text string) *rejection {
	return &rejection{status: status, text: text}
}

// because adds a warn audit line: the caller sent something unacceptable.
func (rej *rejection) because(format string, args ...any) *rejection {
	rej.level, rej.audit = "warn", "http: "+fmt.Sprintf(format, args...)
	return rej
}

// failed adds an error audit line: the cabinet could not do what was asked.
func (rej *rejection) failed(format string, args ...any) *rejection {
	rej.level, rej.audit = "error", "http: "+fmt.Sprintf(format, args...)
	return rej
}

func (rej *rejection) retry(seconds string) *rejection {
	rej.retryAfter = seconds
	return rej
}

func (s *Server) refuse(w http.ResponseWriter, rej *rejection) {
	switch {
	case rej.kind != "":
		s.rejects.append(s.ring, rej.kind, rej.audit)
	case rej.audit != "":
		s.ring.Append(rej.level, rej.audit)
	}
	if rej.retryAfter != "" {
		w.Header().Set("Retry-After", rej.retryAfter)
	}
	http.Error(w, rej.text, rej.status)
}

// readBody reads a body capped at limit, refusing 413 when over and 400 when
// unreadable; what names the request in the audit line.
func readBody(w http.ResponseWriter, r *http.Request, what string, limit int64) ([]byte, *rejection) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, reject(http.StatusRequestEntityTooLarge, fmt.Sprintf("request body too large (max %d bytes)", limit)).
				because("%s rejected: body over the %d byte cap", what, limit)
		}
		return nil, reject(http.StatusBadRequest, "reading request body").
			because("%s rejected: reading body: %v", what, err)
	}
	return body, nil
}

// decodeBody decodes one JSON value into v, refusing unknown fields.
func decodeBody(w http.ResponseWriter, r *http.Request, what string, limit int64, v any) *rejection {
	body, rej := readBody(w, r, what, limit)
	if rej != nil {
		return rej
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return reject(http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err)).
			because("%s rejected: %v", what, err)
	}
	return nil
}

func invalid(what string, err error) *rejection {
	return reject(http.StatusBadRequest, err.Error()).because("%s rejected: %v", what, err)
}

func noStreaming() *rejection {
	return reject(http.StatusInternalServerError, "streaming unsupported by this server")
}
