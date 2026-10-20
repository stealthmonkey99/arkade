// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	stdnet "net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
)

// CartridgeControl is the seam to the cartridge registry, so this package does
// not import internal/cartridge and tests can use a fake. Both methods are
// remote code execution by design; their routes pass the token gate before
// calling either.
type CartridgeControl interface {
	// Install validates bundle as an .rcart and, only if it parses, writes it
	// to the watched library directory so hot-mount registers it. filename is
	// used for error context only. A non-nil error means nothing was written.
	Install(filename string, bundle []byte) (slug, name string, err error)
	// Launch switches the engine to an already-registered slug; an error
	// means the slug is not registered.
	Launch(slug string) error
}

// maxCartridgeUploadBytes bounds memory before rcart.Load enforces content limits.
const maxCartridgeUploadBytes = 16 << 20 // 16 MiB

// Server is the monitoring HTTP surface: JSON status, logs and scores, the
// dashboard, and the token-gated control routes.
type Server struct {
	addr    string
	rt      engine.Remote
	ring    *Ring
	store   *storage.Store
	version string
	boot    time.Time // wall-clock process start

	// token authorizes every route that changes the cabinet or streams its
	// inputs. Empty disables them all.
	token     string
	authLimit *authLimiter
	rejects   *rejectLog
	// cart backs the install and launch routes; nil answers them 503.
	cart CartridgeControl

	// allowedHosts are extra Host names checkHost accepts, lowercased.
	allowedHosts map[string]bool

	previewLimit *tokenBucket
	flightLimit  *tokenBucket
	// periphStreams and micStreams count open streams, which are capped by
	// count rather than rate.
	periphStreams atomic.Int32
	micStreams    atomic.Int32

	httpSrv *http.Server
	ln      stdnet.Listener
}

// NewServer builds a Server bound to addr; call Start to listen. ring is also
// the audit trail. token is the bearer secret for mutating and streaming
// routes; an empty or CheckToken-rejected token disables them. allowedHosts
// are extra Host names accepted beyond localhost and IP literals.
func NewServer(addr string, rt engine.Remote, ring *Ring, store *storage.Store, version, token string, cart CartridgeControl, allowedHosts ...string) *Server {
	s := &Server{
		addr: addr, rt: rt, ring: ring, store: store, version: version,
		boot: time.Now(), token: token, cart: cart,
		authLimit:    newAuthLimiter(),
		rejects:      newRejectLog(),
		previewLimit: newTokenBucket(previewBurst, previewRefillPerSecond),
		flightLimit:  newTokenBucket(flightDumpBurst, flightDumpRefillPerSecond),
	}
	if err := CheckToken(token); err != nil {
		ring.Append("error", fmt.Sprintf("http: %v; remote control routes are disabled", err))
		s.token = ""
	}
	s.allowedHosts = make(map[string]bool, len(allowedHosts))
	for _, h := range allowedHosts {
		s.allowedHosts[strings.ToLower(h)] = true
	}
	s.httpSrv = &http.Server{Addr: addr, Handler: s.checkHost(s.mux())}
	return s
}

// checkHost rejects requests whose Host is not localhost, an IP literal or an
// allowed host, with 421 and a coalesced audit line. This blocks DNS
// rebinding, where an attacker-controlled name resolves to the cabinet.
func (s *Server) checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := stdnet.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host // no port
		}
		host = strings.ToLower(host)
		switch {
		case host == "localhost":
		case stdnet.ParseIP(host) != nil:
		case s.allowedHosts[host]:
		default:
			s.rejects.append(s.ring, "host", fmt.Sprintf("http: rejected Host %q (possible DNS rebinding)", r.Host))
			http.Error(w, "unrecognized Host header", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Start binds the listener and serves in the background. A bind failure is
// returned; the game loop does not depend on the server.
func (s *Server) Start() error {
	ln, err := stdnet.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("net: listen %s: %w", s.addr, err)
	}
	s.ln = ln
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.ring.Append("error", fmt.Sprintf("http: serve: %v", err))
		}
	}()
	return nil
}

// Addr returns the bound address, or "" before Start or after a failed bind.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type statusResponse struct {
	engine.Status
	ProcessUptimeMs float64 `json:"processUptimeMs"`
	Version         string  `json:"version"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) *rejection {
	writeJSON(w, http.StatusOK, statusResponse{
		Status:          s.rt.Status(),
		ProcessUptimeMs: float64(time.Since(s.boot).Microseconds()) / 1000,
		Version:         s.version,
	})
	return nil
}

// maxLogEntries caps /api/logs?n=; larger requests are clamped, not rejected.
const maxLogEntries = 5000

// handleLogs serves the last ?n= ring entries (default 200, at most
// maxLogEntries), oldest first.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) *rejection {
	n := 200
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = min(parsed, maxLogEntries)
		}
	}
	writeJSON(w, http.StatusOK, s.ring.Tail(n))
	return nil
}

// slugRE matches one or two lowercase [a-z0-9_-] path segments; no dots, so
// traversal paths never match.
var slugRE = regexp.MustCompile(`^[a-z0-9_-]+(/[a-z0-9_-]+)?$`)

// handleScoresOne serves GET /api/scores/{app}: the raw stored "scores" JSON,
// 404 if never written, 400 for a malformed slug.
func (s *Server) handleScoresOne(w http.ResponseWriter, r *http.Request) *rejection {
	app := r.PathValue("app")
	if !slugRE.MatchString(app) {
		return reject(http.StatusBadRequest, "invalid app slug")
	}
	raw, err := s.readScores(app)
	if err != nil {
		return reject(http.StatusInternalServerError, "internal error")
	}
	if raw == nil {
		return reject(http.StatusNotFound, "not found")
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

// handleScoresAll serves GET /api/scores: every game's scores keyed by slug,
// omitting games with none saved.
func (s *Server) handleScoresAll(w http.ResponseWriter, r *http.Request) *rejection {
	out := make(map[string]json.RawMessage)
	for _, m := range engine.Apps() {
		if m.Type != engine.TypeGame {
			continue
		}
		raw, err := s.readScores(m.Slug)
		if err == nil && raw != nil {
			out[m.Slug] = raw
		}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// readScores returns the raw "scores" JSON for slug, or nil when none was
// saved, because Store.Read leaves its output untouched on a missing file.
func (s *Server) readScores(slug string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := s.store.Read(slug, "scores", &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// requireToken enforces bearer auth for gated routes. It fails closed with 503
// when no token is configured. The token is compared in constant time, and a
// source sending maxAuthFailures wrong tokens within authFailureWindow gets 429
// for authLockout. Its rejections coalesce per kind in the audit ring.
func (s *Server) requireToken(r *http.Request) *rejection {
	refuse := func(kind, reason string, status int, text string) *rejection {
		rej := reject(status, text).because("rejected %s %s: %s", r.Method, r.URL.Path, reason)
		rej.kind = kind
		return rej
	}
	if s.token == "" {
		return refuse("disabled", "remote control disabled (no usable token configured)", http.StatusServiceUnavailable,
			"remote control disabled: no usable token configured (set ARKADE_HTTP_TOKEN)")
	}
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return refuse("missing", "missing bearer token", http.StatusUnauthorized, "missing bearer token")
	}
	src := sourceOf(r.RemoteAddr)
	if wait := s.authLimit.lockedFor(src); wait > 0 {
		return refuse("locked", fmt.Sprintf("%s is locked out after repeated invalid tokens", src), http.StatusTooManyRequests,
			"too many invalid tokens; try again later").retry(strconv.Itoa(int(wait.Seconds()) + 1))
	}
	given := strings.TrimPrefix(auth, prefix)
	if subtle.ConstantTimeCompare([]byte(given), []byte(s.token)) != 1 {
		if s.authLimit.fail(src) {
			s.rejects.append(s.ring, "lockout", fmt.Sprintf("http: %s locked out for %s after %d invalid tokens", src, authLockout, maxAuthFailures))
		}
		return refuse("invalid", "invalid bearer token", http.StatusUnauthorized, "invalid bearer token")
	}
	s.authLimit.succeed(src)
	return nil
}

// installErrorResponse explains only a lack of disk space; other install
// errors can carry server paths or bundle strings, so they get the generic text.
func installErrorResponse(err error, generic string, status int) (string, int) {
	var full interface{ InsufficientStorage() bool }
	if errors.As(err, &full) && full.InsufficientStorage() {
		return err.Error(), http.StatusInsufficientStorage
	}
	return generic, status
}

func noCartridgeControl() *rejection {
	return reject(http.StatusServiceUnavailable, "remote cartridge control disabled: no CartridgeControl configured")
}

// handleInstallCartridge serves POST /api/cartridges: the body is a raw .rcart
// bundle. Responds 200 with {"slug","name"}, 400 on validation failure, 507
// when it will not fit on disk, 413 when oversize.
func (s *Server) handleInstallCartridge(w http.ResponseWriter, r *http.Request) *rejection {
	if s.cart == nil {
		return noCartridgeControl()
	}
	body, rej := readBody(w, r, "cartridge upload", maxCartridgeUploadBytes)
	if rej != nil {
		return rej
	}
	slug, name, err := s.cart.Install(r.URL.Query().Get("filename"), body)
	if err != nil {
		msg, status := installErrorResponse(err, "invalid cartridge bundle", http.StatusBadRequest)
		return reject(status, msg).because("cartridge install rejected: %v", err)
	}
	s.ring.Append("info", fmt.Sprintf("http: cartridge installed: %s (%q)", slug, name))
	writeJSON(w, http.StatusOK, map[string]string{"slug": slug, "name": name})
	return nil
}

// handleLaunch serves POST /api/launch/{slug}. Responds 200 with {"slug"}, or
// 404 if the slug is not registered.
func (s *Server) handleLaunch(w http.ResponseWriter, r *http.Request) *rejection {
	if s.cart == nil {
		return noCartridgeControl()
	}
	slug := r.PathValue("slug")
	if err := s.cart.Launch(slug); err != nil {
		return reject(http.StatusNotFound, "unknown slug").because("launch %q rejected: %v", slug, err)
	}
	s.ring.Append("info", fmt.Sprintf("http: launch requested: %s", slug))
	writeJSON(w, http.StatusOK, map[string]string{"slug": slug})
	return nil
}
