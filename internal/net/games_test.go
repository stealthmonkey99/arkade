// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/scores"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func registerGame(t *testing.T, m engine.Manifest) {
	t.Helper()
	if m.New == nil {
		m.New = func(*engine.AppContext) engine.App { return noopApp{} }
	}
	engine.Register(m)
	t.Cleanup(func() { engine.Unregister(m.Slug) })
}

func tinyIcon() display.Sprite {
	return display.Sprite{
		{{R: 255, A: 255}, {G: 255, A: 255}},
		{{B: 255, A: 255}, {R: 8, G: 8, B: 8, A: 0}},
	}
}

func newAttractServer(t *testing.T, token string) (*Server, *storage.Store, *Ring) {
	t.Helper()
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{
		StartApp:     "games/testgame",
		DataDir:      dataDir,
		AttractAfter: 5 * time.Minute,
		StandbyAfter: 10 * time.Minute,
	})
	rig.Tick(1)
	store := storage.New(dataDir)
	ring := NewRing(500)
	srv := NewServer("127.0.0.1:0", rig.RT, ring, store, "test-version", token, nil)
	return srv, store, ring
}

func getGames(t *testing.T, srv *Server) gamesResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/games", nil)
	w := httptest.NewRecorder()
	srv.route("GET /api/games")(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/games = %d, want 200: %s", w.Code, w.Body.String())
	}
	var got gamesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, w.Body.String())
	}
	return got
}

func findGame(t *testing.T, resp gamesResponse, slug string) gameInfo {
	t.Helper()
	for _, g := range resp.Games {
		if g.Slug == slug {
			return g
		}
	}
	t.Fatalf("slug %q missing from /api/games (%d rows)", slug, len(resp.Games))
	return gameInfo{}
}

func TestGamesListRowContents(t *testing.T) {
	registerGame(t, engine.Manifest{
		Slug: "cart-iconic", Name: "Iconic", Type: engine.TypeGame,
		Version: "1.2.0", Icon: tinyIcon(),
	})
	srv, store, _ := newAttractServer(t, testToken)
	if err := scores.Save(store, "cart-iconic", scores.Table{Entries: []scores.Entry{
		{Score: 900, Name: "BOB"}, {Score: 100},
	}}); err != nil {
		t.Fatalf("seed scores: %v", err)
	}

	resp := getGames(t, srv)
	if !resp.ControlEnabled {
		t.Error("controlEnabled = false with a token configured")
	}
	g := findGame(t, resp, "cart-iconic")
	if g.Name != "Iconic" || g.Type != "game" || g.Version != "1.2.0" {
		t.Errorf("unexpected row metadata: %+v", g)
	}
	if !strings.HasPrefix(g.Icon, "data:image/png;base64,") {
		t.Errorf("icon = %q, want an inline PNG data URI", g.Icon)
	}
	if !g.ScoresEditable {
		t.Error("a game's scores must be editable")
	}
	if len(g.Scores) != 2 || g.Scores[0].Score != 900 || g.Scores[0].Name != "BOB" {
		t.Errorf("scores = %+v, want the seeded table", g.Scores)
	}
	if g.Attract != attractShowcase {
		t.Errorf("attract = %q, want %q", g.Attract, attractShowcase)
	}
}

func TestGamesListAttractClassification(t *testing.T) {
	registerGame(t, engine.Manifest{
		Slug: "cart-hasattract", Name: "Has Attract", Type: engine.TypeGame,
		NewAttract: func(*engine.AppContext) (engine.AttractRenderer, bool) { return nil, false },
	})
	registerGame(t, engine.Manifest{
		Slug: "cart-bare", Name: "Bare", Type: engine.TypeGame,
	})
	registerGame(t, engine.Manifest{
		Slug: "apps/notagame", Name: "Not A Game", Type: engine.TypeApp,
	})
	srv, _, _ := newAttractServer(t, testToken)
	resp := getGames(t, srv)

	if got := findGame(t, resp, "cart-hasattract").Attract; got != attractCustom {
		t.Errorf("cart-hasattract attract = %q, want %q", got, attractCustom)
	}
	if got := findGame(t, resp, "cart-bare").Attract; got != attractNone {
		t.Errorf("cart-bare attract = %q, want %q (no export, no score)", got, attractNone)
	}
	app := findGame(t, resp, "apps/notagame")
	if app.Attract != attractNone {
		t.Errorf("a non-game must never claim an attract slot, got %q", app.Attract)
	}
	if app.ScoresEditable {
		t.Error("a non-game must not be score-editable")
	}
}

func TestIconDataURIEmptySprite(t *testing.T) {
	if got := iconDataURI(nil); got != "" {
		t.Errorf("nil sprite = %q, want empty", got)
	}
	if got := iconDataURI(display.Sprite{{}, {}}); got != "" {
		t.Errorf("zero-width sprite = %q, want empty", got)
	}
	if got := iconDataURI(tinyIcon()); !strings.HasPrefix(got, "data:image/png;base64,iVBOR") {
		t.Errorf("sprite = %q, want a base64 PNG (magic iVBOR)", got)
	}
}

func attractRequest(slug, token string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/attract/"+slug, nil)
	req.SetPathValue("slug", slug)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestAttractPreviewAuth(t *testing.T) {
	registerGame(t, engine.Manifest{
		Slug: "cart-hasattract", Name: "Has Attract", Type: engine.TypeGame,
		NewAttract: func(*engine.AppContext) (engine.AttractRenderer, bool) { return nil, false },
	})

	t.Run("no token configured fails closed", func(t *testing.T) {
		srv, _, _ := newAttractServer(t, "")
		w := httptest.NewRecorder()
		srv.route("POST /api/attract/{slug...}")(w, attractRequest("cart-hasattract", "anything"))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", w.Code)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		srv, _, _ := newAttractServer(t, testToken)
		w := httptest.NewRecorder()
		srv.route("POST /api/attract/{slug...}")(w, attractRequest("cart-hasattract", ""))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		srv, _, _ := newAttractServer(t, testToken)
		w := httptest.NewRecorder()
		srv.route("POST /api/attract/{slug...}")(w, attractRequest("cart-hasattract", "nope"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}

func TestAttractModeOnlyForGames(t *testing.T) {
	withAttract := func(*engine.AppContext) (engine.AttractRenderer, bool) { return nil, false }
	scored := scores.Table{Entries: []scores.Entry{{Name: "ADA", Score: 10}}}
	for _, typ := range []engine.AppType{engine.TypeApp, engine.TypeScreensaver, engine.TypeBoot} {
		m := engine.Manifest{Slug: "x", Type: typ, NewAttract: withAttract}
		if got := attractModeFor(m, scored); got != attractNone {
			t.Errorf("%s with an attract export and scores: mode %q, want %q", typ, got, attractNone)
		}
	}
	if got := attractModeFor(engine.Manifest{Type: engine.TypeGame}, scored); got != attractShowcase {
		t.Errorf("scored game: mode %q, want %q", got, attractShowcase)
	}
}

func TestAttractPreviewRefusals(t *testing.T) {
	registerGame(t, engine.Manifest{Slug: "cart-bare", Name: "Bare", Type: engine.TypeGame})
	registerGame(t, engine.Manifest{Slug: "apps/notagame", Name: "Not A Game", Type: engine.TypeApp})
	srv, _, _ := newAttractServer(t, testToken)

	cases := []struct {
		name, slug string
		want       int
	}{
		{"unknown slug", "cart-ghost", http.StatusNotFound},
		{"game with no slot", "cart-bare", http.StatusConflict},
		{"non-game app", "apps/notagame", http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.route("POST /api/attract/{slug...}")(w, attractRequest(tc.slug, testToken))
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestAttractPreviewRefusedWithStandbyDisabled(t *testing.T) {
	registerGame(t, engine.Manifest{
		Slug: "cart-hasattract", Name: "Has Attract", Type: engine.TypeGame,
		NewAttract: func(*engine.AppContext) (engine.AttractRenderer, bool) { return nil, false },
	})
	srv, _ := newTestServerWithCart(t, testToken, nil) // AttractAfter 0
	w := httptest.NewRecorder()
	srv.route("POST /api/attract/{slug...}")(w, attractRequest("cart-hasattract", testToken))
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "standby is disabled") {
		t.Errorf("body = %q, want it to name the reason", w.Body.String())
	}
}

func TestAttractPreviewAccepted(t *testing.T) {
	registerGame(t, engine.Manifest{
		Slug: "cart-hasattract", Name: "Has Attract", Type: engine.TypeGame,
		NewAttract: func(*engine.AppContext) (engine.AttractRenderer, bool) { return nil, false },
	})
	srv, _, ring := newAttractServer(t, testToken)

	w := httptest.NewRecorder()
	srv.route("POST /api/attract/{slug...}")(w, attractRequest("cart-hasattract", testToken))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got struct{ Slug, Attract string }
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Slug != "cart-hasattract" || got.Attract != attractCustom {
		t.Errorf("response = %+v, want slug=cart-hasattract attract=custom", got)
	}
	var audited bool
	for _, e := range ring.Tail(0) {
		if strings.Contains(e.Msg, "attract preview requested: cart-hasattract") {
			audited = true
		}
	}
	if !audited {
		t.Error("accepted preview left no audit line in the ring")
	}
}

func putScores(t *testing.T, srv *Server, slug, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/scores/"+slug, bytes.NewReader([]byte(body)))
	req.SetPathValue("app", slug)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.route("PUT /api/scores/{app...}")(w, req)
	return w
}

func TestScoresWriteAuth(t *testing.T) {
	srvNoToken, _, _ := newAttractServer(t, "")
	if w := putScores(t, srvNoToken, "games/testgame", testToken, `{"entries":[]}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no token configured: status = %d, want 503", w.Code)
	}
	srv, _, _ := newAttractServer(t, testToken)
	if w := putScores(t, srv, "games/testgame", "", `{"entries":[]}`); w.Code != http.StatusUnauthorized {
		t.Errorf("missing token: status = %d, want 401", w.Code)
	}
	if w := putScores(t, srv, "games/testgame", "wrong", `{"entries":[]}`); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", w.Code)
	}
}

func TestScoresWriteRejections(t *testing.T) {
	registerGame(t, engine.Manifest{Slug: "apps/notagame", Name: "Not A Game", Type: engine.TypeApp})
	srv, store, _ := newAttractServer(t, testToken)
	if err := scores.Save(store, "games/testgame", scores.Table{Entries: []scores.Entry{{Score: 42, Name: "BOB"}}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cases := []struct {
		name, slug, body string
		want             int
	}{
		{"traversal slug", "../../etc", `{"entries":[]}`, http.StatusBadRequest},
		{"unregistered slug", "games/ghost", `{"entries":[]}`, http.StatusNotFound},
		{"non-game app", "apps/notagame", `{"entries":[]}`, http.StatusNotFound},
		{"missing entries key", "games/testgame", `{}`, http.StatusBadRequest},
		{"typo'd key", "games/testgame", `{"entires":[]}`, http.StatusBadRequest},
		{"not json", "games/testgame", `nope`, http.StatusBadRequest},
		{"too many entries", "games/testgame", `{"entries":[{"score":1},{"score":2},{"score":3},{"score":4},{"score":5},{"score":6},{"score":7},{"score":8},{"score":9},{"score":10},{"score":11}]}`, http.StatusBadRequest},
		{"negative score", "games/testgame", `{"entries":[{"score":-1}]}`, http.StatusBadRequest},
		{"over int32", "games/testgame", `{"entries":[{"score":2147483648}]}`, http.StatusBadRequest},
		{"name too long", "games/testgame", `{"entries":[{"score":1,"name":"ABCDEFGHIJKLM"}]}`, http.StatusBadRequest},
		{"name outside the alphabet", "games/testgame", `{"entries":[{"score":1,"name":"BO!"}]}`, http.StatusBadRequest},
		{"name with a control char", "games/testgame", "{\"entries\":[{\"score\":1,\"name\":\"A\\u0000B\"}]}", http.StatusBadRequest},
		{"name with an emoji", "games/testgame", `{"entries":[{"score":1,"name":"BO🎮"}]}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := putScores(t, srv, tc.slug, testToken, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}

	table := scores.Load(store, "games/testgame")
	if len(table.Entries) != 1 || table.Entries[0].Score != 42 || table.Entries[0].Name != "BOB" {
		t.Fatalf("a rejected write changed the stored table: %+v", table.Entries)
	}
}

func TestScoresWriteNormalizes(t *testing.T) {
	srv, store, ring := newAttractServer(t, testToken)

	w := putScores(t, srv, "games/testgame", testToken,
		`{"entries":[{"score":10,"name":" bob "},{"score":900,"name":"ada"},{"score":500}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var got scores.Table
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []scores.Entry{{Score: 900, Name: "ADA"}, {Score: 500}, {Score: 10, Name: "BOB"}}
	for i, e := range want {
		if got.Entries[i] != e {
			t.Fatalf("normalized entries = %+v, want %+v", got.Entries, want)
		}
	}
	if stored := scores.Load(store, "games/testgame"); len(stored.Entries) != 3 || stored.Entries[0] != want[0] {
		t.Fatalf("stored table = %+v, want %+v", stored.Entries, want)
	}
	var audited bool
	for _, e := range ring.Tail(0) {
		if strings.Contains(e.Msg, "scores written: games/testgame") {
			audited = true
		}
	}
	if !audited {
		t.Error("accepted write left no audit line in the ring")
	}
}

func TestScoresWriteClears(t *testing.T) {
	srv, store, _ := newAttractServer(t, testToken)
	if err := scores.Save(store, "games/testgame", scores.Table{Entries: []scores.Entry{{Score: 7}}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if w := putScores(t, srv, "games/testgame", testToken, `{"entries":[]}`); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if table := scores.Load(store, "games/testgame"); len(table.Entries) != 0 {
		t.Errorf("table = %+v, want cleared", table.Entries)
	}
}

func TestNewRoutesWiredThroughMux(t *testing.T) {
	registerGame(t, engine.Manifest{
		Slug: "cart-hasattract", Name: "Has Attract", Type: engine.TypeGame,
		NewAttract: func(*engine.AppContext) (engine.AttractRenderer, bool) { return nil, false },
	})
	srv, _, _ := newAttractServer(t, testToken)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop(context.Background())
	base := "http://" + srv.Addr()

	resp, err := http.Get(base + "/api/games")
	if err != nil {
		t.Fatalf("GET /api/games: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games = %d, want 200", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, base+"/api/attract/cart-hasattract", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/attract: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/attract = %d, want 200", resp2.StatusCode)
	}

	req3, _ := http.NewRequest(http.MethodPut, base+"/api/scores/games/testgame",
		bytes.NewReader([]byte(`{"entries":[{"score":5,"name":"BOB"}]}`)))
	req3.Header.Set("Authorization", "Bearer "+testToken)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("PUT /api/scores: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/scores = %d, want 200", resp3.StatusCode)
	}
	resp4, err := http.Get(base + "/api/scores/games/testgame")
	if err != nil {
		t.Fatalf("GET /api/scores: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/scores = %d, want 200", resp4.StatusCode)
	}
}
