// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import "github.com/stealthmonkey99/arkade/internal/dsp"

// echoGate judges, bin by bin, whether what is left after cancellation is
// the room or the cabinet. The canceller's filter only learns the echo path
// at frequencies the game has already played, so a new sound leaves a loud
// residual. The gate runs the reference through the detector's filter banks
// and marks every bin where the residual is no louder than the echo the
// speaker would plausibly leave there.
type echoGate struct {
	low, tone *bank
	window    []float64
	refBuf    []float64
	resBuf    []float64
	refTone   []float64 // the reference frame's per-bin power, both banks
	refLow    []float64
	taint     []bool // one per tone-bank bin, for the detector

	// gain is the estimated power ratio from reference to raw capture. It
	// starts at unity, which over-marks rather than under-marks.
	gain      float64
	minRefPow float64
}

// gateMargin is the power ratio above the plausible echo at which a bin
// counts as the room's own sound (3 dB).
const gateMargin = 2.0

func newEchoGate(cfg Config, minRefRMS float64) *echoGate {
	n := cfg.FrameLen
	layout := newBankLayout(cfg)
	g := &echoGate{
		window:    dsp.SymmetricHann(n),
		refBuf:    make([]float64, n),
		resBuf:    make([]float64, n),
		gain:      1,
		minRefPow: minRefRMS * minRefRMS,
	}
	g.low, g.tone = layout.banks()
	g.taint = make([]bool, layout.toneBins())
	g.refTone = make([]float64, len(g.tone.pow))
	g.refLow = make([]float64, len(g.low.pow))
	return g
}

// judge compares the aligned reference frame with the residual (and the
// raw capture, for the gain estimate), returning whether the residual is
// mostly the speaker. The taint mask is left for the detector.
func (g *echoGate) judge(ref, raw, res []float64) bool {
	for i := range g.taint {
		g.taint[i] = false
	}
	if ref == nil || meanPow(ref) < g.minRefPow {
		return false
	}
	for i := range ref {
		g.refBuf[i] = ref[i] * g.window[i]
		g.resBuf[i] = res[i] * g.window[i]
	}
	g.tone.run(g.refBuf)
	g.low.run(g.refBuf)
	copy(g.refTone, g.tone.pow)
	copy(g.refLow, g.low.pow)
	resTone := g.tone.run(g.resBuf)
	resLow := g.low.run(g.resBuf)

	// Path gain, learned only while the raw capture is not far louder than
	// the reference explains, so a person talking does not inflate it.
	if rp, rr := meanPow(ref), meanPow(raw); rp > 4*g.minRefPow {
		ratio := rr / rp
		if ratio < g.gain*4 {
			g.gain += (ratio - g.gain) * 0.05
		}
	}

	tainted, total := 0.0, resTone+resLow
	for i, p := range g.tone.pow {
		if p <= gateMargin*g.gain*g.refTone[i] {
			g.taint[i] = true
			tainted += p
		}
	}
	for i, p := range g.low.pow {
		if p <= gateMargin*g.gain*g.refLow[i] {
			tainted += p
		}
	}
	return total > 0 && tainted >= 0.5*total
}
