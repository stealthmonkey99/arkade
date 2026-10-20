// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeCartControl struct {
	validBundle  []byte
	slug, name   string
	installCalls int
	registered   map[string]bool
	launched     []string
}

func (f *fakeCartControl) Install(filename string, bundle []byte) (string, string, error) {
	f.installCalls++
	if f.validBundle == nil || !bytes.Equal(bundle, f.validBundle) {
		return "", "", fmt.Errorf("fake: not a valid bundle (%d bytes)", len(bundle))
	}
	return f.slug, f.name, nil
}

func (f *fakeCartControl) Launch(slug string) error {
	if !f.registered[slug] {
		return fmt.Errorf("fake: %q not registered", slug)
	}
	f.launched = append(f.launched, slug)
	return nil
}

func newPostRequest(target string, body []byte, token string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestInstallCartridgeFailsClosedWithNoTokenConfigured(t *testing.T) {
	cart := &fakeCartControl{validBundle: []byte("valid"), slug: "cart/demo", name: "Demo"}
	srv, _ := newTestServerWithCart(t, "", cart) // no token configured
	req := newPostRequest("/api/cartridges", []byte("valid"), "irrelevant-token")
	w := httptest.NewRecorder()
	srv.route("POST /api/cartridges")(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (fail closed), body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
	if cart.installCalls != 0 {
		t.Errorf("Install called %d times, want 0 — no token configured must reject before touching CartridgeControl", cart.installCalls)
	}
}

func TestInstallCartridgeAuth(t *testing.T) {
	const token = testToken
	validBody := []byte("valid")
	cart := &fakeCartControl{validBundle: validBody, slug: "cart/demo", name: "Demo"}
	srv, ring := newTestServerWithCart(t, token, cart)

	t.Run("missing token", func(t *testing.T) {
		req := newPostRequest("/api/cartridges", validBody, "")
		w := httptest.NewRecorder()
		srv.route("POST /api/cartridges")(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		req := newPostRequest("/api/cartridges", validBody, "wrong")
		w := httptest.NewRecorder()
		srv.route("POST /api/cartridges")(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("correct token, valid bundle", func(t *testing.T) {
		req := newPostRequest("/api/cartridges", validBody, token)
		w := httptest.NewRecorder()
		srv.route("POST /api/cartridges")(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
		}
		var got struct{ Slug, Name string }
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Slug != "cart/demo" || got.Name != "Demo" {
			t.Errorf("got %+v, want slug=cart/demo name=Demo", got)
		}
	})

	t.Run("correct token, garbage body", func(t *testing.T) {
		req := newPostRequest("/api/cartridges", []byte("not a cartridge"), token)
		w := httptest.NewRecorder()
		srv.route("POST /api/cartridges")(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("oversize body", func(t *testing.T) {
		big := bytes.Repeat([]byte("a"), maxCartridgeUploadBytes+1)
		req := newPostRequest("/api/cartridges", big, token)
		w := httptest.NewRecorder()
		srv.route("POST /api/cartridges")(w, req)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413, body=%s", w.Code, w.Body.String())
		}
	})

	entries := ring.Tail(0)
	if len(entries) == 0 {
		t.Fatal("no ring entries recorded for install attempts")
	}
}

func TestLaunchCartridge(t *testing.T) {
	const token = testToken
	cart := &fakeCartControl{registered: map[string]bool{"cart/demo": true}}
	srv, ring := newTestServerWithCart(t, token, cart)

	t.Run("no token configured fails closed", func(t *testing.T) {
		srv2, _ := newTestServerWithCart(t, "", cart)
		req := httptest.NewRequest(http.MethodPost, "/api/launch/cart/demo", nil)
		req.SetPathValue("slug", "cart/demo")
		w := httptest.NewRecorder()
		srv2.route("POST /api/launch/{slug...}")(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/launch/cart/demo", nil)
		req.SetPathValue("slug", "cart/demo")
		w := httptest.NewRecorder()
		srv.route("POST /api/launch/{slug...}")(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("unknown slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/launch/cart/nosuch", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.SetPathValue("slug", "cart/nosuch")
		w := httptest.NewRecorder()
		srv.route("POST /api/launch/{slug...}")(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404, body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("known slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/launch/cart/demo", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.SetPathValue("slug", "cart/demo")
		w := httptest.NewRecorder()
		srv.route("POST /api/launch/{slug...}")(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
		}
		var got struct{ Slug string }
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Slug != "cart/demo" {
			t.Errorf("slug = %q, want cart/demo", got.Slug)
		}
		if len(cart.launched) != 1 || cart.launched[0] != "cart/demo" {
			t.Errorf("cart.launched = %v, want [cart/demo]", cart.launched)
		}
	})

	entries := ring.Tail(0)
	if len(entries) == 0 {
		t.Fatal("no ring entries recorded for launch attempts")
	}
}

// TestInstallAndLaunchRouteWiredThroughMux covers routing, which the
// direct-handler tests above bypass.
func TestInstallAndLaunchRouteWiredThroughMux(t *testing.T) {
	const token = testToken
	validBody := []byte("valid")
	cart := &fakeCartControl{
		validBundle: validBody, slug: "cart/demo", name: "Demo",
		registered: map[string]bool{"cart/demo": true},
	}
	srv, _ := newTestServerWithCart(t, token, cart)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())

	req, _ := http.NewRequest(http.MethodPost, "http://"+srv.Addr()+"/api/cartridges", bytes.NewReader(validBody))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/cartridges: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install status = %d, want 200", resp.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodPost, "http://"+srv.Addr()+"/api/launch/cart/demo", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("POST /api/launch/cart/demo: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("launch status = %d, want 200", resp2.StatusCode)
	}
}
