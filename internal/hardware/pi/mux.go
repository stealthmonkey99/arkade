// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import "sync"

// MCP23017 at 0x26 in its power-on BANK=0 register layout.
const (
	muxAddr = 0x26

	regIODIRA = 0x00 // direction, port A: 0 bit = output
	regIODIRB = 0x01
	regOLATA  = 0x14 // output latches
	regOLATB  = 0x15

	// button pin is index + 8*port; the fan relay is port A pin 7
	muxPinsPerPort  = 8
	lightsPerPlayer = 6
	fanPin          = 7
)

// mcp23017 is the register-level device; lampWriter owns the output state.
type mcp23017 struct {
	bus  I2CBus
	addr uint16

	mu sync.Mutex
}

// newMCP23017 configures every pin as an output and latches it low.
func newMCP23017(bus I2CBus, addr uint16) (*mcp23017, error) {
	m := &mcp23017{bus: bus, addr: addr}
	for _, w := range [][2]byte{
		{regIODIRA, 0x00},
		{regIODIRB, 0x00},
		{regOLATA, 0x00},
		{regOLATB, 0x00},
	} {
		if err := m.writeReg(w[0], w[1]); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *mcp23017) writeReg(reg, val byte) error {
	return m.bus.Tx(m.addr, []byte{reg, val}, nil)
}

// writeLatches sets all 16 outputs in one 3-byte transaction, since the
// register pointer auto-increments from OLATA to OLATB. Measured 411 µs
// median, 421 µs p99 on the cabinet's 100 kHz bus.
func (m *mcp23017) writeLatches(a, b byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bus.Tx(m.addr, []byte{regOLATA, a, b}, nil)
}
