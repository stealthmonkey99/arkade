// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// route returns the wrapped handler registered for pattern, so a test runs
// the same gate the mux does without going through path routing.
func (s *Server) route(pattern string) http.HandlerFunc {
	for _, rt := range s.routes() {
		if rt.pattern == pattern {
			return s.wrap(rt)
		}
	}
	panic("no route " + pattern)
}

func TestEveryNonGETRouteIsGated(t *testing.T) {
	srv, _, _ := newFakeServer(t, testToken)
	seen := map[string]bool{}
	for _, rt := range srv.routes() {
		if seen[rt.pattern] {
			t.Errorf("%s registered twice", rt.pattern)
		}
		seen[rt.pattern] = true
		if !strings.HasPrefix(rt.pattern, "GET ") && !rt.gated {
			t.Errorf("%s changes the cabinet but is not gated", rt.pattern)
		}
	}
	for _, p := range []string{"GET /api/periph/stream", "GET /api/mic/stream"} {
		if !srv.routes()[indexOfRoute(t, srv, p)].gated {
			t.Errorf("%s streams live input but is not gated", p)
		}
	}
}

func indexOfRoute(t *testing.T, srv *Server, pattern string) int {
	t.Helper()
	for i, rt := range srv.routes() {
		if rt.pattern == pattern {
			return i
		}
	}
	t.Fatalf("no route %s", pattern)
	return -1
}

func TestReadOnlyRefusesANonGETPattern(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("readOnly accepted a POST pattern")
		}
	}()
	readOnly("POST /api/anything", func(http.ResponseWriter, *http.Request) *rejection { return nil })
}

func concretePath(pattern string) (method, path string) {
	method, path, _ = strings.Cut(pattern, " ")
	path = regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(path, "games/testgame")
	return method, strings.TrimSuffix(path, "{$}")
}

func TestGatedRoutesRefuseBeforeTheHandlerRuns(t *testing.T) {
	routes, _, _ := newFakeServer(t, testToken)
	for _, r := range routes.routes() {
		if !r.gated {
			continue
		}
		method, path := concretePath(r.pattern)
		for _, tc := range []struct {
			name, token string
			text        string
		}{
			{"missing", "", "missing bearer token"},
			{"invalid", "not-the-token-0123456789abcdefghij", "invalid bearer token"},
		} {
			// A fresh server each time, so the lockout never trips.
			srv, rt, ring := newFakeServer(t, testToken)
			rec := serve(srv, method, path, tc.token, bytes.NewReader([]byte("{}")))
			if rec.Code != http.StatusUnauthorized || strings.TrimSpace(rec.Body.String()) != tc.text {
				t.Errorf("%s %s with %s token: %d %q, want 401 %q", method, path, tc.name, rec.Code, rec.Body.String(), tc.text)
			}
			if n := rt.requests(); n != 0 {
				t.Errorf("%s %s with %s token reached the engine", method, path, tc.name)
			}
			if !ringHas(ring, "rejected "+method+" "+path+": "+tc.text) {
				t.Errorf("%s %s with %s token was not audited", method, path, tc.name)
			}
		}
	}
}

func TestGatedRoutesFailClosedWithoutAToken(t *testing.T) {
	srv, _, _ := newFakeServer(t, "")
	for _, r := range srv.routes() {
		if !r.gated {
			continue
		}
		method, path := concretePath(r.pattern)
		rec := serve(srv, method, path, testToken, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s with control disabled: %d, want 503", method, path, rec.Code)
		}
	}
}

func TestOpenRoutesNeedNoToken(t *testing.T) {
	srv, _, _ := newFakeServer(t, testToken)
	for _, r := range srv.routes() {
		if r.gated {
			continue
		}
		method, path := concretePath(r.pattern)
		if rec := serve(srv, method, path, "", nil); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusServiceUnavailable {
			t.Errorf("%s %s answered %d without a token", method, path, rec.Code)
		}
	}
}

func TestRejectionWritesStatusTextAndAudit(t *testing.T) {
	srv, _, ring := newFakeServer(t, "")
	rec := httptest.NewRecorder()
	srv.refuse(rec, reject(http.StatusConflict, "busy").because("thing %q rejected: busy", "x").retry("3"))
	if rec.Code != http.StatusConflict || strings.TrimSpace(rec.Body.String()) != "busy" {
		t.Errorf("answer = %d %q, want 409 busy", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "3" {
		t.Errorf("Retry-After = %q, want 3", got)
	}
	last := ring.Tail(1)
	if len(last) != 1 || last[0].Level != "warn" || last[0].Msg != `http: thing "x" rejected: busy` {
		t.Errorf("audit = %+v, want one warn line", last)
	}

	before := len(ring.Tail(100))
	srv.refuse(httptest.NewRecorder(), reject(http.StatusBadRequest, "quiet"))
	if len(ring.Tail(100)) != before {
		t.Error("a rejection with no audit line still wrote to the ring")
	}
}

func TestBodyOverTheCapIsRefusedWithItsLimit(t *testing.T) {
	srv, _, ring := newFakeServer(t, testToken)
	rec := serve(srv, http.MethodPost, "/api/preview/lcd", testToken, bytes.NewReader(make([]byte, maxPreviewLCDBytes+1)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if want := "request body too large (max 16384 bytes)"; strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("body = %q, want %q", rec.Body.String(), want)
	}
	if !ringHas(ring, "preview lcd rejected: body over the 16384 byte cap") {
		t.Error("oversize body was not audited")
	}
}
