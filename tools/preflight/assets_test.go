// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

func tempCart(t *testing.T, slug string) *Cart {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/stealthmonkey99/arkade\n\ngo 1.23.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "cartridges", slug)
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Cart{Slug: slug, Dir: dir, Root: root}
}

func writeTone(t *testing.T, c *Cart, name string, hz, seconds, amplitude float64) {
	t.Helper()
	const rate = 44100
	n := int(seconds * rate)
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = amplitude * math.Sin(2*math.Pi*hz*float64(i)/rate)
	}
	if err := sfxlib.WriteFile(filepath.Join(c.Dir, "assets", name), samples, rate); err != nil {
		t.Fatal(err)
	}
}

func TestWavAudibilitySkipsWithoutWavs(t *testing.T) {
	res := run(t, "wav-audibility", tempCart(t, "silent"))
	if res.Status != StatusSkip {
		t.Errorf("status %s, want skip for a cartridge with no WAVs", res.Status)
	}
}

func TestWavAudibilityPassesMidBandTone(t *testing.T) {
	c := tempCart(t, "beeper")
	writeTone(t, c, "blip.wav", 880, 0.3, 0.9)
	res := run(t, "wav-audibility", c)
	if res.Status != StatusPass {
		t.Errorf("an 880 Hz tone: status %s, want pass\n%s", res.Status, findings(res))
	}
}

func TestWavAudibilityWarnsButNeverFailsOnSubBass(t *testing.T) {
	c := tempCart(t, "thumper")
	writeTone(t, c, "drum.wav", 60, 0.3, 0.9)
	res := run(t, "wav-audibility", c)
	if res.Status != StatusWarn {
		t.Fatalf("a 60 Hz tone: status %s, want warn (never fail)\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "under the speaker model's floor", "the warning names the model, not a law")
	wantContains(t, findings(res), "audition it on the cabinet", "the warning says what to do with it")
}

const underTheFloor = "under the speaker model's floor"

func TestWavAudibilityFloorIsConfigurable(t *testing.T) {
	c := tempCart(t, "tunable")
	writeTone(t, c, "drum.wav", 60, 0.3, 0.9)

	c.Options.AudioFloor = audioFloorModel{hz: audioDefaultFloorHz, frac: audioDefaultFloorFrac}
	if got := findings(run(t, "wav-audibility", c)); !strings.Contains(got, underTheFloor) {
		t.Errorf("the reference model did not flag a 60 Hz tone:\n%s", got)
	}

	c.Options.AudioFloor = audioFloorModel{hz: 40, frac: audioDefaultFloorFrac}
	if got := findings(run(t, "wav-audibility", c)); strings.Contains(got, underTheFloor) {
		t.Errorf("-wav-floor-hz 40 still called a 60 Hz tone sub-floor:\n%s", got)
	}
}

func TestWavAudibilityHonoursPerCartridgeFloor(t *testing.T) {
	c := tempCart(t, "auditioned")
	writeTone(t, c, "drum.wav", 60, 0.3, 0.9)
	writeConfig(t, c, `{"wavFloor": {"hz": 40, "reason": "auditioned on the cabinet and kept"}}`)
	if got := findings(run(t, "wav-audibility", c)); strings.Contains(got, underTheFloor) {
		t.Errorf("a per-cartridge wavFloor override was ignored:\n%s", got)
	}
}

func TestWavAudibilityPassesBrightSound(t *testing.T) {
	c := tempCart(t, "chimer")
	writeTone(t, c, "ting.wav", 5000, 0.3, 0.9)
	res := run(t, "wav-audibility", c)
	if res.Status != StatusPass {
		t.Errorf("a 5 kHz tone: status %s, want pass — treble carries fine on small speakers\n%s", res.Status, findings(res))
	}
}

func TestWavAudibilityWarnsOnNearSilence(t *testing.T) {
	c := tempCart(t, "whisperer")
	writeTone(t, c, "quiet.wav", 880, 0.3, 0.01)
	res := run(t, "wav-audibility", c)
	if res.Status != StatusWarn {
		t.Fatalf("status %s, want warn for a near-silent asset\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "effectively silent", "a near-silent asset says so")
}

func TestWavAudibilityFailsUndecodableFile(t *testing.T) {
	c := tempCart(t, "corrupt")
	if err := os.WriteFile(filepath.Join(c.Dir, "assets", "broken.wav"), []byte("this is not a RIFF file"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := run(t, "wav-audibility", c)
	if res.Status != StatusFail {
		t.Errorf("status %s, want fail for an undecodable WAV\n%s", res.Status, findings(res))
	}
}

func writeSprite(t *testing.T, c *Cart, name string, w, h, alpha int, sprite bool) {
	t.Helper()
	dir := filepath.Join(c.Dir, "assets")
	if sprite {
		dir = filepath.Join(dir, "sprites")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: uint8(alpha)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSpriteHalves(t *testing.T, c *Cart, name string, w, h, topAlpha, bottomAlpha int) {
	t.Helper()
	dir := filepath.Join(c.Dir, "assets", "sprites")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		a := topAlpha
		if y >= h/2 {
			a = bottomAlpha
		}
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: uint8(a)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSpriteAlphaPassesOpaqueSprite(t *testing.T) {
	c := tempCart(t, "solid")
	writeSprite(t, c, "kettle.png", 8, 8, 255, true)
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusPass {
		t.Errorf("a fully opaque sprite: status %s, want pass\n%s", res.Status, findings(res))
	}
}

func TestSpriteAlphaWarnsOnNearOpaqueSprite(t *testing.T) {
	c := tempCart(t, "slipped")
	writeSprite(t, c, "handle.png", 8, 8, 251, true)
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusWarn {
		t.Fatalf("an A=251 sprite: status %s, want warn\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "opacity slider", "the warning names the likely cause")
}

func TestSpriteAlphaBlessesTheCanonDecorativeAlpha(t *testing.T) {
	c := tempCart(t, "doorway")
	writeSpriteHalves(t, c, "doorway.png", 16, 16, 254, 255)
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusPass {
		t.Fatalf("an A=254 region: status %s, want pass\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "decorative alpha", "the pass cites the canon rather than staying silent")
}

func TestSpriteAlphaBlessesACanonPeakWithNoMask(t *testing.T) {
	c := tempCart(t, "puffer")
	writeSprite(t, c, "puff-bright.png", 5, 5, 254, true)
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusPass {
		t.Fatalf("a sprite peaking at A=254: status %s, want pass\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "decorative alpha", "the pass cites the canon")
}

func TestSpriteAlphaStillFlagsTheRestOfTheNearOpaqueBand(t *testing.T) {
	for _, alpha := range []int{250, 251, 252, 253} {
		c := tempCart(t, "slipped")
		writeSprite(t, c, "handle.png", 8, 8, alpha, true)
		res := run(t, "sprite-alpha", c)
		if res.Status != StatusWarn {
			t.Errorf("A=%d: status %s, want warn — only 254 exactly is blessed\n%s", alpha, res.Status, findings(res))
		}
	}
}

func TestSpriteAlphaAcceptsDeliberateSoftArt(t *testing.T) {
	c := tempCart(t, "softie")
	writeSprite(t, c, "puff.png", 8, 8, 180, true)
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusPass {
		t.Errorf("an A=180 decorative sprite: status %s, want pass\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "decorative-overdraw canon", "the pass is explained, not silent")
}

func TestSpriteAlphaIgnoresNonSpriteArtwork(t *testing.T) {
	c := tempCart(t, "arty")
	writeSprite(t, c, "title.png", 16, 16, 251, false)
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusPass {
		t.Errorf("a near-opaque non-sprite PNG: status %s, want pass\n%s", res.Status, findings(res))
	}
}

func TestSpriteAlphaFailsFullyTransparentArt(t *testing.T) {
	c := tempCart(t, "ghost")
	writeSprite(t, c, "icon.png", 8, 8, 0, false)
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusFail {
		t.Fatalf("status %s, want fail for art that draws nothing\n%s", res.Status, findings(res))
	}
	wantContains(t, findings(res), "draws nothing", "the failure says what the player sees")
}

func TestSpriteAlphaFailsUndecodablePNG(t *testing.T) {
	c := tempCart(t, "mangled")
	if err := os.WriteFile(filepath.Join(c.Dir, "assets", "icon.png"), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusFail {
		t.Errorf("status %s, want fail for an undecodable PNG", res.Status)
	}
}

func TestSpriteAlphaIgnoresAntiAliasedEdges(t *testing.T) {
	c := tempCart(t, "smooth")
	dir := filepath.Join(c.Dir, "assets", "sprites")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			a := 255
			if x == 0 || y == 0 { // a one-pixel soft border: 19% of the art
				a = 250
			}
			img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 10, B: 200, A: uint8(a)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ball.png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	res := run(t, "sprite-alpha", c)
	if res.Status != StatusPass {
		t.Errorf("an anti-aliased sprite: status %s, want pass\n%s", res.Status, findings(res))
	}
	if strings.Contains(findings(res), "warn") {
		t.Errorf("anti-aliased edges warned:\n%s", findings(res))
	}
}
