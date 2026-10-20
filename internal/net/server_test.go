// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

const testToken = "test-token-0123456789abcdef-arkade"

type noopApp struct{}

func (noopApp) Init() error                                 { return nil }
func (noopApp) HandleInput(clock.FrameTime)                 {}
func (noopApp) Update(clock.FrameTime)                      {}
func (noopApp) Render(clock.FrameTime, engine.RenderReason) {}
func (noopApp) Teardown() error                             { return nil }

func init() {
	engine.Register(engine.Manifest{
		Slug: "games/testgame",
		Name: "Test Game",
		Type: engine.TypeGame,
		// Not Hidden: engine.Apps() filters hidden manifests.
		New: func(*engine.AppContext) engine.App { return noopApp{} },
	})
}

func newTestServer(t *testing.T) (*Server, *testkit.Rig, *storage.Store, *Ring) {
	t.Helper()
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir})
	rig.Tick(1)
	store := storage.New(dataDir)
	ring := NewRing(500)
	srv := NewServer("127.0.0.1:0", rig.RT, ring, store, "test-version", "", nil)
	return srv, rig, store, ring
}

func newTestServerWithCart(t *testing.T, token string, cart CartridgeControl) (*Server, *Ring) {
	t.Helper()
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir})
	rig.Tick(1)
	store := storage.New(dataDir)
	ring := NewRing(500)
	srv := NewServer("127.0.0.1:0", rig.RT, ring, store, "test-version", token, cart)
	return srv, ring
}

func newTestServerWithAllowedHosts(t *testing.T, allowedHosts ...string) (*Server, *Ring) {
	t.Helper()
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir})
	rig.Tick(1)
	store := storage.New(dataDir)
	ring := NewRing(500)
	srv := NewServer("127.0.0.1:0", rig.RT, ring, store, "test-version", "", nil, allowedHosts...)
	return srv, ring
}

func TestStatusJSONShape(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + srv.Addr() + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{
		"slug", "suspendedSlug", "standbyState", "ticks", "uptimeMs",
		"durationMs", "droppedFrames", "heapMB", "sysMB", "fanOn",
		"motionActive", "processUptimeMs", "version",
	} {
		if _, ok := body[field]; !ok {
			t.Errorf("response missing field %q: %+v", field, body)
		}
	}
	if got := body["slug"]; got != "games/testgame" {
		t.Errorf("slug = %v, want games/testgame", got)
	}
	if got := body["version"]; got != "test-version" {
		t.Errorf("version = %v, want test-version", got)
	}
	if _, present := body["tempF"]; present {
		t.Errorf("tempF should be omitted when unavailable, got %v", body["tempF"])
	}
}

func TestLogsRingAppendTruncationAndNParam(t *testing.T) {
	srv, _, _, ring := newTestServer(t)
	for i := 0; i < 10; i++ {
		ring.Append("info", "line")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/logs?n=3", nil)
	w := httptest.NewRecorder()
	srv.route("GET /api/logs")(w, req)

	var entries []LogEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	wantSeqs := []uint64{8, 9, 10}
	for i, e := range entries {
		if e.Seq != wantSeqs[i] {
			t.Errorf("entries[%d].Seq = %d, want %d", i, e.Seq, wantSeqs[i])
		}
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	w2 := httptest.NewRecorder()
	srv.route("GET /api/logs")(w2, req2)
	var all []LogEntry
	if err := json.Unmarshal(w2.Body.Bytes(), &all); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(all) != 10 {
		t.Fatalf("default n: len = %d, want 10 (everything the ring holds)", len(all))
	}
}

func TestLogsNParamIsClamped(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	ring := NewRing(maxLogEntries + 100)
	srv.ring = ring
	for i := 0; i < maxLogEntries+50; i++ {
		ring.Append("info", "line")
	}
	for _, q := range []string{"n=999999999", "n=" + strconv.Itoa(maxLogEntries+1), "n=99999999999999999999"} {
		w := httptest.NewRecorder()
		srv.route("GET /api/logs")(w, httptest.NewRequest(http.MethodGet, "/api/logs?"+q, nil))
		var entries []LogEntry
		if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
			t.Fatalf("%s: decode: %v", q, err)
		}
		if len(entries) > maxLogEntries {
			t.Errorf("%s: len = %d, want <= %d", q, len(entries), maxLogEntries)
		}
	}
	w := httptest.NewRecorder()
	srv.route("GET /api/logs")(w, httptest.NewRequest(http.MethodGet, "/api/logs?n="+strconv.Itoa(maxLogEntries), nil))
	var entries []LogEntry
	_ = json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != maxLogEntries {
		t.Errorf("n=max: len = %d, want %d", len(entries), maxLogEntries)
	}
}

func TestRingTruncatesAtCapacity(t *testing.T) {
	r := NewRing(3)
	for i := 0; i < 5; i++ {
		r.Append("info", "line")
	}
	got := r.Tail(10)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (ring capacity)", len(got))
	}
	wantSeqs := []uint64{3, 4, 5}
	for i, e := range got {
		if e.Seq != wantSeqs[i] {
			t.Errorf("got[%d].Seq = %d, want %d", i, e.Seq, wantSeqs[i])
		}
	}
}

func TestScoresOneFoundAndNotFound(t *testing.T) {
	srv, _, store, _ := newTestServer(t)
	if err := store.Write("games/testgame", "scores", map[string]int{"highScore": 42}); err != nil {
		t.Fatalf("seed scores: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/scores/games/testgame", nil)
	req.SetPathValue("app", "games/testgame")
	w := httptest.NewRecorder()
	srv.route("GET /api/scores/{app...}")(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var got map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["highScore"] != 42 {
		t.Errorf("highScore = %d, want 42", got["highScore"])
	}

	req404 := httptest.NewRequest(http.MethodGet, "/api/scores/games/nosuchgame", nil)
	req404.SetPathValue("app", "games/nosuchgame")
	w404 := httptest.NewRecorder()
	srv.route("GET /api/scores/{app...}")(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w404.Code)
	}
}

func TestScoresRouteWiredThroughMux(t *testing.T) {
	srv, _, store, _ := newTestServer(t)
	if err := store.Write("games/testgame", "scores", map[string]int{"highScore": 99}); err != nil {
		t.Fatalf("seed scores: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + srv.Addr() + "/api/scores/games/testgame")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["highScore"] != 99 {
		t.Errorf("highScore = %d, want 99", got["highScore"])
	}
}

func TestScoresSlugSanitization(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	bad := []string{
		"../etc/passwd",
		"games/../../etc",
		"GAMES/SNAKE",
		"games//snake",
		"games/snake/extra",
		"",
		"games/sn ake",
	}
	for _, slug := range bad {
		req := httptest.NewRequest(http.MethodGet, "/api/scores/x", nil)
		req.SetPathValue("app", slug)
		w := httptest.NewRecorder()
		srv.route("GET /api/scores/{app...}")(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("slug %q: status = %d, want 400", slug, w.Code)
		}
	}
}

func TestScoresAllIteratesGameManifests(t *testing.T) {
	srv, _, store, _ := newTestServer(t)
	if err := store.Write("games/testgame", "scores", map[string]int{"highScore": 7}); err != nil {
		t.Fatalf("seed scores: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/scores", nil)
	w := httptest.NewRecorder()
	srv.route("GET /api/scores")(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got map[string]map[string]int
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["games/testgame"]["highScore"] != 7 {
		t.Errorf("games/testgame.highScore = %v, want 7: %+v", got["games/testgame"], got)
	}
}

func TestDashboardServesHTML(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.route("GET /{$}")(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html prefix", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<html") || !strings.Contains(body, "Årkade") {
		t.Errorf("dashboard body missing expected markers: %s", body)
	}
}

func TestStatusEndpointRaceUnderConcurrentTicks(t *testing.T) {
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir})
	store := storage.New(dataDir)
	ring := NewRing(50)
	srv := NewServer("127.0.0.1:0", rig.RT, ring, store, "race-test", "", nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())

	const iterations = 150
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			rig.Clock.Advance(rig.RT.FrameBudget())
			_ = rig.RT.Tick()
		}
	}()

	url := "http://" + srv.Addr() + "/api/status"
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				resp, err := http.Get(url)
				if err != nil {
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
}

func requestWithHost(t *testing.T, srv *Server, host, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://"+srv.Addr()+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func TestCheckHostAcceptsLocalhostAndIPLiterals(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())

	for _, host := range []string{"localhost", "localhost:9999", "127.0.0.1", srv.Addr(), "[::1]:9999"} {
		resp := requestWithHost(t, srv, host, "/api/status")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Host %q: status = %d, want 200", host, resp.StatusCode)
		}
	}
}

func TestCheckHostRejectsArbitraryHostname(t *testing.T) {
	srv, _, _, ring := newTestServer(t)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())

	resp := requestWithHost(t, srv, "attacker.example", "/api/status")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want %d (Misdirected Request)", resp.StatusCode, http.StatusMisdirectedRequest)
	}

	found := false
	for _, e := range ring.Tail(10) {
		if e.Level == "warn" && strings.Contains(e.Msg, "rejected Host") && strings.Contains(e.Msg, "attacker.example") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a ring audit line for the rejected Host, got: %+v", ring.Tail(10))
	}
}

func TestCheckHostAcceptsConfiguredAllowedHost(t *testing.T) {
	srv, ring := newTestServerWithAllowedHosts(t, "cabinet.local")
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())

	for _, host := range []string{"cabinet.local", "cabinet.local:8080", "CABINET.LOCAL"} {
		resp := requestWithHost(t, srv, host, "/api/status")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Host %q: status = %d, want 200", host, resp.StatusCode)
		}
	}

	resp := requestWithHost(t, srv, "other-name.local", "/api/status")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("unconfigured Host: status = %d, want %d", resp.StatusCode, http.StatusMisdirectedRequest)
	}

	for _, e := range ring.Tail(10) {
		if strings.Contains(e.Msg, "rejected Host") && strings.Contains(e.Msg, "cabinet.local") {
			t.Errorf("configured host wrongly rejected: %+v", e)
		}
	}
}
