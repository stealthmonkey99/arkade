// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

// The sheet registry is driven directly; sdk/tutorial's goldens cover the
// import wrappers end to end.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/display"
)

type logHost struct {
	assets map[string][]byte
	warns  []string
	draws  []struct {
		pix        []byte
		w, h, x, y int
	}
}

func (h *logHost) BundleAsset(path string) ([]byte, bool) {
	d, ok := h.assets[path]
	return d, ok
}

func (h *logHost) DrawCel(pix []byte, w, hgt, x, y int) {
	h.draws = append(h.draws, struct {
		pix        []byte
		w, h, x, y int
	}{append([]byte(nil), pix...), w, hgt, x, y})
}

func (h *logHost) Log(level LogLevel, msg string) { h.warns = append(h.warns, msg) }

func (h *logHost) Blit([]display.Pixel)                       {}
func (h *logHost) DrawPixel(int, int, display.Pixel)          {}
func (h *logHost) FillRect(int, int, int, int, display.Pixel) {}
func (h *logHost) LoadSprite([]byte) (uint32, error)          { return 0, nil }
func (h *logHost) DrawSprite(uint32, int, int)                {}

var _ sheetHost = (*logHost)(nil)

// buildSheet colours each pixel R=x, G=y, B=7, A=255, so a test can check
// which rectangle came back, not just its length.
func buildSheet(t *testing.T, w, h int, cels []assetfile.Cel) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 7, A: 255})
		}
	}
	var base bytes.Buffer
	if err := png.Encode(&base, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	env := &assetfile.Envelope{
		Format:        assetfile.FormatVersion,
		Kind:          assetfile.KindSprite,
		Name:          "test",
		Generator:     assetfile.Generator{Tool: "sheets_test", Version: "1"},
		SpritePayload: assetfile.SpritePayload{Cels: cels},
	}
	out, err := assetfile.WriteBytes("test.arkadesprite.png", env, base.Bytes())
	if err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	return out
}

func newTestRegistry(t *testing.T, assets map[string][]byte) (*sheetRegistry, *logHost) {
	t.Helper()
	h := &logHost{assets: assets}
	return newSheetRegistry(h), h
}

const testSheetPath = "assets/test.arkadesprite.png"

func TestLoadSheetCutsCelsAndReadsTheRightRectangle(t *testing.T) {
	sheet := buildSheet(t, 8, 4, []assetfile.Cel{
		{Name: "left", X: 0, Y: 0, W: 3, H: 2, Anchor: [2]int{1, 2}},
		{Name: "right", X: 5, Y: 2, W: 3, H: 2},
	})
	reg, host := newTestRegistry(t, map[string][]byte{testSheetPath: sheet})

	sh := reg.loadSheet(testSheetPath)
	if sh == 0 {
		t.Fatalf("loadSheet returned 0; warnings: %v", host.warns)
	}

	right := reg.sheetCel(sh, "right")
	if right == 0 {
		t.Fatalf("sheetCel(right) = 0; warnings: %v", host.warns)
	}
	c, ok := reg.cel(right, "test")
	if !ok {
		t.Fatal("cel handle did not resolve")
	}
	if c.w != 3 || c.h != 2 {
		t.Fatalf("cel is %dx%d, want 3x2", c.w, c.h)
	}
	// The cel starts at (5, 2), so its pixels run from R=5,G=2 to R=7,G=3.
	if got := c.pix[0:4]; !bytes.Equal(got, []byte{5, 2, 7, 255}) {
		t.Errorf("first pixel = %v, want [5 2 7 255]", got)
	}
	if got := c.pix[len(c.pix)-4:]; !bytes.Equal(got, []byte{7, 3, 7, 255}) {
		t.Errorf("last pixel = %v, want [7 3 7 255]", got)
	}

	left, _ := reg.cel(reg.sheetCel(sh, "left"), "test")
	rec := encodeCelInfo(left)
	if want := [celInfoBytes]byte{3, 0, 2, 0, 1, 0, 2, 0}; rec != want {
		t.Errorf("cel info = %v, want %v", rec, want)
	}
}

func TestLoadSheetIsIdempotentPerPath(t *testing.T) {
	sheet := buildSheet(t, 4, 4, []assetfile.Cel{{Name: "a", W: 2, H: 2}})
	reg, host := newTestRegistry(t, map[string][]byte{testSheetPath: sheet})

	first := reg.loadSheet(testSheetPath)
	second := reg.loadSheet(testSheetPath)
	if first == 0 || first != second {
		t.Fatalf("loadSheet twice = %d, %d; want the same non-zero handle (warnings: %v)", first, second, host.warns)
	}
	if len(reg.cels) != 1 {
		t.Errorf("registry holds %d cels after two loads of one sheet, want 1", len(reg.cels))
	}
}

func TestZeroHandlesAreSilentNoOps(t *testing.T) {
	reg, host := newTestRegistry(t, nil)

	if got := reg.sheetCel(0, "anything"); got != 0 {
		t.Errorf("sheetCel(0) = %d, want 0", got)
	}
	if _, ok := reg.cel(0, "host_cel_read"); ok {
		t.Error("cel(0) resolved")
	}
	reg.drawCel(0, 1, 2, 0)

	if len(host.warns) != 0 {
		t.Errorf("zero handles logged %d warnings, want 0: %v", len(host.warns), host.warns)
	}
	if len(host.draws) != 0 {
		t.Errorf("drawCel(0) drew %d times, want 0", len(host.draws))
	}
}

func TestUnknownHandlesWarnAndNeverPanic(t *testing.T) {
	reg, host := newTestRegistry(t, nil)

	if got := reg.sheetCel(99, "a"); got != 0 {
		t.Errorf("sheetCel(99) = %d, want 0", got)
	}
	if _, ok := reg.cel(99, "host_cel_info"); ok {
		t.Error("cel(99) resolved")
	}
	reg.drawCel(99, 0, 0, 0)
	if len(host.warns) != 3 {
		t.Errorf("got %d warnings, want one per bad handle: %v", len(host.warns), host.warns)
	}
}

func TestLoadSheetRefusals(t *testing.T) {
	good := buildSheet(t, 4, 4, []assetfile.Cel{{Name: "a", W: 2, H: 2}})
	oversized := buildSheet(t, 128, 128, []assetfile.Cel{{Name: "big", W: maxCelDim + 1, H: 2}})

	tests := []struct {
		name   string
		path   string
		assets map[string][]byte
	}{
		{"no bundle at all", testSheetPath, nil},
		{"no such asset", "assets/missing.arkadesprite.png", map[string][]byte{testSheetPath: good}},
		{"not a PNG", testSheetPath, map[string][]byte{testSheetPath: []byte("nope")}},
		{"no envelope", testSheetPath, map[string][]byte{testSheetPath: plainPNG(t)}},
		{"cel over the per-axis cap", testSheetPath, map[string][]byte{testSheetPath: oversized}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg, host := newTestRegistry(t, tc.assets)
			if got := reg.loadSheet(tc.path); got != 0 {
				t.Fatalf("loadSheet = %d, want 0", got)
			}
			if len(host.warns) == 0 {
				t.Error("refused silently; a refusal must say why")
			}
			if len(reg.cels) != 0 || reg.pixelBytes != 0 || len(reg.sheets) != 0 {
				t.Errorf("refused load left %d cels / %d bytes / %d sheets behind",
					len(reg.cels), reg.pixelBytes, len(reg.sheets))
			}
		})
	}
}

func TestSheetCountCapRefusesTheNinth(t *testing.T) {
	assets := map[string][]byte{}
	sheet := buildSheet(t, 4, 4, []assetfile.Cel{{Name: "a", W: 2, H: 2}})
	paths := make([]string, 0, maxSheetsPerCart+1)
	for i := 0; i <= maxSheetsPerCart; i++ {
		p := "assets/s" + string(rune('0'+i)) + ".arkadesprite.png"
		assets[p] = sheet
		paths = append(paths, p)
	}
	reg, host := newTestRegistry(t, assets)
	for i := 0; i < maxSheetsPerCart; i++ {
		if reg.loadSheet(paths[i]) == 0 {
			t.Fatalf("sheet %d refused early: %v", i, host.warns)
		}
	}
	if got := reg.loadSheet(paths[maxSheetsPerCart]); got != 0 {
		t.Fatalf("sheet %d loaded = %d, want 0 (cap is %d)", maxSheetsPerCart, got, maxSheetsPerCart)
	}
}

func TestDrawCelFlips(t *testing.T) {
	sheet := buildSheet(t, 2, 2, []assetfile.Cel{{Name: "q", W: 2, H: 2}})
	reg, host := newTestRegistry(t, map[string][]byte{testSheetPath: sheet})
	cel := reg.sheetCel(reg.loadSheet(testSheetPath), "q")
	if cel == 0 {
		t.Fatalf("no cel: %v", host.warns)
	}

	px := func(x, y byte) []byte { return []byte{x, y, 7, 255} }
	tests := []struct {
		flags uint32
		want  [][]byte
	}{
		{0, [][]byte{px(0, 0), px(1, 0), px(0, 1), px(1, 1)}},
		{1, [][]byte{px(1, 0), px(0, 0), px(1, 1), px(0, 1)}},          // flipX
		{2, [][]byte{px(0, 1), px(1, 1), px(0, 0), px(1, 0)}},          // flipY
		{3, [][]byte{px(1, 1), px(0, 1), px(1, 0), px(0, 0)}},          // both
		{1 | 0x8000, [][]byte{px(1, 0), px(0, 0), px(1, 1), px(0, 1)}}, // reserved bits ignored
	}
	for _, tc := range tests {
		host.draws = nil
		reg.drawCel(cel, 3, 4, tc.flags)
		if len(host.draws) != 1 {
			t.Fatalf("flags %#x: %d draws, want 1", tc.flags, len(host.draws))
		}
		d := host.draws[0]
		if d.w != 2 || d.h != 2 || d.x != 3 || d.y != 4 {
			t.Errorf("flags %#x: drew %dx%d at (%d,%d), want 2x2 at (3,4)", tc.flags, d.w, d.h, d.x, d.y)
		}
		var want []byte
		for _, p := range tc.want {
			want = append(want, p...)
		}
		if !bytes.Equal(d.pix, want) {
			t.Errorf("flags %#x: pixels %v, want %v", tc.flags, d.pix, want)
		}
	}
}

// plainPNG has no arkade chunk, which host_load_sheet must refuse.
func plainPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}
