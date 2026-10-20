// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/display"
)

// qrASCII renders a module matrix as one line per row, '#' dark, '.' light.
func qrASCII(modules [][]bool) string {
	var b strings.Builder
	for _, row := range modules {
		for _, dark := range row {
			if dark {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// qrReadFormat reads the top-left format word copy back off the matrix.
func qrReadFormat(modules [][]bool) int {
	size := len(modules)
	_ = size
	bits := 0
	set := func(i int, dark bool) {
		if dark {
			bits |= 1 << i
		}
	}
	for i := 0; i <= 5; i++ {
		set(i, modules[i][8])
	}
	set(6, modules[7][8])
	set(7, modules[8][8])
	set(8, modules[8][7])
	for i := 9; i <= 14; i++ {
		set(i, modules[8][14-i])
	}
	return bits
}

// qrRoundTrip decodes a matrix from qrEncode back to its payload. ECC
// codewords are checked for count only.
func qrRoundTrip(t *testing.T, modules [][]bool) string {
	t.Helper()
	size := len(modules)
	version := (size - 17) / 4

	// Format info: both checking BCH validity and extracting the mask.
	format := qrReadFormat(modules) ^ 0x5412
	// A valid codeword is divisible by the BCH generator 0x537.
	rem := format
	for i := 14; i >= 10; i-- {
		if rem&(1<<i) != 0 {
			rem ^= 0x537 << (i - 10)
		}
	}
	if rem != 0 {
		t.Fatalf("format word %015b fails its BCH check", format)
	}
	if level := (format >> 13) & 3; level != qrECLevelL {
		t.Fatalf("format word encodes EC level %d, want L (%d)", level, qrECLevelL)
	}
	mask := (format >> 10) & 7

	// Rebuild the function-module map the same way the encoder does.
	ref := newQRMatrix(version)
	ref.drawFunctionPatterns(version)

	// Unmask and read the zigzag stream.
	work := make([][]bool, size)
	for y := range modules {
		work[y] = append([]bool(nil), modules[y]...)
	}
	m := &qrMatrix{size: size, modules: work, isFunc: ref.isFunc}
	m.applyMask(mask)
	var bits []bool
	for right := size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = size - 1 - vert
				}
				if !m.isFunc[y][x] {
					bits = append(bits, m.modules[y][x])
				}
			}
		}
	}

	// De-interleave the data codewords (drop the trailing ECC codewords).
	nb := qrNumBlocks[version-1]
	dataLen := qrDataCodewords[version-1]
	stream := make([]byte, 0, len(bits)/8)
	for i := 0; i+8 <= len(bits); i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			b <<= 1
			if bits[i+j] {
				b |= 1
			}
		}
		stream = append(stream, b)
	}
	if want := dataLen + nb*qrECCPerBlock[version-1]; len(stream) != want {
		t.Fatalf("symbol carries %d codewords, want %d", len(stream), want)
	}
	short := dataLen / nb
	numLong := dataLen % nb
	blocks := make([][]byte, nb)
	pos := 0
	for i := 0; i <= short; i++ {
		for b := 0; b < nb; b++ {
			blockLen := short
			if b >= nb-numLong {
				blockLen++
			}
			if i < blockLen {
				blocks[b] = append(blocks[b], stream[pos])
				pos++
			}
		}
	}
	var codewords []byte
	for _, b := range blocks {
		codewords = append(codewords, b...)
	}

	// Parse the byte-mode segment.
	if mode := codewords[0] >> 4; mode != 0b0100 {
		t.Fatalf("mode indicator %04b, want byte mode (0100)", mode)
	}
	if version >= 10 {
		count := int(codewords[0]&0xf)<<12 | int(codewords[1])<<4 | int(codewords[2])>>4
		out := make([]byte, count)
		for i := range out {
			out[i] = codewords[2+i]<<4 | codewords[3+i]>>4
		}
		return string(out)
	}
	count := int(codewords[0]&0xf)<<4 | int(codewords[1])>>4
	out := make([]byte, count)
	for i := range out {
		out[i] = codewords[1+i]<<4 | codewords[2+i]>>4
	}
	return string(out)
}

func TestQRFormatBitsKnownValues(t *testing.T) {
	// Level-L format words from the ISO 18004 table.
	want := []int{0x77c4, 0x72f3, 0x7daa, 0x789d, 0x662f, 0x6318, 0x6c41, 0x6976}
	for mask, w := range want {
		if got := qrFormatBits(mask); got != w {
			t.Errorf("format bits for L/mask %d = %#x, want %#x", mask, got, w)
		}
	}
}

func TestQRVersionSelectionAndDimensions(t *testing.T) {
	cases := []struct {
		length  int
		version int
	}{
		{1, 1}, {17, 1}, {18, 2}, {32, 2}, {33, 3}, {53, 3}, {54, 4},
		{106, 5}, {134, 6}, {154, 7}, {192, 8}, {230, 9}, {231, 10}, {271, 10},
	}
	for _, c := range cases {
		modules, err := qrEncode(strings.Repeat("x", c.length))
		if err != nil {
			t.Fatalf("%d bytes: %v", c.length, err)
		}
		wantSize := c.version*4 + 17
		if len(modules) != wantSize {
			t.Errorf("%d bytes: got %d modules/side, want %d (version %d)",
				c.length, len(modules), wantSize, c.version)
		}
		for y, row := range modules {
			if len(row) != wantSize {
				t.Fatalf("%d bytes: row %d is %d wide, want %d", c.length, y, len(row), wantSize)
			}
		}
	}
	if _, err := qrEncode(strings.Repeat("x", 272)); err == nil {
		t.Error("272 bytes should exceed version 10-L capacity")
	}
}

func TestQREncodeRoundTrip(t *testing.T) {
	payloads := []string{
		"A",
		"WIFI:T:WPA;S:arkade;P:secret123;H:false;;",
		"WIFI:T:WPA;S:a very long network name;P:with-a-long-passphrase-0123456789;H:false;;",
		strings.Repeat("arkade!", 38), // 266 bytes → version 10 (16-bit count)
	}
	for _, p := range payloads {
		modules, err := qrEncode(p)
		if err != nil {
			t.Fatalf("encode %q: %v", p, err)
		}
		if got := qrRoundTrip(t, modules); got != p {
			t.Errorf("round trip = %q, want %q", got, p)
		}
	}
}

func TestQRFunctionPatterns(t *testing.T) {
	modules, err := qrEncode("WIFI:T:WPA;S:arkade;P:secret123;H:false;;")
	if err != nil {
		t.Fatal(err)
	}
	size := len(modules)

	// Finder pattern cores are dark, separator rings light (spot checks).
	for _, corner := range [][2]int{{0, 0}, {size - 7, 0}, {0, size - 7}} {
		cx, cy := corner[0]+3, corner[1]+3
		if !modules[cy][cx] {
			t.Errorf("finder center (%d,%d) should be dark", cx, cy)
		}
		if modules[cy][cx+1] != true || modules[cy][cx+2] != false || modules[cy][cx+3] != true {
			t.Errorf("finder rings at (%d,%d) malformed", cx, cy)
		}
	}
	// Timing patterns alternate along row/column 6.
	for i := 8; i < size-8; i++ {
		if modules[6][i] != (i%2 == 0) || modules[i][6] != (i%2 == 0) {
			t.Fatalf("timing pattern broken at %d", i)
		}
	}
	if !modules[size-8][8] {
		t.Error("dark module (8, size-8) should be dark")
	}
}

// TestQRWifiGolden pins the exact symbol for fixed credentials (version 3,
// 29×29), verified externally with jsQR.
func TestQRWifiGolden(t *testing.T) {
	const payload = "WIFI:T:WPA;S:arkade;P:secret123;H:false;;"
	modules, err := qrEncode(payload)
	if err != nil {
		t.Fatal(err)
	}
	got := qrASCII(modules)
	path := "testdata/qr-wifi-arkade.txt"
	if os.Getenv("UPDATE_GOLDENS") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden updated: %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden missing (run with UPDATE_GOLDENS=1 to create): %v", err)
	}
	if string(want) != got {
		t.Errorf("QR symbol changed\n--- got ---\n%s", got)
	}
}

func TestQRCodeElementSpriteAndQuietZone(t *testing.T) {
	q, err := NewWifiQRCode("arkade", "secret123", QRCodeOptions{Border: intPtr(2)})
	if err != nil {
		t.Fatal(err)
	}
	if want := "WIFI:T:WPA;S:arkade;P:secret123;H:false;;"; q.Text() != want {
		t.Errorf("wifi payload = %q, want %q", q.Text(), want)
	}
	sprite := q.Render(stats())
	// The payload needs version 3 (29 modules) + a 2-module quiet zone.
	const wantSize = 29 + 2*2
	if len(sprite) != wantSize {
		t.Fatalf("sprite is %d rows, want %d", len(sprite), wantSize)
	}
	for y, row := range sprite {
		if len(row) != wantSize {
			t.Fatalf("row %d is %d wide, want %d", y, len(row), wantSize)
		}
		for x, p := range row {
			if p != display.White && p != display.Black {
				t.Fatalf("pixel (%d,%d) = %v, QR sprites are strictly black/white", x, y, p)
			}
			edge := x < 2 || y < 2 || x >= wantSize-2 || y >= wantSize-2
			if edge && p != display.White {
				t.Fatalf("quiet zone pixel (%d,%d) should be white", x, y)
			}
		}
	}

	// SetText re-encodes and marks the element dirty.
	q.MarkClean()
	if err := q.SetText("hello"); err != nil {
		t.Fatal(err)
	}
	if !q.IsDirty() {
		t.Error("SetText with new content should mark the element dirty")
	}
	if len(q.Render(stats())) != 21+2*2 { // "hello" fits version 1
		t.Error("sprite should shrink to version 1 for a short payload")
	}

	// A failed re-encode reports an error and keeps the old symbol.
	if err := q.SetText(strings.Repeat("x", 300)); err == nil {
		t.Error("oversized payload should error")
	}
	if q.Text() != "hello" {
		t.Error("failed SetText should keep the previous payload")
	}
}

// TestQRDumpForDecoder writes 0/1 matrices for external decoder
// verification when QR_DUMP names an output file. Not run normally.
func TestQRDumpForDecoder(t *testing.T) {
	path := os.Getenv("QR_DUMP")
	if path == "" {
		t.Skip("set QR_DUMP=<file> to dump matrices")
	}
	payloads := []string{
		"A",
		"hello world",
		"WIFI:T:WPA;S:arkade;P:secret123;H:false;;",
		"WIFI:T:WPA;S:a very long network name;P:with-a-long-passphrase-0123456789;H:false;;",
	}
	// One max-capacity payload per version 1-10.
	for v := 1; v <= 10; v++ {
		overhead := 2
		if v >= 10 {
			overhead = 3
		}
		payloads = append(payloads, strings.Repeat("x", qrDataCodewords[v-1]-overhead))
	}
	var b strings.Builder
	for _, p := range payloads {
		modules, err := qrEncode(p)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s\n", p)
		for _, row := range modules {
			for _, dark := range row {
				if dark {
					b.WriteByte('1')
				} else {
					b.WriteByte('0')
				}
			}
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func intPtr(v int) *int { return &v }
