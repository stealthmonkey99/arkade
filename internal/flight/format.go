// SPDX-License-Identifier: GPL-2.0-or-later

// Package flight is the on-disk half of the input flight recorder
// (docs/FLIGHT_RECORDER.md): the single-file dump format and the bounded,
// rotating directory dumps are written into.
//
// It imports nothing from internal/engine, which writes the format, so the
// replayer and standalone tools can decode it without the cabinet.
//
// A recording is a JSON head (cartridge, build, seed, reason) followed by a
// flat array of 16-byte little-endian events, gzipped when that helps.
package flight

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// Magic identifies a flight recording. FormatVersion moves only for a
// breaking layout change, never for a new Header field: readers ignore JSON
// fields they do not know.
const (
	Magic         = "ARKFLT"
	FormatVersion = 1
	// Extension is the dump file suffix.
	Extension = ".arkflt"
)

// flagGzip marks a gzipped payload in the file's flags byte.
const flagGzip = 1 << 0

// EventBytes is the fixed on-disk width of one Event.
const EventBytes = 16

// Reading bounds. Decoding treats a dump as untrusted: every length is
// capped before anything is allocated for it.
const (
	// MaxHeaderBytes caps the JSON head.
	MaxHeaderBytes = 1 << 16
	// MaxEvents caps the event array; it is thirty-two times the default ring.
	MaxEvents = 1 << 22
	// MaxPayloadBytes caps the decompressed payload, bounding a gzip bomb.
	MaxPayloadBytes = 128 << 20
)

// Event kinds. Every event is a (frame, kind, u64) triple. Readers skip
// kinds they do not know, so new kinds do not move FormatVersion.
const (
	// KindFrame is one engine tick; Data is the dtMicros fed to cart_update.
	KindFrame uint8 = 0
	// KindInput is one 8-byte ABI input record (docs/CARTRIDGE.md "Input
	// records") as it reached the guest, packed little-endian into Data.
	KindInput uint8 = 1
	// KindRandom is one host_random_u64 result the guest consumed. The
	// engine RNG stream is shared with the shell, so the replayer serves
	// these back in order instead of reseeding.
	KindRandom uint8 = 2

	// KindChannelBind is one successful host_input_bind, filed at cart_init
	// so a replay can bind without the original hardware. Only non-panel
	// channels are recorded; panel channels are rebuilt from KindInput.
	KindChannelBind uint8 = 3
	// KindChannel is a bound channel's value on that frame, filed when it
	// changed and, for a delta channel, whenever it is non-zero.
	KindChannel uint8 = 4
	// KindChannelBlob is one chunk of a changed blob channel's contents.
	KindChannelBlob uint8 = 5
)

// ChannelBlobChunkBytes is how many blob bytes one KindChannelBlob event
// carries: the payload word's eight, less the handle and chunk index.
const ChannelBlobChunkBytes = 6

// PackChannelBind encodes a KindChannelBind payload: the handle the bind
// returned, the ABI channel kind, and (for a blob) its declared length.
// Handles follow bind order, so no channel name is carried.
func PackChannelBind(handle int32, kind uint8, blobLen int) uint64 {
	return uint64(uint8(handle)) | uint64(kind)<<8 | uint64(uint16(blobLen))<<16
}

// UnpackChannelBind is PackChannelBind's inverse.
func UnpackChannelBind(w uint64) (handle int32, kind uint8, blobLen int) {
	return int32(uint8(w)), uint8(w >> 8), int(uint16(w >> 16))
}

// PackChannel encodes a KindChannel payload: handle, kind, and the i32
// value in the high word.
func PackChannel(handle int32, kind uint8, value int32) uint64 {
	return uint64(uint8(handle)) | uint64(kind)<<8 | uint64(uint32(value))<<32
}

// UnpackChannel is PackChannel's inverse.
func UnpackChannel(w uint64) (handle int32, kind uint8, value int32) {
	return int32(uint8(w)), uint8(w >> 8), int32(uint32(w >> 32))
}

// PackChannelBlob encodes one chunk of a blob channel: the handle, the
// chunk's index within the blob, and up to ChannelBlobChunkBytes bytes.
func PackChannelBlob(handle int32, chunk int, payload []byte) uint64 {
	w := uint64(uint8(handle)) | uint64(uint8(chunk))<<8
	for i := 0; i < len(payload) && i < ChannelBlobChunkBytes; i++ {
		w |= uint64(payload[i]) << (16 + 8*uint(i))
	}
	return w
}

// UnpackChannelBlob is PackChannelBlob's inverse. It always writes
// ChannelBlobChunkBytes into out; the caller knows the blob's real length
// from its bind and ignores the tail of the last chunk.
func UnpackChannelBlob(w uint64) (handle int32, chunk int, out [ChannelBlobChunkBytes]byte) {
	handle, chunk = int32(uint8(w)), int(uint8(w>>8))
	for i := range out {
		out[i] = uint8(w >> (16 + 8*uint(i)))
	}
	return handle, chunk, out
}

// Event is one recorded thing that happened at one frame; it is EventBytes
// in memory and on disk.
type Event struct {
	// Frame is the tick index within the app session: 0 is cart_init's own
	// work, 1 the first tick after load.
	Frame uint32
	Kind  uint8
	Data  uint64
}

// Header is the JSON head of a recording: what is needed to decide whether
// a replay is meaningful before any event is fed to a guest.
type Header struct {
	// Slug is the engine app slug ("cart/snake"), Name its display name.
	Slug string `json:"slug"`
	Name string `json:"name,omitempty"`
	// ManifestVersion is the cartridge's release version, WasmSHA256 the
	// SHA-256 of the .wasm that ran. A replay against different bytes is
	// not this session.
	ManifestVersion string `json:"manifestVersion,omitempty"`
	WasmSHA256      string `json:"wasmSHA256,omitempty"`

	// EngineSeed is Options.RandSeed as resolved, never 0.
	EngineSeed int64 `json:"engineSeed"`
	TargetFPS  int   `json:"targetFPS"`
	Cols       int   `json:"cols"`
	Rows       int   `json:"rows"`

	// StartedAt and DumpedAt are wall-clock, for readers only; replay uses
	// the dtMicros in the frame events.
	StartedAt time.Time `json:"startedAt"`
	DumpedAt  time.Time `json:"dumpedAt"`

	// Reason is why the dump happened (the Reason* constants), Note a
	// free-text line from the trigger, such as a crash's trap message.
	Reason string `json:"reason"`
	Note   string `json:"note,omitempty"`

	// EngineVersion and Platform identify the recording build. A rebuilt
	// cartridge's wasm bytes differ across platforms, so a hash mismatch on
	// another platform is expected (docs/FLIGHT_RECORDER.md, "Limits").
	EngineVersion string `json:"engineVersion,omitempty"`
	Platform      string `json:"platform,omitempty"`

	// Dropped is how many events the ring overwrote before this dump.
	// Truncated (Dropped > 0) means the recording starts mid-session and a
	// replay will not reproduce it.
	Dropped   uint64 `json:"droppedEvents"`
	Truncated bool   `json:"truncated"`

	// SuspendedAtFrame is non-zero when standby parked the cartridge at
	// that frame; recording stops there.
	SuspendedAtFrame uint32 `json:"suspendedAtFrame,omitempty"`

	// Frames, Inputs and Randoms are the retained event counts by kind.
	Frames  uint32 `json:"frames"`
	Inputs  uint32 `json:"inputs"`
	Randoms uint32 `json:"randoms"`
	// Channels counts the retained input-channel events of all three kinds.
	Channels uint32 `json:"channels,omitempty"`
}

// Dump reasons.
const (
	ReasonCrash  = "crash"
	ReasonManual = "manual"
	ReasonCombo  = "combo"
)

// Recording is a decoded dump: the head, and the retained events in the
// order they happened.
type Recording struct {
	Header Header
	Events []Event
}

// FirstFrame reports the frame of the first retained event, 0 if none.
func (r *Recording) FirstFrame() uint32 {
	if len(r.Events) == 0 {
		return 0
	}
	return r.Events[0].Frame
}

// LastFrame reports the frame of the last retained event, 0 if none.
func (r *Recording) LastFrame() uint32 {
	if len(r.Events) == 0 {
		return 0
	}
	return r.Events[len(r.Events)-1].Frame
}

// gzipThreshold is the payload size above which Encode compresses; below it
// the gzip framing costs more than it saves.
const gzipThreshold = 1 << 10

func Encode(w io.Writer, rec *Recording) error {
	head, err := json.Marshal(rec.Header)
	if err != nil {
		return fmt.Errorf("flight: encoding header: %w", err)
	}
	if len(head) > MaxHeaderBytes {
		return fmt.Errorf("flight: header is %d bytes, over the %d byte cap", len(head), MaxHeaderBytes)
	}
	if len(rec.Events) > MaxEvents {
		return fmt.Errorf("flight: %d events, over the %d cap", len(rec.Events), MaxEvents)
	}

	var payload bytes.Buffer
	payload.Grow(8 + len(head) + len(rec.Events)*EventBytes)
	var u32 [4]byte
	binary.LittleEndian.PutUint32(u32[:], uint32(len(head)))
	payload.Write(u32[:])
	payload.Write(head)
	binary.LittleEndian.PutUint32(u32[:], uint32(len(rec.Events)))
	payload.Write(u32[:])
	var ev [EventBytes]byte
	for _, e := range rec.Events {
		binary.LittleEndian.PutUint32(ev[0:4], e.Frame)
		ev[4] = e.Kind
		ev[5], ev[6], ev[7] = 0, 0, 0
		binary.LittleEndian.PutUint64(ev[8:16], e.Data)
		payload.Write(ev[:])
	}

	flags := byte(0)
	body := payload.Bytes()
	if len(body) >= gzipThreshold {
		var zipped bytes.Buffer
		zw := gzip.NewWriter(&zipped)
		if _, err := zw.Write(body); err != nil {
			return fmt.Errorf("flight: compressing payload: %w", err)
		}
		if err := zw.Close(); err != nil {
			return fmt.Errorf("flight: compressing payload: %w", err)
		}
		flags |= flagGzip
		body = zipped.Bytes()
	}

	if _, err := w.Write([]byte(Magic)); err != nil {
		return err
	}
	if _, err := w.Write([]byte{FormatVersion, flags}); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// ErrNotARecording is returned for a file with the wrong magic, as distinct
// from a corrupt or too-new recording.
var ErrNotARecording = errors.New("flight: not a flight recording")

// Decode reads a recording written by Encode, checking every length against
// the package caps before allocating for it.
func Decode(r io.Reader) (*Recording, error) {
	var head [len(Magic) + 2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, fmt.Errorf("%w: file is too short to hold a header", ErrNotARecording)
	}
	if string(head[:len(Magic)]) != Magic {
		return nil, fmt.Errorf("%w: bad magic %q", ErrNotARecording, head[:len(Magic)])
	}
	version, flags := head[len(Magic)], head[len(Magic)+1]
	if version != FormatVersion {
		return nil, fmt.Errorf("flight: recording is format version %d, this build reads version %d",
			version, FormatVersion)
	}

	var body io.Reader = r
	if flags&flagGzip != 0 {
		zr, err := gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("flight: payload is marked gzipped but isn't: %w", err)
		}
		defer func() { _ = zr.Close() }()
		body = zr
	}
	body = io.LimitReader(body, MaxPayloadBytes)

	headLen, err := readU32(body)
	if err != nil {
		return nil, fmt.Errorf("flight: reading header length: %w", err)
	}
	if headLen > MaxHeaderBytes {
		return nil, fmt.Errorf("flight: header claims %d bytes, over the %d byte cap", headLen, MaxHeaderBytes)
	}
	headJSON := make([]byte, headLen)
	if _, err := io.ReadFull(body, headJSON); err != nil {
		return nil, fmt.Errorf("flight: reading header: %w", err)
	}
	rec := &Recording{}
	if err := json.Unmarshal(headJSON, &rec.Header); err != nil {
		return nil, fmt.Errorf("flight: parsing header: %w", err)
	}

	count, err := readU32(body)
	if err != nil {
		return nil, fmt.Errorf("flight: reading event count: %w", err)
	}
	if count > MaxEvents {
		return nil, fmt.Errorf("flight: recording claims %d events, over the %d cap", count, MaxEvents)
	}
	// Chunked so a file that overclaims its count allocates only what it carries.
	const chunk = 4096 // events per read
	rec.Events = make([]Event, 0, min(int(count), chunk))
	buf := make([]byte, chunk*EventBytes)
	for remaining := int(count); remaining > 0; {
		n := min(remaining, chunk)
		if _, err := io.ReadFull(body, buf[:n*EventBytes]); err != nil {
			return nil, fmt.Errorf("flight: recording claims %d events but ends after %d: %w",
				count, len(rec.Events), err)
		}
		for i := 0; i < n; i++ {
			o := i * EventBytes
			rec.Events = append(rec.Events, Event{
				Frame: binary.LittleEndian.Uint32(buf[o : o+4]),
				Kind:  buf[o+4],
				Data:  binary.LittleEndian.Uint64(buf[o+8 : o+16]),
			})
		}
		remaining -= n
	}
	return rec, nil
}

func readU32(r io.Reader) (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}
