// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckToken(t *testing.T) {
	cases := []struct {
		name, token string
		ok          bool
	}{
		{"empty means disabled", "", true},
		{"openssl rand -hex 24", "3f9c1a7be04d52e8c6a1f0937b2d4e5a8c6b1f2e3d4a5b6c", true},
		{"exactly the minimum", strings.Repeat("0123456789abcdef", 2), true},
		{"one short", strings.Repeat("0123456789abcdef", 2)[:MinTokenLength-1], false},
		{"four characters", "abcd", false},
		{"long but one character", strings.Repeat("a", 64), false},
		{"long but a repeated word", strings.Repeat("hunter2", 8), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckToken(tc.token)
			if (err == nil) != tc.ok {
				t.Fatalf("CheckToken(%q) = %v, want ok=%v", tc.token, err, tc.ok)
			}
			if err != nil && strings.Contains(err.Error(), tc.token) {
				t.Errorf("error %q echoes the token", err)
			}
		})
	}
}

func TestWeakTokenDisablesControlRoutes(t *testing.T) {
	const weak = "s3cr3t"
	cart := &fakeCartControl{validBundle: []byte("valid"), slug: "cart/demo", name: "Demo"}
	srv, ring := newTestServerWithCart(t, weak, cart)

	w := httptest.NewRecorder()
	srv.route("POST /api/cartridges")(w, newPostRequest("/api/cartridges", []byte("valid"), weak))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 with a weak token configured", w.Code)
	}
	if cart.installCalls != 0 {
		t.Error("a weak token reached CartridgeControl")
	}
	found := false
	for _, e := range ring.Tail(0) {
		if e.Level == "error" && strings.Contains(e.Msg, "ARKADE_HTTP_TOKEN") && !strings.Contains(e.Msg, weak) {
			found = true
		}
	}
	if !found {
		t.Errorf("no error line naming the weak token problem; ring = %+v", ring.Tail(0))
	}
}

func launchFrom(srv *Server, remoteAddr, token string) *httptest.ResponseRecorder {
	req := newPostRequest("/api/launch/games/testgame", nil, token)
	req.SetPathValue("slug", "games/testgame")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	srv.route("POST /api/launch/{slug...}")(w, req)
	return w
}

func TestAuthLockoutAfterRepeatedFailures(t *testing.T) {
	cart := &fakeCartControl{registered: map[string]bool{"games/testgame": true}}
	srv, ring := newTestServerWithCart(t, testToken, cart)
	now := time.Unix(1_000_000, 0)
	srv.authLimit.now = func() time.Time { return now }

	const attacker, owner = "203.0.113.9:4000", "127.0.0.1:5000"
	for i := 0; i < maxAuthFailures; i++ {
		if w := launchFrom(srv, attacker, "wrong"); w.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: status = %d, want 401", i, w.Code)
		}
	}

	w := launchFrom(srv, attacker, testToken)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("locked-out source with the right token: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}
	if len(cart.launched) != 0 {
		t.Error("a locked-out source reached CartridgeControl")
	}
	if !ringHas(ring, "locked out") {
		t.Errorf("no lockout line on the ring; ring = %v", ring.Tail(20))
	}

	if w := launchFrom(srv, owner, testToken); w.Code != http.StatusOK {
		t.Fatalf("another source during the lockout: status = %d, want 200", w.Code)
	}

	now = now.Add(authLockout + time.Second)
	if w := launchFrom(srv, attacker, testToken); w.Code != http.StatusOK {
		t.Fatalf("after the lockout: status = %d, want 200", w.Code)
	}
}

func TestAuthFailuresOutsideTheWindowDoNotLockOut(t *testing.T) {
	srv, _ := newTestServerWithCart(t, testToken, &fakeCartControl{registered: map[string]bool{"games/testgame": true}})
	now := time.Unix(1_000_000, 0)
	srv.authLimit.now = func() time.Time { return now }

	for i := 0; i < 3*maxAuthFailures; i++ {
		launchFrom(srv, "203.0.113.9:4000", "wrong")
		now = now.Add(authFailureWindow / (maxAuthFailures - 2))
	}
	if w := launchFrom(srv, "203.0.113.9:4000", testToken); w.Code != http.StatusOK {
		t.Fatalf("slow failures locked the source out: status = %d", w.Code)
	}
}

func TestAuthLimiterMemoryIsBounded(t *testing.T) {
	l := newAuthLimiter()
	for i := 0; i < 4*maxAuthSources; i++ {
		l.fail(fmt.Sprintf("198.51.100.%d", i))
	}
	if len(l.sources) > maxAuthSources+1 {
		t.Fatalf("limiter holds %d sources, want at most %d", len(l.sources), maxAuthSources+1)
	}
}

func TestRejectionsCannotFlushTheRing(t *testing.T) {
	srv, ring := newTestServerWithCart(t, testToken, &fakeCartControl{})
	ring.Append("info", "http: cartridge installed: cart/evidence")

	for i := 0; i < 4000; i++ {
		launchFrom(srv, fmt.Sprintf("198.51.100.%d:1", i%250), "")
		launchFrom(srv, fmt.Sprintf("198.51.100.%d:1", i%250), "wrong")
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		req.Host = "attacker.example"
		srv.httpSrv.Handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if !ringHas(ring, "cart/evidence") {
		t.Fatal("rejection traffic flushed the earlier audit line")
	}
	if n := len(ring.Tail(0)); n > 10 {
		t.Errorf("12000 rejections wrote %d ring lines, want them coalesced", n)
	}
}

func TestRejectLogCarriesTheSuppressedCount(t *testing.T) {
	ring := NewRing(10)
	l := newRejectLog()
	now := time.Unix(1_000_000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		l.append(ring, "missing", "http: rejected")
	}
	now = now.Add(rejectLogInterval)
	l.append(ring, "missing", "http: rejected")
	got := ring.Tail(0)
	if len(got) != 2 {
		t.Fatalf("ring = %+v, want 2 lines", got)
	}
	if !strings.Contains(got[1].Msg, "+4 similar") {
		t.Errorf("second line = %q, want it to carry the 4 suppressed", got[1].Msg)
	}
}

func TestDashboardHasNoHTMLInjectionSinks(t *testing.T) {
	srv, _, _, _ := newTestServer(t)
	w := httptest.NewRecorder()
	srv.route("GET /{$}")(w, httptest.NewRequest(http.MethodGet, "/", nil))
	body, _ := io.ReadAll(w.Body)
	for _, sink := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(string(body), sink) {
			t.Errorf("dashboard uses %s; place server data with textContent", sink)
		}
	}
}

type errCartControl struct{ err error }

func (c errCartControl) Install(string, []byte) (string, string, error) { return "", "", c.err }
func (c errCartControl) Launch(string) error                            { return c.err }

type fullDiskError struct{}

func (fullDiskError) Error() string             { return "not enough free space: needs 12.0 MiB" }
func (fullDiskError) InsufficientStorage() bool { return true }

func TestInstallErrorIsGeneric(t *testing.T) {
	detail := errors.New("writing /srv/arkade/data/cartridges/x.rcart: read-only file system")
	srv, ring := newTestServerWithCart(t, testToken, errCartControl{detail})

	w := httptest.NewRecorder()
	srv.route("POST /api/cartridges")(w, newPostRequest("/api/cartridges", []byte("x"), testToken))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "invalid cartridge bundle" {
		t.Errorf("body = %q, want the generic message only", got)
	}
	if !ringHas(ring, "/srv/arkade/data/cartridges/x.rcart") {
		t.Errorf("the detail is not on the ring; ring = %v", ring.Tail(10))
	}
}

func TestInstallOutOfSpaceIsExplained(t *testing.T) {
	srv, _ := newTestServerWithCart(t, testToken, errCartControl{fmt.Errorf("cartridge: %w", fullDiskError{})})

	w := httptest.NewRecorder()
	srv.route("POST /api/cartridges")(w, newPostRequest("/api/cartridges", []byte("x"), testToken))
	if w.Code != http.StatusInsufficientStorage {
		t.Fatalf("status = %d, want 507", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not enough free space") {
		t.Errorf("body = %q, want the free-space explanation", w.Body.String())
	}
}
