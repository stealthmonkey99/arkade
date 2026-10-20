// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"reflect"
	"testing"
)

func TestMCP23017InitAllOutputsLow(t *testing.T) {
	bus := &fakeBus{}
	if _, err := newMCP23017(bus, muxAddr); err != nil {
		t.Fatal(err)
	}
	want := []txRecord{
		{muxAddr, []byte{regIODIRA, 0x00}},
		{muxAddr, []byte{regIODIRB, 0x00}},
		{muxAddr, []byte{regOLATA, 0x00}},
		{muxAddr, []byte{regOLATB, 0x00}},
	}
	if !reflect.DeepEqual(bus.recs, want) {
		t.Errorf("init writes = %v, want %v", bus.recs, want)
	}
}

func TestMCP23017WriteLatchesOneTransaction(t *testing.T) {
	bus := &fakeBus{}
	m, err := newMCP23017(bus, muxAddr)
	if err != nil {
		t.Fatal(err)
	}

	for i, want := range [][]byte{{regOLATA, 0x88, 0x02}, {regOLATA, 0x00, 0x00}} {
		bus.reset()
		if err := m.writeLatches(want[1], want[2]); err != nil {
			t.Fatal(err)
		}
		if len(bus.recs) != 1 || !reflect.DeepEqual(bus.recs[0], txRecord{muxAddr, want}) {
			t.Errorf("step %d: writes = %v, want one %v", i, bus.recs, want)
		}
	}

	bus.fail(errFakeBus)
	if err := m.writeLatches(0, 0); err == nil {
		t.Error("a bus failure should surface to the caller")
	}
}
