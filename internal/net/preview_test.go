// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

const previewToken = "preview-secret-0123456789abcdefxyz"

func newPreviewServer(t *testing.T) (*Server, *testkit.Rig, *Ring) {
	t.Helper()
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir})
	rig.Tick(1)
	ring := NewRing(500)
	srv := NewServer("127.0.0.1:0", rig.RT, ring, storage.New(dataDir), "test-version", previewToken, nil)
	tick := 0
	base := time.Now()
	srv.previewLimit.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	}
	return srv, rig, ring
}

// rawPreviewReq builds an unauthenticated request. Host is localhost because
// checkHost rejects httptest's default with 421.
func rawPreviewReq(method, target string, body []byte) *http.Request {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	r.Host = "localhost:8080"
	return r
}

func previewReq(method, target string, body []byte) *http.Request {
	r := rawPreviewReq(method, target, body)
	r.Header.Set("Authorization", "Bearer "+previewToken)
	return r
}

func rgbBody(cols, rows int, r, g, b byte) []byte {
	out := make([]byte, 0, cols*rows*3)
	for i := 0; i < cols*rows; i++ {
		out = append(out, r, g, b)
	}
	return out
}

func TestPreviewRoutesRequireToken(t *testing.T) {
	srv, _, _ := newPreviewServer(t)
	frame := rgbBody(64, 64, 1, 2, 3)

	cases := []struct {
		name, method, target string
		body                 []byte
	}{
		{"frame", http.MethodPost, "/api/preview/frame", frame},
		{"sound", http.MethodPost, "/api/preview/sound", []byte("RIFF")},
		{"lcd", http.MethodPost, "/api/preview/lcd", []byte(`{"line1":"HI"}`)},
		{"cancel", http.MethodDelete, "/api/preview", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/missing", func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, rawPreviewReq(tc.method, tc.target, tc.body))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("no Authorization header: status = %d, want 401", w.Code)
			}
		})
		t.Run(tc.name+"/wrong", func(t *testing.T) {
			r := rawPreviewReq(tc.method, tc.target, tc.body)
			r.Header.Set("Authorization", "Bearer nope")
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("wrong token: status = %d, want 401", w.Code)
			}
		})
	}
}

func TestPreviewRoutesFailClosedWithNoTokenConfigured(t *testing.T) {
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir})
	rig.Tick(1)
	srv := NewServer("127.0.0.1:0", rig.RT, NewRing(50), storage.New(dataDir), "v", "", nil)

	r := rawPreviewReq(http.MethodPost, "/api/preview/lcd", []byte(`{"line1":"HI"}`))
	r.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}

	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Error("a rejected request must not reach the engine")
	}
}

func TestPreviewRawFrameAccepted(t *testing.T) {
	srv, rig, ring := newPreviewServer(t)
	cols, rows := rig.RT.Display.Size()

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/frame?ms=2000", rgbBody(cols, rows, 255, 0, 0)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", w.Code, w.Body.String())
	}
	var resp previewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Kind != "frame" || resp.Frames != 1 || resp.TimeoutMs != 2000 {
		t.Errorf("response = %+v, want kind=frame frames=1 timeoutMs=2000", resp)
	}

	rig.Tick(1)
	if !rig.RT.LabPreviewActive() {
		t.Fatal("engine did not pick up the preview")
	}
	if px := rig.PixelAt(0, 0); px.R != 255 || px.G != 0 || px.B != 0 {
		t.Errorf("panel pixel = %+v, want the posted red", px)
	}
	if !ringHas(ring, "preview frame accepted") {
		t.Error("accepted preview was not audited")
	}
}

func TestPreviewFrameWrongSizeRejected(t *testing.T) {
	srv, rig, ring := newPreviewServer(t)
	cols, rows := rig.RT.Display.Size()

	// One pixel short: an exact-length check, not a prefix read.
	body := rgbBody(cols, rows, 1, 2, 3)[:cols*rows*3-3]
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/frame", body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "want exactly") {
		t.Errorf("body = %q, want it to name the expected size", w.Body.String())
	}
	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Error("a rejected frame must not reach the engine")
	}
	if !ringHas(ring, "preview frame rejected") {
		t.Error("rejected preview was not audited")
	}
}

func TestPreviewFrameOversizeRejected(t *testing.T) {
	srv, _, _ := newPreviewServer(t)
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/frame",
		make([]byte, maxPreviewFrameBytes+1)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

func TestPreviewFrameSequenceAccepted(t *testing.T) {
	srv, rig, _ := newPreviewServer(t)
	cols, rows := rig.RT.Display.Size()

	body := fmt.Sprintf(`{"frames":[{"rgb":%q,"delayMs":50},{"rgb":%q,"delayMs":50}]}`,
		base64.StdEncoding.EncodeToString(rgbBody(cols, rows, 255, 0, 0)),
		base64.StdEncoding.EncodeToString(rgbBody(cols, rows, 0, 255, 0)))
	r := previewReq(http.MethodPost, "/api/preview/frame?loop=1&ms=3000", []byte(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", w.Code, w.Body.String())
	}

	rig.Tick(1)
	if px := rig.PixelAt(0, 0); px.R != 255 {
		t.Fatalf("first frame pixel = %+v, want red", px)
	}
	rig.TickFor("60ms")
	if px := rig.PixelAt(0, 0); px.G != 255 {
		t.Errorf("second frame pixel = %+v, want green", px)
	}
}

func TestPreviewFrameSequenceLimits(t *testing.T) {
	srv, rig, _ := newPreviewServer(t)
	cols, rows := rig.RT.Display.Size()
	one := base64.StdEncoding.EncodeToString(rgbBody(cols, rows, 0, 0, 0))

	cases := []struct{ name, body string }{
		{"empty", `{"frames":[]}`},
		{"unknown field", `{"frames":[],"speed":2}`},
		{"bad base64", `{"frames":[{"rgb":"!!!!"}]}`},
		{"negative delay", fmt.Sprintf(`{"frames":[{"rgb":%q,"delayMs":-1}]}`, one)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := previewReq(http.MethodPost, "/api/preview/frame", []byte(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d (%s), want 400", w.Code, w.Body.String())
			}
		})
	}

	t.Run("too many frames", func(t *testing.T) {
		frames := make([]string, maxPreviewFrames+1)
		for i := range frames {
			frames[i] = fmt.Sprintf(`{"rgb":%q}`, one)
		}
		r := previewReq(http.MethodPost, "/api/preview/frame",
			[]byte(`{"frames":[`+strings.Join(frames, ",")+`]}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.httpSrv.Handler.ServeHTTP(w, r)
		// Either cap may bite first; both are correct refusals.
		if w.Code != http.StatusBadRequest && w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d (%s), want 400 or 413", w.Code, w.Body.String())
		}
	})
	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Error("no rejected sequence should have reached the engine")
	}
}

func TestPreviewSoundAccepted(t *testing.T) {
	srv, rig, _ := newPreviewServer(t)
	wav := []byte("RIFF....WAVEfmt junk")

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/sound?gain=0.25&ms=1500", wav))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", w.Code, w.Body.String())
	}
	var resp previewResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Bytes != len(wav) || resp.Gain != 0.25 || resp.TimeoutMs != 1500 {
		t.Errorf("response = %+v, want bytes=%d gain=0.25 timeoutMs=1500", resp, len(wav))
	}

	rig.Tick(1)
	var played bool
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "play" && ev.Owner == engine.LabPreviewOwner && ev.Bytes == len(wav) {
			played = true
		}
	}
	if !played {
		t.Errorf("engine did not play the uploaded WAV: %+v", rig.Audio.Events())
	}
}

func TestPreviewSoundRejections(t *testing.T) {
	srv, _, _ := newPreviewServer(t)
	cases := []struct {
		name, target string
		body         []byte
		want         int
	}{
		{"empty body", "/api/preview/sound", nil, http.StatusBadRequest},
		{"gain over one", "/api/preview/sound?gain=11", []byte("RIFF"), http.StatusBadRequest},
		{"gain not a number", "/api/preview/sound?gain=loud", []byte("RIFF"), http.StatusBadRequest},
		{"negative ms", "/api/preview/sound?ms=-1", []byte("RIFF"), http.StatusBadRequest},
		{"oversize", "/api/preview/sound", make([]byte, maxPreviewSoundBytes+1), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, tc.target, tc.body))
			if w.Code != tc.want {
				t.Errorf("status = %d (%s), want %d", w.Code, w.Body.String(), tc.want)
			}
		})
	}
}

func TestPreviewLCDAcceptedWithGlyphs(t *testing.T) {
	srv, rig, _ := newPreviewServer(t)
	body := `{"line1":"HELLO ◆","line2":"WORLD",
	          "glyphs":{"◆":[4,14,31,14,4,0,0,0]}}`

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/lcd?ms=1200", []byte(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", w.Code, w.Body.String())
	}
	var resp previewResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Glyphs != 1 || resp.TimeoutMs != 1200 {
		t.Errorf("response = %+v, want glyphs=1 timeoutMs=1200", resp)
	}

	rig.Tick(1)
	if !rig.RT.LabPreviewActive() {
		t.Fatal("engine did not pick up the LCD preview")
	}
}

func TestPreviewLCDPerSeatLines(t *testing.T) {
	srv, rig, _ := newPreviewServer(t)
	body := `{"seats":[{"line1":"P1","line2":"ONE"},{"line1":"P2","line2":"TWO"}]}`
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/lcd", []byte(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", w.Code, w.Body.String())
	}
	rig.Tick(1)
	if !rig.RT.LabPreviewActive() {
		t.Fatal("engine did not pick up the per-seat preview")
	}
}

func TestPreviewLCDRejections(t *testing.T) {
	srv, rig, _ := newPreviewServer(t)
	cases := []struct{ name, body string }{
		{"both shapes", `{"line1":"A","seats":[{"line1":"B"},{"line1":"C"}]}`},
		{"one seat", `{"seats":[{"line1":"B"}]}`},
		{"three seats", `{"seats":[{"line1":"A"},{"line1":"B"},{"line1":"C"}]}`},
		{"unknown field", `{"line_1":"A"}`},
		{"control character", "{\"line1\":\"a\\u0007b\"}"},
		{"long line", `{"line1":"` + strings.Repeat("X", maxPreviewLineRunes+1) + `"}`},
		{"multi-rune glyph key", `{"line1":"A","glyphs":{"ab":[0,0,0,0,0,0,0,0]}}`},
		{"short glyph", `{"line1":"A","glyphs":{"◆":[0,0,0]}}`},
		{"row out of range", `{"line1":"A","glyphs":{"◆":[32,0,0,0,0,0,0,0]}}`},
		{"negative row", `{"line1":"A","glyphs":{"◆":[-1,0,0,0,0,0,0,0]}}`},
		{"too many glyphs", `{"line1":"A","glyphs":{
			"a":[0,0,0,0,0,0,0,0],"b":[0,0,0,0,0,0,0,0],"c":[0,0,0,0,0,0,0,0],
			"d":[0,0,0,0,0,0,0,0],"e":[0,0,0,0,0,0,0,0],"f":[0,0,0,0,0,0,0,0],
			"g":[0,0,0,0,0,0,0,0],"h":[0,0,0,0,0,0,0,0],"i":[0,0,0,0,0,0,0,0]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/lcd", []byte(tc.body)))
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d (%s), want 400", w.Code, w.Body.String())
			}
		})
	}
	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Error("no rejected LCD body should have reached the engine")
	}
}

func TestPreviewLCDOversizeRejected(t *testing.T) {
	srv, _, _ := newPreviewServer(t)
	body := `{"line1":"` + strings.Repeat("X", maxPreviewLCDBytes) + `"}`
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/lcd", []byte(body)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

func TestPreviewCancelRestoresTheCabinet(t *testing.T) {
	srv, rig, _ := newPreviewServer(t)
	cols, rows := rig.RT.Display.Size()

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/frame?ms=60000", rgbBody(cols, rows, 9, 9, 9)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("frame: status = %d, want 202", w.Code)
	}
	rig.Tick(1)
	if !rig.RT.LabPreviewActive() {
		t.Fatal("preview should be active")
	}

	w = httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodDelete, "/api/preview", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("cancel: status = %d, want 202", w.Code)
	}
	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Error("cancel did not end the preview")
	}
}

func TestPreviewCancelWithNothingRunningSucceeds(t *testing.T) {
	srv, _, _ := newPreviewServer(t)
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodDelete, "/api/preview", nil))
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202: cancelling nothing is the caller's desired state", w.Code)
	}
}

func TestPreviewRateLimit(t *testing.T) {
	srv, rig, ring := newPreviewServer(t)
	cols, rows := rig.RT.Display.Size()
	frame := rgbBody(cols, rows, 1, 1, 1)

	// Pin the limiter clock so no sleeping is needed.
	now := time.Now()
	srv.previewLimit.now = func() time.Time { return now }

	for i := 0; i < previewBurst; i++ {
		w := httptest.NewRecorder()
		srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/frame", frame))
		if w.Code != http.StatusAccepted {
			t.Fatalf("request %d: status = %d, want 202 (inside the burst)", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/frame", frame))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("past the burst: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("a 429 should carry Retry-After")
	}
	if !ringHas(ring, "rate limited") {
		t.Error("rate-limited request was not audited")
	}

	w = httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodDelete, "/api/preview", nil))
	if w.Code != http.StatusAccepted {
		t.Errorf("cancel while rate limited: status = %d, want 202", w.Code)
	}

	now = now.Add(time.Second)
	w = httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/preview/frame", frame))
	if w.Code != http.StatusAccepted {
		t.Errorf("after refill: status = %d, want 202", w.Code)
	}
}
