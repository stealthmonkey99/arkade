// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"math"
	"testing"
)

func TestLevelMeterPublishesOncePerWindow(t *testing.T) {
	m := newLevelMeter()
	if _, ok := m.latestLevel(); ok {
		t.Fatal("a level before any audio")
	}
	window := make([]float64, levelWindow)
	for i := range window {
		window[i] = 0.5
	}
	m.push(window[:levelWindow/2])
	if _, ok := m.latestLevel(); ok {
		t.Fatal("a level after half a window")
	}
	m.push(window[levelWindow/2:])
	l, ok := m.latestLevel()
	if !ok || l.Seq != 1 || math.Abs(l.RMS-0.5) > 1e-12 || l.Peak != 0.5 || l.Clipped {
		t.Fatalf("after one window of 0.5: %+v ok=%v", l, ok)
	}

	loud := make([]float64, levelWindow)
	loud[7] = -1
	m.push(loud)
	l, _ = m.latestLevel()
	if l.Seq != 2 || l.Peak != 1 || !l.Clipped {
		t.Fatalf("a full-scale sample was not reported as a clipped peak: %+v", l)
	}

	m.push(make([]float64, levelWindow))
	l, _ = m.latestLevel()
	if l.Seq != 3 || l.RMS != 0 || l.Peak != 0 || l.Clipped {
		t.Fatalf("a silent window carried the last one's peak or clip: %+v", l)
	}
}

func TestLevelMeterReportsBandsAlongsideTheLevel(t *testing.T) {
	m := newLevelMeter()
	m.push(make([]float64, 3*levelWindow))
	if _, ok := m.latestBands(); !ok {
		t.Fatal("no band frame after a full analysis length")
	}
}
