// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestTestPatternIsExactlyPanelSized(t *testing.T) {
	// The route refuses anything but an exact cols×rows×3 buffer.
	for _, size := range [][2]int{{64, 64}, {32, 16}, {1, 1}} {
		cols, rows := size[0], size[1]
		got := testPattern(cols, rows)
		if want := cols * rows * 3; len(got) != want {
			t.Errorf("%dx%d pattern is %d bytes, want %d", cols, rows, len(got), want)
		}
	}
}

func TestTestPatternMarksItsCorners(t *testing.T) {
	const cols, rows = 64, 64
	p := testPattern(cols, rows)
	at := func(x, y int) [3]byte {
		i := (y*cols + x) * 3
		return [3]byte{p[i], p[i+1], p[i+2]}
	}
	// The border overwrites the outermost ring, so the markers show one pixel in.
	if got := at(0, 0); got != [3]byte{255, 255, 255} {
		t.Errorf("top-left border pixel = %v, want white", got)
	}
	if got := at(1, 1); got != [3]byte{255, 0, 0} {
		t.Errorf("top-left marker = %v, want red", got)
	}
	if got := at(cols-2, 1); got != [3]byte{0, 255, 0} {
		t.Errorf("top-right marker = %v, want green", got)
	}
	if got := at(1, rows-2); got != [3]byte{0, 0, 255} {
		t.Errorf("bottom-left marker = %v, want blue", got)
	}
	if got := at(cols-2, rows-2); got != [3]byte{255, 255, 255} {
		t.Errorf("bottom-right marker = %v, want white", got)
	}
}

func solidImage(w, h int, c color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestImageToRGBCentersASmallSprite(t *testing.T) {
	const cols, rows = 8, 8
	img := solidImage(2, 2, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	got, err := imageToRGB(img, cols, rows, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != cols*rows*3 {
		t.Fatalf("buffer is %d bytes, want %d", len(got), cols*rows*3)
	}
	at := func(x, y int) [3]byte {
		i := (y*cols + x) * 3
		return [3]byte{got[i], got[i+1], got[i+2]}
	}
	// (8-2)/2 = 3, so the sprite occupies x,y in 3..4.
	if px := at(3, 3); px != [3]byte{10, 20, 30} {
		t.Errorf("centered pixel = %v, want the sprite's colour", px)
	}
	if px := at(0, 0); px != [3]byte{0, 0, 0} {
		t.Errorf("padding pixel = %v, want black", px)
	}
}

func TestImageToRGBRejectsAnOversizeSprite(t *testing.T) {
	_, err := imageToRGB(solidImage(65, 64, color.NRGBA{A: 255}), 64, 64, "big.png")
	if err == nil {
		t.Fatal("an image larger than the panel must be refused, not rescaled")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("error = %q, want it to say the image is too large", err)
	}
}

func TestImageToRGBTreatsTransparentAsBlack(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 200, B: 200, A: 0})
	img.SetNRGBA(1, 0, color.NRGBA{R: 200, G: 200, B: 200, A: 255})
	got, err := imageToRGB(img, 2, 1, "t")
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0 || got[1] != 0 || got[2] != 0 {
		t.Errorf("transparent pixel = %v, want black (the panel has no alpha)", got[:3])
	}
	if got[3] != 200 {
		t.Errorf("opaque pixel = %v, want 200", got[3:])
	}
}

func TestBuildSequenceProducesTheRouteEnvelope(t *testing.T) {
	const cols, rows = 4, 4
	img := solidImage(cols, rows, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	// buildSequence reads files; this exercises its encoding half.
	rgb, err := imageToRGB(img, cols, rows, "t")
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(sequenceBody{Frames: []sequenceFrame{
		{RGB: base64.StdEncoding.EncodeToString(rgb), DelayMs: 80},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var back sequenceBody
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Frames) != 1 || back.Frames[0].DelayMs != 80 {
		t.Fatalf("envelope = %+v, want one frame at 80ms", back)
	}
	raw, err := base64.StdEncoding.DecodeString(back.Frames[0].RGB)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != cols*rows*3 {
		t.Errorf("decoded frame is %d bytes, want %d", len(raw), cols*rows*3)
	}
}

func TestGlyphFlagParsing(t *testing.T) {
	var g glyphFlag
	if err := g.Set("◆=4,14,31,14,4,0,0,0"); err != nil {
		t.Fatalf("decimal rows: %v", err)
	}
	if err := g.Set("★=0x04,0x0e,0x1f,0x0e,0x1b,0x11,0x00,0x00"); err != nil {
		t.Fatalf("hex rows: %v", err)
	}
	if len(g.entries) != 2 {
		t.Fatalf("collected %d glyphs, want 2", len(g.entries))
	}
	if got := g.entries[0].rows; got[1] != 14 || got[2] != 31 {
		t.Errorf("decimal rows = %v", got)
	}
	if got := g.entries[1].rows; got[1] != 0x0e || got[4] != 0x1b {
		t.Errorf("hex rows = %v", got)
	}

	for _, bad := range []string{
		"no-equals",
		"=1,2,3,4,5,6,7,8",
		"ab=1,2,3,4,5,6,7,8",
		"◆=1,2,3",
		"◆=1,2,3,4,5,6,7,8,9",
		"◆=32,0,0,0,0,0,0,0",
		"◆=-1,0,0,0,0,0,0,0",
		"◆=a,b,c,d,e,f,g,h",
	} {
		var bg glyphFlag
		if err := bg.Set(bad); err == nil {
			t.Errorf("-glyph %q should have been refused", bad)
		}
	}
}

func TestBuildLCDBody(t *testing.T) {
	var g glyphFlag
	if err := g.Set("◆=4,14,31,14,4,0,0,0"); err != nil {
		t.Fatal(err)
	}
	body, err := buildLCD([]string{"HELLO ◆", "WORLD"}, g)
	if err != nil {
		t.Fatal(err)
	}
	var back lcdBody
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatal(err)
	}
	if back.Line1 != "HELLO ◆" || back.Line2 != "WORLD" {
		t.Errorf("lines = %q/%q", back.Line1, back.Line2)
	}
	if rows, ok := back.Glyphs["◆"]; !ok || len(rows) != 8 || rows[2] != 31 {
		t.Errorf("glyphs = %v", back.Glyphs)
	}

	body, err = buildLCD([]string{"ONLY"}, glyphFlag{})
	if err != nil {
		t.Fatal(err)
	}
	// Fresh destination: decoding into the previous one would hide a missing glyph key.
	var plain lcdBody
	if err := json.Unmarshal(body, &plain); err != nil {
		t.Fatal(err)
	}
	if plain.Line1 != "ONLY" || plain.Line2 != "" {
		t.Errorf("single-line body = %+v", plain)
	}
	if plain.Glyphs != nil {
		t.Errorf("no -glyph flags should mean no glyphs key, got %v", plain.Glyphs)
	}
}

func TestQueryRendersOnlyWhatWasSet(t *testing.T) {
	if got := (options{}).query(); got != "" {
		t.Errorf("default options produced %q, want no query string", got)
	}
	if got := (options{ms: 2500}).query("loop=1"); got != "?ms=2500&loop=1" {
		t.Errorf("query = %q", got)
	}
	if got := (options{}).query("gain=0.5"); got != "?gain=0.5" {
		t.Errorf("query = %q", got)
	}
}

func TestClientURLDefaultsTheScheme(t *testing.T) {
	c := &client{host: "localhost:8080"}
	if got := c.url("/api/preview"); got != "http://localhost:8080/api/preview" {
		t.Errorf("url = %q", got)
	}
	c = &client{host: "http://127.0.0.1:9090/"}
	if got := c.url("/api/preview"); got != "http://127.0.0.1:9090/api/preview" {
		t.Errorf("url = %q", got)
	}
}

func TestRunRefusesWithoutAToken(t *testing.T) {
	err := run([]string{"-token", "", "cancel"})
	if err == nil || !strings.Contains(err.Error(), "ARKADE_HTTP_TOKEN") {
		t.Errorf("err = %v, want it to name ARKADE_HTTP_TOKEN", err)
	}
}
