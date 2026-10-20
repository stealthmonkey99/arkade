// SPDX-License-Identifier: GPL-2.0-or-later

// Package pngchunk is a chunk-level view of a PNG file: split a byte stream into
// chunks, join them back, and read or write a tEXt chunk's value without
// disturbing anything else. image/png is a pixel codec and drops all other
// metadata on a round trip.
//
// The asset envelope (assetfile) rides in a sprite's tEXt chunk so the file stays an
// ordinary PNG. tEXt is used rather than a private chunk because pngcheck,
// exiftool and strings can read it. zTXt, for larger envelopes, is assetfile's,
// built on Split and Join.
//
// A tEXt value is Latin-1 while a Go string is UTF-8: Text decodes each byte as
// one rune and SetText encodes each rune below U+0100 as one byte, refusing
// anything above.
package pngchunk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"strings"
)

// Signature is the eight bytes every PNG file begins with.
const Signature = "\x89PNG\r\n\x1a\n"

// maxChunk is the PNG limit on chunk data length (2^31-1); it also stops a corrupt
// length from becoming a huge allocation.
const maxChunk = 0x7FFFFFFF

// ErrNotPNG reports bytes with the wrong signature or too short to have one.
var ErrNotPNG = errors.New("not a PNG file")

// A Chunk is one PNG chunk. Length and CRC are derived by Join, so they are not stored.
type Chunk struct {
	Type string
	Data []byte
}

// Split parses a PNG byte stream into its chunks, in file order. It is strict:
// signature, lengths, type spelling and CRCs must check out, and the stream must
// begin with IHDR and reach IEND. Bytes after IEND are ignored, so
// Join(Split(x)) can be shorter than x.
func Split(data []byte) ([]Chunk, error) {
	if len(data) < len(Signature) || string(data[:len(Signature)]) != Signature {
		return nil, ErrNotPNG
	}
	rest := data[len(Signature):]
	var chunks []Chunk
	for {
		if len(rest) < 8 {
			return nil, fmt.Errorf("truncated PNG: %d trailing bytes cannot begin a chunk", len(rest))
		}
		n := int(binary.BigEndian.Uint32(rest[:4]))
		if n < 0 || n > maxChunk {
			return nil, fmt.Errorf("chunk declares %d bytes, past the PNG limit", uint32(n))
		}
		typ := string(rest[4:8])
		if err := checkType(typ); err != nil {
			return nil, err
		}
		if len(rest) < 12+n {
			return nil, fmt.Errorf("%s chunk declares %d bytes but only %d remain", typ, n, len(rest)-8)
		}
		body := rest[8 : 8+n]
		want := binary.BigEndian.Uint32(rest[8+n : 12+n])
		if got := crc32.ChecksumIEEE(rest[4 : 8+n]); got != want {
			return nil, fmt.Errorf("%s chunk CRC is %08x, want %08x — the file is damaged", typ, got, want)
		}
		// Copied so a Chunk does not alias the caller's buffer.
		chunks = append(chunks, Chunk{Type: typ, Data: append([]byte(nil), body...)})
		rest = rest[12+n:]
		if typ == "IEND" {
			break
		}
	}
	if chunks[0].Type != "IHDR" {
		return nil, fmt.Errorf("first chunk is %s, want IHDR", chunks[0].Type)
	}
	return chunks, nil
}

// Join reassembles chunks into a PNG byte stream, recomputing lengths and CRCs.
func Join(chunks []Chunk) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, errors.New("no chunks to write")
	}
	if chunks[0].Type != "IHDR" {
		return nil, fmt.Errorf("first chunk is %s, want IHDR", chunks[0].Type)
	}
	if last := chunks[len(chunks)-1]; last.Type != "IEND" {
		return nil, fmt.Errorf("last chunk is %s, want IEND", last.Type)
	}
	size := len(Signature)
	for _, c := range chunks {
		if err := checkType(c.Type); err != nil {
			return nil, err
		}
		if len(c.Data) > maxChunk {
			return nil, fmt.Errorf("%s chunk is %d bytes, past the PNG limit", c.Type, len(c.Data))
		}
		size += 12 + len(c.Data)
	}
	out := make([]byte, 0, size)
	out = append(out, Signature...)
	for _, c := range chunks {
		var head [8]byte
		binary.BigEndian.PutUint32(head[:4], uint32(len(c.Data)))
		copy(head[4:], c.Type)
		out = append(out, head[:]...)
		out = append(out, c.Data...)
		var sum [4]byte
		h := crc32.NewIEEE()
		h.Write(head[4:])
		h.Write(c.Data)
		binary.BigEndian.PutUint32(sum[:], h.Sum32())
		out = append(out, sum[:]...)
	}
	return out, nil
}

// Text returns the value of the first tEXt chunk carrying keyword, and whether there was one.
func Text(chunks []Chunk, keyword string) (string, bool) {
	for _, c := range chunks {
		if c.Type != "tEXt" {
			continue
		}
		k, v, ok := splitText(c.Data)
		if ok && k == keyword {
			return v, true
		}
	}
	return "", false
}

// SetText returns chunks with keyword's tEXt set to value, leaving the input
// unmodified. An existing chunk is replaced in place and later duplicates are
// dropped; a new one goes directly after IHDR.
func SetText(chunks []Chunk, keyword, value string) ([]Chunk, error) {
	if err := checkKeyword(keyword); err != nil {
		return nil, err
	}
	body, err := textChunk(keyword, value)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 || chunks[0].Type != "IHDR" {
		return nil, fmt.Errorf("cannot set %q: the chunk list does not start with IHDR", keyword)
	}
	out := make([]Chunk, 0, len(chunks)+1)
	placed := false
	for _, c := range chunks {
		if c.Type == "tEXt" {
			if k, _, ok := splitText(c.Data); ok && k == keyword {
				if placed {
					continue // a duplicate of one we have already written
				}
				out = append(out, Chunk{Type: "tEXt", Data: body})
				placed = true
				continue
			}
		}
		out = append(out, c)
		if !placed && c.Type == "IHDR" {
			out = append(out, Chunk{Type: "tEXt", Data: body})
			placed = true
		}
	}
	return out, nil
}

// DeleteText returns chunks without any tEXt carrying keyword, leaving the input unmodified.
func DeleteText(chunks []Chunk, keyword string) []Chunk {
	out := make([]Chunk, 0, len(chunks))
	for _, c := range chunks {
		if c.Type == "tEXt" {
			if k, _, ok := splitText(c.Data); ok && k == keyword {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// ReadText returns one keyword's value from PNG bytes. The error means the file is
// not a readable PNG; the bool means the keyword is absent.
func ReadText(data []byte, keyword string) (string, bool, error) {
	chunks, err := Split(data)
	if err != nil {
		return "", false, err
	}
	v, ok := Text(chunks, keyword)
	return v, ok, nil
}

// WriteText returns data with keyword's tEXt set to value; every other chunk is unchanged.
func WriteText(data []byte, keyword, value string) ([]byte, error) {
	chunks, err := Split(data)
	if err != nil {
		return nil, err
	}
	chunks, err = SetText(chunks, keyword, value)
	if err != nil {
		return nil, err
	}
	return Join(chunks)
}

// splitText parses a tEXt payload; !ok means no NUL separator, a malformed chunk.
func splitText(data []byte) (keyword, value string, ok bool) {
	i := -1
	for j, b := range data {
		if b == 0 {
			i = j
			break
		}
	}
	if i < 0 {
		return "", "", false
	}
	return fromLatin1(data[:i]), fromLatin1(data[i+1:]), true
}

func textChunk(keyword, value string) ([]byte, error) {
	k, err := toLatin1(keyword)
	if err != nil {
		return nil, fmt.Errorf("keyword: %w", err)
	}
	v, err := toLatin1(value)
	if err != nil {
		return nil, fmt.Errorf("value: %w", err)
	}
	for _, b := range v {
		// A NUL is the separator, so one in the value would parse back shorter.
		if b == 0 {
			return nil, errors.New("value: a tEXt value cannot contain a NUL byte")
		}
	}
	out := make([]byte, 0, len(k)+1+len(v))
	out = append(out, k...)
	out = append(out, 0)
	return append(out, v...), nil
}

// checkKeyword applies the PNG rule: 1..79 printable Latin-1 characters, no
// leading, trailing or doubled spaces. It is enforced on write only.
func checkKeyword(keyword string) error {
	if keyword == "" {
		return errors.New("keyword is empty")
	}
	if len(keyword) > 79 {
		return fmt.Errorf("keyword %q is %d bytes, over the 79 the format allows", keyword, len(keyword))
	}
	if strings.HasPrefix(keyword, " ") || strings.HasSuffix(keyword, " ") || strings.Contains(keyword, "  ") {
		return fmt.Errorf("keyword %q has a leading, trailing or doubled space", keyword)
	}
	for _, r := range keyword {
		if (r < 32 || r > 126) && (r < 161 || r > 255) {
			return fmt.Errorf("keyword %q contains a character the format does not allow (%q)", keyword, r)
		}
	}
	return nil
}

// checkType requires four ASCII letters; every case combination is legal.
func checkType(typ string) error {
	if len(typ) != 4 {
		return fmt.Errorf("chunk type %q is not four bytes", typ)
	}
	for i := 0; i < 4; i++ {
		c := typ[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return fmt.Errorf("chunk type %q is not four letters", typ)
		}
	}
	return nil
}

func toLatin1(s string) ([]byte, error) {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			return nil, fmt.Errorf("%q is outside Latin-1, which is the only text a tEXt chunk can hold", r)
		}
		out = append(out, byte(r))
	}
	return out, nil
}

func fromLatin1(b []byte) string {
	ascii := true
	for _, c := range b {
		if c > 0x7F {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	rs := make([]rune, len(b))
	for i, c := range b {
		rs[i] = rune(c)
	}
	return string(rs)
}
