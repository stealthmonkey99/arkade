// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

const flightToken = "flight-secret-0123456789abcdefxyz"

func newFlightServer(t *testing.T) (*Server, *testkit.Rig, *Ring, string) {
	t.Helper()
	dataDir := t.TempDir()
	dumpDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{
		StartApp: "games/testgame", DataDir: dataDir, FlightDir: dumpDir,
	})
	rig.Tick(1)
	ring := NewRing(500)
	srv := NewServer("127.0.0.1:0", rig.RT, ring, storage.New(dataDir), "test-version", flightToken, nil)
	tick := 0
	base := time.Now()
	srv.flightLimit.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Minute)
	}
	return srv, rig, ring, dumpDir
}

func rawFlightReq(target string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, nil)
	r.Host = "localhost:8080"
	return r
}

func flightReq(target string) *http.Request {
	r := rawFlightReq(target)
	r.Header.Set("Authorization", "Bearer "+flightToken)
	return r
}

func TestFlightDumpRequiresToken(t *testing.T) {
	srv, _, _, _ := newFlightServer(t)
	cases := []struct {
		name string
		auth string
	}{
		{"missing", ""},
		{"wrong", "Bearer nope"},
		{"not bearer", "Basic " + flightToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := rawFlightReq("/api/flight/dump")
			if tc.auth != "" {
				r.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestFlightDumpFailsClosedWithNoTokenConfigured(t *testing.T) {
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir, FlightDir: t.TempDir()})
	rig.Tick(1)
	srv := NewServer("127.0.0.1:0", rig.RT, NewRing(50), storage.New(dataDir), "v", "", nil)

	r := rawFlightReq("/api/flight/dump")
	r.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestFlightDumpRejectsAForeignHost(t *testing.T) {
	srv, _, _, _ := newFlightServer(t)
	r := flightReq("/api/flight/dump")
	r.Host = "cabinet.local:8080"
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusMisdirectedRequest {
		t.Errorf("status = %d, want 421", w.Code)
	}
}

func TestFlightDumpRejectsABadNote(t *testing.T) {
	srv, _, ring, _ := newFlightServer(t)
	cases := []struct {
		name, note, wantMsg string
	}{
		{"too long", strings.Repeat("x", maxFlightNoteRunes+1), "character limit"},
		{"control character", "line one\nline two", "control character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, flightReq("/api/flight/dump?note="+urlEscape(tc.note)))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %q)", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantMsg) {
				t.Errorf("body = %q, want it to mention %q", w.Body.String(), tc.wantMsg)
			}
		})
	}
	if !ringHas(ring, "flight dump rejected") {
		t.Error("a rejected dump was not audited to the log ring")
	}
}

func TestFlightDumpWithNoCartridgeIs409(t *testing.T) {
	srv, _, ring, dumpDir := newFlightServer(t)
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, flightReq("/api/flight/dump"))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %q)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no cartridge session") {
		t.Errorf("body = %q, want it to explain that nothing is being recorded", w.Body.String())
	}
	if entries, _ := os.ReadDir(dumpDir); len(entries) != 0 {
		t.Errorf("a refused dump still wrote %d files", len(entries))
	}
	if !ringHas(ring, "flight dump rejected") {
		t.Error("the refusal was not audited")
	}
}

func TestFlightDumpWritesAndReports(t *testing.T) {
	srv, rig, ring, dumpDir := newFlightServer(t)
	startFakeCartridgeSession(t, rig)

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, flightReq("/api/flight/dump?note=that+was+weird"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	var out engine.FlightDump
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	if out.Path == "" || out.Slug != fakeCartSlug || out.Events == 0 || out.Bytes == 0 {
		t.Fatalf("response = %+v, want a real path, slug, event count and size", out)
	}
	if _, err := os.Stat(out.Path); err != nil {
		t.Fatalf("the response named %s but it is not on disk: %v", out.Path, err)
	}
	if !strings.HasPrefix(out.Path, dumpDir) {
		t.Errorf("dump written to %s, want it under the configured %s", out.Path, dumpDir)
	}
	if !ringHas(ring, "flight dump written") {
		t.Error("the accepted dump was not audited")
	}
}

func TestFlightDumpRateLimit(t *testing.T) {
	srv, rig, _, _ := newFlightServer(t)
	startFakeCartridgeSession(t, rig)
	// Pin the clock so the bucket never refills mid-test.
	frozen := time.Now()
	srv.flightLimit.now = func() time.Time { return frozen }

	for i := 0; i < flightDumpBurst; i++ {
		w := httptest.NewRecorder()
		srv.httpSrv.Handler.ServeHTTP(w, flightReq("/api/flight/dump"))
		if w.Code != http.StatusOK {
			t.Fatalf("burst request %d: status = %d, want 200 (%s)", i, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, flightReq("/api/flight/dump"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request %d: status = %d, want 429", flightDumpBurst+1, w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("a 429 without a Retry-After leaves the caller guessing")
	}
}

func TestFlightAndPreviewBudgetsAreSeparate(t *testing.T) {
	srv, rig, _, _ := newFlightServer(t)
	startFakeCartridgeSession(t, rig)
	frozen := time.Now()
	srv.previewLimit.now = func() time.Time { return frozen }
	srv.flightLimit.now = func() time.Time { return frozen }

	for i := 0; i < previewBurst+2; i++ {
		srv.previewLimit.allow()
	}
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, flightReq("/api/flight/dump"))
	if w.Code != http.StatusOK {
		t.Fatalf("a drained preview budget blocked a flight dump: status = %d", w.Code)
	}
}

// fakeCartSlug is a manifest the engine treats as a cartridge because it has a
// WasmSHA256; this package imports no wasm host.
const fakeCartSlug = "cart/testflight"

func init() {
	engine.Register(engine.Manifest{
		Slug:       fakeCartSlug,
		Name:       "Test Flight",
		Type:       engine.TypeGame,
		Hidden:     true,
		Version:    "1.0.0",
		WasmSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		New:        func(*engine.AppContext) engine.App { return noopApp{} },
	})
}

func startFakeCartridgeSession(t *testing.T, rig *testkit.Rig) {
	t.Helper()
	rig.RT.RequestLaunchNow(fakeCartSlug)
	rig.Tick(5)
	if rig.RT.CurrentSlug() != fakeCartSlug {
		t.Fatalf("current app is %q, want %q", rig.RT.CurrentSlug(), fakeCartSlug)
	}
	if !rig.RT.FlightRecording() {
		t.Fatal("the recorder did not open a session for a manifest with a wasm hash")
	}
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c == '\n' || c == ' ' || c == '%' || c == '&' || c == '+' || c == '#' {
			b.WriteString("%" + strings.ToUpper(hexByte(c)))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func hexByte(c byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[c>>4], digits[c&0x0f]})
}
