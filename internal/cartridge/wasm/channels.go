// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"bytes"
	"context"
	"strings"

	"github.com/tetratelabs/wazero/api"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// Channel sandbox caps, mirrored in the browser host so both refuse the same
// cartridge.
const (
	maxBoundChannels    = 64 // bounds the per-tick sample
	maxChannelBlobBytes = assetfile.MaxBlobBytes
	maxChannelNameBytes = 96
	maxBundleProfiles   = rcart.MaxMountedHIDProfiles
)

// ChannelKind is host_input_bind's kind argument. Integers only, so a replay
// is platform-independent. A bind whose kind disagrees with what the host
// offers fails rather than reinterpreting the value.
type ChannelKind uint32

const (
	ChannelButton ChannelKind = 0 // 0 or 1
	ChannelAxis   ChannelKind = 1 // int16 range
	ChannelDelta  ChannelKind = 2 // int32, accumulated since the last cart_update
	ChannelBlob   ChannelKind = 3 // a fixed length the profile declares
)

func (k ChannelKind) String() string {
	switch k {
	case ChannelButton:
		return "button"
	case ChannelAxis:
		return "axis"
	case ChannelDelta:
		return "delta"
	case ChannelBlob:
		return "blob"
	default:
		return "unknown"
	}
}

func kindFromAsset(k assetfile.ChannelKind) (ChannelKind, bool) {
	switch k {
	case assetfile.ChannelButton:
		return ChannelButton, true
	case assetfile.ChannelAxis:
		return ChannelAxis, true
	case assetfile.ChannelDelta:
		return ChannelDelta, true
	case assetfile.ChannelBlob:
		return ChannelBlob, true
	}
	return 0, false
}

// ChannelSource is optionally implemented by a Host with physical input
// beyond the cabinet panel. It is asked only about non-panel names, always
// in full with the seat prefix; the panel's channels are derived from the
// input records, so every Host offers them.
type ChannelSource interface {
	// OfferedChannel reports whether a channel is offered under name, with
	// its kind and, for a blob, its byte length. Consulted at bind time only.
	OfferedChannel(name string) (kind ChannelKind, blobLen int, ok bool)
	// SampleChannel reads a button, axis or delta value, resetting a
	// delta's accumulator. Called exactly once per tick.
	SampleChannel(name string) int32
	// SampleChannelBlob copies a blob's bytes into dst, of the declared
	// length, and returns how many it wrote.
	SampleChannelBlob(name string, dst []byte) int
	// SetSessionProfiles replaces the cartridge's bundled profiles, which
	// win over known ones for the session, and receives nil at teardown.
	SetSessionProfiles(profiles []*controller.Spec)
}

type panelLeaf struct {
	leaf string
	kind ChannelKind
	// key is empty for the axes and directions, derived from the arrows.
	key input.KeyName
}

// panelLeaves is the standard seat-relative vocabulary in assetfile's order.
// dpad.* is deliberately absent: the cabinet has one stick per seat, and a
// profile's hat aliases onto the panel's own dpad handling.
var panelLeaves = func() []panelLeaf {
	out := []panelLeaf{
		{leaf: assetfile.StdJoyX, kind: ChannelAxis},
		{leaf: assetfile.StdJoyY, kind: ChannelAxis},
		{leaf: assetfile.StdJoyNorth, kind: ChannelButton},
		{leaf: assetfile.StdJoySouth, kind: ChannelButton},
		{leaf: assetfile.StdJoyEast, kind: ChannelButton},
		{leaf: assetfile.StdJoyWest, kind: ChannelButton},
	}
	for _, name := range assetfile.StandardButtons {
		key, ok := panelKeys[name]
		if !ok {
			continue
		}
		out = append(out, panelLeaf{leaf: name, kind: ChannelButton, key: key})
	}
	return out
}()

// panelKeys mirrors internal/controller's stdKeys for the other end of the wire.
var panelKeys = map[string]input.KeyName{
	"a": input.KeyJoyA, "b": input.KeyJoyB, "c": input.KeyJoyC, "d": input.KeyJoyD,
	"aux1": input.KeyJoyAux1, "aux2": input.KeyJoyAux2,
	"l1": input.KeyL1, "r1": input.KeyR1, "l2": input.KeyL2, "r2": input.KeyR2,
	"select": input.KeySelect, "start": input.KeyStart,
	"turbo": input.KeyTurbo, "clear": input.KeyClear,
	"home": input.KeyHome, "enter": input.KeyEnter,
}

var panelChannels = func() map[string]panelLeaf {
	m := make(map[string]panelLeaf, 2*len(panelLeaves))
	for _, seat := range []string{"p1.", "p2."} {
		for _, l := range panelLeaves {
			m[seat+l.leaf] = l
		}
	}
	return m
}()

// PanelChannelNames lists every standard channel name, p1 then p2, in
// vocabulary order.
func PanelChannelNames() []string {
	out := make([]string, 0, 2*len(panelLeaves))
	for _, seat := range []string{"p1.", "p2."} {
		for _, l := range panelLeaves {
			out = append(out, seat+l.leaf)
		}
	}
	return out
}

// panelState is one seat's held keys, rebuilt from the records the guest
// receives. Axes come from the arrow keys, which every input source emits.
type panelState struct {
	held map[input.KeyName]bool
}

type panelStates [2]panelState

func newPanelStates() *panelStates {
	return &panelStates{{held: map[input.KeyName]bool{}}, {held: map[input.KeyName]bool{}}}
}

// apply ignores keypress records: an edge, not a level.
func (p *panelStates) apply(rec InputRecord) {
	seat := 0
	if rec.Player >= 2 {
		seat = 1
	}
	switch rec.Kind {
	case RecordKindKeydown, RecordKindKeyup:
		key, ok := OrdinalToKey(rec.Key)
		if !ok {
			return
		}
		p[seat].held[key] = rec.Kind == RecordKindKeydown
	}
}

// clear drops every held key so a missed keyup cannot leave a channel stuck.
func (p *panelStates) clear() {
	for i := range p {
		for k := range p[i].held {
			delete(p[i].held, k)
		}
	}
}

func (p *panelStates) value(seat int, l panelLeaf) int32 {
	held := p[seat].held
	b := func(k input.KeyName) int32 {
		if held[k] {
			return 1
		}
		return 0
	}
	switch l.leaf {
	case assetfile.StdJoyX:
		return b(input.KeyRight) - b(input.KeyLeft)
	case assetfile.StdJoyY:
		return b(input.KeyDown) - b(input.KeyUp)
	case assetfile.StdJoyNorth:
		return b(input.KeyUp)
	case assetfile.StdJoySouth:
		return b(input.KeyDown)
	case assetfile.StdJoyEast:
		return b(input.KeyRight)
	case assetfile.StdJoyWest:
		return b(input.KeyLeft)
	default:
		return b(l.key)
	}
}

type boundChannel struct {
	name  string
	kind  ChannelKind
	panel *panelLeaf // non-nil for a panel channel
	seat  int

	value int32
	blob  []byte
	// recBlob is the recorder's last copy; sample overwrites blob in place.
	recBlob []byte
}

// channelRegistry is one cartridge's bound channels and bundled profiles.
type channelRegistry struct {
	host  bundleReader
	panel *panelStates

	bound  []boundChannel
	byName map[string]int32

	sealed bool // at the first sample; binding is Init-only

	profiles []*controller.Spec

	recorder ChannelRecorder
	replay   *ChannelReplay
}

// ChannelRecorder receives a cartridge's non-panel channel stream for the
// flight recorder. Panel channels are rebuilt from the recorded input
// records instead.
type ChannelRecorder interface {
	// RecordChannelBind files each successful bind, so a replay can make the
	// same binds succeed on a machine with none of the original hardware.
	RecordChannelBind(handle int32, kind ChannelKind, blobLen int)
	// RecordChannel files a button, axis or delta value on a tick it changed
	// (every tick for a non-zero delta), so a quiet channel costs nothing.
	RecordChannel(handle int32, kind ChannelKind, value int32)
	// RecordChannelBlob files a blob channel's bytes on a tick they changed.
	RecordChannelBlob(handle int32, blob []byte)
}

// ChannelReplay plays a recording's channel stream back, bind table and
// values, so a replay reproduces on a machine without the original hardware.
type ChannelReplay struct {
	// Binds is the recorded bind result per handle; a handle is its index.
	Binds []ChannelBind

	values map[int32]int32
	blobs  map[int32][]byte
}

type ChannelBind struct {
	Kind    ChannelKind
	BlobLen int
}

func NewChannelReplay(binds []ChannelBind) *ChannelReplay {
	return &ChannelReplay{Binds: binds, values: map[int32]int32{}, blobs: map[int32][]byte{}}
}

func (c *ChannelReplay) SetValue(handle int32, value int32) { c.values[handle] = value }

// SetBlobChunk applies one KindChannelBlob event, growing the handle's
// buffer as chunks arrive.
func (c *ChannelReplay) SetBlobChunk(handle int32, chunk int, payload []byte) {
	off := chunk * len(payload)
	buf := c.blobs[handle]
	if len(buf) < off+len(payload) {
		grown := make([]byte, off+len(payload))
		copy(grown, buf)
		buf = grown
		c.blobs[handle] = buf
	}
	copy(buf[off:], payload)
}

// take consumes a delta, as the live bus's read resets its accumulator.
func (c *ChannelReplay) take(handle int32) (int32, []byte) {
	kind := ChannelButton
	if int(handle) < len(c.Binds) {
		kind = c.Binds[handle].Kind
	}
	v := c.values[handle]
	if kind == ChannelDelta {
		c.values[handle] = 0
	}
	return v, c.blobs[handle]
}

func newChannelRegistry(host bundleReader, recorder ChannelRecorder, replay *ChannelReplay) *channelRegistry {
	return &channelRegistry{host: host, panel: newPanelStates(), byName: map[string]int32{}, recorder: recorder, replay: replay}
}

func (r *channelRegistry) source() ChannelSource {
	s, _ := r.host.(ChannelSource)
	return s
}

// bind returns the channel's handle, or -1 when the name is not offered, the
// kind disagrees, the table is full or init is over.
func (r *channelRegistry) bind(name string, kind ChannelKind) int32 {
	if h, ok := r.byName[name]; ok {
		if r.bound[h].kind == kind {
			return h
		}
		hostWarnf(r.host, "host_input_bind: %q is already bound as a %s, not a %s", name, r.bound[h].kind, kind)
		return -1
	}
	if r.sealed {
		hostWarnf(r.host, "host_input_bind: %q refused — channels bind during cart_init, and this cartridge is already running", name)
		return -1
	}
	if len(r.bound) >= maxBoundChannels {
		hostWarnf(r.host, "host_input_bind: %q refused, already holding %d channels", name, maxBoundChannels)
		return -1
	}
	switch kind {
	case ChannelButton, ChannelAxis, ChannelDelta, ChannelBlob:
	default:
		hostWarnf(r.host, "host_input_bind: %q asked for kind %d, which is not one of button/axis/delta/blob", name, uint32(kind))
		return -1
	}

	ch := boundChannel{name: name, kind: kind}
	if leaf, ok := panelChannels[name]; ok {
		if leaf.kind != kind {
			hostWarnf(r.host, "host_input_bind: %q is a %s, not a %s", name, leaf.kind, kind)
			return -1
		}
		l := leaf
		ch.panel = &l
		if strings.HasPrefix(name, "p2.") {
			ch.seat = 1
		}
	} else if rp := r.replay; rp != nil {
		at := len(r.bound)
		if at >= len(rp.Binds) || rp.Binds[at].Kind != kind {
			hostWarnf(r.host, "host_input_bind: %q is not in this recording's bind table at handle %d", name, at)
			return -1
		}
		if kind == ChannelBlob {
			ch.blob = make([]byte, rp.Binds[at].BlobLen)
		}
	} else {
		src := r.source()
		if src == nil {
			hostWarnf(r.host, "host_input_bind: %q is not offered (this host has no input beyond the panel)", name)
			return -1
		}
		offered, blobLen, ok := src.OfferedChannel(name)
		if !ok {
			hostWarnf(r.host, "host_input_bind: %q is not offered by anything attached", name)
			return -1
		}
		if offered != kind {
			hostWarnf(r.host, "host_input_bind: %q is a %s, not a %s", name, offered, kind)
			return -1
		}
		if kind == ChannelBlob {
			if blobLen <= 0 || blobLen > maxChannelBlobBytes {
				hostWarnf(r.host, "host_input_bind: %q declares %d blob bytes, outside 1..%d", name, blobLen, maxChannelBlobBytes)
				return -1
			}
			ch.blob = make([]byte, blobLen)
		}
	}

	handle := int32(len(r.bound))
	r.bound = append(r.bound, ch)
	r.byName[name] = handle
	if r.recorder != nil {
		r.recorder.RecordChannelBind(handle, kind, len(ch.blob))
	}
	return handle
}

// sample reads every bound channel once per tick, before cart_update, so two
// reads of a delta in one frame agree.
func (r *channelRegistry) sample() {
	r.sealed = true
	src := r.source()
	for i := range r.bound {
		ch := &r.bound[i]
		if ch.panel != nil {
			ch.value = r.panel.value(ch.seat, *ch.panel)
			continue
		}
		if r.replay != nil {
			v, blob := r.replay.take(int32(i))
			ch.value = v
			if ch.blob != nil {
				copy(ch.blob, blob)
			}
			continue
		}
		if src == nil {
			continue
		}
		prev := ch.value
		if ch.kind == ChannelBlob {
			n := src.SampleChannelBlob(ch.name, ch.blob)
			for j := n; j < len(ch.blob); j++ {
				ch.blob[j] = 0
			}
		} else {
			ch.value = src.SampleChannel(ch.name)
		}
		if r.recorder != nil {
			r.noteChange(int32(i), ch, prev)
		}
	}
}

// noteChange records a changed level, or any non-zero delta, since a
// repeated delta is more motion.
func (r *channelRegistry) noteChange(handle int32, ch *boundChannel, prev int32) {
	switch ch.kind {
	case ChannelDelta:
		if ch.value != 0 {
			r.recorder.RecordChannel(handle, ch.kind, ch.value)
		}
	case ChannelBlob:
		if ch.recBlob == nil {
			ch.recBlob = make([]byte, len(ch.blob))
		}
		if !bytes.Equal(ch.blob, ch.recBlob) {
			copy(ch.recBlob, ch.blob)
			r.recorder.RecordChannelBlob(handle, ch.blob)
		}
	default:
		if ch.value != prev {
			r.recorder.RecordChannel(handle, ch.kind, ch.value)
		}
	}
}

// get returns nil, read as zero, for an unknown handle or a kind mismatch.
func (r *channelRegistry) get(handle int32, want ChannelKind, what string) *boundChannel {
	if handle < 0 || int(handle) >= len(r.bound) {
		// -1 is the failed-bind value; warning on it every frame would flood the log.
		if handle != -1 {
			hostWarnf(r.host, "%s: unknown channel handle %d", what, handle)
		}
		return nil
	}
	ch := &r.bound[handle]
	if ch.kind != want {
		hostWarnf(r.host, "%s: channel %q is a %s", what, ch.name, ch.kind)
		return nil
	}
	return ch
}

func (r *channelRegistry) loadBundle(paths []string) {
	src := r.source()
	for _, p := range paths {
		if len(r.profiles) >= maxBundleProfiles {
			hostWarnf(r.host, "hid profile %q: refused, already holding %d profiles", p, maxBundleProfiles)
			continue
		}
		env, ok := readBundleEnvelope(r.host, "hid profile", p, assetfile.KindHID)
		if !ok {
			continue
		}
		spec, err := controller.CompileSpec(env)
		if err != nil {
			hostWarnf(r.host, "hid profile %q: %v", p, err)
			continue
		}
		r.profiles = append(r.profiles, spec)
	}
	if src != nil && len(r.profiles) > 0 {
		src.SetSessionProfiles(r.profiles)
	}
}

func (r *channelRegistry) dropBundle() {
	if src := r.source(); src != nil && len(r.profiles) > 0 {
		src.SetSessionProfiles(nil)
	}
	r.profiles = nil
	r.panel.clear()
}

type channelImports struct{ *channelRegistry }

func (c channelImports) bind(_ context.Context, mod api.Module, ptr, length, kind uint32) int32 {
	name, ok := guestString(c.host, mod, "host_input_bind:", ptr, length, maxChannelNameBytes)
	if !ok {
		return -1
	}
	return c.channelRegistry.bind(name, ChannelKind(kind))
}

func (c channelImports) button(_ context.Context, _ api.Module, handle int32) uint32 {
	ch := c.get(handle, ChannelButton, "host_input_button")
	if ch == nil || ch.value == 0 {
		return 0
	}
	return 1
}

func (c channelImports) axis(_ context.Context, _ api.Module, handle int32) int32 {
	ch := c.get(handle, ChannelAxis, "host_input_axis")
	if ch == nil {
		return 0
	}
	return ch.value
}

func (c channelImports) delta(_ context.Context, _ api.Module, handle int32) int32 {
	ch := c.get(handle, ChannelDelta, "host_input_delta")
	if ch == nil {
		return 0
	}
	return ch.value
}

func (c channelImports) blob(_ context.Context, mod api.Module, handle int32, dstPtr, dstCap uint32) uint32 {
	ch := c.get(handle, ChannelBlob, "host_input_blob")
	if ch == nil {
		return 0
	}
	// All or nothing: a half-written blob would parse as garbage.
	if dstCap < uint32(len(ch.blob)) {
		hostWarnf(c.host, "host_input_blob: channel %q is %d bytes, cap is %d", ch.name, len(ch.blob), dstCap)
		return 0
	}
	if !mod.Memory().Write(dstPtr, ch.blob) {
		hostWarnf(c.host, "host_input_blob: dst ptr %#x/%d out of bounds", dstPtr, len(ch.blob))
		return 0
	}
	return uint32(len(ch.blob))
}

// outputBind always fails through the -1 path: outputs are reserved.
func (c channelImports) outputBind(_ context.Context, mod api.Module, ptr, length uint32) int32 {
	name, ok := guestString(c.host, mod, "host_output_bind:", ptr, length, maxChannelNameBytes)
	if !ok {
		return -1
	}
	hostWarnf(c.host, "host_output_bind: %q — output channels are reserved and no host drives one yet", name)
	return -1
}

func (c channelImports) outputSet(_ context.Context, _ api.Module, handle, value int32) {}
