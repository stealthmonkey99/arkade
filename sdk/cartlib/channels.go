// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

type channelKind uint32

const (
	kindButton channelKind = 0
	kindAxis   channelKind = 1
	kindDelta  channelKind = 2
	kindBlob   channelKind = 3
)

// P1 and P2 are the seat prefixes every channel name begins with; there is
// no seatless channel. `cartlib.P1 + cartlib.ChanA` is a constant.
const (
	P1 = "p1."
	P2 = "p2."
)

// Standard channel names, joined to [P1] or [P2]. [ChanJoyX] and [ChanJoyY]
// read the stick as axes of -1, 0 or +1 (the cabinet's stick is an
// eight-way switch), and [ChanJoyNorth] and its neighbours read the same
// stick as four buttons. [ChanA] through [ChanAux2] are the six arcade
// buttons the lamp calls address. The rest exist only on controllers, so
// binding one fails on a plain cabinet.
const (
	ChanJoyX = "joy.x"
	ChanJoyY = "joy.y"

	ChanJoyNorth = "joy.north"
	ChanJoySouth = "joy.south"
	ChanJoyEast  = "joy.east"
	ChanJoyWest  = "joy.west"

	ChanA    = "a"
	ChanB    = "b"
	ChanC    = "c"
	ChanD    = "d"
	ChanAux1 = "aux1"
	ChanAux2 = "aux2"

	ChanL1     = "l1"
	ChanR1     = "r1"
	ChanL2     = "l2"
	ChanR2     = "r2"
	ChanSelect = "select"
	ChanStart  = "start"
	ChanTurbo  = "turbo"
	ChanClear  = "clear"
	ChanHome   = "home"
	ChanEnter  = "enter"
)

// MaxBoundChannels is how many channels one cartridge may bind, and
// MaxBlobBytes the largest blob a channel may carry. A bind past
// MaxBoundChannels fails.
const (
	MaxBoundChannels = 64
	MaxBlobBytes     = 256
)

// Channel is a bound input channel (docs/ASSETS.md, "Input channels"). An
// input record reports an event; a channel reports a current level, such as
// a held button, an axis position or a spinner's motion since the last
// Update.
//
// Channels are bound through [Setup] in Init, the only phase in which the
// host hands out a handle. A failed bind, meaning nothing on this cabinet
// offers the channel, returns the zero Channel, whose Ok reports false and
// whose readers all return zero, so the game can check once and play
// differently:
//
//	func (g *myGame) Init(s *cartlib.Setup) {
//	    g.wheel = s.BindDelta(cartlib.P1 + "spinner")
//	}
//
//	func (g *myGame) Update(dtMicros uint64) {
//	    if g.wheel.Ok() {
//	        g.paddleX += g.wheel.Delta()
//	    } else {
//	        g.paddleX += g.dpadStep()
//	    }
//	}
//
// Binding passes the name's own bytes and reading is one host call, so
// neither allocates.
type Channel struct {
	handle int32
	bound  bool
}

// Ok reports whether the bind succeeded.
func (c Channel) Ok() bool { return c.bound }

// Button reports whether a button channel is held. It returns false for an
// unbound channel or one of another kind.
func (c Channel) Button() bool {
	if !c.bound {
		return false
	}
	return host.InputButton(c.handle) != 0
}

// Axis returns an axis channel's current position, or 0 for an unbound
// channel or one of another kind.
func (c Channel) Axis() int {
	if !c.bound {
		return 0
	}
	return int(host.InputAxis(c.handle))
}

// Delta returns a delta channel's motion since the previous Update, or
// 0 for an unbound channel or one of another kind. The host samples once
// per tick, so repeated reads within a tick return the same value.
func (c Channel) Delta() int {
	if !c.bound {
		return 0
	}
	return int(host.InputDelta(c.handle))
}

// Blob copies a blob channel's bytes into dst and returns how many it
// wrote. A dst shorter than the channel's declared length copies nothing
// and returns 0, as do an unbound channel and one of another kind. A dst of
// [MaxBlobBytes] always fits.
func (c Channel) Blob(dst []byte) int {
	if !c.bound || len(dst) == 0 {
		return 0
	}
	return int(host.InputBlob(c.handle, dst))
}

// Output is a bound output channel, such as rumble or a pad's LEDs. The
// surface is reserved: [Setup.BindOutput] fails on every current host, and
// Set on the zero Output does nothing, so code written against it is safe to
// ship.
type Output struct {
	handle int32
	bound  bool
}

// Ok reports whether the output bind succeeded. Always false on current
// hosts.
func (o Output) Ok() bool { return o.bound }

// Set drives an output channel. A no-op on an unbound one.
func (o Output) Set(value int) {
	if !o.bound {
		return
	}
	host.OutputSet(o.handle, int32(value))
}
