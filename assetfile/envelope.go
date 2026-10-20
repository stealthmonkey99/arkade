// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import "encoding/json"

// FormatVersion is the envelope format version this build reads and writes
// ("arkade" in the JSON). Validate refuses any other.
const FormatVersion = 1

// Kind names one of the asset kinds the envelope can carry and is the naming
// rule's middle component. KindLevels has no container component; see
// KindFromFilename.
type Kind string

const (
	KindSprite Kind = "sprite"
	KindSound  Kind = "sound"
	KindGlyph  Kind = "glyph"
	KindHID    Kind = "hid"
	KindLevels Kind = "levels"
	KindLights Kind = "lights"
	KindMic    Kind = "mic"
)

// containerExt omits KindLevels, which has no container extension.
var containerExt = map[Kind]string{
	KindSprite: "png",
	KindSound:  "wav",
	KindGlyph:  "json",
	KindHID:    "json",
	KindLights: "json",
	KindMic:    "json",
}

func (k Kind) valid() bool {
	if k == KindLevels {
		return true
	}
	_, ok := containerExt[k]
	return ok
}

// Generator pins the tool and version that produced an envelope's rendered
// payload.
type Generator struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
}

// Envelope is the one JSON document docs/ASSETS.md describes: the fields
// every kind shares, then one payload per kind. Only the payload of its own
// Kind is read; omitempty keeps the others out of the encoded document.
type Envelope struct {
	Format      int             `json:"arkade"`
	Kind        Kind            `json:"kind"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Generator   Generator       `json:"generator"`
	Recipe      json.RawMessage `json:"recipe,omitempty"`

	// The payloads, one per kind. Their fields sit at the top level of the
	// JSON, in this order, which is what keeps Marshal canonical.
	SpritePayload
	SoundPayload
	GlyphPayload
	HIDPayload
	MicPayload
	LightsPayload
	LevelsPayload
}

// SpritePayload is a sprite sheet's cels and the animations over them.
type SpritePayload struct {
	Cels       []Cel       `json:"cels,omitempty"`
	Animations []Animation `json:"animations,omitempty"`
}

// SoundPayload is the format a sound's samples were rendered to.
type SoundPayload struct {
	Render *Render `json:"render,omitempty"`
}

// GlyphPayload is one LCD glyph: its code point and 5x8 rows.
type GlyphPayload struct {
	Codepoint int    `json:"codepoint,omitempty"`
	Rows      []int  `json:"rows,omitempty"`
	Category  string `json:"category,omitempty"`
}

// HIDPayload is a HID profile: the device it describes, its channels and
// their aliases onto the standard panel names.
type HIDPayload struct {
	Device   *Device           `json:"device,omitempty"`
	Channels []Channel         `json:"channels,omitempty"`
	Standard map[string]string `json:"standard,omitempty"`
}

// MicPayload is a sound model: the front end, the endpointer and the
// classes it recognises.
type MicPayload struct {
	Features *MicFeatures `json:"features,omitempty"`
	Segment  *MicSegment  `json:"segment,omitempty"`
	Classes  []MicClass   `json:"classes,omitempty"`
}

// LightsPayload is one lamp sequence, so its loop mode sits beside its
// frames rather than inside an animation.
type LightsPayload struct {
	Frames []LightFrame `json:"frames,omitempty"`
	Loop   string       `json:"loop,omitempty"`
}

// LevelsPayload is a levels pack's text without its front matter. It is not
// part of the JSON; ReadBytes fills it and WriteBytes consumes it.
type LevelsPayload struct {
	Body []byte `json:"-"`
}

// Cel is one named rectangle in a sprite sheet, plus the point (relative
// to its own top-left corner) a drawing routine anchors it on.
type Cel struct {
	Name   string `json:"name"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Anchor [2]int `json:"anchor"`
}

// The loop modes of an Animation or lights sequence.
const (
	LoopCycle    = "cycle"
	LoopOnce     = "once"
	LoopPingPong = "pingpong"
)

// Frame is one showing of a cel. Several frames may name the same cel, and
// the hold belongs to the frame.
type Frame struct {
	Cel    string `json:"cel"`
	HoldMs int    `json:"holdMs"`
}

// Animation names a sequence of Frames over a sprite's cels.
type Animation struct {
	Name   string  `json:"name"`
	Frames []Frame `json:"frames"`
	Loop   string  `json:"loop,omitempty"`
}

// CelNames returns the cel of each frame, in play order.
func (a Animation) CelNames() []string {
	out := make([]string, len(a.Frames))
	for i, f := range a.Frames {
		out[i] = f.Cel
	}
	return out
}

// Render is the WAV format a sound envelope's samples were rendered to.
type Render struct {
	SampleRate int `json:"sampleRate"`
	Channels   int `json:"channels"`
	Bits       int `json:"bits"`
}

// Device identifies a HID envelope's physical device, informationally.
type Device struct {
	Vendor  int    `json:"vendor,omitempty"`
	Product int    `json:"product,omitempty"`
	Name    string `json:"name,omitempty"`
	// Directional is "stick" or "dpad"; nothing in the decode path uses it.
	Directional string `json:"directional,omitempty"`
	// Bus restricts this profile to "usb" or "bluetooth"; empty matches any.
	// One VID/PID can report a different byte layout per transport.
	Bus string `json:"bus,omitempty"`
}

// ChannelKind is one of the four HID channel kinds. Reports are integers
// only, so replays stay platform-independent.
type ChannelKind string

const (
	ChannelButton ChannelKind = "button"
	ChannelAxis   ChannelKind = "axis"
	ChannelDelta  ChannelKind = "delta"
	ChannelBlob   ChannelKind = "blob"
)

// Report locates a channel's value in a HID input report. It has two forms
// (sdk/docs/reference-hid.md):
//
//   - Bit form: a Bits-wide field at bit Bit of byte Byte, low bit first.
//     Buttons always use it, one bit wide; an axis or delta uses it when
//     Bits > 0.
//   - Byte form: Size bytes at Byte, little-endian, two's complement when
//     Signed. The default for axis, delta and blob; Size defaults to 1
//     except for a blob, where it is required.
//
// For axis and delta channels, Map, Bias and Divide then transform the
// extracted integer in that order.
type Report struct {
	// Byte is the report byte the field starts at (0-based, required).
	Byte int `json:"byte"`
	// Bit is the starting bit within Byte, 0..7, 0 being the least
	// significant. Bit form only.
	Bit int `json:"bit,omitempty"`
	// Bits selects the bit form for an axis or a delta and gives the
	// field's width in bits, 1..8. A button is one bit and ignores it.
	Bits int `json:"bits,omitempty"`
	// Size is the byte form's width in bytes: 1..4 for an axis or a delta,
	// 1..MaxBlobBytes for a blob, where it is required.
	Size int `json:"size,omitempty"`
	// Signed reads the byte form's bytes as two's complement.
	Signed bool `json:"signed,omitempty"`

	// Map replaces the extracted value by Map[value], for enumerated fields
	// such as a hat switch. A value outside the table yields MapDefault.
	Map []int `json:"map,omitempty"`
	// MapDefault is Map's out-of-range result (default 0).
	MapDefault int `json:"mapDefault,omitempty"`
	// Bias is subtracted from the value.
	Bias int `json:"bias,omitempty"`
	// Divide divides the value, rounding halves toward +infinity as
	// JavaScript's Math.round does, so -0.5 is 0. 0 means no division.
	Divide int `json:"divide,omitempty"`
}

// Channel is one named input of a HID profile.
type Channel struct {
	Name   string      `json:"name"`
	Kind   ChannelKind `json:"kind"`
	Seat   int         `json:"seat,omitempty"`
	Report Report      `json:"report"`
}
