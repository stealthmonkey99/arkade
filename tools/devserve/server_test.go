// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testIndex = "<!doctype html><html><head></head><body><div id=\"screen\"></div></body></html>"

func newTestPlayerFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":        testIndex,
		"dist/bundle.js":    "console.log('player');",
		"dist/player.css":   "body{}",
		"dist/.env":         "SECRET=1",
		"favicon.svg":       "<svg/>",
		"dist/sub/deep.txt": "deep",
	}
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// startServer serves s on a kernel-chosen port behind the real handler, Host check included.
func startServer(t *testing.T, s *devServer) *httptest.Server {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	_, port, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = s.handler(port)
	ts.Start()
	t.Cleanup(ts.Close)
	return ts
}

func newHostedServer(t *testing.T, fb Builder) (*devServer, *httptest.Server) {
	t.Helper()
	s := newDevServer(cartManifest{Slug: "widget", Title: "Widget"}, fb)
	return s, startServer(t, s)
}

func newPlayerServer(t *testing.T, fb Builder) (*devServer, *httptest.Server) {
	t.Helper()
	s := newDevServer(cartManifest{Slug: "widget", Title: "Widget"}, fb)
	s.playerDir = newTestPlayerFiles(t)
	return s, startServer(t, s)
}

// startBuildLoop runs s's build loop for the test and returns each build's outcome in order.
func startBuildLoop(t *testing.T, s *devServer) <-chan bool {
	t.Helper()
	built := make(chan bool, 16)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.runBuildLoop(ctx, func(ok bool) { built <- ok })
	return built
}

func okBuilder() *fakeBuilder {
	return &fakeBuilder{results: []buildResult{{OK: true, RCart: []byte("rc")}}}
}

func get(t *testing.T, ts *httptest.Server, path string, header map[string]string) *http.Response {
	t.Helper()
	return do(t, ts, http.MethodGet, path, header)
}

func do(t *testing.T, ts *httptest.Server, method, path string, header map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBuildLoopBuildsOnceAtStart(t *testing.T) {
	s, ts := newHostedServer(t, okBuilder())
	built := startBuildLoop(t, s)
	if !<-built {
		t.Fatal("the start-up build should have succeeded")
	}
	assertRcartBody(t, ts, "/__devserve/cart.rcart", "rc")
	select {
	case <-built:
		t.Fatal("the loop built again with no trigger")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestForeignHostIsRefused(t *testing.T) {
	_, ts := newHostedServer(t, okBuilder())
	_, port, _ := net.SplitHostPort(ts.Listener.Addr().String())

	for _, host := range []string{"evil.example:" + port, "127.0.0.1:1", "localhost", "127.0.0.1", "0.0.0.0:" + port} {
		res := get(t, ts, "/__devserve/info", map[string]string{"Host": host, "Origin": "https://arkade.build"})
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("Host %q: status = %d, want 403", host, res.StatusCode)
		}
		if acao := res.Header.Get("Access-Control-Allow-Origin"); acao != "" {
			t.Errorf("Host %q: Access-Control-Allow-Origin = %q on a refused request", host, acao)
		}
	}
	for _, host := range []string{"127.0.0.1:" + port, "localhost:" + port, "LocalHost:" + port} {
		if res := get(t, ts, "/__devserve/info", map[string]string{"Host": host}); res.StatusCode != http.StatusOK {
			t.Errorf("Host %q: status = %d, want 200", host, res.StatusCode)
		}
	}
}

func TestCORSEchoesOnlyAllowedOrigins(t *testing.T) {
	s, ts := newHostedServer(t, okBuilder())
	s.allowOrigins("http://localhost:5173")

	for _, origin := range []string{"https://arkade.build", "https://www.arkade.build", "http://localhost:5173"} {
		res := get(t, ts, "/__devserve/info", map[string]string{"Origin": origin})
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("Origin %q: Access-Control-Allow-Origin = %q, want it echoed", origin, got)
		}
		if !strings.Contains(strings.Join(res.Header.Values("Vary"), ","), "Origin") {
			t.Errorf("Origin %q: Vary = %q, want Origin", origin, res.Header.Values("Vary"))
		}
	}
	for _, origin := range []string{"https://evil.example", "https://arkade.build.evil.example", "http://arkade.build", "null", ""} {
		res := get(t, ts, "/__devserve/info", map[string]string{"Origin": origin})
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Origin %q: Access-Control-Allow-Origin = %q, want none", origin, got)
		}
		if !strings.Contains(strings.Join(res.Header.Values("Vary"), ","), "Origin") {
			t.Errorf("Origin %q: Vary = %q, want Origin", origin, res.Header.Values("Vary"))
		}
	}
}

func TestCORSNeverSendsAWildcard(t *testing.T) {
	_, ts := newPlayerServer(t, okBuilder())
	paths := []string{"/", "/__devserve/info", "/__devserve/cart.rcart", "/widget/play", "/dist/bundle.js", "/nope"}
	for _, path := range paths {
		for _, origin := range []string{"https://arkade.build", "https://evil.example", "*"} {
			if got := get(t, ts, path, map[string]string{"Origin": origin}).Header.Get("Access-Control-Allow-Origin"); got == "*" {
				t.Errorf("GET %s from %q: Access-Control-Allow-Origin = *", path, origin)
			}
		}
	}
}

func TestPreflightAnswersPrivateNetworkAccess(t *testing.T) {
	_, ts := newHostedServer(t, okBuilder())

	res := do(t, ts, http.MethodOptions, "/__devserve/cart.rcart", map[string]string{
		"Origin":                                 "https://arkade.build",
		"Access-Control-Request-Method":          "GET",
		"Access-Control-Request-Private-Network": "true",
	})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", res.StatusCode)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":          "https://arkade.build",
		"Access-Control-Allow-Private-Network": "true",
		"Access-Control-Allow-Methods":         "GET, OPTIONS",
	}
	for k, v := range want {
		if got := res.Header.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}

	plain := do(t, ts, http.MethodOptions, "/__devserve/info", map[string]string{
		"Origin":                        "https://arkade.build",
		"Access-Control-Request-Method": "GET",
	})
	if got := plain.Header.Get("Access-Control-Allow-Private-Network"); got != "" {
		t.Errorf("Access-Control-Allow-Private-Network = %q without the request header, want none", got)
	}

	refused := do(t, ts, http.MethodOptions, "/__devserve/info", map[string]string{
		"Origin":                                 "https://evil.example",
		"Access-Control-Request-Method":          "GET",
		"Access-Control-Request-Private-Network": "true",
	})
	if refused.StatusCode != http.StatusForbidden {
		t.Errorf("preflight from a disallowed origin: status = %d, want 403", refused.StatusCode)
	}
	for _, k := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Private-Network"} {
		if got := refused.Header.Get(k); got != "" {
			t.Errorf("preflight from a disallowed origin: %s = %q, want none", k, got)
		}
	}
}

func readInfo(t *testing.T, ts *httptest.Server) devInfo {
	t.Helper()
	res := get(t, ts, "/__devserve/info", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("info status = %d", res.StatusCode)
	}
	var info devInfo
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	return info
}

func TestInfoFollowsTheBuildState(t *testing.T) {
	gb := newGatedBuilder(
		buildResult{OK: true, RCart: []byte("v1")},
		buildResult{OK: false, Output: "game.go:1:1: syntax error"},
		buildResult{OK: true, RCart: []byte("v2")},
	)
	s, ts := newHostedServer(t, gb)

	want := devInfo{Devserve: 1, Slug: "widget", Title: "Widget", State: "building"}
	if got := readInfo(t, ts); got != want {
		t.Fatalf("before the first build: info = %+v, want %+v", got, want)
	}

	built := startBuildLoop(t, s)
	<-gb.started
	if got := readInfo(t, ts); got.State != "building" {
		t.Fatalf("during the first build: state = %q, want building", got.State)
	}
	gb.release <- struct{}{}
	<-built
	if got := readInfo(t, ts); got.State != "ready" || got.Error != "" {
		t.Fatalf("after a good build: info = %+v, want ready with no error", got)
	}

	s.triggerBuild()
	<-gb.started
	gb.release <- struct{}{}
	<-built
	got := readInfo(t, ts)
	if got.State != "error" || !strings.Contains(got.Error, "syntax error") {
		t.Fatalf("after a failed build: info = %+v, want error with the compiler output", got)
	}

	s.triggerBuild()
	<-gb.started
	gb.release <- struct{}{}
	<-built
	if got := readInfo(t, ts); got.State != "ready" || got.Error != "" {
		t.Fatalf("after recovering: info = %+v, want ready with no error", got)
	}
}

func TestCartRcartBeforeAnyBuildIs503(t *testing.T) {
	_, ts := newPlayerServer(t, okBuilder())

	for _, path := range []string{"/__devserve/cart.rcart", "/widget/cart.rcart"} {
		res := get(t, ts, path, map[string]string{"Origin": "https://arkade.build"})
		if res.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("GET %s: status = %d, want 503 before any successful build", path, res.StatusCode)
		}
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://arkade.build" {
			t.Errorf("GET %s: the 503 lacks CORS (%q), so the hosted page could not read it", path, got)
		}
	}
}

func TestCartRcartServesLastGoodBuildAcrossAFailure(t *testing.T) {
	s, ts := newHostedServer(t, &fakeBuilder{results: []buildResult{
		{OK: true, RCart: []byte("good-v1")},
		{OK: false, Output: "game.go:1:1: syntax error"},
	}})

	built := startBuildLoop(t, s)
	if !<-built {
		t.Fatal("first build should have succeeded")
	}
	assertRcartBody(t, ts, "/__devserve/cart.rcart", "good-v1")

	s.triggerBuild()
	if <-built {
		t.Fatal("second build should have failed")
	}
	assertRcartBody(t, ts, "/__devserve/cart.rcart", "good-v1")
}

func assertRcartBody(t *testing.T, ts *httptest.Server, path, want string) {
	t.Helper()
	res := get(t, ts, path, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", path, res.StatusCode)
	}
	if got := readAll(t, res); got != want {
		t.Fatalf("GET %s: body = %q, want %q", path, got, want)
	}
}

func TestHostedModeServesNoPlayerRoutes(t *testing.T) {
	s, ts := newHostedServer(t, okBuilder())
	built := startBuildLoop(t, s)
	<-built

	for _, path := range []string{"/play", "/widget/play", "/widget/cart.rcart", "/dist/bundle.js", "/favicon.svg"} {
		if res := get(t, ts, path, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s without -player: status = %d, want 404", path, res.StatusCode)
		}
	}
	root := get(t, ts, "/", nil)
	if body := readAll(t, root); !strings.Contains(body, "https://arkade.build/play?dev=http://127.0.0.1:") {
		t.Errorf("GET / = %q, want the hosted play link", body)
	}
}

func TestPlayerPageServesIndexUnmodified(t *testing.T) {
	_, ts := newPlayerServer(t, okBuilder())

	for _, path := range []string{"/play", "/widget/play"} {
		res := get(t, ts, path, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, res.StatusCode)
		}
		if body := readAll(t, res); body != testIndex {
			t.Fatalf("GET %s = %q, want the player's index.html byte for byte", path, body)
		}
	}
}

func TestPlayerRootRedirectsToTheDevClient(t *testing.T) {
	_, ts := newPlayerServer(t, okBuilder())
	client := *ts.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if want := "/play?dev=" + ts.URL; res.Header.Get("Location") != want {
		t.Fatalf("Location = %q, want %q", res.Header.Get("Location"), want)
	}
}

func TestOtherSlugsAreNotServed(t *testing.T) {
	_, ts := newPlayerServer(t, okBuilder())

	for _, path := range []string{"/other/play", "/other/cart.rcart", "/widget/../other/play"} {
		if res := get(t, ts, path, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", path, res.StatusCode)
		}
	}
}

func TestDistFilesAreServedFlatAndNothingElse(t *testing.T) {
	_, ts := newPlayerServer(t, okBuilder())

	for path, want := range map[string]string{"/dist/bundle.js": "console.log('player');", "/dist/player.css": "body{}"} {
		res := get(t, ts, path, nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, res.StatusCode)
		}
		if got := readAll(t, res); got != want {
			t.Fatalf("GET %s = %q, want %q", path, got, want)
		}
	}
	if ct := get(t, ts, "/dist/player.css", nil).Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("player.css Content-Type = %q, want text/css", ct)
	}

	// ServeMux cleans .. segments before the handler sees the request.
	for _, path := range []string{"/dist/../index.html", "/dist/bundle.js.evil", "/dist/", "/dist/.env", "/dist/sub", "/dist/sub/deep.txt"} {
		if res := get(t, ts, path, nil); res.StatusCode == http.StatusOK {
			t.Errorf("GET %s: status 200, want anything but 200", path)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, ts := newPlayerServer(t, okBuilder())

	for _, path := range []string{"/__devserve/info", "/__devserve/cart.rcart", "/__devserve/events", "/widget/play", "/widget/cart.rcart", "/dist/bundle.js"} {
		res, err := ts.Client().Post(ts.URL+path, "text/plain", nil)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("POST %s: status = %d, want 405", path, res.StatusCode)
		}
	}
}

type sseStream struct {
	t  *testing.T
	sc *bufio.Scanner
}

func openEvents(t *testing.T, ts *httptest.Server) *sseStream {
	t.Helper()
	res := get(t, ts, "/__devserve/events", map[string]string{"Origin": "https://arkade.build"})
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://arkade.build" {
		t.Fatalf("Access-Control-Allow-Origin = %q on the event stream", got)
	}
	st := &sseStream{t: t, sc: bufio.NewScanner(res.Body)}
	if line := st.line(); !strings.HasPrefix(line, ":") {
		t.Fatalf("first line = %q, want the connect handshake comment", line)
	}
	return st
}

func (st *sseStream) line() string {
	st.t.Helper()
	if !st.sc.Scan() {
		st.t.Fatalf("SSE stream ended early: %v", st.sc.Err())
	}
	return st.sc.Text()
}

// event reads up to the next event and returns its name and joined data.
func (st *sseStream) event() (string, string) {
	st.t.Helper()
	var name string
	var data []string
	for {
		line := st.line()
		switch {
		case line == "" && name != "":
			return name, strings.Join(data, "\n")
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = append(data, strings.TrimPrefix(line, "data: "))
		}
	}
}

func (st *sseStream) expect(names ...string) {
	st.t.Helper()
	for _, want := range names {
		if got, _ := st.event(); got != want {
			st.t.Fatalf("event = %q, want %q (sequence %v)", got, want, names)
		}
	}
}

func TestSSEBroadcastsBuildEvents(t *testing.T) {
	s, ts := newHostedServer(t, &fakeBuilder{results: []buildResult{
		{OK: true, RCart: []byte("v1")},
		{OK: false, Output: "boom"},
	}})

	st := openEvents(t, ts)
	st.expect("building") // the replay: nothing built yet

	built := startBuildLoop(t, s)
	<-built
	s.triggerBuild()
	st.expect("building", "reload", "building")
	if name, data := st.event(); name != "error" || data != "boom" {
		t.Fatalf("event = %q %q, want error with the compiler output", name, data)
	}
}

func TestSSEReplaysTheCurrentStateOnConnect(t *testing.T) {
	gb := newGatedBuilder(
		buildResult{OK: false, Output: "first: undefined: x"},
		buildResult{OK: true, RCart: []byte("v1")},
		buildResult{OK: false, Output: "line one\nline two"},
	)
	s, ts := newHostedServer(t, gb)
	built := startBuildLoop(t, s)
	<-gb.started

	openEvents(t, ts).expect("building")

	gb.release <- struct{}{}
	<-built
	if name, data := openEvents(t, ts).event(); name != "error" || data != "first: undefined: x" {
		t.Fatalf("after a failed first build: replay = %q %q, want the error", name, data)
	}

	s.triggerBuild()
	<-gb.started
	gb.release <- struct{}{}
	<-built
	ready := openEvents(t, ts)
	ready.expect("reload")

	s.triggerBuild()
	<-gb.started
	openEvents(t, ts).expect("reload", "building")
	gb.release <- struct{}{}
	<-built

	late := openEvents(t, ts)
	late.expect("reload")
	if name, data := late.event(); name != "error" || data != "line one\nline two" {
		t.Fatalf("after a failure over a good build: replay = %q %q, want the multi-line error", name, data)
	}

	ready.expect("building", "error") // a replayed subscriber still gets live events
}
