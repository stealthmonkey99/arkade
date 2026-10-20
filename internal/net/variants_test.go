// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeVariantControl struct {
	fakeCartControl
	available map[string][]string
	active    map[string]string
	set       []string // "slug=variant" per accepted SetVariant
	setErr    error
}

func (f *fakeVariantControl) Variants() (map[string][]string, map[string]string) {
	return f.available, f.active
}

func (f *fakeVariantControl) SetVariant(slug, variant string) (string, error) {
	if f.setErr != nil {
		return "", f.setErr
	}
	f.set = append(f.set, slug+"="+variant)
	f.active[slug] = variant
	return strings.ToUpper(variant) + " build", nil
}

func newVariantControl() *fakeVariantControl {
	return &fakeVariantControl{
		fakeCartControl: fakeCartControl{registered: map[string]bool{"games/testgame": true}},
		available:       map[string][]string{"games/testgame": {"public", "private"}},
		active:          map[string]string{"games/testgame": "private"},
	}
}

func variantPost(target string, body []byte, token string) *http.Request {
	req := newPostRequest(target, body, token)
	req.Host = "localhost"
	return req
}

func gamesBody(t *testing.T, srv *Server) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	srv.route("GET /api/games")(w, httptest.NewRequest(http.MethodGet, "/api/games", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/games = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /api/games: %v", err)
	}
	return body
}

func gameRow(t *testing.T, body map[string]any, slug string) map[string]any {
	t.Helper()
	for _, g := range body["games"].([]any) {
		row := g.(map[string]any)
		if row["slug"] == slug {
			return row
		}
	}
	t.Fatalf("no %q row in /api/games", slug)
	return nil
}

func TestGamesOmitsVariantsWithoutAVariantStore(t *testing.T) {
	for _, tc := range []struct {
		name string
		cart CartridgeControl
	}{
		{"no cartridge control", nil},
		{"cartridge control without variants", &fakeCartControl{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServerWithCart(t, testToken, tc.cart)
			row := gameRow(t, gamesBody(t, srv), "games/testgame")
			if _, ok := row["variants"]; ok {
				t.Errorf("row has a \"variants\" key: %v", row["variants"])
			}
			if _, ok := row["activeVariant"]; ok {
				t.Errorf("row has an \"activeVariant\" key: %v", row["activeVariant"])
			}
		})
	}
}

func TestGamesReportsVariantsAndActive(t *testing.T) {
	srv, _ := newTestServerWithCart(t, testToken, newVariantControl())
	row := gameRow(t, gamesBody(t, srv), "games/testgame")

	got, _ := json.Marshal(row["variants"])
	if string(got) != `["public","private"]` {
		t.Errorf("variants = %s, want [\"public\",\"private\"] in display order", got)
	}
	if row["activeVariant"] != "private" {
		t.Errorf("activeVariant = %v, want private", row["activeVariant"])
	}
}

func TestGamesOmitsSingleVariant(t *testing.T) {
	vc := newVariantControl()
	vc.available["games/testgame"] = []string{"public"}
	srv, _ := newTestServerWithCart(t, testToken, vc)
	row := gameRow(t, gamesBody(t, srv), "games/testgame")
	if _, ok := row["variants"]; ok {
		t.Errorf("row has a \"variants\" key for a single stored build: %v", row["variants"])
	}
}

func TestSetVariantRequiresToken(t *testing.T) {
	for _, tc := range []struct {
		name       string
		srvToken   string
		reqToken   string
		wantStatus int
	}{
		{"no token configured", "", "anything", http.StatusServiceUnavailable},
		{"no token sent", testToken, "", http.StatusUnauthorized},
		{"wrong token", testToken, "wrong", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vc := newVariantControl()
			srv, _ := newTestServerWithCart(t, tc.srvToken, vc)
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, variantPost("/api/variants/public/games/testgame", nil, tc.reqToken))
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if len(vc.set) != 0 {
				t.Errorf("SetVariant called %v; an unauthorized request must not reach it", vc.set)
			}
		})
	}
}

func TestSetVariantInstallsAndAudits(t *testing.T) {
	vc := newVariantControl()
	srv, ring := newTestServerWithCart(t, testToken, vc)

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, variantPost("/api/variants/public/games/testgame", nil, testToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if len(vc.set) != 1 || vc.set[0] != "games/testgame=public" {
		t.Fatalf("SetVariant calls = %v, want [games/testgame=public]", vc.set)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["variant"] != "public" || body["slug"] != "games/testgame" {
		t.Errorf("response = %v", body)
	}
	if !strings.Contains(body["note"], "relaunch") {
		t.Errorf("note = %q, want the relaunch caveat: a swap does not affect a running session", body["note"])
	}

	if !ringHas(ring, "variant installed: games/testgame -> public") {
		t.Errorf("no audit line for the accepted swap; ring = %v", ring.Tail(20))
	}
}

func TestSetVariantRejections(t *testing.T) {
	for _, tc := range []struct {
		name       string
		target     string
		cart       CartridgeControl
		wantStatus int
		wantAudit  string
	}{
		{
			name: "no variant store on this cabinet", target: "/api/variants/public/games/testgame",
			cart: &fakeCartControl{}, wantStatus: http.StatusServiceUnavailable,
			wantAudit: "no build-variant store",
		},
		{
			name: "unregistered slug", target: "/api/variants/public/cart/nope",
			cart: newVariantControl(), wantStatus: http.StatusNotFound, wantAudit: "not registered",
		},
		{
			name: "no such stored build", target: "/api/variants/staging/games/testgame",
			cart: newVariantControl(), wantStatus: http.StatusNotFound, wantAudit: "no such stored build",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, ring := newTestServerWithCart(t, testToken, tc.cart)
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, variantPost(tc.target, nil, testToken))
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d (%s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if !ringHas(ring, tc.wantAudit) {
				t.Errorf("no audit line containing %q; ring = %v", tc.wantAudit, ring.Tail(20))
			}
		})
	}
}

func TestSetVariantStoreFailure(t *testing.T) {
	vc := newVariantControl()
	vc.setErr = fmt.Errorf("not a valid bundle")
	srv, ring := newTestServerWithCart(t, testToken, vc)

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, variantPost("/api/variants/public/games/testgame", nil, testToken))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "not a valid bundle") {
		t.Errorf("body = %q leaks the store's error text", w.Body.String())
	}
	if !ringHas(ring, "not a valid bundle") {
		t.Errorf("the store's error text is not on the ring; ring = %v", ring.Tail(20))
	}
	if !ringHas(ring, "variant \"public\" for \"games/testgame\" failed") {
		t.Errorf("no audit line for the failure; ring = %v", ring.Tail(20))
	}
}

func ringHas(ring *Ring, want string) bool {
	for _, e := range ring.Tail(50) {
		if strings.Contains(e.Msg, want) {
			return true
		}
	}
	return false
}
