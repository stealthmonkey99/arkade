// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/stealthmonkey99/arkade/assetfile/pngchunk"
)

const pngKeyword = "arkade"

// zTXtThreshold is the canonical JSON size above which the payload goes in a
// zTXt chunk instead of tEXt.
const zTXtThreshold = 1024

func loadPNGChunks(data []byte) (chunks []pngchunk.Chunk, w, h int, err error) {
	chunks, err = pngchunk.Split(data)
	if err != nil {
		return nil, 0, 0, err
	}
	if len(chunks) == 0 || chunks[0].Type != "IHDR" || len(chunks[0].Data) < 8 {
		return nil, 0, 0, errors.New("assetfile: PNG has no readable IHDR")
	}
	w = int(binary.BigEndian.Uint32(chunks[0].Data[0:4]))
	h = int(binary.BigEndian.Uint32(chunks[0].Data[4:8]))
	return chunks, w, h, nil
}

// envelopeFromPNGChunks returns the envelope in the arkade tEXt or zTXt
// chunk, or (nil, nil) if there is none.
func envelopeFromPNGChunks(chunks []pngchunk.Chunk) (*Envelope, error) {
	if v, ok := pngchunk.Text(chunks, pngKeyword); ok {
		env, err := Unmarshal([]byte(v))
		if err != nil {
			return nil, fmt.Errorf("assetfile: %s tEXt: %w", pngKeyword, err)
		}
		return env, nil
	}
	for _, c := range chunks {
		if c.Type != "zTXt" {
			continue
		}
		kw, value, err := decodeZTXt(c.Data)
		if err != nil {
			return nil, fmt.Errorf("assetfile: zTXt: %w", err)
		}
		if kw != pngKeyword {
			continue
		}
		env, err := Unmarshal(value)
		if err != nil {
			return nil, fmt.Errorf("assetfile: %s zTXt: %w", pngKeyword, err)
		}
		return env, nil
	}
	return nil, nil
}

// ReadPNGEnvelope reads the "arkade" tEXt or zTXt envelope from PNG bytes,
// validates it against the IHDR's dimensions and normalizes it. It returns
// (nil, w, h, nil) when there is none, as for a plain sprite.
func ReadPNGEnvelope(data []byte) (*Envelope, int, int, error) {
	chunks, w, h, err := loadPNGChunks(data)
	if err != nil {
		return nil, 0, 0, err
	}
	env, err := envelopeFromPNGChunks(chunks)
	if err != nil {
		return nil, w, h, err
	}
	if env == nil {
		return nil, w, h, nil
	}
	if err := validated(env, KindSprite, w, h); err != nil {
		return nil, w, h, err
	}
	return env, w, h, nil
}

// writePNG returns data with its arkade chunk set to env's canonical
// encoding, tEXt up to zTXtThreshold and zTXt above it. Other chunks are
// untouched.
func writePNG(data []byte, env *Envelope) ([]byte, error) {
	payload, err := Marshal(env)
	if err != nil {
		return nil, err
	}
	chunks, err := pngchunk.Split(data)
	if err != nil {
		return nil, err
	}
	if len(payload) <= zTXtThreshold {
		chunks = removeArkadeZTXt(chunks)
		chunks, err = pngchunk.SetText(chunks, pngKeyword, string(payload))
		if err != nil {
			return nil, err
		}
	} else {
		chunks = pngchunk.DeleteText(chunks, pngKeyword)
		body, err := encodeZTXt(pngKeyword, payload)
		if err != nil {
			return nil, err
		}
		chunks, err = setZTXt(chunks, pngKeyword, body)
		if err != nil {
			return nil, err
		}
	}
	return pngchunk.Join(chunks)
}

// removeArkadeZTXt drops zTXt chunks whose keyword is pngKeyword and leaves
// undecodable ones alone.
func removeArkadeZTXt(chunks []pngchunk.Chunk) []pngchunk.Chunk {
	out := make([]pngchunk.Chunk, 0, len(chunks))
	for _, c := range chunks {
		if c.Type == "zTXt" {
			if kw, _, err := decodeZTXt(c.Data); err == nil && kw == pngKeyword {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// setZTXt sets keyword's zTXt chunk to body, replacing an existing one in
// place or inserting it after IHDR.
func setZTXt(chunks []pngchunk.Chunk, keyword string, body []byte) ([]pngchunk.Chunk, error) {
	if len(chunks) == 0 || chunks[0].Type != "IHDR" {
		return nil, errors.New("assetfile: chunk list does not start with IHDR")
	}
	out := make([]pngchunk.Chunk, 0, len(chunks)+1)
	placed := false
	for _, c := range chunks {
		if c.Type == "zTXt" {
			if kw, _, err := decodeZTXt(c.Data); err == nil && kw == keyword {
				if placed {
					continue // a duplicate of one we have already written
				}
				out = append(out, pngchunk.Chunk{Type: "zTXt", Data: body})
				placed = true
				continue
			}
		}
		out = append(out, c)
		if !placed && c.Type == "IHDR" {
			out = append(out, pngchunk.Chunk{Type: "zTXt", Data: body})
			placed = true
		}
	}
	return out, nil
}

func decodeZTXt(data []byte) (keyword string, value []byte, err error) {
	i := bytes.IndexByte(data, 0)
	if i < 0 {
		return "", nil, errors.New("no keyword separator")
	}
	keyword = string(data[:i])
	if i+1 >= len(data) {
		return "", nil, errors.New("missing compression method byte")
	}
	if method := data[i+1]; method != 0 {
		return "", nil, fmt.Errorf("unsupported compression method %d", method)
	}
	zr, err := zlib.NewReader(bytes.NewReader(data[i+2:]))
	if err != nil {
		return "", nil, err
	}
	defer zr.Close()
	value, err = io.ReadAll(zr)
	if err != nil {
		return "", nil, err
	}
	return keyword, value, nil
}

func encodeZTXt(keyword string, value []byte) ([]byte, error) {
	for _, r := range keyword {
		if r > 0xFF {
			return nil, fmt.Errorf("zTXt keyword %q is outside Latin-1", keyword)
		}
	}
	var buf bytes.Buffer
	buf.WriteString(keyword)
	buf.WriteByte(0)
	buf.WriteByte(0) // compression method: zlib
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(value); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
