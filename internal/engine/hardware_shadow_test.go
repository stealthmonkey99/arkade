// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// textRecorder keeps every LCD write so tests can count what reached the
// device, not only what it ends up showing.
type textRecorder struct {
	lampRecorder
	writes []textWriteRec
}

type textWriteRec struct {
	index        int
	line1, line2 string
}

func (r *textRecorder) SetText(index int, line1, line2 string) {
	r.writes = append(r.writes, textWriteRec{index, line1, line2})
}

func newTextRig() (*shadowHardware, *textRecorder) {
	rec := &textRecorder{}
	return newShadowHardware(rec), rec
}

func TestShadowTextFirstWriteAlwaysLands(t *testing.T) {
	s, rec := newTextRig()
	s.SetText(-1, "", "")
	if len(rec.writes) != 1 {
		t.Fatalf("a first blank write made %d device writes, want 1: the startup card must be cleared", len(rec.writes))
	}
	if rec.writes[0].index != -1 {
		t.Errorf("both seats agreeing went out as %d, want one -1 write", rec.writes[0].index)
	}
}

func TestShadowTextDedupesIdenticalWrites(t *testing.T) {
	s, rec := newTextRig()
	for range 5 {
		s.SetText(-1, "SCORE 10", "LIVES 3")
	}
	if len(rec.writes) != 1 {
		t.Fatalf("five identical writes made %d device writes, want 1", len(rec.writes))
	}
	s.SetText(-1, "SCORE 20", "LIVES 3")
	if len(rec.writes) != 2 {
		t.Fatalf("a changed line made no device write")
	}
}

func TestShadowTextDedupesPerSeat(t *testing.T) {
	s, rec := newTextRig()
	s.SetText(0, "P1", "")
	s.SetText(1, "P2", "")
	rec.writes = nil
	s.SetText(-1, "P1", "")
	if len(rec.writes) != 1 || rec.writes[0].index != 1 {
		t.Fatalf("writes = %+v, want only seat 1, whose text changed", rec.writes)
	}
}

func TestShadowTextReleaseReplaysOnlyWhatThePreviewChanged(t *testing.T) {
	s, rec := newTextRig()
	s.SetText(-1, "GAME", "HUD")
	s.holdText()
	s.SetText(-1, "GAME", "HUD") // the app keeps writing under the hold
	s.overrideText(0, "PREVIEW", "")
	rec.writes = nil
	s.releaseText()
	if len(rec.writes) != 1 || rec.writes[0] != (textWriteRec{0, "GAME", "HUD"}) {
		t.Fatalf("release wrote %+v, want the app's text back on seat 0 only", rec.writes)
	}
	s.releaseText()
	if len(rec.writes) != 1 {
		t.Error("an unbalanced release repainted the LCDs")
	}
}

func TestShadowTextHoldStopsForwardingButKeepsMirroring(t *testing.T) {
	s, rec := newTextRig()
	s.holdText()
	s.SetText(-1, "LATE", "WRITE")
	if len(rec.writes) != 0 {
		t.Fatal("a write under a hold reached the device")
	}
	s.releaseText()
	if len(rec.writes) != 1 || rec.writes[0].line1 != "LATE" {
		t.Fatalf("release wrote %+v, want the text mirrored under the hold", rec.writes)
	}
}

func TestShadowRestoreReplaysASnapshot(t *testing.T) {
	s, rec := newTextRig()
	s.SetText(-1, "GAME", "HUD")
	snap := s.snapshot()
	s.SetText(-1, "ATTRACT", "REEL")
	rec.writes = nil
	s.restore(snap)
	if len(rec.writes) != 2 {
		t.Fatalf("restore made %d writes, want one per seat", len(rec.writes))
	}
	for _, w := range rec.writes {
		if w.line1 != "GAME" || w.line2 != "HUD" {
			t.Errorf("restore wrote %+v, want the snapshot's text", w)
		}
	}
}

func TestShadowForwardsOnlyTheCapabilitiesTheSuiteHas(t *testing.T) {
	s, _ := newTextRig()
	if _, ok := s.CPUTempF(); ok {
		t.Error("a suite with no thermal sensor reported a temperature")
	}
	if s.FanOn() || s.FanMode() != hardware.FanAuto {
		t.Error("a suite with no fan reported one")
	}
	if s.MotionActive() {
		t.Error("a suite with no motion sensor reported presence")
	}
	if _, ok := s.LastMotion(); ok {
		t.Error("a suite with no motion history reported an edge")
	}
	v := newShadowHardware(hardware.NewVirtual())
	v.InputSignal()
	if _, ok := v.LastMotion(); !ok {
		t.Error("Virtual's input spy did not reach it through the shadow")
	}
}
