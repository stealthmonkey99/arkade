// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/weather"
)

func registerTestSaver(t *testing.T, slug string) {
	t.Helper()
	engine.Register(engine.Manifest{
		Slug: slug, Name: "Saver", Type: engine.TypeScreensaver, Hidden: true,
		New: func(*engine.AppContext) engine.App { return noopApp{} },
	})
	t.Cleanup(func() { engine.Unregister(slug) })
}

func getScreensavers(t *testing.T, srv *Server) []screensaverInfo {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/screensavers", nil)
	w := httptest.NewRecorder()
	srv.route("GET /api/screensavers")(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/screensavers = %d, want 200: %s", w.Code, w.Body.String())
	}
	var got []screensaverInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, w.Body.String())
	}
	return got
}

func findScreensaver(t *testing.T, list []screensaverInfo, slug string) screensaverInfo {
	t.Helper()
	for _, s := range list {
		if s.Slug == slug {
			return s
		}
	}
	t.Fatalf("slug %q missing from /api/screensavers (%d rows)", slug, len(list))
	return screensaverInfo{}
}

func putScreensaver(t *testing.T, srv *Server, slug, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/screensavers/"+slug, bytes.NewReader([]byte(body)))
	req.SetPathValue("slug", slug)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.route("PUT /api/screensavers/{slug...}")(w, req)
	return w
}

func TestScreensaverListShape(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, store, _ := newAttractServer(t, testToken)
	if err := store.Write("test/saver", "", map[string]any{
		"schedule": map[string]any{"enabled": true},
		"lat":      37.77,
		"lon":      -122.42,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	list := getScreensavers(t, srv)
	s := findScreensaver(t, list, "test/saver")
	if s.Name != "Saver" {
		t.Errorf("name = %q, want %q", s.Name, "Saver")
	}
	if s.Lat == nil || s.Lon == nil || *s.Lat != 37.77 || *s.Lon != -122.42 {
		t.Errorf("location = %+v, want lat=37.77 lon=-122.42", s)
	}
}

func TestScreensaverListReportsTheMode(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, store, _ := newAttractServer(t, testToken)

	if got := findScreensaver(t, getScreensavers(t, srv), "test/saver"); got.Location != "auto" {
		t.Errorf("a screensaver with no settings file = %q, want auto", got.Location)
	}

	if err := store.Write("test/saver", "", map[string]any{"lat": 37.77, "lon": -122.42}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := findScreensaver(t, getScreensavers(t, srv), "test/saver")
	if got.Location != "manual" {
		t.Errorf("lat/lon with no location key = %q, want manual", got.Location)
	}
	if got.Lat == nil || *got.Lat != 37.77 {
		t.Errorf("stored coordinates should still be reported, got %+v", got)
	}
}

func TestScreensaverListCarriesTheResolvedFix(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, store, _ := newAttractServer(t, testToken)

	if got := findScreensaver(t, getScreensavers(t, srv), "test/saver"); got.Fix != nil {
		t.Errorf("a screensaver that never resolved anything = %+v, want no fix", got.Fix)
	}

	at := time.Date(2026, 9, 15, 4, 11, 7, 0, time.UTC)
	if err := store.Write("test/saver", screensaverFixLabel, weather.Location{
		Lat: 37.77, Lon: -122.42, City: "San Francisco", Region: "California",
		Country: "US", Timezone: "America/Los_Angeles", Source: "cloudflare-ip", At: at,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fix := findScreensaver(t, getScreensavers(t, srv), "test/saver").Fix
	if fix == nil {
		t.Fatal("a cached fix should show up in the listing")
	}
	if fix.City != "San Francisco" || fix.Lat != 37.77 || fix.Lon != -122.42 {
		t.Errorf("fix = %+v, want the cached one", *fix)
	}
	if fix.Source != "cloudflare-ip" || !fix.At.Equal(at) {
		t.Errorf("fix provenance = %q at %v", fix.Source, fix.At)
	}
}

func TestScreensaverListOmitsMissingLocation(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, _, _ := newAttractServer(t, testToken)

	list := getScreensavers(t, srv)
	s := findScreensaver(t, list, "test/saver")
	if s.Lat != nil || s.Lon != nil {
		t.Errorf("location = %+v, want both nil for a screensaver with no settings file", s)
	}
}

func TestScreensaverWriteAuth(t *testing.T) {
	registerTestSaver(t, "test/saver")

	srvNoToken, _, _ := newAttractServer(t, "")
	if w := putScreensaver(t, srvNoToken, "test/saver", testToken, `{"lat":37.77,"lon":-122.42}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no token configured: status = %d, want 503", w.Code)
	}

	srv, _, _ := newAttractServer(t, testToken)
	if w := putScreensaver(t, srv, "test/saver", "", `{"lat":37.77,"lon":-122.42}`); w.Code != http.StatusUnauthorized {
		t.Errorf("missing token: status = %d, want 401", w.Code)
	}
	if w := putScreensaver(t, srv, "test/saver", "wrong", `{"lat":37.77,"lon":-122.42}`); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", w.Code)
	}
}

func TestScreensaverWriteRejections(t *testing.T) {
	registerTestSaver(t, "test/saver")
	registerGame(t, engine.Manifest{Slug: "apps/notasaver", Name: "Not A Saver", Type: engine.TypeApp})
	srv, _, _ := newAttractServer(t, testToken)

	cases := []struct {
		name, slug, body string
		want             int
	}{
		{"unregistered slug", "test/ghost", `{"lat":1,"lon":1}`, http.StatusNotFound},
		{"registered but not a screensaver", "apps/notasaver", `{"lat":1,"lon":1}`, http.StatusNotFound},
		{"not json", "test/saver", `nope`, http.StatusBadRequest},
		{"unknown field", "test/saver", `{"lat":1,"lon":1,"alt":1}`, http.StatusBadRequest},
		{"lat with no lon", "test/saver", `{"lat":1}`, http.StatusBadRequest},
		{"lon with no lat", "test/saver", `{"lon":1}`, http.StatusBadRequest},
		{"lat out of range", "test/saver", `{"lat":91,"lon":0}`, http.StatusBadRequest},
		{"lat below range", "test/saver", `{"lat":-91,"lon":0}`, http.StatusBadRequest},
		{"lon out of range", "test/saver", `{"lat":0,"lon":181}`, http.StatusBadRequest},
		{"lon below range", "test/saver", `{"lat":0,"lon":-181}`, http.StatusBadRequest},
		{"empty body", "test/saver", `{}`, http.StatusBadRequest},
		{"unknown mode", "test/saver", `{"location":"guess"}`, http.StatusBadRequest},
		{"null against a number", "test/saver", `{"lat":1,"lon":null}`, http.StatusBadRequest},
		{"lat is a string", "test/saver", `{"lat":"here","lon":0}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := putScreensaver(t, srv, tc.slug, testToken, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestScreensaverWritePreservesSchedule(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, store, ring := newAttractServer(t, testToken)

	type schedule struct {
		Enabled  bool   `json:"enabled"`
		From     string `json:"from"`
		To       string `json:"to"`
		Priority int    `json:"priority"`
	}
	seeded := schedule{Enabled: true, From: "06:00", To: "09:00", Priority: 10}
	if err := store.Write("test/saver", "", map[string]any{
		"schedule":   seeded,
		"brightness": 15,
		"units":      "F",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := putScreensaver(t, srv, "test/saver", testToken, `{"lat":37.77,"lon":-122.42}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got screensaverInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Slug != "test/saver" || got.Lat == nil || got.Lon == nil || *got.Lat != 37.77 || *got.Lon != -122.42 {
		t.Fatalf("response = %+v, want the written location", got)
	}

	var doc struct {
		Schedule   schedule `json:"schedule"`
		Brightness int      `json:"brightness"`
		Units      string   `json:"units"`
		Lat        *float64 `json:"lat"`
		Lon        *float64 `json:"lon"`
	}
	if err := store.Read("test/saver", "", &doc); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if doc.Schedule != seeded {
		t.Errorf("schedule = %+v, want it untouched: %+v", doc.Schedule, seeded)
	}
	if doc.Brightness != 15 || doc.Units != "F" {
		t.Errorf("brightness/units = %d/%q, want them untouched", doc.Brightness, doc.Units)
	}
	if doc.Lat == nil || doc.Lon == nil || *doc.Lat != 37.77 || *doc.Lon != -122.42 {
		t.Errorf("stored location = %+v, want lat=37.77 lon=-122.42", doc)
	}

	var audited bool
	for _, e := range ring.Tail(0) {
		if strings.Contains(e.Msg, "screensaver location written: test/saver") {
			audited = true
		}
	}
	if !audited {
		t.Error("accepted write left no audit line in the ring")
	}
}

func TestScreensaverWriteClears(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, store, _ := newAttractServer(t, testToken)
	if err := store.Write("test/saver", "", map[string]any{"lat": 1.0, "lon": 2.0, "schedule": map[string]any{"enabled": true}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := putScreensaver(t, srv, "test/saver", testToken, `{"lat":null,"lon":null}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got screensaverInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Lat != nil || got.Lon != nil {
		t.Errorf("response location = %+v, want both nil", got)
	}

	var doc map[string]json.RawMessage
	if err := store.Read("test/saver", "", &doc); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if _, ok := doc["lat"]; ok {
		t.Error("lat key still present after clearing")
	}
	if _, ok := doc["lon"]; ok {
		t.Error("lon key still present after clearing")
	}
	if _, ok := doc["schedule"]; !ok {
		t.Error("clearing the location must not remove the schedule key")
	}
}

func showScreensaver(t *testing.T, srv *Server, slug, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/screensavers/"+slug+"/show", nil)
	req.SetPathValue("slug", slug+"/show")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.route("POST /api/screensavers/{slug...}")(w, req)
	return w
}

func TestScreensaverShowAccepted(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, _, ring := newAttractServer(t, testToken)

	w := showScreensaver(t, srv, "test/saver", testToken)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got screensaverInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Slug != "test/saver" || got.Name != "Saver" {
		t.Errorf("response = %+v, want the shown screensaver", got)
	}
	var audited bool
	for _, e := range ring.Tail(0) {
		if strings.Contains(e.Msg, "screensaver show requested: test/saver") {
			audited = true
		}
	}
	if !audited {
		t.Error("accepted show left no audit line in the ring")
	}
}

func TestScreensaverShowRejections(t *testing.T) {
	registerTestSaver(t, "test/saver")
	registerGame(t, engine.Manifest{Slug: "apps/notasaver", Name: "Not A Saver", Type: engine.TypeApp})
	srv, _, _ := newAttractServer(t, testToken)

	cases := []struct {
		name, slug, token string
		want              int
	}{
		{"missing token", "test/saver", "", http.StatusUnauthorized},
		{"unregistered slug", "test/ghost", testToken, http.StatusNotFound},
		{"registered but not a screensaver", "apps/notasaver", testToken, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := showScreensaver(t, srv, tc.slug, tc.token)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestScreensaverShowRefusedWithStandbyDisabled(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, _ := newTestServerWithCart(t, testToken, nil)
	w := showScreensaver(t, srv, "test/saver", testToken)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
}

func TestScreensaverWriteSurvivesAShowSuffix(t *testing.T) {
	registerTestSaver(t, "test/saver/show")
	srv, _, _ := newAttractServer(t, testToken)

	req := httptest.NewRequest(http.MethodPost, "/api/screensavers/test/saver/show", bytes.NewReader([]byte(`{"lat":1,"lon":2}`)))
	req.SetPathValue("slug", "test/saver/show")
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	srv.route("POST /api/screensavers/{slug...}")(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
}

func TestScreensaverWriteRoundTripsBothShapes(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, store, _ := newAttractServer(t, testToken)

	readMode := func() screensaverLocation {
		t.Helper()
		var doc screensaverLocation
		if err := store.Read("test/saver", "", &doc); err != nil {
			t.Fatalf("read back: %v", err)
		}
		return doc
	}
	put := func(body string) screensaverInfo {
		t.Helper()
		w := putScreensaver(t, srv, "test/saver", testToken, body)
		if w.Code != http.StatusOK {
			t.Fatalf("PUT %s = %d, want 200 (body %s)", body, w.Code, w.Body.String())
		}
		var got screensaverInfo
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return got
	}

	got := put(`{"lat":37.77,"lon":-122.42}`)
	if got.Location != "manual" || got.Lat == nil || *got.Lat != 37.77 {
		t.Errorf("bare lat/lon echoed %+v, want manual at 37.77", got)
	}
	if doc := readMode(); doc.Location != "manual" || doc.Lat == nil || *doc.Lat != 37.77 {
		t.Errorf("bare lat/lon stored %+v, want manual at 37.77", doc)
	}

	got = put(`{"location":"auto"}`)
	if got.Location != "auto" {
		t.Errorf("mode echoed %q, want auto", got.Location)
	}
	if doc := readMode(); doc.Location != "auto" || doc.Lat == nil || *doc.Lat != 37.77 {
		t.Errorf("auto stored %+v, want the coordinates left alone", doc)
	}

	got = put(`{"location":"manual","lat":51.5,"lon":-0.13}`)
	if got.Location != "manual" || got.Lat == nil || *got.Lat != 51.5 || got.Lon == nil || *got.Lon != -0.13 {
		t.Errorf("manual with coordinates echoed %+v", got)
	}
	if doc := readMode(); doc.Location != "manual" || *doc.Lat != 51.5 {
		t.Errorf("manual with coordinates stored %+v", doc)
	}

	got = put(`{"location":"manual","lat":null,"lon":null}`)
	if got.Location != "manual" || got.Lat != nil || got.Lon != nil {
		t.Errorf("clearing echoed %+v, want manual with no coordinates", got)
	}
	if doc := readMode(); doc.Location != "manual" || doc.Lat != nil {
		t.Errorf("clearing stored %+v", doc)
	}
}

func TestScreensaverBareClearStaysManual(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, store, _ := newAttractServer(t, testToken)
	if err := store.Write("test/saver", "", map[string]any{"location": "auto", "lat": 1.0, "lon": 2.0}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if w := putScreensaver(t, srv, "test/saver", testToken, `{"lat":null,"lon":null}`); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var doc screensaverLocation
	if err := store.Read("test/saver", "", &doc); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if doc.Location != "manual" || doc.Lat != nil {
		t.Errorf("stored %+v, want manual with no coordinates", doc)
	}
}

func TestScreensaverWritePOSTAlsoWorks(t *testing.T) {
	registerTestSaver(t, "test/saver")
	srv, _, _ := newAttractServer(t, testToken)

	req := httptest.NewRequest(http.MethodPost, "/api/screensavers/test/saver", bytes.NewReader([]byte(`{"lat":1,"lon":2}`)))
	req.SetPathValue("slug", "test/saver")
	req.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	srv.route("PUT /api/screensavers/{slug...}")(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
}
