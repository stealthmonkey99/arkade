// SPDX-License-Identifier: GPL-2.0-or-later

package hosttest

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// withChunk inserts a chunk after a PNG's IHDR.
func withChunk(t *testing.T, pngData []byte, typ string, body []byte) []byte {
	t.Helper()
	const ihdrEnd = 8 + 12 + 13
	var chunk bytes.Buffer
	binary.Write(&chunk, binary.BigEndian, uint32(len(body)))
	chunk.WriteString(typ)
	chunk.Write(body)
	binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(append([]byte(typ), body...)))
	out := append([]byte(nil), pngData[:ihdrEnd]...)
	out = append(out, chunk.Bytes()...)
	return append(out, pngData[ihdrEnd:]...)
}

// sheetPNG is a 4x2 image whose left half is red and right half
// half-transparent blue.
func sheetPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 2 {
				c = color.NRGBA{B: 200, A: 128}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const twoCels = `{"arkade":1,"kind":"sprite","name":"duo","cels":[` +
	`{"name":"red","x":0,"y":0,"w":2,"h":2,"anchor":[1,1]},` +
	`{"name":"blue","x":2,"y":0,"w":2,"h":2,"anchor":[0,0]}]}`

type sheetProbe struct {
	loaded, zipped, plain bool
	red, blue             *ui.Sprite
}

var sheetProbeBytes = []byte(`{"abi":1,"slug":"sheetprobe","name":"Sheet probe"}`)

func (p *sheetProbe) Manifest() []byte { return sheetProbeBytes }

func (p *sheetProbe) Init(s *cartlib.Setup) {
	text := s.LoadSheet("assets/duo.arkadesprite.png")
	zipped := s.LoadSheet("assets/sub/zipped.arkadesprite.png")
	p.loaded, p.zipped = text.Ok(), zipped.Ok()
	p.plain = s.LoadSheet("assets/plain.png").Ok()
	p.red, p.blue = text.Cel("red"), zipped.Cel("blue")
}

func (p *sheetProbe) HandleInput([]cartlib.InputRecord) {}
func (p *sheetProbe) Update(uint64)                     {}
func (p *sheetProbe) Render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)
	p.red.Draw(fb, 0, 0)
}

func TestMountAssetsServesEveryEnvelopedSheet(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := sheetPNG(t)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write([]byte(twoCels))
	zw.Close()
	files := map[string][]byte{
		"duo.arkadesprite.png":        withChunk(t, base, "tEXt", append([]byte("arkade\x00"), twoCels...)),
		"sub/zipped.arkadesprite.png": withChunk(t, base, "zTXt", append([]byte("arkade\x00\x00"), z.Bytes()...)),
		"plain.png":                   base,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, "assets", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h := New(t)
	if err := h.MountAssets(dir); err != nil {
		t.Fatal(err)
	}
	g := &sheetProbe{}
	h.Init(g)
	if !g.loaded || !g.zipped {
		t.Fatal("an enveloped sheet did not load")
	}
	if g.plain {
		t.Error("a PNG with no envelope loaded; the cabinet refuses one")
	}
	red := g.red
	if red == nil || red.W != 2 || red.H != 2 {
		t.Fatalf("red cel = %+v, want 2x2", red)
	}
	if want := []byte{255, 0, 0, 255}; red.Pix[:4] != string(want) {
		t.Errorf("red cel's first pixel = %v, want %v", red.Pix[:4], want)
	}
	blue := g.blue
	if want := []byte{0, 0, 200, 128}; blue == nil || blue.Pix[:4] != string(want) {
		t.Errorf("zTXt sheet's blue cel = %+v, want straight-alpha %v", blue, want)
	}
	h.AdvancePastSplash()
	h.Render()
	if got := h.Pixel(0, 0); got != (cartlib.Color{R: 255, A: 255}) {
		t.Errorf("drawn red cel = %v at (0, 0)", got)
	}
}

func TestMountAssetsRefusesACelOffTheImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := `{"arkade":1,"kind":"sprite","name":"bad","cels":[{"name":"wide","x":0,"y":0,"w":5,"h":2,"anchor":[0,0]}]}`
	data := withChunk(t, sheetPNG(t), "tEXt", append([]byte("arkade\x00"), bad...))
	if err := os.WriteFile(filepath.Join(dir, "assets", "bad.arkadesprite.png"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(t).MountAssets(dir); err == nil {
		t.Error("MountAssets accepted a cel wider than its sheet")
	}
}

func TestMountAssetsWithoutAnAssetsDirMountsNothing(t *testing.T) {
	if err := New(t).MountAssets(t.TempDir()); err != nil {
		t.Error(err)
	}
}
