// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "math"

// SweepMode selects how a layer with a moving frequency is evaluated.
type SweepMode int

const (
	// SweepPointwise evaluates sin(2π·f(t)·t) at absolute time. The
	// instantaneous frequency is f(t) + t·f'(t), so the sweep overshoots
	// SlideTo. This is the default.
	SweepPointwise SweepMode = iota
	// SweepPhase integrates the frequency, so the sound arrives at SlideTo.
	// The first sample sits at phase zero.
	SweepPhase
	// SweepPhaseLead advances the phase before evaluating each sample, so
	// every sample leads SweepPhase's by one increment. The two are
	// different waveforms, not a rounding difference.
	SweepPhaseLead
)

// EnvKind selects a layer's envelope shape.
type EnvKind int

const (
	// EnvADSR is the linear ADSR envelope, evaluated at the layer's own time.
	EnvADSR EnvKind = iota
	// EnvExpDecay is e^(-t·Rate): instant onset, exponential tail.
	EnvExpDecay
	// EnvAttackDecay is min(1, t/AttackS) · e^(-t·Rate). It is a product so
	// the end of the ramp does not step down.
	EnvAttackDecay
	// EnvFadeOut is a linear 1→0 across the layer raised to Curve.
	EnvFadeOut
	// EnvArch is sin(π·u): silent at both ends, peaking in the middle.
	EnvArch
	// EnvConst holds 1 for the layer's whole length.
	EnvConst
)

// Env is a layer's amplitude shape over its own duration.
type Env struct {
	Kind EnvKind
	// ADSR is EnvADSR's envelope; it reads 0 past its own end.
	ADSR ADSR
	// Rate is the decay rate of EnvExpDecay and EnvAttackDecay; larger is faster.
	Rate float64
	// AttackS is EnvAttackDecay's linear onset in seconds.
	AttackS float64
	// StartLevel in [0,1) is where the EnvADSR or EnvAttackDecay attack
	// starts rising from instead of silence. It needs an attack to mean anything.
	StartLevel float64
	// Curve is EnvFadeOut's exponent; <= 0 reads as 1.
	Curve float64
}

// At returns the envelope value at t seconds into a layer of length dur.
func (e Env) At(t, dur float64) float64 {
	if t < 0 {
		return 0
	}
	switch e.Kind {
	case EnvExpDecay:
		return ExpDecay(t, e.Rate)
	case EnvAttackDecay:
		return e.onset(t, e.AttackS) * ExpDecay(t, e.Rate)
	case EnvFadeOut:
		u := frac(t, dur)
		c := e.Curve
		if c <= 0 {
			c = 1
		}
		return math.Pow(1-u, c)
	case EnvArch:
		return math.Sin(math.Pi * frac(t, dur))
	case EnvConst:
		return 1
	default:
		if t < e.ADSR.AttackS {
			return e.onset(t, e.ADSR.AttackS)
		}
		return e.ADSR.At(t)
	}
}

// onset is the linear rise over attackS from StartLevel; 1 once the attack is
// over or absent.
func (e Env) onset(t, attackS float64) float64 {
	if attackS <= 0 || t >= attackS {
		return 1
	}
	// The conversion forbids arm64's fused multiply-add so output is identical across machines.
	return e.StartLevel + float64((1-e.StartLevel)*(t/attackS))
}

// frac is a layer's elapsed time over its length, clamped to [0,1].
func frac(t, dur float64) float64 {
	if dur <= 0 {
		return 0
	}
	u := t / dur
	switch {
	case u < 0:
		return 0
	case u > 1:
		return 1
	}
	return u
}

// Partial is one extra component of a layer's source.
type Partial struct {
	// Ratio multiplies the layer's frequency, so the partial follows a slide.
	Ratio float64
	// Hz fixes the partial's frequency and takes precedence over Ratio. It
	// does not follow the layer's slide.
	Hz float64
	// Gain scales the partial against the fundamental's 1.
	Gain float64
	// Wave is the partial's shape; the zero value is a sine.
	Wave Wave
	// Decay, when positive, is an extra exponential decay on this partial.
	Decay float64
}

// Gate restricts a layer to part of its run, in seconds from the end of the
// layer's Delay. A gated-out sample draws no noise, so the shared stream is
// unaffected.
type Gate struct {
	// From and To bound the window; To <= 0 means to the end.
	From, To float64
	// Period and On, when both positive, repeat the window: active for On
	// seconds of every Period.
	Period, On float64
}

// openAt reports whether the gate admits the layer at time t.
func (g *Gate) openAt(t float64) bool {
	if g == nil {
		return true
	}
	if t < g.From {
		return false
	}
	if g.To > 0 && t >= g.To {
		return false
	}
	if g.Period > 0 && g.On > 0 {
		return math.Mod(t-g.From, g.Period) < g.On
	}
	return true
}

// LFO is a sinusoidal modulator, Base + Depth·sin(2π·Hz·t), used as tremolo.
type LFO struct {
	Hz, Base, Depth float64
}

// Vibrato wobbles a layer's pitch with an LFO at Hz. DepthHz is a width in
// hertz; Depth in [0,1) is a fraction of the current frequency. Naming both is
// refused. Wave is WaveSine or WaveTriangle; under SweepPointwise a triangle
// trills between two pitches.
type Vibrato struct {
	Hz, DepthHz float64
	Depth       float64
	Wave        Wave
}

// apply returns f wobbled at time t.
func (v *Vibrato) apply(t, f float64) float64 {
	if v == nil || v.Hz <= 0 {
		return f
	}
	lfo := Sine(t, v.Hz)
	if v.Wave == WaveTriangle {
		lfo = Triangle(t, v.Hz)
	}
	if v.Depth != 0 {
		return f * (1 + v.Depth*lfo)
	}
	return f + v.DepthHz*lfo
}

// NoiseAM multiplies a layer's source by Base + Depth·noise from the shared
// stream. It applies at source time, ahead of the envelope.
type NoiseAM struct {
	Base, Depth float64
}

func (l *LFO) at(t float64) float64 {
	if l == nil || l.Hz <= 0 {
		return 1
	}
	return l.Base + l.Depth*Sine(t, l.Hz)
}

// FilterKind selects a layer's filter.
type FilterKind int

const (
	// FilterNone leaves the layer unfiltered.
	FilterNone FilterKind = iota
	// FilterLowpass and FilterHighpass are the one-pole filters Lowpass and Highpass.
	FilterLowpass
	FilterHighpass
	// FilterBandPass is the resonant BandPass.
	FilterBandPass
)

// Filter is one shaping stage applied to a layer.
type Filter struct {
	Kind FilterKind
	// Hz is the corner, or the band's centre at the start of the layer.
	Hz float64
	// SweepTo, when positive, moves a band-pass centre to it across the layer.
	SweepTo float64
	// Arc sends the centre to SweepTo and back, following sin(π·u).
	Arc bool
	// Q is FilterBandPass's resonance. Higher is narrower.
	Q float64
	// Gain scales the filter output; a narrow band-pass discards most of the energy.
	Gain float64
	// BeforeEnv applies the filter to the raw source ahead of the envelope.
	// Filtering before and after a moving gain are different sounds.
	BeforeEnv bool
}

// apply runs the stage over raw in place.
func (f Filter) apply(raw []float64, dur float64, sampleRate int) {
	switch f.Kind {
	case FilterLowpass:
		Lowpass(raw, sampleRate, f.Hz)
	case FilterHighpass:
		Highpass(raw, sampleRate, f.Hz)
	case FilterBandPass:
		centre := []float64{f.Hz}
		if f.SweepTo > 0 && len(raw) > 1 {
			centre = make([]float64, len(raw))
			for i := range centre {
				u := frac(SampleTime(sampleRate, i), dur)
				if f.Arc {
					u = math.Sin(math.Pi * u)
				}
				centre[i] = f.Hz + (f.SweepTo-f.Hz)*u
			}
		}
		copy(raw, BandPass(raw, sampleRate, centre, f.Q))
	default:
		return
	}
	if f.Gain > 0 && f.Gain != 1 {
		for i := range raw {
			raw[i] *= f.Gain
		}
	}
}

// Layer is one component of a patch. Layers share a time base and the
// patch's noise stream, and are summed in declared order.
type Layer struct {
	Name string
	// Duration is the layer's length in seconds; 0 means the rest of the patch.
	Duration float64
	// Delay holds the layer silent for this many seconds, then starts its clock.
	Delay float64

	Wave Wave
	// Freq is the fundamental in Hz. Ignored by WaveNoise.
	Freq float64
	// SlideTo and SlideCurve set the frequency at fraction u to
	// Freq + (SlideTo-Freq)·u^SlideCurve. SlideTo 0 means no slide unless
	// SlideToZero is set.
	SlideTo, SlideCurve float64
	// SlideToZero makes a SlideTo of 0 mean a slide to zero hertz.
	SlideToZero bool
	// SlideS is the glide length in seconds, after which the frequency holds
	// at SlideTo; 0 means the whole layer.
	SlideS float64
	// SlideDelayS holds the pitch at Freq this long before the glide begins.
	SlideDelayS float64
	// SlideRate, when positive, makes the slide an exponential approach,
	// SlideTo + (Freq-SlideTo)·e^(-t·SlideRate). SlideCurve and SlideS are ignored.
	SlideRate float64
	// Duty is the pulse width of square waves in [0,1); 0 is the plain square.
	// It applies to the fundamental and any square partial.
	Duty float64
	// Sweep selects how the moving frequency is turned into samples.
	Sweep SweepMode
	// Level weights the fundamental against the Partials; 0 reads as 1.
	Level float64
	// Partials are extra components at fixed ratios to Freq.
	Partials []Partial
	// NoiseMix in [0,1] crossfades the oscillator toward shared-stream noise.
	NoiseMix float64

	Vibrato *Vibrato

	Env     Env
	Gate    *Gate
	AM      *LFO
	NoiseAM *NoiseAM
	// Filter is the layer's first shaping stage; Filters follow it.
	Filter Filter
	// Filters is a cascade run in declared order. Stages with BeforeEnv run
	// first, then the envelope, then the rest.
	Filters []Filter
	// Gain scales the layer's source ahead of envelope and filter; 0 reads as
	// 1. It is source-time because float multiplication does not associate.
	Gain float64
}

// Patch is a whole designed sound: layers, length and normalization level.
type Patch struct {
	// Duration is the sound's length in seconds.
	Duration float64
	// Layers are summed in declared order, which is observable when more than
	// one draws from the shared noise stream.
	Layers []Layer
	// Peak is the normalization target; <= 0 leaves the sum unnormalized.
	Peak float64
	// FadeOutS ramps the last FadeOutS seconds of the mix to zero ahead of
	// normalization, so the tail does not click. <= 0 is no fade.
	FadeOutS float64
	// Seed seeds the shared noise stream.
	Seed uint64
}

// Render synthesizes the patch deterministically. It returns nil for a
// non-positive rate or duration and panics if Validate fails.
func (p Patch) Render(sampleRate int) []float64 {
	if sampleRate <= 0 || p.Duration <= 0 {
		return nil
	}
	n := int(math.Round(p.Duration * float64(sampleRate)))
	if n <= 0 {
		return nil
	}
	if err := p.Validate(); err != nil {
		panic(err)
	}
	out := make([]float64, n)
	rng := NewRNG(p.Seed)

	// Layers advance together sample by sample because they share rng.
	type layerState struct {
		phase    []float64
		dur      float64
		startIdx int
		endIdx   int
		gain     float64
		raw      []float64
	}
	states := make([]layerState, len(p.Layers))
	for li, l := range p.Layers {
		dur := l.Duration
		if dur <= 0 {
			dur = p.Duration - l.Delay
		}
		start := int(math.Round(l.Delay * float64(sampleRate)))
		end := start + int(math.Round(dur*float64(sampleRate)))
		end = min(end, n)
		gain := l.Gain
		if gain == 0 {
			gain = 1
		}
		states[li] = layerState{
			phase:    make([]float64, 2+len(l.Partials)),
			dur:      dur,
			startIdx: start,
			endIdx:   end,
			gain:     gain,
			raw:      make([]float64, max(0, end-start)),
		}
	}

	for i := range n {
		for li := range p.Layers {
			l := &p.Layers[li]
			st := &states[li]
			if i < st.startIdx || i >= st.endIdx {
				continue
			}
			t := SampleTime(sampleRate, i-st.startIdx)
			if !l.Gate.openAt(t) {
				continue
			}
			st.raw[i-st.startIdx] = l.sampleAt(t, st.dur, sampleRate, st.phase, rng, st.gain)
		}
	}

	for li := range p.Layers {
		l := &p.Layers[li]
		st := &states[li]
		if len(st.raw) == 0 {
			continue
		}
		l.shape(st.raw, st.dur, sampleRate)
		for j, v := range st.raw {
			out[st.startIdx+j] += v
		}
	}

	FadeOut(out, sampleRate, p.FadeOutS)
	if p.Peak > 0 {
		Normalize(out, p.Peak)
	}
	return out
}

// sampleAt evaluates the layer's raw source at time t, advancing the partial phases.
func (l *Layer) sampleAt(t, dur float64, sampleRate int, phase []float64, rng func() float64, gain float64) float64 {
	if l.Wave == WaveNoise {
		return gain * NoiseSample(rng)
	}
	f := l.freqAt(t, dur)
	s := l.evalWave(l.Wave, t, f, &phase[0], &phase[1], sampleRate)
	if l.Level != 0 {
		s *= l.Level
	}
	for pi := range l.Partials {
		pa := &l.Partials[pi]
		pf := pa.Hz
		if pf <= 0 {
			if pa.Ratio <= 0 {
				continue
			}
			pf = f * pa.Ratio
		}
		v := pa.Gain * l.evalWave(pa.Wave, t, pf, &phase[2+pi], nil, sampleRate)
		if pa.Decay > 0 {
			v *= ExpDecay(t, pa.Decay)
		}
		s += v
	}
	if l.NoiseMix > 0 {
		m := clamp01(l.NoiseMix)
		s = (1-m)*s + m*NoiseSample(rng)
	}
	if n := l.NoiseAM; n != nil {
		s *= n.Base + n.Depth*NoiseSample(rng)
	}
	return gain * s
}

// freqAt is the layer's frequency at t: slide, then vibrato.
func (l *Layer) freqAt(t, dur float64) float64 {
	return l.Vibrato.apply(t, l.slideAt(t, dur))
}

// slides reports whether the layer's frequency moves at all.
func (l *Layer) slides() bool {
	return l.SlideTo > 0 || l.SlideToZero
}

// slideAt is the frequency at t before vibrato.
func (l *Layer) slideAt(t, dur float64) float64 {
	if !l.slides() || l.Freq <= 0 {
		return l.Freq
	}
	if l.SlideDelayS > 0 {
		if t < l.SlideDelayS {
			return l.Freq
		}
		t -= l.SlideDelayS
		dur -= l.SlideDelayS
	}
	if l.SlideRate > 0 {
		return l.SlideTo + (l.Freq-l.SlideTo)*ExpDecay(t, l.SlideRate)
	}
	c := l.SlideCurve
	if c <= 0 {
		c = 1
	}
	if l.SlideS > 0 {
		dur = l.SlideS
	}
	return l.Freq + (l.SlideTo-l.Freq)*math.Pow(frac(t, dur), c)
}

// pulse returns the layer's duty when w is a pulsed square.
func (l *Layer) pulse(w Wave) (duty float64, ok bool) {
	if w != WaveSquare || l.Duty <= 0 || l.Duty >= 1 {
		return 0, false
	}
	return l.Duty, true
}

func pulseAt(ph, duty float64) float64 {
	if ph < duty {
		return 1
	}
	return -1
}

// evalWave evaluates one component at frequency f, advancing phase when the
// sweep mode integrates. woodPhase is nil for a partial; validation refuses
// a wooden partial.
func (l *Layer) evalWave(w Wave, t, f float64, phase, woodPhase *float64, sampleRate int) float64 {
	wood := w == WaveWood && woodPhase != nil
	step := func() {
		*phase = advance(*phase, f, sampleRate)
		if wood {
			*woodPhase = advance(*woodPhase, f*BarModeRatio, sampleRate)
		}
	}
	duty, pulsed := l.pulse(w)
	at := func() float64 {
		switch {
		case wood:
			return sinePhase(*phase) + woodPartialGain*sinePhase(*woodPhase)
		case pulsed:
			return pulseAt(*phase, duty)
		}
		return shapeAt(w, *phase)
	}
	switch l.Sweep {
	case SweepPhase:
		v := at()
		step()
		return v
	case SweepPhaseLead:
		step()
		return at()
	}
	switch w {
	case WaveSquare:
		if pulsed {
			return pulseAt(phase01(t, f), duty)
		}
		return Square(t, f)
	case WaveTriangle:
		return Triangle(t, f)
	case WaveSaw:
		return Saw(t, f)
	case WaveWood:
		return Sine(t, f) + woodPartialGain*Sine(t, f*BarModeRatio)
	default:
		return Sine(t, f)
	}
}

// shape applies the BeforeEnv filters, the envelope and tremolo, then the
// remaining filters.
func (l *Layer) shape(raw []float64, dur float64, sampleRate int) {
	chain := l.filterChain()
	for _, f := range chain {
		if f.BeforeEnv {
			f.apply(raw, dur, sampleRate)
		}
	}
	for i := range raw {
		t := SampleTime(sampleRate, i)
		raw[i] *= l.Env.At(t, dur) * l.AM.at(t)
	}
	for _, f := range chain {
		if !f.BeforeEnv {
			f.apply(raw, dur, sampleRate)
		}
	}
}

// filterChain is Filter when set, then Filters.
func (l *Layer) filterChain() []Filter {
	if l.Filter.Kind == FilterNone {
		return l.Filters
	}
	return append([]Filter{l.Filter}, l.Filters...)
}
