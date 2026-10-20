// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const wavChunkID = "arkd"

// riffChunk holds a chunk's id and body; the even-padding byte is not stored.
type riffChunk struct {
	id   [4]byte
	data []byte
}

// splitRIFF parses a RIFF stream into its form type and chunks in file
// order, refusing any chunk whose declared size runs past the data.
func splitRIFF(data []byte) (form [4]byte, chunks []riffChunk, err error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" {
		return form, nil, errors.New("assetfile: not a RIFF file")
	}
	copy(form[:], data[8:12])
	rest := data[12:]
	for len(rest) > 0 {
		if len(rest) < 8 {
			return form, nil, errors.New("assetfile: truncated RIFF chunk header")
		}
		var id [4]byte
		copy(id[:], rest[0:4])
		size := binary.LittleEndian.Uint32(rest[4:8])
		if uint64(len(rest)-8) < uint64(size) {
			return form, nil, fmt.Errorf("assetfile: RIFF chunk %q declares %d bytes past EOF", id, size)
		}
		body := append([]byte(nil), rest[8:8+size]...)
		chunks = append(chunks, riffChunk{id: id, data: body})
		pad := size % 2
		rest = rest[8+size+pad:]
	}
	return form, chunks, nil
}

func joinRIFF(form [4]byte, chunks []riffChunk) []byte {
	var body []byte
	body = append(body, form[:]...)
	for _, c := range chunks {
		var head [8]byte
		copy(head[0:4], c.id[:])
		binary.LittleEndian.PutUint32(head[4:8], uint32(len(c.data)))
		body = append(body, head[:]...)
		body = append(body, c.data...)
		if len(c.data)%2 == 1 {
			body = append(body, 0)
		}
	}
	out := make([]byte, 0, 8+len(body))
	out = append(out, []byte("RIFF")...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(body)))
	out = append(out, size[:]...)
	out = append(out, body...)
	return out
}

// readWAVEnvelope returns the envelope in the "arkd" chunk, or (nil, nil).
func readWAVEnvelope(data []byte) (*Envelope, error) {
	form, chunks, err := splitRIFF(data)
	if err != nil {
		return nil, err
	}
	if string(form[:]) != "WAVE" {
		return nil, fmt.Errorf("assetfile: RIFF form is %q, want WAVE", form[:])
	}
	for _, c := range chunks {
		if string(c.id[:]) == wavChunkID {
			env, err := Unmarshal(c.data)
			if err != nil {
				return nil, fmt.Errorf("assetfile: %s chunk: %w", wavChunkID, err)
			}
			return env, nil
		}
	}
	return nil, nil
}

// writeWAV returns data with its "arkd" chunk set to env's canonical
// encoding, placed just before "data". Every other chunk crosses unchanged.
func writeWAV(data []byte, env *Envelope) ([]byte, error) {
	payload, err := Marshal(env)
	if err != nil {
		return nil, err
	}
	form, chunks, err := splitRIFF(data)
	if err != nil {
		return nil, err
	}
	var out []riffChunk
	inserted := false
	for _, c := range chunks {
		id := string(c.id[:])
		if id == wavChunkID {
			continue
		}
		if id == "data" && !inserted {
			out = append(out, riffChunk{id: wavID(), data: payload})
			inserted = true
		}
		out = append(out, c)
	}
	if !inserted {
		out = append(out, riffChunk{id: wavID(), data: payload})
	}
	return joinRIFF(form, out), nil
}

func wavID() [4]byte {
	var id [4]byte
	copy(id[:], wavChunkID)
	return id
}
