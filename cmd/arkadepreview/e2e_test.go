// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/net"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// These drive a real internal/net server over a real engine with this tool's
// client, so a mismatch in a field name, route, query parameter or
// Content-Type fails here even when both sides' unit tests pass.

const e2eToken = "smoke-token-0123456789abcdef-arkade"

type smokeApp struct{}

func (smokeApp) Init() error                                 { return nil }
func (smokeApp) HandleInput(clock.FrameTime)                 {}
func (smokeApp) Update(clock.FrameTime)                      {}
func (smokeApp) Render(clock.FrameTime, engine.RenderReason) {}
func (smokeApp) Teardown() error                             { return nil }

func init() {
	engine.Register(engine.Manifest{
		Slug:   "games/smoke",
		Name:   "Smoke",
		Type:   engine.TypeGame,
		Hidden: true,
		New:    func(*engine.AppContext) engine.App { return smokeApp{} },
	})
}

func newSmokeCabinet(t *testing.T) (*client, *testkit.Rig) {
	t.Helper()
	dataDir := t.TempDir()
	rig := testkit.NewRig(t, testkit.Options{StartApp: "games/smoke", DataDir: dataDir})
	rig.Tick(1)

	srv := net.NewServer("127.0.0.1:0", rig.RT, net.NewRing(100), storage.New(dataDir), "smoke", e2eToken, nil)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return &client{host: srv.Addr(), token: e2eToken, http: &http.Client{Timeout: 5 * time.Second}}, rig
}

func writePNG(t *testing.T, dir, name string, w, h int, c color.NRGBA) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, solidImage(w, h, c)); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSmokeStatusReportsThePanelSize(t *testing.T) {
	c, _ := newSmokeCabinet(t)
	st, err := c.fetchStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.DisplayCols != 64 || st.DisplayRows != 64 {
		t.Errorf("panel = %dx%d, want 64x64", st.DisplayCols, st.DisplayRows)
	}
	if st.Slug != "games/smoke" {
		t.Errorf("slug = %q", st.Slug)
	}
}

func TestSmokeFrameFromPNGReachesThePanel(t *testing.T) {
	c, rig := newSmokeCabinet(t)
	path := writePNG(t, t.TempDir(), "red.png", 64, 64, color.NRGBA{R: 255, A: 255})

	if err := c.frame(options{ms: 2000}, []string{path}); err != nil {
		t.Fatalf("frame: %v", err)
	}
	rig.Tick(1)
	if px := rig.PixelAt(0, 0); px.R != 255 || px.G != 0 {
		t.Errorf("panel pixel = %+v, want the PNG's red", px)
	}
}

func TestSmokeTestPatternReachesThePanel(t *testing.T) {
	c, rig := newSmokeCabinet(t)
	if err := c.frame(options{pattern: true, ms: 2000}, nil); err != nil {
		t.Fatalf("frame -pattern: %v", err)
	}
	rig.Tick(1)
	if px := rig.PixelAt(0, 0); px.R != 255 || px.G != 255 || px.B != 255 {
		t.Errorf("panel pixel = %+v, want the pattern's white border", px)
	}
}

func TestSmokeAnimSequenceReachesThePanel(t *testing.T) {
	c, rig := newSmokeCabinet(t)
	dir := t.TempDir()
	a := writePNG(t, dir, "a.png", 64, 64, color.NRGBA{R: 255, A: 255})
	b := writePNG(t, dir, "b.png", 64, 64, color.NRGBA{G: 255, A: 255})

	if err := c.anim(options{ms: 5000, delay: 50, loop: true}, []string{a, b}); err != nil {
		t.Fatalf("anim: %v", err)
	}
	rig.Tick(1)
	if px := rig.PixelAt(0, 0); px.R != 255 {
		t.Fatalf("first cel = %+v, want red", px)
	}
	rig.TickFor("60ms")
	if px := rig.PixelAt(0, 0); px.G != 255 {
		t.Errorf("second cel = %+v, want green", px)
	}
}

func TestSmokeLCDAndGlyphReachTheCabinet(t *testing.T) {
	c, rig := newSmokeCabinet(t)
	var g glyphFlag
	if err := g.Set("◆=0x04,0x0e,0x1f,0x0e,0x04,0,0,0"); err != nil {
		t.Fatal(err)
	}
	if err := c.lcd(options{ms: 2000, glyphs: g}, []string{"SMOKE ◆", "TEST"}); err != nil {
		t.Fatalf("lcd: %v", err)
	}
	rig.Tick(1)
	if !rig.RT.LabPreviewActive() {
		t.Fatal("the LCD preview never reached the engine")
	}
}

func TestSmokeSoundAndCancel(t *testing.T) {
	c, rig := newSmokeCabinet(t)
	path := filepath.Join(t.TempDir(), "s.wav")
	if err := os.WriteFile(path, []byte("RIFF....WAVEfmt fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.sound(options{ms: 30000, gain: 0.3}, []string{path}); err != nil {
		t.Fatalf("sound: %v", err)
	}
	rig.Tick(1)
	var played bool
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "play" && ev.Owner == engine.LabPreviewOwner && ev.Gain == 0.3 {
			played = true
		}
	}
	if !played {
		t.Fatalf("the uploaded WAV never reached the audio service: %+v", rig.Audio.Events())
	}

	if err := c.cancel(); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	rig.Tick(1)
	if rig.RT.LabPreviewActive() {
		t.Error("cancel did not end the preview")
	}
}

func TestSmokeServerRefusesABadToken(t *testing.T) {
	c, _ := newSmokeCabinet(t)
	c.token = "wrong"
	err := c.cancel()
	if err == nil {
		t.Fatal("a wrong token must fail")
	}
	if got := err.Error(); !strings.Contains(got, "401") {
		t.Errorf("err = %q, want the server's 401 passed through", got)
	}
}
