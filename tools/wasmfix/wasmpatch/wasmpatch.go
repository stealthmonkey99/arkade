// SPDX-License-Identifier: GPL-2.0-or-later

// Package wasmpatch rewrites a TinyGo export from sret lowering into a genuine
// two-i32-result function, as cart_manifest() -> (ptr, len) requires (see the
// wasmfix command). It is a package so cartridge tests can patch in-process.
package wasmpatch

import (
	"fmt"
)

const (
	secType     = 1
	secImport   = 2
	secFunction = 3
	secExport   = 7
	secCode     = 10
)

const (
	externKindFunc = 0 // import/export "kind" byte for a function
)

type wasmSection struct {
	id      byte
	payload []byte
}

// PatchSretExport rewrites exportName and returns the module plus the (ptr, len)
// constants recovered from its sret body.
func PatchSretExport(data []byte, exportName string) (out []byte, ptr, length uint32, err error) {
	if len(data) < 8 || string(data[0:4]) != "\x00asm" {
		return nil, 0, 0, fmt.Errorf("not a wasm module (bad magic)")
	}
	header := append([]byte(nil), data[0:8]...)
	sections, err := splitSections(data[8:])
	if err != nil {
		return nil, 0, 0, err
	}

	numFuncImports, err := countFuncImports(findSection(sections, secImport))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("import section: %w", err)
	}

	typeSec := findSection(sections, secType)
	if typeSec == nil {
		return nil, 0, 0, fmt.Errorf("module has no type section")
	}
	typeCount, typeEntriesRaw, err := parseVectorRaw(typeSec.payload, parseFuncType)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("type section: %w", err)
	}

	funcSec := findSection(sections, secFunction)
	if funcSec == nil {
		return nil, 0, 0, fmt.Errorf("module has no function section")
	}
	funcTypeIdxs, err := parseFunctionSection(funcSec.payload)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("function section: %w", err)
	}

	exportSec := findSection(sections, secExport)
	if exportSec == nil {
		return nil, 0, 0, fmt.Errorf("module has no export section")
	}
	exports, err := parseExportSection(exportSec.payload)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("export section: %w", err)
	}
	expIdx := -1
	for i, e := range exports {
		if e.name == exportName && e.kind == externKindFunc {
			expIdx = i
			break
		}
	}
	if expIdx == -1 {
		return nil, 0, 0, fmt.Errorf("no function export named %q", exportName)
	}
	targetGlobalIdx := exports[expIdx].index
	if targetGlobalIdx < numFuncImports {
		return nil, 0, 0, fmt.Errorf("export %q resolves to an imported function, not a defined one", exportName)
	}
	definedIdx := targetGlobalIdx - numFuncImports
	if int(definedIdx) >= len(funcTypeIdxs) {
		return nil, 0, 0, fmt.Errorf("export %q function index %d out of range of function section (%d entries)", exportName, definedIdx, len(funcTypeIdxs))
	}

	codeSec := findSection(sections, secCode)
	if codeSec == nil {
		return nil, 0, 0, fmt.Errorf("module has no code section")
	}
	codeEntries, err := splitCodeEntries(codeSec.payload)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("code section: %w", err)
	}
	if int(definedIdx) >= len(codeEntries) {
		return nil, 0, 0, fmt.Errorf("export %q function index %d out of range of code section (%d entries)", exportName, definedIdx, len(codeEntries))
	}

	ptr, length, err = readSretConstants(codeEntries[definedIdx].body)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("export %q body did not match the expected sret-of-two-constants shape: %w", exportName, err)
	}

	newTypeIdx := uint32(typeCount)
	newTypeBytes := []byte{
		0x60,       // func type tag
		0x00,       // 0 params
		0x02,       // 2 results
		0x7F, 0x7F, // i32, i32
	}
	newTypeSectionPayload := uleb32(typeCount + 1)
	newTypeSectionPayload = append(newTypeSectionPayload, typeEntriesRaw...)
	newTypeSectionPayload = append(newTypeSectionPayload, newTypeBytes...)

	newFuncSectionPayload := encodeFunctionSection(append(append([]uint32(nil), funcTypeIdxs...), newTypeIdx))

	newBody := []byte{0x00}         // 0 local-declarations
	newBody = append(newBody, 0x41) // i32.const
	newBody = append(newBody, sleb128(int64(int32(ptr)))...)
	newBody = append(newBody, 0x41) // i32.const
	newBody = append(newBody, sleb128(int64(int32(length)))...)
	newBody = append(newBody, 0x0B) // end
	newCodeEntry := append(uleb32(uint32(len(newBody))), newBody...)

	var newCodeSectionPayload []byte
	newCodeSectionPayload = append(newCodeSectionPayload, uleb32(uint32(len(codeEntries)+1))...)
	for _, e := range codeEntries {
		newCodeSectionPayload = append(newCodeSectionPayload, e.raw...)
	}
	newCodeSectionPayload = append(newCodeSectionPayload, newCodeEntry...)

	newFuncGlobalIdx := numFuncImports + uint32(len(funcTypeIdxs))
	exports[expIdx].index = newFuncGlobalIdx
	newExportSectionPayload := encodeExportSection(exports)

	rewritten := make([]wasmSection, len(sections))
	copy(rewritten, sections)
	for i := range rewritten {
		switch rewritten[i].id {
		case secType:
			rewritten[i].payload = newTypeSectionPayload
		case secFunction:
			rewritten[i].payload = newFuncSectionPayload
		case secCode:
			rewritten[i].payload = newCodeSectionPayload
		case secExport:
			rewritten[i].payload = newExportSectionPayload
		}
	}

	out = append(out, header...)
	for _, s := range rewritten {
		out = append(out, s.id)
		out = append(out, uleb32(uint32(len(s.payload)))...)
		out = append(out, s.payload...)
	}
	return out, ptr, length, nil
}

func findSection(sections []wasmSection, id byte) *wasmSection {
	for i := range sections {
		if sections[i].id == id {
			return &sections[i]
		}
	}
	return nil
}

// splitSections splits the module body into raw (id, payload) sections in file
// order; custom sections pass through.
func splitSections(buf []byte) ([]wasmSection, error) {
	var sections []wasmSection
	r := &reader{buf: buf}
	for r.pos < len(r.buf) {
		id, err := r.byte()
		if err != nil {
			return nil, err
		}
		size, err := r.u32()
		if err != nil {
			return nil, fmt.Errorf("section %d size: %w", id, err)
		}
		payload, err := r.bytes(int(size))
		if err != nil {
			return nil, fmt.Errorf("section %d payload: %w", id, err)
		}
		sections = append(sections, wasmSection{id: id, payload: payload})
	}
	return sections, nil
}

func countFuncImports(sec *wasmSection) (uint32, error) {
	if sec == nil {
		return 0, nil
	}
	r := &reader{buf: sec.payload}
	count, err := r.u32()
	if err != nil {
		return 0, err
	}
	var funcImports uint32
	for i := uint32(0); i < count; i++ {
		if _, err := r.name(); err != nil { // module
			return 0, err
		}
		if _, err := r.name(); err != nil { // field
			return 0, err
		}
		kind, err := r.byte()
		if err != nil {
			return 0, err
		}
		switch kind {
		case 0x00: // func: typeidx
			if _, err := r.u32(); err != nil {
				return 0, err
			}
			funcImports++
		case 0x01: // table: reftype + limits
			if _, err := r.byte(); err != nil {
				return 0, err
			}
			if err := r.limits(); err != nil {
				return 0, err
			}
		case 0x02: // memory: limits
			if err := r.limits(); err != nil {
				return 0, err
			}
		case 0x03: // global: valtype + mutability
			if _, err := r.byte(); err != nil {
				return 0, err
			}
			if _, err := r.byte(); err != nil {
				return 0, err
			}
		default:
			return 0, fmt.Errorf("import %d: unknown kind byte %#x", i, kind)
		}
	}
	return funcImports, nil
}

func parseVectorRaw(payload []byte, parseOne func(r *reader) error) (count uint32, entriesRaw []byte, err error) {
	r := &reader{buf: payload}
	count, err = r.u32()
	if err != nil {
		return 0, nil, err
	}
	start := r.pos
	for i := uint32(0); i < count; i++ {
		if err := parseOne(r); err != nil {
			return 0, nil, fmt.Errorf("entry %d: %w", i, err)
		}
	}
	return count, payload[start:r.pos:r.pos], nil
}

func parseFuncType(r *reader) error {
	form, err := r.byte()
	if err != nil {
		return err
	}
	if form != 0x60 {
		return fmt.Errorf("expected functype tag 0x60, got %#x", form)
	}
	pc, err := r.u32()
	if err != nil {
		return err
	}
	if _, err := r.bytes(int(pc)); err != nil {
		return err
	}
	rc, err := r.u32()
	if err != nil {
		return err
	}
	if _, err := r.bytes(int(rc)); err != nil {
		return err
	}
	return nil
}

func parseFunctionSection(payload []byte) ([]uint32, error) {
	r := &reader{buf: payload}
	count, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]uint32, count)
	for i := range out {
		v, err := r.u32()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func encodeFunctionSection(idxs []uint32) []byte {
	out := uleb32(uint32(len(idxs)))
	for _, v := range idxs {
		out = append(out, uleb32(v)...)
	}
	return out
}

type wasmExport struct {
	name  string
	kind  byte
	index uint32
}

func parseExportSection(payload []byte) ([]wasmExport, error) {
	r := &reader{buf: payload}
	count, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]wasmExport, count)
	for i := range out {
		name, err := r.name()
		if err != nil {
			return nil, err
		}
		kind, err := r.byte()
		if err != nil {
			return nil, err
		}
		idx, err := r.u32()
		if err != nil {
			return nil, err
		}
		out[i] = wasmExport{name: name, kind: kind, index: idx}
	}
	return out, nil
}

func encodeExportSection(exports []wasmExport) []byte {
	out := uleb32(uint32(len(exports)))
	for _, e := range exports {
		out = append(out, uleb32(uint32(len(e.name)))...)
		out = append(out, e.name...)
		out = append(out, e.kind)
		out = append(out, uleb32(e.index)...)
	}
	return out
}

type codeEntry struct {
	raw  []byte // size prefix + body, exactly as it appeared in the file
	body []byte // just the body (locals + expr), no size prefix
}

func splitCodeEntries(payload []byte) ([]codeEntry, error) {
	r := &reader{buf: payload}
	count, err := r.u32()
	if err != nil {
		return nil, err
	}
	out := make([]codeEntry, count)
	for i := range out {
		start := r.pos
		size, err := r.u32()
		if err != nil {
			return nil, err
		}
		body, err := r.bytes(int(size))
		if err != nil {
			return nil, err
		}
		out[i] = codeEntry{raw: payload[start:r.pos:r.pos], body: body}
	}
	return out, nil
}

// readSretConstants decodes the body TinyGo emits for func() (uint32, uint32): local 0
// is the sret pointer and two compile-time constants are stored through it, in one
// of two shapes:
//
//	local.get 0; i64.const N;      i64.store align off=0            (packed)
//	local.get 0; i32.const N; i32.store align off={0,4}  (x2, either order)
//
// Anything else is refused rather than guessed at.
func readSretConstants(body []byte) (ptr, length uint32, err error) {
	r := &reader{buf: body}
	localGroups, err := r.u32()
	if err != nil {
		return 0, 0, err
	}
	for i := uint32(0); i < localGroups; i++ {
		if _, err := r.u32(); err != nil {
			return 0, 0, err
		}
		if _, err := r.byte(); err != nil {
			return 0, 0, err
		}
	}

	var gotPtr, gotLen bool
	for {
		op, err := r.byte()
		if err != nil {
			return 0, 0, err
		}
		if op == 0x0B { // end
			break
		}
		if op != 0x20 { // local.get
			return 0, 0, fmt.Errorf("unexpected opcode %#x, want local.get (0x20)", op)
		}
		localIdx, err := r.u32()
		if err != nil {
			return 0, 0, err
		}
		if localIdx != 0 {
			return 0, 0, fmt.Errorf("local.get %d, want the sret pointer at local 0", localIdx)
		}
		constOp, err := r.byte()
		if err != nil {
			return 0, 0, err
		}
		var val int64
		switch constOp {
		case 0x41: // i32.const
			v, err := r.sleb(32)
			if err != nil {
				return 0, 0, err
			}
			val = v
		case 0x42: // i64.const
			v, err := r.sleb(64)
			if err != nil {
				return 0, 0, err
			}
			val = v
		default:
			return 0, 0, fmt.Errorf("unexpected opcode %#x after local.get 0, want i32.const/i64.const", constOp)
		}
		storeOp, err := r.byte()
		if err != nil {
			return 0, 0, err
		}
		if _, err := r.u32(); err != nil { // align
			return 0, 0, err
		}
		offset, err := r.u32()
		if err != nil {
			return 0, 0, err
		}
		switch {
		case storeOp == 0x37 && constOp == 0x42 && offset == 0: // i64.store, packed
			ptr = uint32(val)
			length = uint32(uint64(val) >> 32)
			gotPtr, gotLen = true, true
		case storeOp == 0x36 && constOp == 0x41 && offset == 0: // i32.store ptr
			ptr = uint32(val)
			gotPtr = true
		case storeOp == 0x36 && constOp == 0x41 && offset == 4: // i32.store len
			length = uint32(val)
			gotLen = true
		default:
			return 0, 0, fmt.Errorf("unexpected store opcode %#x const %#x offset %d", storeOp, constOp, offset)
		}
	}
	if !gotPtr || !gotLen {
		return 0, 0, fmt.Errorf("body did not store both a pointer and a length (gotPtr=%v gotLen=%v)", gotPtr, gotLen)
	}
	return ptr, length, nil
}

type reader struct {
	buf []byte
	pos int
}

func (r *reader) byte() (byte, error) {
	if r.pos >= len(r.buf) {
		return 0, fmt.Errorf("unexpected EOF")
	}
	b := r.buf[r.pos]
	r.pos++
	return b, nil
}

func (r *reader) bytes(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.buf) {
		return nil, fmt.Errorf("unexpected EOF reading %d bytes", n)
	}
	b := r.buf[r.pos : r.pos+n : r.pos+n]
	r.pos += n
	return b, nil
}

// u32 decodes an unsigned LEB128 value.
func (r *reader) u32() (uint32, error) {
	var result uint64
	var shift uint
	for {
		b, err := r.byte()
		if err != nil {
			return 0, err
		}
		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return uint32(result), nil
		}
		shift += 7
		if shift >= 64 {
			return 0, fmt.Errorf("uleb128 too long")
		}
	}
}

// sleb decodes a signed LEB128 value of the given bit width.
func (r *reader) sleb(width int) (int64, error) {
	var result int64
	var shift uint
	var b byte
	var err error
	for {
		b, err = r.byte()
		if err != nil {
			return 0, err
		}
		result |= int64(b&0x7F) << shift
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	if shift < uint(width) && b&0x40 != 0 {
		result |= -1 << shift
	}
	return result, nil
}

func (r *reader) name() (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	b, err := r.bytes(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// limits skips a table or memory limits entry.
func (r *reader) limits() error {
	flag, err := r.byte()
	if err != nil {
		return err
	}
	if _, err := r.u32(); err != nil { // min
		return err
	}
	if flag&0x01 != 0 {
		if _, err := r.u32(); err != nil { // max
			return err
		}
	}
	return nil
}

func uleb32(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			out = append(out, b|0x80)
		} else {
			out = append(out, b)
			break
		}
	}
	return out
}

func sleb128(v int64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		signBitSet := b&0x40 != 0
		if (v == 0 && !signBitSet) || (v == -1 && signBitSet) {
			out = append(out, b)
			break
		}
		out = append(out, b|0x80)
	}
	return out
}
