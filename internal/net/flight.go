// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/internal/engine"
)

// A dump writes up to a couple of megabytes to the SD card, so the limit
// suits a person, not a script.
const (
	flightDumpBurst           = 3
	flightDumpRefillPerSecond = 0.5
)

const maxFlightNoteRunes = 200

// handleFlightDump serves POST /api/flight/dump: write the running cartridge's
// flight recording to disk. ?note= is a line stored in the recording header.
// It answers 200 rather than 202 because the file exists on return, and 409
// when no cartridge session is being recorded. It has its own rate limiter so
// previews cannot lock it out.
func (s *Server) handleFlightDump(w http.ResponseWriter, r *http.Request) *rejection {
	note, err := flightNote(r)
	if err != nil {
		return invalid("flight dump", err)
	}
	if !s.flightLimit.allow() {
		return reject(http.StatusTooManyRequests, "too many flight dumps: slow down").
			because("flight dump rejected: rate limited").retry("2")
	}
	out, err := s.rt.DumpFlight("manual", note)
	if err != nil {
		var none engine.ErrNoFlightSession
		if errors.As(err, &none) {
			return reject(http.StatusConflict, err.Error()).because("flight dump rejected: %v", err)
		}
		return reject(http.StatusInternalServerError, "writing the flight recording failed").failed("flight dump failed: %v", err)
	}
	s.ring.Append("info", fmt.Sprintf("http: flight dump written: %s (%s, %d events, %d bytes%s)",
		out.Path, out.Slug, out.Events, out.Bytes, truncatedSuffix(out.Truncated)))
	writeJSON(w, http.StatusOK, out)
	return nil
}

func truncatedSuffix(truncated bool) string {
	if truncated {
		return ", TRUNCATED"
	}
	return ""
}

// flightNote reads and validates ?note=. Control characters are refused
// because a newline would split the audit line.
func flightNote(r *http.Request) (string, error) {
	note := strings.TrimSpace(r.URL.Query().Get("note"))
	if note == "" {
		return "", nil
	}
	if !utf8.ValidString(note) {
		return "", errors.New("invalid note: not valid UTF-8")
	}
	if n := utf8.RuneCountInString(note); n > maxFlightNoteRunes {
		return "", fmt.Errorf("invalid note: %d characters is longer than the %d character limit",
			n, maxFlightNoteRunes)
	}
	for _, c := range note {
		if unicode.IsControl(c) {
			return "", fmt.Errorf("invalid note: contains control character %q", c)
		}
	}
	return note, nil
}
