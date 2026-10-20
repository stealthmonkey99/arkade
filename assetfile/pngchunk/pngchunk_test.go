// SPDX-License-Identifier: GPL-2.0-or-later

package pngchunk

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func samplePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 254})
	img.SetNRGBA(2, 1, color.NRGBA{B: 255, A: 128})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func types(chunks []Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Type
	}
	return out
}

func TestSplitJoinRoundTrip(t *testing.T) {
	data := samplePNG(t)
	chunks, err := Split(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 {
		t.Fatalf("split into %d chunks (%v), want at least IHDR/IDAT/IEND", len(chunks), types(chunks))
	}
	if chunks[0].Type != "IHDR" || chunks[len(chunks)-1].Type != "IEND" {
		t.Fatalf("chunk order is %v, want IHDR first and IEND last", types(chunks))
	}
	back, err := Join(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, data) {
		t.Fatalf("Join(Split(png)) is %d bytes, want the original %d, identical", len(back), len(data))
	}
}

func TestSplitCopiesData(t *testing.T) {
	data := samplePNG(t)
	chunks, err := Split(data)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), data...)
	for _, c := range chunks {
		for i := range c.Data {
			c.Data[i] ^= 0xFF
		}
	}
	if !bytes.Equal(data, before) {
		t.Error("scribbling on a Chunk's Data changed the bytes it was split from")
	}
}

func TestTextRoundTrip(t *testing.T) {
	data := samplePNG(t)
	const kw, val = "arkade:frames", `{"version":1,"layout":"strip","cels":3}`

	out, err := WriteText(data, kw, val)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadText(out, kw)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != val {
		t.Fatalf("ReadText = %q, %v; want %q, true", got, ok, val)
	}

	if !bytes.Contains(out, []byte(kw)) {
		t.Error("the keyword is not present in the file's bytes")
	}

	before, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	after, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("the rewritten file no longer decodes: %v", err)
	}
	if before.Bounds() != after.Bounds() {
		t.Fatalf("bounds changed: %v -> %v", before.Bounds(), after.Bounds())
	}
	for y := 0; y < after.Bounds().Dy(); y++ {
		for x := 0; x < after.Bounds().Dx(); x++ {
			if before.At(x, y) != after.At(x, y) {
				t.Fatalf("pixel (%d,%d) changed: %v -> %v", x, y, before.At(x, y), after.At(x, y))
			}
		}
	}
}

func TestTextLandsAfterIHDR(t *testing.T) {
	out, err := WriteText(samplePNG(t), "arkade:frames", "x")
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := Split(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 || chunks[1].Type != "tEXt" {
		t.Fatalf("chunk order is %v, want tEXt second", types(chunks))
	}
	for _, c := range chunks {
		if c.Type == "IDAT" {
			break
		}
		if c.Type == "tEXt" {
			return
		}
	}
	t.Error("the tEXt chunk is not before the first IDAT")
}

func TestSetTextUpdatesInPlace(t *testing.T) {
	data, err := WriteText(samplePNG(t), "arkade:frames", "first")
	if err != nil {
		t.Fatal(err)
	}
	first, err := Split(data)
	if err != nil {
		t.Fatal(err)
	}
	data, err = WriteText(data, "arkade:frames", "second")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Split(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("updating grew the file from %d chunks to %d (%v)", len(first), len(second), types(second))
	}
	for i := range first {
		if first[i].Type != second[i].Type {
			t.Fatalf("chunk order changed: %v -> %v", types(first), types(second))
		}
	}
	if v, _ := Text(second, "arkade:frames"); v != "second" {
		t.Errorf("value = %q, want %q", v, "second")
	}
	if bytes.Contains(data, []byte("first")) {
		t.Error("the old value is still in the file")
	}
}

func TestForeignChunksSurvive(t *testing.T) {
	chunks, err := Split(samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := textChunk("Copyright", "somebody else")
	if err != nil {
		t.Fatal(err)
	}
	phys := []byte{0, 0, 11, 19, 0, 0, 11, 19, 1} // 2835 px/m both axes
	private := []byte{7, 7, 7}
	extended := append([]Chunk{chunks[0],
		{Type: "tEXt", Data: theirs},
		{Type: "pHYs", Data: phys},
		{Type: "moOn", Data: private},
	}, chunks[1:]...)
	data, err := Join(extended)
	if err != nil {
		t.Fatal(err)
	}

	out, err := WriteText(data, "arkade:frames", "ours")
	if err != nil {
		t.Fatal(err)
	}
	after, err := Split(out)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := Text(after, "Copyright"); !ok || v != "somebody else" {
		t.Errorf("Copyright = %q, %v; want it untouched", v, ok)
	}
	if v, _ := Text(after, "arkade:frames"); v != "ours" {
		t.Errorf("our own value = %q, want %q", v, "ours")
	}
	found := map[string][]byte{}
	for _, c := range after {
		found[c.Type] = c.Data
	}
	if !bytes.Equal(found["pHYs"], phys) {
		t.Errorf("pHYs = %v, want %v", found["pHYs"], phys)
	}
	if !bytes.Equal(found["moOn"], private) {
		t.Errorf("the private moOn chunk = %v, want %v", found["moOn"], private)
	}
	pruned := DeleteText(after, "arkade:frames")
	if _, ok := Text(pruned, "arkade:frames"); ok {
		t.Error("DeleteText left our keyword behind")
	}
	if v, ok := Text(pruned, "Copyright"); !ok || v != "somebody else" {
		t.Error("DeleteText took somebody else's text with it")
	}
}

func TestSetTextReplacesDuplicates(t *testing.T) {
	chunks, err := Split(samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	dupe, err := textChunk("arkade:frames", "old")
	if err != nil {
		t.Fatal(err)
	}
	with := append([]Chunk{chunks[0],
		{Type: "tEXt", Data: dupe},
		{Type: "tEXt", Data: dupe},
	}, chunks[1:]...)

	out, err := SetText(with, "arkade:frames", "new")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range out {
		if k, _, ok := splitText(c.Data); c.Type == "tEXt" && ok && k == "arkade:frames" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("file carries %d copies of the keyword, want 1", n)
	}
	if v, _ := Text(out, "arkade:frames"); v != "new" {
		t.Errorf("value = %q, want %q", v, "new")
	}
	if v, _ := Text(with, "arkade:frames"); v != "old" {
		t.Error("SetText mutated its input")
	}
}

func TestCorruptInputIsRefused(t *testing.T) {
	good := samplePNG(t)

	cases := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"short", good[:4]},
		{"not a png", []byte("GIF89a and then some bytes to be going on with")},
		{"signature damaged", append([]byte{'x'}, good[1:]...)},
		{"truncated mid-file", good[:len(good)-9]},
		{"no IEND", good[:len(good)-12]},
	}
	for _, tc := range cases {
		if _, err := Split(tc.data); err == nil {
			t.Errorf("Split(%s) succeeded, want an error", tc.name)
		}
	}

	bad := append([]byte(nil), good...)
	bad[len(bad)-5] ^= 0xFF
	if _, err := Split(bad); err == nil {
		t.Error("Split accepted a chunk with a wrong CRC")
	}

	huge := append([]byte(nil), good...)
	binary.BigEndian.PutUint32(huge[8:12], 0x7FFFFFF0)
	if _, err := Split(huge); err == nil {
		t.Error("Split accepted a chunk longer than the file")
	}

	odd := append([]byte(nil), good...)
	copy(odd[12:16], "1234")
	if _, err := Split(odd); err == nil {
		t.Error("Split accepted a non-alphabetic chunk type")
	}

	if _, err := Split([]byte("nope")); !errors.Is(err, ErrNotPNG) {
		t.Errorf("Split of non-PNG bytes = %v, want ErrNotPNG", err)
	}
	if _, ok, err := ReadText(bad, "arkade:frames"); err == nil || ok {
		t.Errorf("ReadText of a damaged file = %v, %v; want an error", ok, err)
	}
	if _, err := WriteText(bad, "arkade:frames", "x"); err == nil {
		t.Error("WriteText rewrote a damaged file")
	}
}

func TestTrailingBytesIgnored(t *testing.T) {
	data := append(samplePNG(t), []byte("appended by something else")...)
	chunks, err := Split(data)
	if err != nil {
		t.Fatal(err)
	}
	if chunks[len(chunks)-1].Type != "IEND" {
		t.Fatalf("last chunk is %s, want IEND", chunks[len(chunks)-1].Type)
	}
	out, err := Join(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("appended by something else")) {
		t.Error("bytes after IEND were carried into the rewritten file")
	}
}

func TestMissingKeyword(t *testing.T) {
	v, ok, err := ReadText(samplePNG(t), "arkade:frames")
	if err != nil {
		t.Fatal(err)
	}
	if ok || v != "" {
		t.Errorf("ReadText of a plain PNG = %q, %v; want \"\", false", v, ok)
	}
}

func TestMalformedTextChunkIsSkipped(t *testing.T) {
	chunks, err := Split(samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	ours, err := textChunk("arkade:frames", "found me")
	if err != nil {
		t.Fatal(err)
	}
	with := append([]Chunk{chunks[0],
		{Type: "tEXt", Data: []byte("no separator here")},
		{Type: "tEXt", Data: ours},
	}, chunks[1:]...)
	if v, ok := Text(with, "arkade:frames"); !ok || v != "found me" {
		t.Errorf("Text = %q, %v; want %q, true", v, ok, "found me")
	}
	out, err := SetText(with, "arkade:frames", "replaced")
	if err != nil {
		t.Fatal(err)
	}
	kept := false
	for _, c := range out {
		if c.Type == "tEXt" && string(c.Data) == "no separator here" {
			kept = true
		}
	}
	if !kept {
		t.Error("a malformed tEXt chunk belonging to someone else was removed")
	}
}

func TestKeywordAndValueRules(t *testing.T) {
	chunks, err := Split(samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	bad := []struct {
		name, keyword, value string
	}{
		{"empty keyword", "", "v"},
		{"keyword too long", strings.Repeat("k", 80), "v"},
		{"leading space", " k", "v"},
		{"trailing space", "k ", "v"},
		{"doubled space", "a  b", "v"},
		{"control character in keyword", "a\tb", "v"},
		{"keyword outside Latin-1", "arkade:frāmesĀ", "v"},
		{"NUL in value", "k", "a\x00b"},
		{"value outside Latin-1", "k", "— an em dash"},
	}
	for _, tc := range bad {
		if _, err := SetText(chunks, tc.keyword, tc.value); err == nil {
			t.Errorf("SetText(%s) succeeded, want a refusal", tc.name)
		}
	}
	const kw, val = "Café", "prix: 3 50 £"
	out, err := SetText(chunks, kw, val)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Join(out)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadText(data, kw)
	if err != nil || !ok || got != val {
		t.Errorf("Latin-1 round trip = %q, %v, %v; want %q", got, ok, err, val)
	}
}

func TestJoinRebuildsCRCs(t *testing.T) {
	chunks, err := Split(samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	chunks, err = SetText(chunks, "arkade:frames", "some value or other")
	if err != nil {
		t.Fatal(err)
	}
	data, err := Join(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Split(data); err != nil {
		t.Fatalf("the file Join wrote does not pass its own CRC check: %v", err)
	}
	// The CRC covers the type and the data.
	off := len(Signature)
	n := int(binary.BigEndian.Uint32(data[off : off+4]))
	want := binary.BigEndian.Uint32(data[off+8+n : off+12+n])
	if got := crc32.ChecksumIEEE(data[off+4 : off+8+n]); got != want {
		t.Errorf("IHDR CRC = %08x, want %08x", got, want)
	}
}

func TestJoinRefusesNonsense(t *testing.T) {
	good, err := Split(samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		chunks []Chunk
	}{
		{"nothing at all", nil},
		{"no IHDR", good[1:]},
		{"no IEND", good[:len(good)-1]},
		{"bad chunk type", append(append([]Chunk{}, good[:1]...), Chunk{Type: "no", Data: nil}, good[len(good)-1])},
	}
	for _, tc := range cases {
		if _, err := Join(tc.chunks); err == nil {
			t.Errorf("Join(%s) succeeded, want a refusal", tc.name)
		}
	}
}
