// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"strings"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/input"
)

func chordEvent(kind input.EventKind, name input.KeyName) input.Event {
	return input.Event{Kind: kind, Name: name}
}

func TestSeatChordClaimsTheSeatTheGestureAimsAt(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shoulder input.KeyName
		want     input.Seat
	}{
		{"R1 claims the right-hand seat, P1", input.KeyR1, input.SeatP1},
		{"L1 claims the left-hand seat, P2", input.KeyL1, input.SeatP2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(input.NewHandler(), newFakeEnum(), nil)
			claimed := input.NoSeat
			m.OnSeatClaim(func(_ string, seat input.Seat) { claimed = seat })
			a := &activeDevice{info: DeviceInfo{Path: "/bt", Serial: "AA:BB"}, profile: DualShockBT}

			if m.seatChord(a, chordEvent(input.KindKeydown, input.KeySelect)) {
				t.Error("SELECT was swallowed; it must pass through to the game")
			}
			if !m.seatChord(a, chordEvent(input.KindKeydown, tc.shoulder)) {
				t.Error("the shoulder keydown reached the game mid-chord")
			}
			if !m.seatChord(a, chordEvent(input.KindKeypress, tc.shoulder)) {
				t.Error("the shoulder keypress reached the game mid-chord")
			}
			if claimed != tc.want {
				t.Errorf("claimed seat %v, want %v", claimed, tc.want)
			}
		})
	}
}

func TestShouldersStayGameButtonsWithoutSelect(t *testing.T) {
	m := NewManager(input.NewHandler(), newFakeEnum(), nil)
	claimed := false
	m.OnSeatClaim(func(string, input.Seat) { claimed = true })
	a := &activeDevice{info: DeviceInfo{Path: "/bt"}, profile: DualShockBT}

	for _, name := range []input.KeyName{input.KeyR1, input.KeyL1} {
		for _, kind := range []input.EventKind{input.KindKeydown, input.KindKeypress, input.KindKeyup} {
			if m.seatChord(a, chordEvent(kind, name)) {
				t.Errorf("%v %v was swallowed with SELECT up", kind, name)
			}
		}
	}
	if claimed {
		t.Error("a seat was claimed without the modifier held")
	}
}

func TestReleasingSelectEndsTheChord(t *testing.T) {
	m := NewManager(input.NewHandler(), newFakeEnum(), nil)
	a := &activeDevice{info: DeviceInfo{Path: "/bt"}, profile: DualShockBT}

	m.seatChord(a, chordEvent(input.KindKeydown, input.KeySelect))
	m.seatChord(a, chordEvent(input.KindKeyup, input.KeySelect))
	if m.seatChord(a, chordEvent(input.KindKeypress, input.KeyR1)) {
		t.Error("R1 still swallowed after SELECT was released")
	}
}

func TestRememberedSeatSurvivesAReconnect(t *testing.T) {
	enum := newFakeEnum()
	m := NewManager(input.NewHandler(), enum, nil)

	const mac = "02:1a:2b:3c:4d:03"
	m.RememberSeat(mac, input.SeatP1) // claim P1

	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-a"})
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-b"})
	enum.plug(DeviceInfo{VID: DualShockBT.VID, PID: DualShockBT.PID, Path: "/hidraw9", Bus: "bluetooth", Serial: mac})
	m.scan()
	defer m.Stop()

	var pad *Attached
	for _, at := range m.List() {
		if at.StableID == mac {
			a := at
			pad = &a
		}
	}
	if pad == nil {
		t.Fatal("the pad did not attach")
	}
	if pad.Seat != input.SeatP1 {
		t.Errorf("pad attached at seat %v, want the remembered seat P1 — attach order gave it %d", pad.Seat, pad.Index)
	}
}

func TestUnrememberedDeviceTakesAttachOrder(t *testing.T) {
	enum := newFakeEnum()
	m := NewManager(input.NewHandler(), enum, nil)
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-a"})
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-b"})
	m.scan()
	defer m.Stop()

	for _, at := range m.List() {
		if at.Seat != input.SeatOf(at.Index) {
			t.Errorf("%s sits at seat %v with index %d; unremembered devices take attach order", at.ID, at.Seat, at.Index)
		}
	}
}

func TestClaimingASeatLeavesTheWiredStickAlone(t *testing.T) {
	enum := newFakeEnum()
	m := NewManager(input.NewHandler(), enum, nil)
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-p1"})
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-p2"})
	enum.plug(DeviceInfo{VID: DualShockBT.VID, PID: DualShockBT.PID, Path: "/pad", Bus: "bluetooth", Serial: "aa:bb"})
	m.scan()
	defer m.Stop()

	seats := func() map[string]input.Seat {
		out := map[string]input.Seat{}
		for _, at := range m.List() {
			out[at.ID] = at.Seat
		}
		return out
	}
	before := seats()
	if before["/stick-p1"] != input.SeatP1 {
		t.Fatalf("the wired stick did not start at seat P1 (got %v)", before["/stick-p1"])
	}

	m.AssignSeat("/pad", input.SeatP1) // the pad claims P1

	after := seats()
	if after["/pad"] != input.SeatP1 {
		t.Errorf("the pad is at seat %v, want P1", after["/pad"])
	}
	if after["/stick-p1"] != input.SeatP1 {
		t.Errorf("the wired stick moved to seat %v; bolted hardware must never be displaced", after["/stick-p1"])
	}
	if after["/stick-p2"] != before["/stick-p2"] {
		t.Errorf("the other stick moved from seat %v to %v", before["/stick-p2"], after["/stick-p2"])
	}
}

func TestRememberedSeatAppliesToAPadThatWakesLater(t *testing.T) {
	enum := newFakeEnum()
	m := NewManager(input.NewHandler(), enum, nil)

	const mac = "02:1a:2b:3c:4d:03"
	m.RememberSeat(mac, input.SeatP2) // as a boot-time seed would

	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-a"})
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-b"})
	m.scan()
	enum.plug(DeviceInfo{VID: DualShockBT.VID, PID: DualShockBT.PID, Path: "/hidraw7", Bus: "bluetooth", Serial: mac})
	m.scan()
	defer m.Stop()

	for _, at := range m.List() {
		if at.StableID != mac {
			continue
		}
		if at.Seat != input.SeatP2 {
			t.Fatalf("the pad woke into seat %v, want its remembered seat P2 (attach order would have given it %d)", at.Seat, at.Index)
		}
		return
	}
	t.Fatal("the pad never attached")
}

func TestAttachIsAnnounced(t *testing.T) {
	enum := newFakeEnum()
	m := NewManager(input.NewHandler(), enum, nil)

	seen := make(chan Attached, 4)
	m.OnAttach(func(at Attached) { seen <- at })

	const mac = "02:1a:2b:3c:4d:03"
	enum.plug(DeviceInfo{VID: DualShockBT.VID, PID: DualShockBT.PID, Path: "/hidraw5", Bus: "bluetooth", Serial: mac})
	m.scan()
	defer m.Stop()

	select {
	case at := <-seen:
		if at.StableID != mac {
			t.Errorf("announced %q, want the pad's own identity %q", at.StableID, mac)
		}
		if at.Bus != "bluetooth" {
			t.Errorf("announced bus %q, want bluetooth", at.Bus)
		}
	default:
		t.Fatal("a controller attached and nothing was announced")
	}
}

func TestAttachWithNoHookIsHarmless(t *testing.T) {
	enum := newFakeEnum()
	m := NewManager(input.NewHandler(), enum, nil)
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick"})
	m.scan()
	m.Stop()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
		time.Sleep(time.Millisecond)
	}
}

func hasPrefix(names []string, prefix string) bool {
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func dualShockBTRest() []byte {
	report := make([]byte, 64)
	report[3], report[4], report[7] = 128, 128, 8
	return report
}

func TestChannelsFollowTheSeat(t *testing.T) {
	for _, tc := range []struct {
		name string
		move func(m *Manager)
	}{
		{"AssignSeat", func(m *Manager) { m.AssignSeat("/pad", input.SeatP2) }},
		{"PairSeat", func(m *Manager) { m.PairSeat("/pad", input.SeatP2) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enum := newFakeEnum()
			m := NewManager(input.NewHandler(), enum, nil)
			dev := enum.plug(DeviceInfo{VID: DualShockBT.VID, PID: DualShockBT.PID, Path: "/pad", Bus: "bluetooth"})
			m.scan()
			defer m.Stop()
			bus := m.Channels()

			if !hasPrefix(bus.Names(), "p1.") || hasPrefix(bus.Names(), "p2.") {
				t.Fatalf("attached at seat 0, offered %v", bus.Names())
			}

			tc.move(m)
			if hasPrefix(bus.Names(), "p1.") || !hasPrefix(bus.Names(), "p2.") {
				t.Fatalf("moved to seat 1, offered %v", bus.Names())
			}

			report := dualShockBTRest()
			report[9] = 1 // ps
			dev.reports <- report
			waitFor(t, "the report never reached p2.ps", func() bool { return bus.Read("p2.ps") == 1 })

			dev.Close()
			waitFor(t, "detach left channels behind", func() bool { return len(bus.Names()) == 0 })
		})
	}
}

func TestReseatingWhileReportsFlow(t *testing.T) {
	enum := newFakeEnum()
	handler := input.NewHandler()
	m := NewManager(handler, enum, nil)
	dev := enum.plug(DeviceInfo{VID: DualShockBT.VID, PID: DualShockBT.PID, Path: "/pad", Bus: "bluetooth"})
	m.scan()
	defer m.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			dev.reports <- dualShockBTRest()
		}
	}()
	for i := 0; i < 200; i++ {
		m.AssignSeat("/pad", input.SeatOf(i%2))
		m.PairSeat("/pad", input.SeatOf((i+1)%2))
		handler.Drain()
	}
	<-done
}

func TestAThirdDeviceHasNoSeat(t *testing.T) {
	enum := newFakeEnum()
	handler := input.NewHandler()
	m := NewManager(handler, enum, nil)
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-a"})
	enum.plug(DeviceInfo{VID: DragonRise.VID, PID: DragonRise.PID, Path: "/stick-b"})
	pad := enum.plug(DeviceInfo{VID: DualShockBT.VID, PID: DualShockBT.PID, Path: "/pad", Bus: "bluetooth"})
	m.scan()
	defer m.Stop()

	for _, at := range m.List() {
		if at.ID == "/pad" && at.Seat != input.NoSeat {
			t.Errorf("third device at seat %v, want NoSeat", at.Seat)
		}
	}
	for _, n := range m.Channels().Names() {
		if strings.HasSuffix(n, ".ps") {
			t.Errorf("a seatless device offered %s", n)
		}
	}

	pad.reports <- dualShockBTRest()
	pressed := dualShockBTRest()
	pressed[7] |= 0x20 // cross
	pad.reports <- pressed
	for _, ev := range drainEventually(t, handler) {
		if ev.Source.Seat != input.NoSeat {
			t.Errorf("seatless device's event carries seat %v", ev.Source.Seat)
		}
	}

	m.AssignSeat("/pad", input.SeatP1)
	if !hasPrefix(m.Channels().Names(), "p1.") {
		t.Errorf("claiming seat 0 offered %v", m.Channels().Names())
	}
}
