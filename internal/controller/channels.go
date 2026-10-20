// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"fmt"
	"sync"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// abiName is the seat-prefixed name a channel is offered under, such as
// "p1.spinner". The channel's seat is an offset on the device's, so one
// two-mat pad can offer p1.* and p2.*; a sum past the second seat is not
// offered.
func abiName(deviceSeat input.Seat, channelSeat int, name string) (string, bool) {
	seat := input.SeatOf(deviceSeat.Index() + channelSeat)
	if !seat.Valid() {
		return "", false
	}
	return fmt.Sprintf("p%d.%s", seat.Number(), name), true
}

// SeatNames returns the channels' ABI names for a device at deviceSeat,
// index-aligned with Spec.Channels; "" marks a channel past the second seat.
// Nil for NoSeat.
func (s *Spec) SeatNames(deviceSeat input.Seat) []string {
	if !deviceSeat.Valid() {
		return nil
	}
	return s.seatNames[deviceSeat.Index()]
}

func (s *Spec) buildSeatNames() {
	for _, seat := range []input.Seat{input.SeatP1, input.SeatP2} {
		names := make([]string, len(s.Channels))
		for i, c := range s.Channels {
			if n, ok := abiName(seat, c.Seat, c.Name); ok {
				names[i] = n
			}
		}
		s.seatNames[seat.Index()] = names
	}
}

type entry struct {
	kind assetfile.ChannelKind
	size int // blob length, 0 for the scalar kinds
	refs int // how many attached devices offer this name

	value int32  // button/axis level, or a delta's accumulator
	blob  []byte // len == size
}

// ChannelBus holds every channel the attached devices currently offer: the
// parts of a report the panel vocabulary has no word for. Report goroutines
// write it and the engine tick reads it, so it is mutex-guarded.
type ChannelBus struct {
	mu      sync.Mutex
	entries map[string]*entry
}

func NewChannelBus() *ChannelBus { return &ChannelBus{entries: map[string]*entry{}} }

// Offer registers a profile's channels for a device paired to deviceSeat,
// at their rest values. It runs at attach, not on the first report, because
// a cartridge binds during cart_init.
func (b *ChannelBus) Offer(deviceSeat input.Seat, spec *Spec) {
	if b == nil || spec == nil {
		return
	}
	names := spec.SeatNames(deviceSeat)
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, name := range names {
		if name == "" {
			continue
		}
		c := spec.Channels[i]
		if e, ok := b.entries[name]; ok {
			e.refs++
			continue
		}
		e := &entry{kind: c.Kind, refs: 1}
		if c.Kind == assetfile.ChannelBlob {
			e.size = c.field.size
			e.blob = make([]byte, e.size)
		}
		b.entries[name] = e
	}
}

// Withdraw drops a detached device's channels. A name two devices offered
// survives until the second one goes.
func (b *ChannelBus) Withdraw(deviceSeat input.Seat, spec *Spec) {
	if b == nil || spec == nil {
		return
	}
	names := spec.SeatNames(deviceSeat)
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, name := range names {
		if name == "" {
			continue
		}
		if e, ok := b.entries[name]; ok {
			if e.refs--; e.refs <= 0 {
				delete(b.entries, name)
			}
		}
	}
}

// Publish folds one raw report into the bus. Levels are set and deltas
// accumulate, so a spinner that turned three counts between ticks reads as
// three.
func (b *ChannelBus) Publish(deviceSeat input.Seat, spec *Spec, data []byte) {
	if b == nil || spec == nil {
		return
	}
	names := spec.SeatNames(deviceSeat)
	if len(names) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	i := -1
	spec.Sample(data, func(v ChannelValue) {
		i++
		if i >= len(names) {
			return // a Spec whose channel count outran its name cache
		}
		name := names[i]
		if name == "" {
			return
		}
		e, ok := b.entries[name]
		if !ok {
			return
		}
		switch v.Kind {
		case assetfile.ChannelDelta:
			e.value += v.Int
		case assetfile.ChannelBlob:
			copy(e.blob, v.Blob)
		default:
			e.value = v.Int
		}
	})
}

// Offered reports a channel's kind and (for a blob) its declared length.
func (b *ChannelBus) Offered(name string) (kind assetfile.ChannelKind, size int, ok bool) {
	if b == nil {
		return "", 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[name]
	if !ok {
		return "", 0, false
	}
	return e.kind, e.size, true
}

// Read returns a scalar channel's current value and resets a delta's
// accumulator, so it must be called once per tick.
func (b *ChannelBus) Read(name string) int32 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[name]
	if !ok {
		return 0
	}
	v := e.value
	if e.kind == assetfile.ChannelDelta {
		e.value = 0
	}
	return v
}

// ReadBlob copies a blob channel's current bytes into dst and returns how
// many it wrote.
func (b *ChannelBus) ReadBlob(name string, dst []byte) int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.entries[name]
	if !ok || e.kind != assetfile.ChannelBlob {
		return 0
	}
	return copy(dst, e.blob)
}

func (b *ChannelBus) Names() []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.entries))
	for n := range b.entries {
		out = append(out, n)
	}
	return out
}
