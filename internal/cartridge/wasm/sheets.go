// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"
	"encoding/binary"

	"github.com/tetratelabs/wazero/api"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// Sheet sandbox caps, mirrored in the browser host so both refuse the same
// cartridge.
const (
	maxSheetPathLen  = 512
	maxSheetBytes    = 1 << 20 // bounds the decode; the cel caps bound memory
	maxCelNameBytes  = 64
	maxSheetsPerCart = 8
	maxCelsPerCart   = 256
	maxCelDim        = 64      // where ui.Mask's uint64 rows run out
	maxCelPixelBytes = 1 << 20 // resident cel RGBA across all sheets
	// maxSheetPixelBytes is checked against the IHDR before decoding, so a
	// small PNG declaring a huge image cannot force a huge allocation.
	maxSheetPixelBytes = 4 * maxCelPixelBytes
)

// celEntry is cut eagerly at load, so the per-frame calls never allocate.
type celEntry struct {
	name             string
	w, h             int
	anchorX, anchorY int
	pix              []byte // assetfile.CelRGBA: straight-alpha RGBA, row-major
}

type sheetEntry struct {
	path string
	cels map[string]uint32
}

type sheetHost interface {
	bundleReader
	Surface
}

// sheetRegistry is one cartridge's sheets and cels. Sheet and cel handles
// are separate 1-based counters.
type sheetRegistry struct {
	host sheetHost

	sheets map[uint32]*sheetEntry
	byPath map[string]uint32
	cels   map[uint32]*celEntry

	nextSheet uint32
	nextCel   uint32

	pixelBytes int

	flip []byte // reused mirror scratch; a per-frame draw must not allocate
}

func newSheetRegistry(host sheetHost) *sheetRegistry {
	return &sheetRegistry{
		host:   host,
		sheets: map[uint32]*sheetEntry{},
		byPath: map[string]uint32{},
		cels:   map[uint32]*celEntry{},
	}
}

// loadSheet returns the same handle for a path already loaded, and 0 with a
// warning for every failure. A refused sheet registers no cels.
func (r *sheetRegistry) loadSheet(path string) uint32 {
	if h, ok := r.byPath[path]; ok {
		return h
	}
	if len(r.sheets) >= maxSheetsPerCart {
		hostWarnf(r.host, "host_load_sheet: %q refused, already holding %d sheets", path, maxSheetsPerCart)
		return 0
	}
	data, ok := r.host.BundleAsset(path)
	if !ok {
		hostWarnf(r.host, "host_load_sheet: no asset %q in this cartridge's bundle", path)
		return 0
	}
	if len(data) > maxSheetBytes {
		hostWarnf(r.host, "host_load_sheet: %q is %d bytes, over the %d-byte cap", path, len(data), maxSheetBytes)
		return 0
	}
	w, h, interlaced, err := assetfile.SheetHeader(data)
	if err != nil {
		hostWarnf(r.host, "host_load_sheet: %q: %v", path, err)
		return 0
	}
	if interlaced {
		hostWarnf(r.host, "host_load_sheet: %q is an interlaced PNG; save it non-interlaced (the browser host cannot decode Adam7, and the two hosts must agree)", path)
		return 0
	}
	if w*h*4 > maxSheetPixelBytes {
		hostWarnf(r.host, "host_load_sheet: %q is %dx%d, over the %d-byte decode cap", path, w, h, maxSheetPixelBytes)
		return 0
	}
	sheet, err := assetfile.DecodeSheet(path, data)
	if err != nil {
		hostWarnf(r.host, "host_load_sheet: %v", err)
		return 0
	}

	cels := sheet.Envelope.Cels
	if len(r.cels)+len(cels) > maxCelsPerCart {
		hostWarnf(r.host, "host_load_sheet: %q would take the cel count to %d, over the %d cap",
			path, len(r.cels)+len(cels), maxCelsPerCart)
		return 0
	}
	added := 0
	for _, c := range cels {
		if c.W > maxCelDim || c.H > maxCelDim {
			hostWarnf(r.host, "host_load_sheet: %q cel %q is %dx%d, over the %d-pixel-per-axis cap",
				path, c.Name, c.W, c.H, maxCelDim)
			return 0
		}
		added += c.W * c.H * 4
	}
	if r.pixelBytes+added > maxCelPixelBytes {
		hostWarnf(r.host, "host_load_sheet: %q would take cel pixels to %d bytes, over the %d cap",
			path, r.pixelBytes+added, maxCelPixelBytes)
		return 0
	}

	entry := &sheetEntry{path: path, cels: make(map[string]uint32, len(cels))}
	for _, c := range cels {
		x, y, w, h, ok := sheet.Cel(c.Name)
		if !ok {
			continue
		}
		r.nextCel++
		r.cels[r.nextCel] = &celEntry{
			name:    c.Name,
			w:       w,
			h:       h,
			anchorX: clampU16(c.Anchor[0]),
			anchorY: clampU16(c.Anchor[1]),
			pix:     assetfile.CelRGBA(sheet.Image, x, y, w, h),
		}
		entry.cels[c.Name] = r.nextCel
	}
	r.pixelBytes += added
	r.nextSheet++
	r.sheets[r.nextSheet] = entry
	r.byPath[path] = r.nextSheet
	return r.nextSheet
}

// clampU16 keeps a hand-edited anchor from overflowing host_cel_info's u16.
func clampU16(v int) int {
	if v < 0 {
		return 0
	}
	if v > 0xFFFF {
		return 0xFFFF
	}
	return v
}

// cel treats handle 0 as a silent no-op, so a stored failed lookup drawn
// every frame cannot flood the log; any other unknown handle warns.
func (r *sheetRegistry) cel(handle uint32, what string) (*celEntry, bool) {
	if handle == 0 {
		return nil, false
	}
	c, ok := r.cels[handle]
	if !ok {
		hostWarnf(r.host, "%s: unknown cel handle %d", what, handle)
		return nil, false
	}
	return c, true
}

func (r *sheetRegistry) sheetCel(sheet uint32, name string) uint32 {
	if sheet == 0 {
		return 0
	}
	s, ok := r.sheets[sheet]
	if !ok {
		hostWarnf(r.host, "host_sheet_cel: unknown sheet handle %d", sheet)
		return 0
	}
	h, ok := s.cels[name]
	if !ok {
		hostWarnf(r.host, "host_sheet_cel: %q has no cel %q", s.path, name)
		return 0
	}
	return h
}

// celInfoBytes is host_cel_info's record: little-endian u16 w, h, anchorX,
// anchorY, readable in place from wasm memory.
const celInfoBytes = 8

func encodeCelInfo(c *celEntry) [celInfoBytes]byte {
	var out [celInfoBytes]byte
	binary.LittleEndian.PutUint16(out[0:2], uint16(c.w))
	binary.LittleEndian.PutUint16(out[2:4], uint16(c.h))
	binary.LittleEndian.PutUint16(out[4:6], uint16(c.anchorX))
	binary.LittleEndian.PutUint16(out[6:8], uint16(c.anchorY))
	return out
}

// drawCel mirrors on flags bit 0 (horizontal) and bit 1 (vertical). Other
// bits are reserved and ignored, not refused.
func (r *sheetRegistry) drawCel(handle uint32, x, y int, flags uint32) {
	c, ok := r.cel(handle, "host_draw_cel")
	if !ok {
		return
	}
	flipX := flags&1 != 0
	flipY := flags&2 != 0
	pix := c.pix
	if flipX || flipY {
		pix = r.mirror(c, flipX, flipY)
	}
	r.host.DrawCel(pix, c.w, c.h, x, y)
}

func (r *sheetRegistry) mirror(c *celEntry, flipX, flipY bool) []byte {
	n := c.w * c.h * 4
	if cap(r.flip) < n {
		r.flip = make([]byte, n)
	}
	dst := r.flip[:n]
	for y := 0; y < c.h; y++ {
		sy := y
		if flipY {
			sy = c.h - 1 - y
		}
		for x := 0; x < c.w; x++ {
			sx := x
			if flipX {
				sx = c.w - 1 - x
			}
			copy(dst[(y*c.w+x)*4:(y*c.w+x)*4+4], c.pix[(sy*c.w+sx)*4:])
		}
	}
	return dst
}

type sheetImports struct{ *sheetRegistry }

func (s sheetImports) load(_ context.Context, mod api.Module, ptr, length uint32) uint32 {
	path, ok := guestString(s.host, mod, "host_load_sheet:", ptr, length, maxSheetPathLen)
	if !ok {
		return 0
	}
	return s.loadSheet(path)
}

func (s sheetImports) celNamed(_ context.Context, mod api.Module, sheet, ptr, length uint32) uint32 {
	name, ok := guestString(s.host, mod, "host_sheet_cel:", ptr, length, maxCelNameBytes)
	if !ok {
		return 0
	}
	return s.sheetCel(sheet, name)
}

func (s sheetImports) info(_ context.Context, mod api.Module, handle, outPtr uint32) uint32 {
	c, ok := s.cel(handle, "host_cel_info")
	if !ok {
		return 0
	}
	rec := encodeCelInfo(c)
	if !mod.Memory().Write(outPtr, rec[:]) {
		hostWarnf(s.host, "host_cel_info: out ptr %#x/%d out of bounds", outPtr, celInfoBytes)
		return 0
	}
	return 1
}

func (s sheetImports) read(_ context.Context, mod api.Module, handle, rgbaPtr, rgbaCap uint32) uint32 {
	c, ok := s.cel(handle, "host_cel_read")
	if !ok {
		return 0
	}
	need := uint32(len(c.pix))
	// All or nothing: a half-copied cel would read as success.
	if rgbaCap < need {
		hostWarnf(s.host, "host_cel_read: cel %q needs %d bytes, cap is %d", c.name, need, rgbaCap)
		return 0
	}
	if !mod.Memory().Write(rgbaPtr, c.pix) {
		hostWarnf(s.host, "host_cel_read: out ptr %#x/%d out of bounds", rgbaPtr, need)
		return 0
	}
	return need
}

// draw is host_draw_cel: it draws a cel at x, y, mirrored by flags bit 0
// (horizontal) and bit 1 (vertical); other bits are ignored.
func (s sheetImports) draw(_ context.Context, _ api.Module, handle uint32, x, y int32, flags uint32) {
	s.drawCel(handle, int(x), int(y), flags)
}
