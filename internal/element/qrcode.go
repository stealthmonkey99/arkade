// SPDX-License-Identifier: GPL-2.0-or-later

package element

import (
	"fmt"
	"strings"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// QRCode renders text as a QR code, one display pixel per module, with a
// white quiet-zone border. Encoding is byte mode, level L, smallest version.
type QRCode struct {
	Base
	text   string
	border int
	sprite display.Sprite
}

// QRCodeOptions configures a QRCode element.
type QRCodeOptions struct {
	Options
	// Border is the quiet-zone width in modules (default 1).
	Border *int
}

// NewQRCode builds a QR code element for arbitrary text.
func NewQRCode(text string, opts QRCodeOptions) (*QRCode, error) {
	q := &QRCode{border: 1}
	if opts.Border != nil {
		q.border = *opts.Border
	}
	q.InitBase(opts.Options)
	q.Impl = q.renderImpl
	if err := q.SetText(text); err != nil {
		return nil, err
	}
	return q, nil
}

// NewWifiQRCode encodes WPA network credentials in the standard WIFI:
// scheme, escaping the SSID and passkey.
func NewWifiQRCode(ssid, passkey string, opts QRCodeOptions) (*QRCode, error) {
	return NewQRCode(fmt.Sprintf("WIFI:T:WPA;S:%s;P:%s;H:false;;", escapeWifi(ssid), escapeWifi(passkey)), opts)
}

// escapeWifi backslash-escapes the characters reserved by the WiFi-QR
// grammar (\ ; , : ") in an SSID or passphrase value.
func escapeWifi(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`;`, `\;`,
		`,`, `\,`,
		`:`, `\:`,
		`"`, `\"`,
	).Replace(s)
}

func (q *QRCode) renderImpl(clock.FrameTime) display.Sprite { return q.sprite }

// Text returns the encoded payload.
func (q *QRCode) Text() string { return q.text }

// SetText re-encodes the symbol. The sprite is unchanged if encoding fails.
func (q *QRCode) SetText(v string) error {
	modules, err := qrEncode(v)
	if err != nil {
		return err
	}
	size := len(modules) + 2*q.border
	sprite := make(display.Sprite, 0, size)
	blankRow := func() []display.Pixel {
		row := make([]display.Pixel, size)
		for i := range row {
			row[i] = display.White
		}
		return row
	}
	for i := 0; i < q.border; i++ {
		sprite = append(sprite, blankRow())
	}
	for _, mrow := range modules {
		row := blankRow()
		for x, dark := range mrow {
			if dark {
				row[q.border+x] = display.Black
			}
		}
		sprite = append(sprite, row)
	}
	for i := 0; i < q.border; i++ {
		sprite = append(sprite, blankRow())
	}
	q.dirty = q.dirty || q.text != v
	q.text = v
	q.sprite = sprite
	return nil
}

// qrECLevelL is the level L indicator from table 25 ("01").
const qrECLevelL = 1

// Per-version level-L capacity tables (versions 1-10).
var (
	qrDataCodewords = [...]int{19, 34, 55, 80, 108, 136, 156, 194, 232, 274}
	qrECCPerBlock   = [...]int{7, 10, 15, 20, 26, 18, 20, 24, 30, 18}
	qrNumBlocks     = [...]int{1, 1, 1, 1, 1, 2, 2, 2, 2, 4}
	qrAlignCenters  = [...][]int{
		nil, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34},
		{6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50},
	}
)

// GF(256) log/antilog tables over the QR polynomial x^8+x^4+x^3+x^2+1.
var qrExp, qrLog = func() (exp, log [256]int) {
	x := 1
	for i := 0; i < 255; i++ {
		exp[i] = x
		log[x] = i
		x <<= 1
		if x >= 256 {
			x ^= 0x11d
		}
	}
	return
}()

func qrMul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return qrExp[(qrLog[a]+qrLog[b])%255]
}

// qrECC computes the degree-n Reed-Solomon remainder for one block.
func qrECC(data []byte, n int) []byte {
	// Generator polynomial Π(x − α^i) for i = 0..n-1, highest degree first.
	gen := []int{1}
	for i := 0; i < n; i++ {
		next := make([]int, len(gen)+1)
		for j, c := range gen {
			next[j] ^= c
			next[j+1] ^= qrMul(c, qrExp[i])
		}
		gen = next
	}
	// Polynomial long division of data·x^n by gen.
	rem := make([]int, n)
	for _, d := range data {
		factor := int(d) ^ rem[0]
		copy(rem, rem[1:])
		rem[n-1] = 0
		for j := 0; j < n; j++ {
			rem[j] ^= qrMul(factor, gen[j+1])
		}
	}
	out := make([]byte, n)
	for i, v := range rem {
		out[i] = byte(v)
	}
	return out
}

// qrFormatBits builds the masked 15-bit format word for level L + mask.
func qrFormatBits(mask int) int {
	data := qrECLevelL<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	return (data<<10 | rem) ^ 0x5412
}

// qrVersionBits builds the 18-bit version word (only used for version ≥ 7).
func qrVersionBits(version int) int {
	rem := version
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1f25)
	}
	return version<<12 | rem
}

// qrCodewords packs the payload into the version's data codewords: mode +
// count + data, terminator, then the EC/11 pad byte cycle.
func qrCodewords(data []byte, version int) []byte {
	total := qrDataCodewords[version-1]
	var bits []bool
	appendBits := func(val, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (val>>i)&1 == 1)
		}
	}
	appendBits(0b0100, 4) // byte mode
	countBits := 8
	if version >= 10 {
		countBits = 16
	}
	appendBits(len(data), countBits)
	for _, b := range data {
		appendBits(int(b), 8)
	}
	for i := 0; i < 4 && len(bits) < total*8; i++ { // terminator
		bits = append(bits, false)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	out := make([]byte, 0, total)
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			b <<= 1
			if bits[i+j] {
				b |= 1
			}
		}
		out = append(out, b)
	}
	for pad := 0; len(out) < total; pad++ {
		if pad%2 == 0 {
			out = append(out, 0xec)
		} else {
			out = append(out, 0x11)
		}
	}
	return out
}

// qrInterleave splits the data codewords into the version's RS blocks,
// appends ECC, and interleaves both per the spec.
func qrInterleave(codewords []byte, version int) []byte {
	nb := qrNumBlocks[version-1]
	eccLen := qrECCPerBlock[version-1]
	short := len(codewords) / nb
	numLong := len(codewords) % nb
	blocks := make([][]byte, nb)
	eccs := make([][]byte, nb)
	pos := 0
	for i := 0; i < nb; i++ {
		size := short
		if i >= nb-numLong { // long blocks come last
			size++
		}
		blocks[i] = codewords[pos : pos+size]
		eccs[i] = qrECC(blocks[i], eccLen)
		pos += size
	}
	out := make([]byte, 0, len(codewords)+nb*eccLen)
	for i := 0; i <= short; i++ {
		for _, b := range blocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	for i := 0; i < eccLen; i++ {
		for _, e := range eccs {
			out = append(out, e[i])
		}
	}
	return out
}

// qrMatrix carries the module grid plus the function-module reservations
// used during placement and masking.
type qrMatrix struct {
	size    int
	modules [][]bool // [row][col], true = dark
	isFunc  [][]bool
}

func newQRMatrix(version int) *qrMatrix {
	size := version*4 + 17
	m := &qrMatrix{size: size, modules: make([][]bool, size), isFunc: make([][]bool, size)}
	for i := range m.modules {
		m.modules[i] = make([]bool, size)
		m.isFunc[i] = make([]bool, size)
	}
	return m
}

func (m *qrMatrix) setFunc(x, y int, dark bool) {
	m.modules[y][x] = dark
	m.isFunc[y][x] = true
}

// drawFinder stamps a 7×7 finder pattern (plus its light separator ring)
// with its top-left module at x0,y0, clipped to the symbol.
func (m *qrMatrix) drawFinder(x0, y0 int) {
	for dy := -1; dy <= 7; dy++ {
		for dx := -1; dx <= 7; dx++ {
			x, y := x0+dx, y0+dy
			if x < 0 || y < 0 || x >= m.size || y >= m.size {
				continue
			}
			inner := dx >= 0 && dx <= 6 && dy >= 0 && dy <= 6
			dark := inner && (dx == 0 || dx == 6 || dy == 0 || dy == 6 ||
				(dx >= 2 && dx <= 4 && dy >= 2 && dy <= 4))
			m.setFunc(x, y, dark)
		}
	}
}

// drawFunctionPatterns stamps finders, timing, alignment, the dark module,
// and version info; format areas are reserved with a placeholder word.
func (m *qrMatrix) drawFunctionPatterns(version int) {
	m.drawFinder(0, 0)
	m.drawFinder(m.size-7, 0)
	m.drawFinder(0, m.size-7)

	for i := 8; i < m.size-8; i++ { // timing patterns
		m.setFunc(i, 6, i%2 == 0)
		m.setFunc(6, i, i%2 == 0)
	}

	centers := qrAlignCenters[version-1]
	for _, cy := range centers {
		for _, cx := range centers {
			// Skip the three corners occupied by finder patterns.
			if (cx == 6 && cy == 6) || (cx == 6 && cy == m.size-7) || (cx == m.size-7 && cy == 6) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					m.setFunc(cx+dx, cy+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}

	m.drawFormatBits(0) // placeholder to reserve the areas
	if version >= 7 {
		bits := qrVersionBits(version)
		for i := 0; i < 18; i++ {
			dark := (bits>>i)&1 == 1
			a := m.size - 11 + i%3
			b := i / 3
			m.setFunc(a, b, dark)
			m.setFunc(b, a, dark)
		}
	}
}

// drawFormatBits writes both copies of the format word for the given mask,
// plus the always-dark module above the bottom-left finder.
func (m *qrMatrix) drawFormatBits(mask int) {
	bits := qrFormatBits(mask)
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	// Copy around the top-left finder.
	for i := 0; i <= 5; i++ {
		m.setFunc(8, i, bit(i))
	}
	m.setFunc(8, 7, bit(6))
	m.setFunc(8, 8, bit(7))
	m.setFunc(7, 8, bit(8))
	for i := 9; i <= 14; i++ {
		m.setFunc(14-i, 8, bit(i))
	}
	// Split copy along the other two finders.
	for i := 0; i <= 7; i++ {
		m.setFunc(m.size-1-i, 8, bit(i))
	}
	for i := 8; i <= 14; i++ {
		m.setFunc(8, m.size-15+i, bit(i))
	}
	m.setFunc(8, m.size-8, true) // dark module
}

// drawCodewords places the interleaved codewords in the standard two-column
// zigzag, skipping function modules and the vertical timing column.
func (m *qrMatrix) drawCodewords(data []byte) {
	i := 0
	for right := m.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < m.size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				upward := (right+1)&2 == 0
				y := vert
				if upward {
					y = m.size - 1 - vert
				}
				if !m.isFunc[y][x] && i < len(data)*8 {
					m.modules[y][x] = (data[i/8]>>(7-i%8))&1 == 1
					i++
				}
				// Any leftover remainder bits stay light (zero).
			}
		}
	}
}

// applyMask XORs the mask pattern over the data modules (self-inverse).
func (m *qrMatrix) applyMask(mask int) {
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if m.isFunc[y][x] {
				continue
			}
			var invert bool
			switch mask {
			case 0:
				invert = (x+y)%2 == 0
			case 1:
				invert = y%2 == 0
			case 2:
				invert = x%3 == 0
			case 3:
				invert = (x+y)%3 == 0
			case 4:
				invert = (x/3+y/2)%2 == 0
			case 5:
				invert = x*y%2+x*y%3 == 0
			case 6:
				invert = (x*y%2+x*y%3)%2 == 0
			case 7:
				invert = ((x+y)%2+x*y%3)%2 == 0
			}
			if invert {
				m.modules[y][x] = !m.modules[y][x]
			}
		}
	}
}

// penalty scores the symbol with the four ISO 18004 mask evaluation rules.
func (m *qrMatrix) penalty() int {
	const n1, n2, n3, n4 = 3, 3, 40, 10
	score := 0
	at := func(x, y int) bool { return m.modules[y][x] }

	// Rule 1: runs of ≥5 same-colored modules in a row/column.
	for y := 0; y < m.size; y++ {
		for _, vertical := range []bool{false, true} {
			run, prev := 0, false
			for x := 0; x < m.size; x++ {
				cur := at(x, y)
				if vertical {
					cur = at(y, x)
				}
				if x > 0 && cur == prev {
					run++
					if run == 5 {
						score += n1
					} else if run > 5 {
						score++
					}
				} else {
					run = 1
				}
				prev = cur
			}
		}
	}

	// Rule 2: 2×2 blocks of the same color.
	for y := 0; y < m.size-1; y++ {
		for x := 0; x < m.size-1; x++ {
			c := at(x, y)
			if c == at(x+1, y) && c == at(x, y+1) && c == at(x+1, y+1) {
				score += n2
			}
		}
	}

	// Rule 3: finder-like 1011101 pattern with 4 light modules on a side.
	pattern := [11]bool{false, false, false, false, true, false, true, true, true, false, true}
	matches := func(x, y, dx, dy int, reversed bool) bool {
		for i := 0; i < 11; i++ {
			want := pattern[i]
			if reversed {
				want = pattern[10-i]
			}
			if at(x+dx*i, y+dy*i) != want {
				return false
			}
		}
		return true
	}
	for y := 0; y < m.size; y++ {
		for x := 0; x+11 <= m.size; x++ {
			if matches(x, y, 1, 0, false) || matches(x, y, 1, 0, true) {
				score += n3
			}
		}
	}
	for x := 0; x < m.size; x++ {
		for y := 0; y+11 <= m.size; y++ {
			if matches(x, y, 0, 1, false) || matches(x, y, 0, 1, true) {
				score += n3
			}
		}
	}

	// Rule 4: dark-module proportion, 10 points per 5% step from 50%.
	dark := 0
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if at(x, y) {
				dark++
			}
		}
	}
	total := m.size * m.size
	k := (abs(dark*20-total*10)+total-1)/total - 1
	return score + k*n4
}

// qrEncode is a minimal ISO/IEC 18004 encoder: byte mode, level L, the
// smallest of versions 1-10 that fits (271 bytes at version 10), and mask
// selection by penalty score. It returns the module matrix, [row][col], true
// = dark.
func qrEncode(text string) ([][]bool, error) {
	data := []byte(text)
	version := 0
	for v := 1; v <= len(qrDataCodewords); v++ {
		overhead := 12 // mode (4 bits) + count (8 bits)
		if v >= 10 {
			overhead = 20 // count widens to 16 bits
		}
		if len(data)*8+overhead <= qrDataCodewords[v-1]*8 {
			version = v
			break
		}
	}
	if version == 0 {
		return nil, fmt.Errorf("qr: payload too long (%d bytes, max 271)", len(data))
	}

	final := qrInterleave(qrCodewords(data, version), version)

	m := newQRMatrix(version)
	m.drawFunctionPatterns(version)
	m.drawCodewords(final)

	// Pick the mask with the lowest penalty (ties break to the lowest id).
	best, bestScore := 0, -1
	for mask := 0; mask < 8; mask++ {
		m.applyMask(mask)
		m.drawFormatBits(mask)
		if score := m.penalty(); bestScore < 0 || score < bestScore {
			best, bestScore = mask, score
		}
		m.applyMask(mask) // undo (XOR is self-inverse)
	}
	m.applyMask(best)
	m.drawFormatBits(best)
	return m.modules, nil
}
