// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"encoding/json"
	"unicode/utf16"
	"unicode/utf8"
)

// Marshal renders env as compact JSON with every non-ASCII code point
// escaped as \uXXXX (a surrogate pair above U+FFFF). Every container carries
// these bytes as-is, so the same envelope always encodes identically.
func Marshal(env *Envelope) ([]byte, error) {
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	return asciiEscape(raw), nil
}

// Unmarshal parses envelope JSON without validating it; see Validate.
func Unmarshal(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// WriteJSON renders env as the standalone JSON file of a glyph, hid, lights
// or mic envelope: Marshal's bytes indented two spaces, with a trailing
// newline.
func WriteJSON(env *Envelope) ([]byte, error) {
	compact, err := Marshal(env)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// encoding/json never puts a non-ASCII byte outside a string literal, so the
// whole document can be rewritten without reparsing.
func asciiEscape(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		c := b[i]
		if c < utf8.RuneSelf {
			out = append(out, c)
			i++
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		if r > 0xFFFF {
			r1, r2 := utf16.EncodeRune(r)
			out = append(out, []byte(`\u`)...)
			out = appendHex4(out, uint16(r1))
			out = append(out, []byte(`\u`)...)
			out = appendHex4(out, uint16(r2))
		} else {
			out = append(out, []byte(`\u`)...)
			out = appendHex4(out, uint16(r))
		}
		i += size
	}
	return out
}

const hexDigits = "0123456789abcdef"

func appendHex4(out []byte, v uint16) []byte {
	return append(out,
		hexDigits[(v>>12)&0xF],
		hexDigits[(v>>8)&0xF],
		hexDigits[(v>>4)&0xF],
		hexDigits[v&0xF],
	)
}
