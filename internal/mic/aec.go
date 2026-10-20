// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/dsp"
)

// Canceller subtracts the cabinet's own playback from what its microphone
// hears, using the host's known playback as the reference. Per block it
// aligns the reference to the capture, cancels with an NLMS filter, and marks
// frames echo-dominated when the residual is within the leakage the filter's
// ERLE predicts. The output is the residual.
type Canceller struct {
	taps      int
	preRoll   int // taps kept ahead of the estimated delay, for jitter
	mu        float64
	minRefPow float64 // per-sample power below which there is no echo to learn
	maxDelay  int     // search range either side of nominal alignment, samples

	w   []float64 // filter weights, w[0] applies to the newest aligned reference sample
	x   history   // the last Taps aligned reference samples
	ref *ring     // decimated reference, absolute-indexed
	mic *ring     // raw capture, absolute-indexed, for delay estimation

	// delay is the capture offset of the echo: mic[m] ≈ h * ref[m - delay].
	delay     int
	aligned   bool
	estimated int     // mic index at the last estimate attempt
	lastScore float64 // correlation score of the current alignment
	tryScore  float64 // see CancellerStats
	tryShape  float64

	micPow, errPow float64 // slow averages, for ERLE
	erleDB         float64 // instantaneous
	// erleHeld is what the gate trusts: it follows erleDB up at once and decays
	// 1 dB per second of playback, so talking over the game doesn't widen the allowance.
	erleHeld float64

	// per-frame judgement, updated by Process for the block's last frame
	frameEcho, frameRes float64

	scratchRef, scratchMic []float64
	envRef, envMic         []float64
	refWin                 []float64 // the aligned reference span for the frame being processed
	refOK                  bool
}

// CancellerConfig holds the canceller's tunables; see DefaultCancellerConfig.
type CancellerConfig struct {
	// Taps is the adaptive filter length at Rate. 768 covers 52 ms: the
	// envelope estimate's slack plus a small room's early reflections.
	Taps int
	// PreRoll is how many taps sit ahead of the estimated delay. It must
	// exceed the envelope estimate's error of about a hundred samples.
	PreRoll int
	// Mu is the NLMS step size, 0..1. 0.25 converges in about a second of
	// game audio.
	Mu float64
	// MaxDelayMs bounds the delay search; player, device and arecord
	// buffering sum to well under half a second.
	MaxDelayMs int
	// MinRefRMS is the reference level below which the canceller neither
	// adapts nor estimates.
	MinRefRMS float64
}

// DefaultCancellerConfig is tuned for the cabinet's 20 ms device buffer and
// 40 ms player buffer.
func DefaultCancellerConfig() CancellerConfig {
	return CancellerConfig{
		Taps:       768,
		PreRoll:    192,
		Mu:         0.25,
		MaxDelayMs: 600,
		MinRefRMS:  0.003,
	}
}

// delayWindow, estimateGuard and negativeLag are in samples at Rate: the span
// one estimate correlates over, how far short of the newest capture sample it
// stops, and how far the reference may trail the capture and still be found.
const (
	delayWindow   = Rate
	estimateGuard = Rate * 3 / 20
	negativeLag   = Rate / 10
)

// erleSmoothing weights the per-sample power averages. doubleTalkRatio (12 dB)
// is how far a residual may exceed the predicted echo plus mean reference
// power before adaptation freezes. leakMargin is how far above predicted
// leakage a residual may sit and still count as echo.
const (
	erleSmoothing   = 0.001
	doubleTalkRatio = 16
	leakMargin      = 3
)

// erleFloorDB and erleCeilDB bound the ERLE the gate credits: below the floor
// the filter counts as unconverged, above the ceiling further gain is not
// believed.
const (
	erleFloorDB = 0
	erleCeilDB  = 26
)

// NewCanceller builds a canceller over the shared reference and capture
// rings, which must hold at least delayWindow plus estimateGuard plus
// MaxDelayMs of history.
func NewCanceller(cfg CancellerConfig, ref, mic *ring) *Canceller {
	if cfg.Taps <= 0 {
		cfg = DefaultCancellerConfig()
	}
	return &Canceller{
		taps:      cfg.Taps,
		preRoll:   cfg.PreRoll,
		mu:        cfg.Mu,
		minRefPow: cfg.MinRefRMS * cfg.MinRefRMS,
		w:         make([]float64, cfg.Taps),
		x:         newHistory(cfg.Taps),
		ref:       ref,
		mic:       mic,
		maxDelay:  cfg.MaxDelayMs * Rate / 1000,
	}
}

// ERLE reports the echo return loss enhancement in dB: how much quieter the
// residual is than the raw capture while the cabinet plays.
func (c *Canceller) ERLE() float64 { return c.erleDB }

// Delay reports the estimated echo offset in samples at Rate, and whether
// one has been estimated at all.
func (c *Canceller) Delay() (int, bool) { return c.delay, c.aligned }

// Score reports the normalized cross-correlation the current alignment was
// found at, 0..1; 0 before any alignment.
func (c *Canceller) Score() float64 { return c.lastScore }

// CancellerStats is a copy of the canceller's state for status pages.
type CancellerStats struct {
	ERLE    float64
	Delay   int
	Aligned bool
	Score   float64
	// TryScore and TryShape are the latest delay estimate's correlation
	// score and envelope shape, whether or not it was accepted: why
	// alignment is or is not happening.
	TryScore, TryShape float64
}

// Stats copies the canceller's state. Like Process it is not safe for
// concurrent use; a caller sharing it publishes the copy.
func (c *Canceller) Stats() CancellerStats {
	return CancellerStats{
		ERLE: c.erleDB, Delay: c.delay, Aligned: c.aligned, Score: c.lastScore,
		TryScore: c.tryScore, TryShape: c.tryShape,
	}
}

// Reset forgets the filter and the alignment, for a capture restart.
func (c *Canceller) Reset() {
	for i := range c.w {
		c.w[i] = 0
	}
	c.x.reset()
	c.aligned, c.estimated, c.lastScore = false, 0, 0
	c.micPow, c.errPow, c.erleDB, c.erleHeld = 0, 0, 0, 0
}

// Process cancels the echo in samples, already pushed to the mic ring and
// ending at logical index end, writing the residual into out.
func (c *Canceller) Process(samples, out []float64, end int) {
	start := end - len(samples)
	if !c.align(start, end) {
		copy(out, samples)
		c.frameEcho, c.frameRes, c.refOK = 0, rmsOf(samples), false
		return
	}
	echoSum := 0.0
	loudSamples := 0
	for i, d := range samples {
		r := 0.0
		if c.refOK {
			r = c.refWin[i+c.preRoll]
		}
		c.x.push(r)
		y := c.predict()
		e := d - y
		out[i] = e
		echoSum += y * y
		if c.adapt(d, e, y) {
			loudSamples++
		}
	}
	c.updateERLE(loudSamples)
	c.frameEcho = math.Sqrt(echoSum / float64(len(samples)))
	c.frameRes = rmsOf(out)
}

// align re-estimates the delay when due and, once aligned, loads the
// reference span the frame [start, end) echoes: sample i carries
// ref[start+i-delay], with PreRoll more ahead of it as the newest tap.
func (c *Canceller) align(start, end int) bool {
	if c.dueForEstimate(start) {
		c.estimateDelay(end)
	}
	if !c.aligned {
		return false
	}
	c.refWin = c.ref.window(start-c.delay, end-start+c.preRoll, c.refWin)
	c.refOK = c.refWin != nil
	return true
}

func (c *Canceller) predict() float64 {
	return dot(c.w, c.x.newestFirst())
}

// adapt takes one NLMS step when the reference is loud enough to carry an
// echo and the residual does not dwarf the prediction (double talk). It
// reports whether the reference was loud.
func (c *Canceller) adapt(d, e, y float64) bool {
	n := float64(c.taps)
	pow := max(c.x.pow, 0)
	if pow <= c.minRefPow*n {
		return false
	}
	c.micPow += (d*d - c.micPow) * erleSmoothing
	c.errPow += (e*e - c.errPow) * erleSmoothing
	if e*e < doubleTalkRatio*(y*y+pow/n) {
		addScaled(c.w, c.x.newestFirst(), c.mu*e/(pow+1e-6))
	}
	return true
}

func (c *Canceller) updateERLE(loudSamples int) {
	if c.micPow > 1e-9 && c.errPow > 1e-12 {
		c.erleDB = 10 * math.Log10(c.micPow/c.errPow)
	}
	if c.erleDB > c.erleHeld {
		c.erleHeld = c.erleDB
	} else {
		c.erleHeld = max(c.erleHeld-float64(loudSamples)/Rate, 0)
	}
}

// history is the last n samples pushed, readable newest first as one
// contiguous slice: each sample is written twice, n apart, so the span
// starting at head never wraps.
type history struct {
	buf  []float64
	head int
	pow  float64 // running sum of squares, kept incrementally
}

func newHistory(n int) history { return history{buf: make([]float64, 2*n)} }

func (h *history) push(r float64) {
	n := len(h.buf) / 2
	if h.head == 0 {
		h.head = n
	}
	h.head--
	old := h.buf[h.head]
	h.pow += r*r - old*old
	h.buf[h.head], h.buf[h.head+n] = r, r
}

func (h *history) newestFirst() []float64 { return h.buf[h.head : h.head+len(h.buf)/2] }

func (h *history) reset() {
	clear(h.buf)
	h.head, h.pow = 0, 0
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i, v := range a {
		s += v * b[i]
	}
	return s
}

// addScaled is a[i] += g·b[i].
func addScaled(a, b []float64, g float64) {
	for i := range a {
		a[i] += g * b[i]
	}
}

// Reference returns the reference samples aligned with the last processed
// frame, or nil before alignment. Valid until the next Process.
func (c *Canceller) Reference() []float64 {
	if !c.refOK {
		return nil
	}
	return c.refWin[:len(c.refWin)-c.preRoll]
}

// EchoDominated reports whether the last processed block was mostly the
// cabinet hearing itself: the predicted echo was audible and the residual
// was within the leakage the current ERLE predicts. A detector should not
// fire on such a frame.
func (c *Canceller) EchoDominated(floor float64) bool {
	if !c.aligned || c.frameEcho < floor {
		return false
	}
	erle := math.Max(erleFloorDB, math.Min(erleCeilDB, c.erleHeld))
	leak := c.frameEcho * math.Pow(10, -erle/20)
	return c.frameRes < leakMargin*leak
}

func (c *Canceller) dueForEstimate(micIndex int) bool {
	// Every quarter second until aligned, then every few seconds until ERLE
	// reaches 6 dB.
	interval := Rate / 4
	if c.aligned {
		if c.erleDB >= 6 {
			return false
		}
		interval = 4 * Rate
	}
	return micIndex-c.estimated > interval
}

// estimateDelay cross-correlates a window of capture against the reference,
// the reference window starting negativeLag later so a correlation lag of L is
// an echo offset of L - negativeLag.
//
// It correlates energy envelopes, not waveforms: game sounds are near-pure
// tones, whose waveforms correlate equally well at every whole period of lag,
// while an envelope onset happens once.
func (c *Canceller) estimateDelay(micEnd int) {
	c.estimated = micEnd
	end := micEnd - estimateGuard
	start := end - delayWindow
	if start < 0 || c.ref.len() < end+negativeLag {
		return
	}
	c.scratchMic = c.mic.window(start, delayWindow, c.scratchMic)
	c.scratchRef = c.ref.window(start+negativeLag, delayWindow, c.scratchRef)
	if c.scratchMic == nil || c.scratchRef == nil {
		return
	}
	if meanPow(c.scratchRef) < c.minRefPow {
		return
	}
	var shape float64
	c.envRef, shape = envelope(c.scratchRef, c.envRef[:0])
	c.tryShape, c.tryScore = shape, 0
	if shape < minEnvelopeShape {
		return
	}
	c.envMic, _ = envelope(c.scratchMic, c.envMic[:0])
	lag, score := dsp.DelayEstimateWhole(c.envRef, c.envMic, (c.maxDelay+negativeLag)/envelopeHop)
	c.tryScore = score
	// A real effect through a real room scores about 0.34 (whistle.wav off
	// the cabinet); the shape gate already excludes steady tones.
	if score < minAlignScore || (c.aligned && score < minAlignScore+0.1) {
		return
	}
	delay := lag*envelopeHop + envelopeHop/2 - negativeLag
	if fine := c.ref.window(start-delay+refineRange, delayWindow, c.scratchRef); fine != nil {
		if lag2, score2 := dsp.DelayEstimateWhole(fine, c.scratchMic, 2*refineRange); score2 >= 0.3 {
			delay += lag2 - refineRange
		}
	}
	if c.aligned && abs(delay-c.delay) <= envelopeHop {
		return
	}
	c.delay, c.aligned = delay, true
	c.lastScore = score
	for i := range c.w {
		c.w[i] = 0
	}
	c.erleDB, c.erleHeld = 0, 0
}

// envelopeHop is the envelope's resolution in samples, about a millisecond.
// minEnvelopeShape rejects steady tones, whose faint RMS ripple correlates
// perfectly at the wrong lag. refineRange is how far either side of the
// envelope's answer the waveform refinement looks.
const (
	envelopeHop      = 16
	envelopeDeadband = 0.04
	minEnvelopeShape = 0.5
	minAlignScore    = 0.25
	refineRange      = 160
)

// envelope appends the slope of v's smoothed RMS envelope, hop by hop, to
// out, and reports the envelope's shape: peak-to-trough range as a fraction of
// the peak. Slopes, because a steady level correlates perfectly at every lag;
// slopes below envelopeDeadband of the peak are zeroed, leaving onsets and
// releases.
func envelope(v []float64, out []float64) ([]float64, float64) {
	const (
		window = 4 * envelopeHop
		smooth = 5
		span   = 5
	)
	var levels [4096]float64
	n := 0
	for i := 0; i+window <= len(v) && n < len(levels); i += envelopeHop {
		s := 0.0
		for _, x := range v[i : i+window] {
			s += x * x
		}
		levels[n] = math.Sqrt(s / window)
		n++
	}
	if n <= smooth+span {
		return out, 0
	}
	var smoothed [4096]float64
	lo, hi := math.Inf(1), 0.0
	m := 0
	for i := 0; i+smooth <= n; i++ {
		s := 0.0
		for _, r := range levels[i : i+smooth] {
			s += r
		}
		s /= smooth
		smoothed[m] = s
		m++
		lo, hi = math.Min(lo, s), math.Max(hi, s)
	}
	if hi <= 0 {
		return out, 0
	}
	dead := envelopeDeadband * hi
	for i := 0; i+span < m; i++ {
		d := smoothed[i+span] - smoothed[i]
		if math.Abs(d) < dead {
			d = 0
		}
		out = append(out, d)
	}
	return out, (hi - lo) / hi
}

func rmsOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return math.Sqrt(meanPow(v))
}

func meanPow(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x * x
	}
	return s / float64(len(v))
}
