// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// field is one compiled report field: where the bits are and the small
// arithmetic that follows them (assetfile.Report's two forms).
type field struct {
	kind   assetfile.ChannelKind
	byteAt int
	bit    int
	bits   int // 0 = byte form
	size   int // byte form width in bytes
	signed bool
	mapTab []int
	mapDef int
	bias   int
	divide int
}

func (f field) end() int {
	if f.bits > 0 {
		return f.byteAt + 1
	}
	return f.byteAt + f.size
}

// value extracts and transforms the field's integer. The report must be at
// least Spec.MinReportLen long.
func (f field) value(data []byte) int32 {
	var v int
	if f.bits > 0 {
		v = int(data[f.byteAt]>>uint(f.bit)) & ((1 << uint(f.bits)) - 1)
	} else {
		var u uint32
		for i := 0; i < f.size; i++ {
			u |= uint32(data[f.byteAt+i]) << (8 * uint(i))
		}
		if f.signed && f.size < 4 {
			shift := uint(32 - 8*f.size)
			v = int(int32(u<<shift) >> shift)
		} else if f.signed {
			v = int(int32(u))
		} else {
			v = int(u)
		}
	}
	if f.mapTab != nil {
		if v >= 0 && v < len(f.mapTab) {
			v = f.mapTab[v]
		} else {
			v = f.mapDef
		}
	}
	v -= f.bias
	if f.divide > 0 {
		v = jsRound(float64(v) / float64(f.divide))
	}
	return int32(v)
}

type channel struct {
	Name  string
	Kind  assetfile.ChannelKind
	Seat  int
	field field
}

// Spec is a compiled `.arkadehid.json` profile: the channel list a host
// samples, plus the panel wiring its `standard` block describes. It is
// immutable and shareable; per-device state belongs to the reader.
type Spec struct {
	Envelope *assetfile.Envelope
	// Channels are in authored order, which fixes both the panel button bit
	// order and the order a host offers channels in.
	Channels []channel
	// MinReportLen is the shortest report every field fits inside. A shorter
	// report decodes to nothing.
	MinReportLen int

	std stdWiring
	// seatNames caches seat-prefixed channel names so the read path formats
	// no strings.
	seatNames [input.SeatCount][]string
}

type stdWiring struct {
	// buttons are in first-appearance order over Spec.Channels.
	buttons []stdButton
	joy     axisWire
	dpad    axisWire
	hasDpad bool
}

type stdButton struct {
	key input.KeyName
	// from are channel indices OR'd together.
	from []int
}

// axisWire is one directional: axis channels and/or four direction button
// lists, which are summed onto the axis.
type axisWire struct {
	x, y         int // channel index, or -1
	north, south []int
	east, west   []int
	used         bool
}

func newAxisWire() axisWire { return axisWire{x: -1, y: -1} }

var stdKeys = map[string]input.KeyName{
	"a": input.KeyJoyA, "b": input.KeyJoyB, "c": input.KeyJoyC, "d": input.KeyJoyD,
	"aux1": input.KeyJoyAux1, "aux2": input.KeyJoyAux2,
	"l1": input.KeyL1, "r1": input.KeyR1, "l2": input.KeyL2, "r2": input.KeyR2,
	"select": input.KeySelect, "start": input.KeyStart,
	"turbo": input.KeyTurbo, "clear": input.KeyClear,
	"home": input.KeyHome, "enter": input.KeyEnter,
}

// CompileSpec turns an HID envelope into a [Spec]. It rejects a `standard`
// block that drives the same panel axis twice.
func CompileSpec(env *assetfile.Envelope) (*Spec, error) {
	if env == nil || env.Kind != assetfile.KindHID {
		return nil, fmt.Errorf("controller: not a hid envelope")
	}
	if err := assetfile.Validate(env, assetfile.KindHID, 0, 0); err != nil {
		return nil, err
	}
	s := &Spec{Envelope: env}
	s.std.joy, s.std.dpad = newAxisWire(), newAxisWire()

	for _, c := range env.Channels {
		f := field{
			kind: c.Kind, byteAt: c.Report.Byte, bit: c.Report.Bit,
			bits: c.Report.Bits, size: c.Report.Size, signed: c.Report.Signed,
			mapTab: c.Report.Map, mapDef: c.Report.MapDefault,
			bias: c.Report.Bias, divide: c.Report.Divide,
		}
		switch c.Kind {
		case assetfile.ChannelButton:
			f.bits = 1
		case assetfile.ChannelBlob:
		default:
			if f.bits == 0 && f.size == 0 {
				f.size = 1
			}
		}
		s.Channels = append(s.Channels, channel{Name: c.Name, Kind: c.Kind, Seat: c.Seat, field: f})
		if e := f.end(); e > s.MinReportLen {
			s.MinReportLen = e
		}
	}

	// Walked in channel order, not map order, so the compiled wiring is
	// deterministic.
	seenKey := map[input.KeyName]int{}
	for i, c := range env.Channels {
		to, ok := env.Standard[c.Name]
		if !ok {
			continue
		}
		switch to {
		case assetfile.StdJoyX, assetfile.StdJoyY, assetfile.StdDpadX, assetfile.StdDpadY:
			wire := &s.std.joy
			if to == assetfile.StdDpadX || to == assetfile.StdDpadY {
				wire, s.std.hasDpad = &s.std.dpad, true
			}
			slot := &wire.x
			if to == assetfile.StdJoyY || to == assetfile.StdDpadY {
				slot = &wire.y
			}
			if *slot >= 0 {
				return nil, fmt.Errorf("controller: %q and %q both drive %s", env.Channels[*slot].Name, c.Name, to)
			}
			*slot, wire.used = i, true
		case assetfile.StdJoyNorth, assetfile.StdJoySouth, assetfile.StdJoyEast, assetfile.StdJoyWest,
			assetfile.StdDpadNorth, assetfile.StdDpadSouth, assetfile.StdDpadEast, assetfile.StdDpadWest:
			wire := &s.std.joy
			leaf := to[len("joy."):]
			if len(to) > 5 && to[:5] == "dpad." {
				wire, s.std.hasDpad = &s.std.dpad, true
				leaf = to[len("dpad."):]
			}
			switch leaf {
			case "north":
				wire.north = append(wire.north, i)
			case "south":
				wire.south = append(wire.south, i)
			case "east":
				wire.east = append(wire.east, i)
			case "west":
				wire.west = append(wire.west, i)
			}
			wire.used = true
		default:
			key, ok := stdKeys[to]
			if !ok {
				return nil, fmt.Errorf("controller: standard alias %q targets unknown panel name %q", c.Name, to)
			}
			if at, seen := seenKey[key]; seen {
				s.std.buttons[at].from = append(s.std.buttons[at].from, i)
			} else {
				seenKey[key] = len(s.std.buttons)
				s.std.buttons = append(s.std.buttons, stdButton{key: key, from: []int{i}})
			}
		}
	}
	s.buildSeatNames()
	return s, nil
}

// Buttons is the panel key per bit of the Report mask this Spec produces.
func (s *Spec) Buttons() []input.KeyName {
	out := make([]input.KeyName, len(s.std.buttons))
	for i, b := range s.std.buttons {
		out[i] = b.key
	}
	return out
}

// Decode assembles the panel [Report] from one raw HID report. A report
// shorter than MinReportLen returns the zero Report.
func (s *Spec) Decode(data []byte) Report {
	if len(data) < s.MinReportLen {
		return Report{}
	}
	var rep Report
	for i, b := range s.std.buttons {
		for _, ch := range b.from {
			if s.Channels[ch].field.value(data) != 0 {
				rep.Buttons |= 1 << uint(i)
				break
			}
		}
	}
	rep.X, rep.Y = s.axis(data, s.std.joy)
	if s.std.hasDpad {
		x, y := s.axis(data, s.std.dpad)
		rep.HasDpad = true
		rep.Dpad = posFromAxes(x, y)
	}
	return rep
}

func (s *Spec) axis(data []byte, w axisWire) (int, int) {
	if !w.used {
		return 0, 0
	}
	var x, y int
	if w.x >= 0 {
		x = int(s.Channels[w.x].field.value(data))
	}
	if w.y >= 0 {
		y = int(s.Channels[w.y].field.value(data))
	}
	for _, i := range w.west {
		if s.Channels[i].field.value(data) != 0 {
			x--
		}
	}
	for _, i := range w.east {
		if s.Channels[i].field.value(data) != 0 {
			x++
		}
	}
	for _, i := range w.north {
		if s.Channels[i].field.value(data) != 0 {
			y--
		}
	}
	for _, i := range w.south {
		if s.Channels[i].field.value(data) != 0 {
			y++
		}
	}
	return x, y
}

// ChannelValue is one channel sampled out of one report: an integer for a
// button, an axis or a delta, and bytes for a blob.
type ChannelValue struct {
	Name string
	Kind assetfile.ChannelKind
	Seat int
	Int  int32
	Blob []byte
}

// Sample hands every channel of one report to fn in authored order. A report
// shorter than MinReportLen yields nothing. Blob bytes alias the report
// buffer; fn must copy anything it keeps.
func (s *Spec) Sample(data []byte, fn func(ChannelValue)) {
	if len(data) < s.MinReportLen {
		return
	}
	for _, c := range s.Channels {
		v := ChannelValue{Name: c.Name, Kind: c.Kind, Seat: c.Seat}
		if c.Kind == assetfile.ChannelBlob {
			v.Blob = data[c.field.byteAt : c.field.byteAt+c.field.size]
		} else {
			v.Int = c.field.value(data)
		}
		fn(v)
	}
}

// ProfileFromEnvelope compiles an HID envelope into a [Profile]; identity
// comes from its `device` block and decoding from [CompileSpec].
func ProfileFromEnvelope(env *assetfile.Envelope) (Profile, error) {
	spec, err := CompileSpec(env)
	if err != nil {
		return Profile{}, err
	}
	p := Profile{
		Name:        env.Name,
		Directional: "stick",
		Spec:        spec,
	}
	if d := env.Device; d != nil {
		if d.Name != "" {
			p.Name = d.Name
		}
		if d.Vendor < 0 || d.Vendor > math.MaxUint16 || d.Product < 0 || d.Product > math.MaxUint16 {
			return Profile{}, fmt.Errorf("controller: %q has vendor/product outside 0..0xffff", env.Name)
		}
		p.VID, p.PID = uint16(d.Vendor), uint16(d.Product)
		if d.Directional != "" {
			p.Directional = d.Directional
		}
		p.Bus = d.Bus
	}
	return p, nil
}
