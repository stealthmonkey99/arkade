// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

func newFakeServer(t *testing.T, token string) (*Server, *fakeRemote, *Ring) {
	t.Helper()
	rt := &fakeRemote{status: engine.Status{Slug: "games/testgame", StandbyEnabled: true}}
	ring := NewRing(500)
	srv := NewServer("127.0.0.1:0", rt, ring, storage.New(t.TempDir()), "fake-version", token, nil)
	return srv, rt, ring
}

func serve(srv *Server, method, target, token string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	req.Host = "localhost"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(rec, req)
	return rec
}

func TestStatusServesTheRemoteSnapshot(t *testing.T) {
	srv, _, _ := newFakeServer(t, "")
	rec := serve(srv, http.MethodGet, "/api/status", "", nil)
	var body statusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Slug != "games/testgame" || body.Version != "fake-version" {
		t.Errorf("status = %+v, want the fake's slug and version", body)
	}
}

func TestMicStatusWithoutAMicrophone(t *testing.T) {
	srv, _, _ := newFakeServer(t, "")
	rec := serve(srv, http.MethodGet, "/api/mic/status", "", nil)
	if got := strings.TrimSpace(rec.Body.String()); got != `{"available":false}` {
		t.Errorf("body = %s, want {\"available\":false}", got)
	}
}

// fakeRemote is an engine.Remote that serves a fixed Status and records the
// requests handlers make, so route tests need no running engine.
type fakeRemote struct {
	mu          sync.Mutex
	status      engine.Status
	mic         engine.MicModels
	flight      engine.FlightDump
	flightErr   error
	attracts    []string
	screensaver []string
	previews    []engine.LabPreview
	lightShows  []engine.LightShow
	cancels     int
}

var _ engine.Remote = (*fakeRemote)(nil)

func (f *fakeRemote) Status() engine.Status                 { return f.status }
func (f *fakeRemote) PeriphSnapshot() engine.PeriphSnapshot { return engine.PeriphSnapshot{} }
func (f *fakeRemote) WatchPeriph() func()                   { return func() {} }
func (f *fakeRemote) Microphone() engine.MicModels          { return f.mic }

func (f *fakeRemote) DrainPeriphInputs(cursor uint64) ([]engine.PeriphInput, uint64, uint64) {
	return nil, cursor, 0
}

func (f *fakeRemote) DumpFlight(reason, note string) (engine.FlightDump, error) {
	return f.flight, f.flightErr
}

func (f *fakeRemote) RequestAttractPreview(slug string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attracts = append(f.attracts, slug)
}

func (f *fakeRemote) ShowScreensaver(slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.screensaver = append(f.screensaver, slug)
	return nil
}

func (f *fakeRemote) RequestLabPreview(p engine.LabPreview) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.previews = append(f.previews, p)
}

func (f *fakeRemote) RequestLightShow(s engine.LightShow) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lightShows = append(f.lightShows, s)
}

func (f *fakeRemote) CancelLabPreview() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels++
}

func (f *fakeRemote) CancelLightShow() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels++
}

func (f *fakeRemote) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.attracts) + len(f.screensaver) + len(f.previews) + len(f.lightShows) + f.cancels
}
