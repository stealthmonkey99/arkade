// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var defaultOrigins = []string{"https://arkade.build", "https://www.arkade.build"}

const (
	stateBuilding = "building"
	stateReady    = "ready"
	stateError    = "error"
)

// devServer owns the built state and serves it to the hosted player, plus the
// local player's files when playerDir is set.
type devServer struct {
	manifest  cartManifest
	playerDir string // absolute; "" serves no player
	origins   map[string]bool

	broker  *sseBroker
	builder Builder
	buildCh chan struct{} // capacity 1, so bursts coalesce

	mu       sync.Mutex
	rcart    []byte // nil until the first build succeeds
	state    string
	buildErr string // the last failed build's output, while state is error

	logf func(format string, args ...any)
}

func newDevServer(m cartManifest, builder Builder) *devServer {
	s := &devServer{
		manifest: m,
		origins:  make(map[string]bool),
		broker:   newSSEBroker(),
		builder:  builder,
		buildCh:  make(chan struct{}, 1),
		state:    stateBuilding,
		logf:     func(string, ...any) {},
	}
	s.allowOrigins(defaultOrigins...)
	return s
}

// allowOrigins takes origins already in normalizeOrigin's form.
func (s *devServer) allowOrigins(origins ...string) {
	for _, o := range origins {
		s.origins[o] = true
	}
}

func (s *devServer) playPath() string  { return "/" + s.manifest.Slug + "/play" }
func (s *devServer) rcartPath() string { return "/" + s.manifest.Slug + "/cart.rcart" }

// triggerBuild requests a rebuild without blocking; bursts collapse into one.
func (s *devServer) triggerBuild() {
	select {
	case s.buildCh <- struct{}{}:
	default:
	}
}

// runBuildLoop builds once straight away, then once per trigger until ctx
// ends; builds never overlap because this loop is the only place one runs.
// After each build it updates the served state, notifies every SSE
// connection, and passes the outcome to built.
func (s *devServer) runBuildLoop(ctx context.Context, built func(ok bool)) {
	build := func() {
		s.mu.Lock()
		s.state = stateBuilding
		s.mu.Unlock()
		s.broker.broadcast(formatSSE("building", ""))

		res := s.builder.Build(ctx)

		s.mu.Lock()
		if res.OK {
			s.rcart = res.RCart
			s.state, s.buildErr = stateReady, ""
		} else {
			s.state, s.buildErr = stateError, res.Output
		}
		s.mu.Unlock()

		if res.OK {
			s.logf("devserve: build ok (%s), reloading the player\n", res.Duration.Round(time.Millisecond))
			s.broker.broadcast(formatSSE("reload", ""))
		} else {
			s.logf("devserve: build FAILED (%s) — last good build (if any) keeps running:\n%s\n", res.Duration.Round(time.Millisecond), res.Output)
			s.broker.broadcast(formatSSE("error", res.Output))
		}
		built(res.OK)
	}

	build()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.buildCh:
			build()
		}
	}
}

// handler serves every route behind the Host check and CORS; port is the bound one.
func (s *devServer) handler(port string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/__devserve/info", s.handleInfo)
	mux.HandleFunc("/__devserve/cart.rcart", s.handleCartRcart)
	mux.HandleFunc("/__devserve/events", s.handleEvents)
	if s.playerDir != "" {
		mux.HandleFunc("/play", s.handlePlayerPage)
		mux.HandleFunc(s.playPath(), s.handlePlayerPage)
		mux.HandleFunc(s.rcartPath(), s.handleCartRcart)
		mux.HandleFunc("/dist/", s.handleDistFile)
		mux.HandleFunc("/favicon.svg", s.handleFavicon)
	}
	return s.guard(port, mux)
}

// guard applies the loopback rules: refuse a foreign Host (DNS rebinding),
// echo CORS only for an allowed origin, and answer preflights.
func (s *devServer) guard(port string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host, port) {
			httpErr(w, http.StatusForbidden, fmt.Sprintf("devserve: refusing Host %q; use http://127.0.0.1:%s", r.Host, port))
			return
		}
		h := w.Header()
		h.Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		allowed := s.origins[origin]
		if allowed {
			h.Set("Access-Control-Allow-Origin", origin)
		}
		if r.Method != http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if !allowed {
			httpErr(w, http.StatusForbidden, fmt.Sprintf("devserve: origin %q not allowed; add it with -origin", origin))
			return
		}
		h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
			h.Set("Access-Control-Allow-Headers", req)
		}
		if strings.EqualFold(r.Header.Get("Access-Control-Request-Private-Network"), "true") {
			h.Set("Access-Control-Allow-Private-Network", "true")
		}
		h.Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
	})
}

func loopbackHost(host, port string) bool {
	return host == "127.0.0.1:"+port || strings.EqualFold(host, "localhost:"+port)
}

func (s *devServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	devOrigin := "http://" + r.Host
	if r.TLS != nil {
		devOrigin = "https://" + r.Host
	}
	if s.playerDir != "" {
		http.Redirect(w, r, "/play?dev="+devOrigin, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "devserve: open %s?dev=%s\n", hostedPlayURL, devOrigin)
}

type devInfo struct {
	Devserve int    `json:"devserve"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
}

func (s *devServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	s.mu.Lock()
	info := devInfo{Devserve: 1, Slug: s.manifest.Slug, Title: s.manifest.Title, State: s.state}
	if s.state == stateError {
		info.Error = s.buildErr
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(info)
}

// handleCartRcart serves the last successful build, or 503 if none has succeeded yet.
func (s *devServer) handleCartRcart(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	s.mu.Lock()
	data := s.rcart
	s.mu.Unlock()
	if data == nil {
		httpErr(w, http.StatusServiceUnavailable, "devserve: no successful build yet — check the terminal for the compile error, fix it, and save again")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

// handlePlayerPage serves the local player's index.html unmodified, read fresh per request.
func (s *devServer) handlePlayerPage(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	path := filepath.Join(s.playerDir, "index.html")
	html, err := os.ReadFile(path)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, fmt.Sprintf("devserve: couldn't read %s: %v", path, err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(html)
}

// handleDistFile serves a regular file directly inside the player's dist/, never a subdirectory or dotfile.
func (s *devServer) handleDistFile(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/dist/")
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.playerDir, "dist", name)
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		httpErr(w, http.StatusNotFound, fmt.Sprintf("devserve: %s not found; build the player first (npm ci && npm run build)", path))
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		httpErr(w, http.StatusInternalServerError, fmt.Sprintf("devserve: couldn't read %s: %v", path, err))
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

func (s *devServer) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.playerDir, "favicon.svg"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

// replayEvents is what a new subscriber missed: reload if a good build
// exists, then the current state unless that is ready.
func (s *devServer) replayEvents() []string {
	var events []string
	if s.rcart != nil {
		events = append(events, formatSSE("reload", ""))
	}
	switch s.state {
	case stateBuilding:
		events = append(events, formatSSE("building", ""))
	case stateError:
		events = append(events, formatSSE("error", s.buildErr))
	}
	return events
}

// handleEvents is the SSE endpoint: "building", "error" with the compiler output, "reload".
func (s *devServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpErr(w, http.StatusInternalServerError, "devserve: streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Subscribing under mu means any state change after the replay also arrives on ch.
	s.mu.Lock()
	ch := s.broker.subscribe()
	replay := s.replayEvents()
	s.mu.Unlock()
	defer s.broker.unsubscribe(ch)

	fmt.Fprint(w, ": connected\n\n")
	for _, msg := range replay {
		fmt.Fprint(w, msg)
	}
	flusher.Flush()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if _, err := fmt.Fprint(w, msg); err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func getOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	httpErr(w, http.StatusMethodNotAllowed, "GET only")
	return false
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintln(w, msg)
}
