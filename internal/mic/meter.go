// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"math"
	"sync"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// levelWindow is the level meter's window at Rate: 50 ms.
const levelWindow = Rate / 20

// clipLevel is the sample magnitude counted as clipping.
const clipLevel = 0.999

// levelMeter reduces the raw capture to a level frame every levelWindow
// samples, and to band peaks every bandHop. push runs on the capture
// goroutine; the readers are safe from any other.
type levelMeter struct {
	mu      sync.Mutex
	level   hardware.MicLevel
	levelOK bool
	bands   [hardware.MicBandCount]float64
	bandsOK bool

	sum, peak float64
	count     int
	clipped   bool
	analyser  *bands
}

func newLevelMeter() *levelMeter { return &levelMeter{analyser: newBands()} }

func (m *levelMeter) push(block []float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range block {
		a := math.Abs(s)
		m.sum += s * s
		m.peak = max(m.peak, a)
		if a >= clipLevel {
			m.clipped = true
		}
		if f, ok := m.analyser.push(s); ok {
			m.bands, m.bandsOK = f, true
		}
		m.count++
		if m.count == levelWindow {
			m.level = hardware.MicLevel{
				Seq:     m.level.Seq + 1,
				RMS:     math.Sqrt(m.sum / float64(levelWindow)),
				Peak:    m.peak,
				Clipped: m.clipped,
			}
			m.levelOK = true
			m.sum, m.peak, m.count, m.clipped = 0, 0, 0, false
		}
	}
}

func (m *levelMeter) latestLevel() (hardware.MicLevel, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.level, m.levelOK
}

func (m *levelMeter) latestBands() ([hardware.MicBandCount]float64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bands, m.bandsOK
}
