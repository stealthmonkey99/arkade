// SPDX-License-Identifier: GPL-2.0-or-later

package mic

// unalignedGateMs is how long after the mixer goes quiet the fallback gate
// keeps the detectors off while the canceller is unaligned.
const unalignedGateMs = 800

// echoVerdict is what the judge decided about one analysis frame: whether
// it is the cabinet hearing itself, and which tone-bank bins carry the
// cabinet's playback (nil when no bin-level judgement was made).
type echoVerdict struct {
	echo  bool
	taint []bool
}

// echoJudge cancels the cabinet's playback out of each analysis frame and
// judges what is left, in one of three regimes: bypassed (Options.NoCancel),
// aligned (the bin-by-bin gate or the canceller's leakage test), and
// unaligned (any recent playback is echo). It runs on the capture goroutine
// and is not safe for concurrent use.
type echoJudge struct {
	aec        *Canceller
	gate       *echoGate
	ref        *ring
	bypass     bool
	minEchoRMS float64
	minRefRMS  float64
	recent     []float64
}

func newEchoJudge(det Config, cfg CancellerConfig, ref, mic *ring, bypass bool) *echoJudge {
	return &echoJudge{
		aec:        NewCanceller(cfg, ref, mic),
		gate:       newEchoGate(det, cfg.MinRefRMS),
		ref:        ref,
		bypass:     bypass,
		minEchoRMS: det.MinRMS * 0.5,
		minRefRMS:  cfg.MinRefRMS,
	}
}

// judge writes frame's residual into clean and returns the verdict. The
// canceller adapts even when bypassed, so its stats stay live.
func (j *echoJudge) judge(frame, clean []float64, end int) echoVerdict {
	j.aec.Process(frame, clean, end)
	switch ref := j.aec.Reference(); {
	case j.bypass:
		copy(clean, frame)
		return echoVerdict{}
	case ref != nil:
		echo := j.gate.judge(ref, frame, clean) || j.aec.EchoDominated(j.minEchoRMS)
		return echoVerdict{echo: echo, taint: j.gate.taint}
	default:
		return echoVerdict{echo: j.playingRecently()}
	}
}

// playingRecently reports whether the reference carried sound in the last
// unalignedGateMs.
func (j *echoJudge) playingRecently() bool {
	n := unalignedGateMs * Rate / 1000
	end := j.ref.len()
	j.recent = j.ref.window(end-n, n, j.recent)
	return j.recent != nil && meanPow(j.recent) > j.minRefRMS*j.minRefRMS
}
