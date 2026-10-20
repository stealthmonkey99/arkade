// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/weather"
)

// Location modes as apps/morning writes them.
const (
	screensaverAuto   = "auto"
	screensaverManual = "manual"
)

// screensaverFixLabel is where a screensaver caches its resolved fix.
const screensaverFixLabel = "location"

type screensaverInfo struct {
	Slug     string          `json:"slug"`
	Name     string          `json:"name"`
	Location string          `json:"location,omitempty"`
	Lat      *float64        `json:"lat,omitempty"`
	Lon      *float64        `json:"lon,omitempty"`
	Fix      *screensaverFix `json:"fix,omitempty"`
}

type screensaverFix struct {
	City   string    `json:"city,omitempty"`
	Lat    float64   `json:"lat"`
	Lon    float64   `json:"lon"`
	Source string    `json:"source,omitempty"`
	At     time.Time `json:"at"`
}

type screensaverLocation struct {
	Location string   `json:"location"`
	Lat      *float64 `json:"lat"`
	Lon      *float64 `json:"lon"`
}

// screensaverMode mirrors the app-side default: a file with coordinates but no
// location key is manual.
func screensaverMode(stored string, hasCoords bool) string {
	switch stored {
	case screensaverAuto, screensaverManual:
		return stored
	}
	if hasCoords {
		return screensaverManual
	}
	return screensaverAuto
}

func screensaverManifest(slug string) (engine.Manifest, bool) {
	m, err := engine.Lookup(slug)
	if err != nil || m.Type != engine.TypeScreensaver {
		return engine.Manifest{}, false
	}
	return m, true
}

func (s *Server) readScreensaverLocation(slug string) (screensaverLocation, error) {
	var loc screensaverLocation
	if err := s.store.Read(slug, "", &loc); err != nil {
		return loc, err
	}
	loc.Location = screensaverMode(loc.Location, loc.Lat != nil && loc.Lon != nil)
	return loc, nil
}

func (s *Server) readScreensaverFix(slug string) *screensaverFix {
	var fix weather.Location
	if err := s.store.Read(slug, screensaverFixLabel, &fix); err != nil || fix.At.IsZero() {
		return nil
	}
	return &screensaverFix{City: fix.City, Lat: fix.Lat, Lon: fix.Lon, Source: fix.Source, At: fix.At}
}

// handleScreensavers serves GET /api/screensavers. It is unauthenticated, like
// the other read routes.
func (s *Server) handleScreensavers(w http.ResponseWriter, r *http.Request) *rejection {
	manifests := engine.Screensavers()
	out := make([]screensaverInfo, 0, len(manifests))
	for _, m := range manifests {
		loc, err := s.readScreensaverLocation(m.Slug)
		if err != nil {
			return reject(http.StatusInternalServerError, "reading screensaver settings").
				failed("screensaver %q settings unreadable: %v", m.Slug, err)
		}
		out = append(out, screensaverInfo{
			Slug: m.Slug, Name: m.Name, Location: loc.Location,
			Lat: loc.Lat, Lon: loc.Lon, Fix: s.readScreensaverFix(m.Slug),
		})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

const maxScreensaverWriteBytes = 1 << 10

// screensaverWriteRequest keeps coordinates raw so an absent key (leave alone)
// differs from null (clear).
type screensaverWriteRequest struct {
	Location *string         `json:"location"`
	Lat      json.RawMessage `json:"lat"`
	Lon      json.RawMessage `json:"lon"`
}

func coordinate(raw json.RawMessage) (present bool, v *float64, err error) {
	if raw == nil {
		return false, nil, nil
	}
	if string(raw) == "null" {
		return true, nil, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return true, nil, err
	}
	return true, &f, nil
}

func docFloat(doc map[string]json.RawMessage, key string) *float64 {
	raw, ok := doc[key]
	if !ok {
		return nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil
	}
	return &f
}

// handleScreensaverWrite serves PUT and POST /api/screensavers/{slug}: set the
// location mode and write or clear the manual coordinates. Bodies:
//
//	{"location":"auto"}                          coordinates untouched
//	{"location":"manual","lat":37.77,"lon":-122} use these
//	{"location":"manual","lat":null,"lon":null}  manual with nothing set
//	{"lat":37.77,"lon":-122.42}                  implies manual
//
// It does not call InvalidateSchedules, which is unsafe off the engine
// goroutine and unneeded for these keys.
func (s *Server) handleScreensaverWrite(w http.ResponseWriter, r *http.Request) *rejection {
	slug := r.PathValue("slug")
	what := fmt.Sprintf("screensaver location %q", slug)
	m, err := engine.Lookup(slug)
	if err != nil {
		return reject(http.StatusNotFound, "unknown slug").because("%s rejected: not registered", what)
	}
	if m.Type != engine.TypeScreensaver {
		return reject(http.StatusNotFound, "not a screensaver").because("%s rejected: not a screensaver", what)
	}

	var req screensaverWriteRequest
	if rej := decodeBody(w, r, what, maxScreensaverWriteBytes, &req); rej != nil {
		return rej
	}
	latSet, lat, latErr := coordinate(req.Lat)
	lonSet, lon, lonErr := coordinate(req.Lon)
	if latErr != nil || lonErr != nil {
		return reject(http.StatusBadRequest, "lat and lon must be numbers, or null to clear the location").
			because("%s rejected: lat/lon must be numbers or null", what)
	}
	if latSet != lonSet || (lat == nil) != (lon == nil) {
		return reject(http.StatusBadRequest, "lat and lon must both be set, or both null to clear the location").
			because("%s rejected: only one of lat/lon set", what)
	}

	mode := screensaverManual
	switch {
	case req.Location != nil:
		mode = *req.Location
		if mode != screensaverAuto && mode != screensaverManual {
			return reject(http.StatusBadRequest, `location must be "auto" or "manual"`).
				because("%s rejected: unknown mode %q", what, mode)
		}
	case !latSet:
		return reject(http.StatusBadRequest, "send a location mode, a lat/lon pair, or both").
			because("%s rejected: empty body", what)
	}

	if lat != nil && (*lat < -90 || *lat > 90) {
		return reject(http.StatusBadRequest, "lat must be between -90 and 90").because("%s rejected: lat %v out of range", what, *lat)
	}
	if lon != nil && (*lon < -180 || *lon > 180) {
		return reject(http.StatusBadRequest, "lon must be between -180 and 180").because("%s rejected: lon %v out of range", what, *lon)
	}

	var doc map[string]json.RawMessage
	if err := s.store.Read(slug, "", &doc); err != nil {
		return reject(http.StatusInternalServerError, "reading screensaver settings").failed("%s failed: reading settings: %v", what, err)
	}
	if doc == nil {
		doc = make(map[string]json.RawMessage)
	}
	doc["location"], _ = json.Marshal(mode)
	if latSet {
		if lat == nil {
			delete(doc, "lat")
			delete(doc, "lon")
		} else {
			doc["lat"], _ = json.Marshal(*lat)
			doc["lon"], _ = json.Marshal(*lon)
		}
	}
	if err := s.store.Write(slug, "", doc); err != nil {
		return reject(http.StatusInternalServerError, "writing screensaver settings").failed("%s failed: %v", what, err)
	}

	s.ring.Append("info", fmt.Sprintf("http: screensaver location written: %s (%s)", slug, mode))
	writeJSON(w, http.StatusOK, screensaverInfo{
		Slug: slug, Name: m.Name, Location: mode,
		Lat: docFloat(doc, "lat"), Lon: docFloat(doc, "lon"),
		Fix: s.readScreensaverFix(slug),
	})
	return nil
}

// handleScreensaverPost splits POST /api/screensavers/{slug...} between the
// location write and the .../show preview, since slugs contain a slash and
// ServeMux allows one trailing wildcard. The suffix is peeled only when the
// full slug is not itself a registered screensaver.
func (s *Server) handleScreensaverPost(w http.ResponseWriter, r *http.Request) *rejection {
	slug := r.PathValue("slug")
	if base, ok := strings.CutSuffix(slug, "/show"); ok {
		if _, registered := screensaverManifest(slug); !registered {
			return s.handleScreensaverShow(w, base)
		}
	}
	return s.handleScreensaverWrite(w, r)
}

// handleScreensaverShow serves POST /api/screensavers/{slug}/show: show that
// screensaver now regardless of its schedule. It answers 404 for an unknown
// screensaver and 409 when standby is disabled.
func (s *Server) handleScreensaverShow(w http.ResponseWriter, slug string) *rejection {
	m, ok := screensaverManifest(slug)
	if !ok {
		return reject(http.StatusNotFound, "unknown screensaver").because("screensaver show %q rejected: not a registered screensaver", slug)
	}
	if !s.rt.Status().StandbyEnabled {
		return reject(http.StatusConflict, "standby is disabled (--attract-after 0): there is no screensaver state to enter").
			because("screensaver show %q rejected: standby disabled", slug)
	}
	if err := s.rt.ShowScreensaver(slug); err != nil {
		return reject(http.StatusNotFound, err.Error()).failed("screensaver show %q failed: %v", slug, err)
	}
	s.ring.Append("info", fmt.Sprintf("http: screensaver show requested: %s", slug))
	writeJSON(w, http.StatusOK, screensaverInfo{Slug: slug, Name: m.Name})
	return nil
}
