// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bufio"
	"context"
	"encoding/json"

	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func newPeriphServer(t *testing.T) (*Server, *testkit.Rig, *Ring) {
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

func lightsBody(t *testing.T, loop bool, steps ...periphLightStep) []byte {
	t.Helper()
	b, err := json.Marshal(periphLightsRequest{Steps: steps, Loop: loop})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func oneStep(p1, p2 []string, holdMs int) periphLightStep {
	return periphLightStep{Seats: [][]string{p1, p2}, HoldMs: holdMs}
}

func TestPeriphRoutesRequireToken(t *testing.T) {
	srv, _, _ := newPeriphServer(t)
	body := lightsBody(t, false, oneStep([]string{"joya"}, nil, 0))

	cases := []struct {
		name, method, target string
		body                 []byte
	}{
		{"stream", http.MethodGet, "/api/periph/stream", nil},
		{"lights", http.MethodPost, "/api/periph/lights", body},
		{"cancel", http.MethodDelete, "/api/periph/lights", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/missing", func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, rawPreviewReq(tc.method, tc.target, tc.body))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
		t.Run(tc.name+"/wrong", func(t *testing.T) {
			r := rawPreviewReq(tc.method, tc.target, tc.body)
			r.Header.Set("Authorization", "Bearer not-the-token")
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestPeriphRoutesFailClosedWithNoToken(t *testing.T) {
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/testgame", DataDir: dataDir})
	rig.Tick(1)
	srv := NewServer("127.0.0.1:0", rig.RT, NewRing(50), storage.New(dataDir), "v", "", nil)

	for _, target := range []string{"/api/periph/stream", "/api/periph/lights"} {
		r := previewReq(http.MethodGet, target, nil)
		if target == "/api/periph/lights" {
			r = previewReq(http.MethodPost, target, lightsBody(t, false, oneStep(nil, nil, 0)))
		}
		w := httptest.NewRecorder()
		srv.httpSrv.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503", target, w.Code)
		}
	}
}

func TestPeriphLightsAccepted(t *testing.T) {
	srv, rig, ring := newPeriphServer(t)

	body := lightsBody(t, true,
		oneStep([]string{"joya"}, []string{"joyd"}, 120),
		oneStep([]string{"joyb", "joyc"}, nil, 0),
	)
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/periph/lights?ms=8000", body))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", w.Code, w.Body.String())
	}
	var resp periphLightsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Kind != "lights" || resp.Steps != 2 || !resp.Loop {
		t.Errorf("response = %+v, want lights/2 steps/looping", resp)
	}
	if resp.TimeoutMs != 8000 {
		t.Errorf("timeoutMs = %d, want the post-clamp 8000", resp.TimeoutMs)
	}
	if !ringHas(ring, "periph lights accepted") {
		t.Error("an accepted choreography should be audited to the ring")
	}

	if rig.RT.LightShowActive() {
		t.Error("the show must be queued, not applied from the handler goroutine")
	}
	rig.Tick(1)
	if !rig.RT.LightShowActive() {
		t.Error("the show should be live after one tick")
	}
	if !rig.RT.Status().LightShowActive {
		t.Error("Status should report the live show")
	}
}

func TestPeriphLightsTimeoutIsEchoedPostClamp(t *testing.T) {
	srv, _, _ := newPeriphServer(t)
	body := lightsBody(t, false, oneStep([]string{"joya"}, nil, 0))

	cases := []struct {
		query string
		want  int64
	}{
		{"", engine.DefaultLightShowTimeout.Milliseconds()},
		{"?ms=1", engine.MinLightShowTimeout.Milliseconds()},
		{"?ms=600000", engine.MaxLightShowTimeout.Milliseconds()},
	}
	for _, tc := range cases {
		t.Run("ms"+tc.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/periph/lights"+tc.query, body))
			if w.Code != http.StatusAccepted {
				t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
			}
			var resp periphLightsResponse
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.TimeoutMs != tc.want {
				t.Errorf("timeoutMs = %d, want %d", resp.TimeoutMs, tc.want)
			}
		})
	}
}

func TestPeriphLightsRefusals(t *testing.T) {
	srv, _, _ := newPeriphServer(t)

	tooMany := make([]periphLightStep, engine.MaxLightShowSteps+1)
	for i := range tooMany {
		tooMany[i] = oneStep([]string{"joya"}, nil, 10)
	}

	cases := []struct {
		name string
		body []byte
		want int
	}{
		{"no steps", []byte(`{"steps":[]}`), http.StatusBadRequest},
		{"missing steps", []byte(`{}`), http.StatusBadRequest},
		{"one seat", []byte(`{"steps":[{"seats":[["joya"]]}]}`), http.StatusBadRequest},
		{"three seats", []byte(`{"steps":[{"seats":[[],[],[]]}]}`), http.StatusBadRequest},
		{"unknown button", []byte(`{"steps":[{"seats":[["joye"],[]]}]}`), http.StatusBadRequest},
		{"negative hold", []byte(`{"steps":[{"seats":[[],[]],"holdMs":-1}]}`), http.StatusBadRequest},
		{"unknown field", []byte(`{"steps":[{"seats":[[],[]]}],"tempo":120}`), http.StatusBadRequest},
		{"not json", []byte(`nope`), http.StatusBadRequest},
		{"too many steps", lightsBody(t, false, tooMany...), http.StatusBadRequest},
		{"oversize body", make([]byte, maxPeriphLightsBytes+1), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/periph/lights", tc.body))
			if w.Code != tc.want {
				t.Errorf("status = %d (%s), want %d", w.Code, strings.TrimSpace(w.Body.String()), tc.want)
			}
			if tc.want == http.StatusBadRequest && strings.TrimSpace(w.Body.String()) == "" {
				t.Error("a refusal must name what was wrong")
			}
		})
	}
}

func TestBuildLightStepsRepairsFadeMs(t *testing.T) {
	cases := []struct {
		name   string
		fadeMs int
		want   time.Duration
	}{
		{"zero is a hard cut", 0, 0},
		{"negative repairs to zero", -250, 0},
		{"in range is kept", 250, 250 * time.Millisecond},
		{"over the cap is clamped", 20000, maxPeriphLightFadeMs * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps, err := buildLightSteps([]periphLightStep{
				{Seats: [][]string{{"joya"}, nil}, FadeMs: tc.fadeMs},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := steps[0].Fade; got != tc.want {
				t.Errorf("Fade = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPeriphLightsFadeMsReachesTheEngine(t *testing.T) {
	srv, rig, _ := newPeriphServer(t)
	body := lightsBody(t, false, periphLightStep{
		Seats: [][]string{{"joya"}, nil}, HoldMs: 2000, FadeMs: 400,
	})
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/periph/lights", body))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d (%s), want 202", w.Code, w.Body.String())
	}
	rig.Tick(1)
	if !rig.RT.LightShowActive() {
		t.Fatal("the show should be live after one tick")
	}

	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}
	rig.Tick(5) // ~167ms into a 400ms fade
	if got := hw.LampLevels(0)[0]; got == 0 || got == hardware.LampLevelMax {
		t.Errorf("joya mid-fade = %d, want it strictly between 0 and %d (fadeMs never reached the engine)", got, hardware.LampLevelMax)
	}
}

func TestPeriphLightsSharesThePreviewRateLimit(t *testing.T) {
	srv, _, _ := newPeriphServer(t)
	pinned := time.Now()
	srv.previewLimit.now = func() time.Time { return pinned }
	body := lightsBody(t, false, oneStep([]string{"joya"}, nil, 0))

	var last int
	for i := 0; i < previewBurst+2; i++ {
		w := httptest.NewRecorder()
		srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodPost, "/api/periph/lights", body))
		last = w.Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("status after the burst = %d, want 429", last)
	}

	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodDelete, "/api/periph/lights", nil))
	if w.Code != http.StatusAccepted {
		t.Errorf("cancel status = %d, want 202 (the undo route is never rate limited)", w.Code)
	}
}

func TestPeriphLightsCancelWithNothingPlayingSucceeds(t *testing.T) {
	srv, rig, _ := newPeriphServer(t)
	w := httptest.NewRecorder()
	srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodDelete, "/api/periph/lights", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	rig.Tick(1)
	if rig.RT.LightShowActive() {
		t.Error("cancelling should leave nothing playing")
	}
}

func TestPeriphStreamParamRefusals(t *testing.T) {
	srv, _, _ := newPeriphServer(t)
	for _, q := range []string{"?hz=0", "?hz=61", "?hz=lots", "?ms=0", "?ms=-1", "?ms=99999999"} {
		t.Run(q, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, previewReq(http.MethodGet, "/api/periph/stream"+q, nil))
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func streamServer(t *testing.T) (*httptest.Server, *testkit.Rig, *Server) {
	t.Helper()
	srv, rig, _ := newPeriphServer(t)
	ts := httptest.NewServer(srv.httpSrv.Handler)
	t.Cleanup(ts.Close)
	// The rig engine is single-threaded, so tick it from one goroutine while the client reads.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
				rig.Clock.Advance(rig.RT.FrameBudget())
				//nolint:errcheck // a tick error would fail the assertions
				rig.RT.Tick()
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })
	return ts, rig, srv
}

type sseEvent struct{ Name, Data string }

type stream struct {
	events <-chan sseEvent
	raw    <-chan string
	close  func()
}

func openStream(t *testing.T, ts *httptest.Server, query string) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/periph/stream"+query, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+previewToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		cancel()
		t.Fatalf("stream status = %d: %s", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	events := make(chan sseEvent, 256)
	raw := make(chan string, 512)
	go func() {
		defer close(events)
		defer close(raw)
		sc := bufio.NewScanner(res.Body)
		var name string
		for sc.Scan() {
			line := sc.Text()
			select {
			case raw <- line:
			default:
			}
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				select {
				case events <- sseEvent{Name: name, Data: strings.TrimPrefix(line, "data: ")}:
				default:
					return // the test stopped listening
				}
			}
		}
	}()
	s := &stream{events: events, raw: raw, close: func() { cancel(); res.Body.Close() }}
	t.Cleanup(s.close)
	return s
}

func (s *stream) next(t *testing.T, want string, into any) bool {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				return false
			}
			if ev.Name != want {
				continue
			}
			if into != nil {
				if err := json.Unmarshal([]byte(ev.Data), into); err != nil {
					t.Fatalf("decoding %q event: %v", want, err)
				}
			}
			return true
		case <-deadline:
			return false
		}
	}
}

func TestPeriphStreamHelloThenState(t *testing.T) {
	ts, rig, _ := streamServer(t)
	sc := openStream(t, ts, "?hz=60")

	var hello periphHello
	if !sc.next(t, "hello", &hello) {
		t.Fatal("no hello event")
	}
	if hello.PollHz != 60 || !hello.Mic {
		t.Errorf("hello = %+v, want 60Hz with the mic on", hello)
	}
	if len(hello.Buttons) != len(hardware.LightButtons) || hello.Buttons[0] != hardware.LightButtons[0] {
		t.Errorf("hello buttons = %v, want %v", hello.Buttons, hardware.LightButtons)
	}
	if hello.LevelMax != hardware.LampLevelMax {
		t.Errorf("hello levelMax = %d, want %d", hello.LevelMax, hardware.LampLevelMax)
	}

	var lights periphLights
	if !sc.next(t, "lights", &lights) {
		t.Fatal("no initial lights event")
	}
	if lights.Show {
		t.Error("no choreography is playing")
	}

	var info hardware.MicInfo
	if !sc.next(t, "micinfo", &info) {
		t.Fatal("no micinfo event")
	}
	if !info.Available || !info.Synthetic {
		t.Errorf("micInfo = %+v, want the virtual suite's synthetic source", info)
	}

	var mic periphMic
	if !sc.next(t, "mic", &mic) {
		t.Fatal("no mic level event")
	}
	if !mic.Synthetic {
		t.Error("a synthetic level frame must be labelled as one")
	}

	if !rig.RT.PeriphWatched() {
		t.Error("an open stream should arm the observation")
	}
	sc.close()
	deadline := time.Now().Add(3 * time.Second)
	for rig.RT.PeriphWatched() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if rig.RT.PeriphWatched() {
		t.Error("closing the stream should disarm the observation")
	}
}

func TestPeriphStreamCarriesInputAndLampChanges(t *testing.T) {
	ts, rig, _ := streamServer(t)
	sc := openStream(t, ts, "?hz=60&mic=0")

	if !sc.next(t, "hello", nil) {
		t.Fatal("no hello event")
	}

	rig.Press("joya")
	var batch periphInputBatch
	if !sc.next(t, "input", &batch) {
		t.Fatal("no input event after a press")
	}
	if len(batch.Events) == 0 || batch.Events[0].Name != "joya" {
		t.Errorf("input batch = %+v, want a joya press", batch)
	}
	if batch.Events[0].Source != "controller" {
		t.Errorf("input source = %q, want controller", batch.Events[0].Source)
	}

	rig.RT.Hardware.SetButtonLights(0, hardware.LightMask(1<<2).ButtonLights(), true)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var lights periphLights
		if !sc.next(t, "lights", &lights) {
			t.Fatal("no lights event after a lamp change")
		}
		if lights.Masks[0] == 1<<2 {
			if got := lights.Seats[0]; len(got) != 1 || got[0] != "joyc" {
				t.Errorf("lights seat 0 names = %v, want [joyc]", got)
			}
			return
		}
	}
	t.Fatal("the lamp change never arrived on the stream")
}

func TestPeriphStreamLightsCarriesLevels(t *testing.T) {
	ts, rig, _ := streamServer(t)
	sc := openStream(t, ts, "?hz=60&mic=0")
	if !sc.next(t, "hello", nil) {
		t.Fatal("no hello event")
	}
	if !sc.next(t, "lights", nil) {
		t.Fatal("no initial lights event")
	}

	rig.RT.Hardware.SetLampLevels(0, hardware.LampLevels{0: 7})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var lights periphLights
		if !sc.next(t, "lights", &lights) {
			t.Fatal("no lights event after a level-only change")
		}
		if lights.Levels[0][0] == 7 {
			return
		}
	}
	t.Fatal("the level change never arrived on the stream")
}

func TestPeriphStreamMicOffLeavesTheChannelOut(t *testing.T) {
	ts, rig, _ := streamServer(t)
	sc := openStream(t, ts, "?hz=60&mic=0")

	var hello periphHello
	if !sc.next(t, "hello", &hello) {
		t.Fatal("no hello event")
	}
	if hello.Mic {
		t.Error("hello should report the mic channel off")
	}
	for i := 0; i < 5; i++ {
		rig.Press("joya")
		time.Sleep(20 * time.Millisecond)
	}
	stop := time.After(300 * time.Millisecond)
	for {
		select {
		case line, ok := <-sc.raw:
			if !ok {
				return
			}
			if line == "event: mic" || line == "event: micinfo" {
				t.Fatalf("mic channel is off but %q arrived", line)
			}
		case <-stop:
			return
		}
	}
}

func TestPeriphStreamConcurrencyIsBounded(t *testing.T) {
	ts, _, _ := streamServer(t)
	for i := 0; i < maxPeriphStreams; i++ {
		sc := openStream(t, ts, "")
		if !sc.next(t, "hello", nil) {
			t.Fatalf("stream %d never said hello", i)
		}
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/periph/stream", nil)
	req.Header.Set("Authorization", "Bearer "+previewToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("stream %d status = %d, want 429", maxPeriphStreams+1, res.StatusCode)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Error("a refused stream should say when to try again")
	}
}

func TestPeriphStreamEndsOnItsDurationLimit(t *testing.T) {
	ts, rig, _ := streamServer(t)
	sc := openStream(t, ts, "?hz=60&ms=1")

	var bye map[string]string
	if !sc.next(t, "bye", &bye) {
		t.Fatal("no bye event before the duration ran out")
	}
	if !strings.Contains(bye["reason"], "duration") {
		t.Errorf("bye reason = %q, want it to name the duration limit", bye["reason"])
	}
	deadline := time.Now().Add(3 * time.Second)
	for rig.RT.PeriphWatched() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if rig.RT.PeriphWatched() {
		t.Error("a stream that ended on its own must disarm the observation")
	}
}

func TestPeriphStreamDroppedMidFlightReleasesTheEngine(t *testing.T) {
	ts, rig, _ := streamServer(t)
	sc := openStream(t, ts, "?hz=60")
	if !sc.next(t, "hello", nil) {
		t.Fatal("no hello event")
	}
	sc.close()

	deadline := time.Now().Add(5 * time.Second)
	for rig.RT.PeriphWatched() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if rig.RT.PeriphWatched() {
		t.Fatal("a vanished client left the observation armed")
	}
	before := rig.RT.Status().Ticks
	time.Sleep(50 * time.Millisecond)
	if rig.RT.Status().Ticks == before {
		t.Error("the engine stopped ticking after a client disconnect")
	}
}
