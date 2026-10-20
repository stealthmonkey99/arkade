// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"errors"
	"fmt"
	"math"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/dsp"
)

// A sound model is a cartridge's own vocabulary: a few words or sounds, each
// a handful of recorded templates, matched by DTW over MFCCs. It is a pure
// function of the samples, so a clip classifies identically offline and live.

// Hit is a classification result: the class that fired, or Cue -1.
type Hit struct {
	Cue   int
	Name  string
	Score float64
}

var noHit = Hit{Cue: -1}

// Featurizer turns a stream of samples at Rate into cepstral-mean-subtracted
// MFCC segments: an energy gate opens a segment and a hangover closes it.
// The live classifier and the trainer share it.
type Featurizer struct {
	mfcc    *dsp.MFCC
	frame   int
	hop     int
	coeffs  int
	minRMS  float64
	snr     float64
	minLen  int // frames
	maxLen  int
	hang    int
	preRoll int

	window []float64 // last frame samples, circular
	head   int
	filled int
	sinceH int

	floor  float64
	active bool
	silent int

	feats   [][]float64 // segment frames plus pre-roll
	recent  [][]float64 // pre-roll ring
	nRec    int
	echoed  int
	frames  int
	scratch []float64
}

// segmentPreRoll is how many hops before the gate opens are kept, so an
// onset is not clipped.
const segmentPreRoll = 3

// NewFeaturizer builds a featurizer from a model's feature and segment
// parameters. The model must have been rendered at Rate.
func NewFeaturizer(f *assetfile.MicFeatures, s *assetfile.MicSegment) (*Featurizer, error) {
	if f == nil || s == nil {
		return nil, errors.New("mic model: features and segment are required")
	}
	if f.SampleRateHz != Rate {
		return nil, fmt.Errorf("mic model: rendered at %d Hz, the cabinet analyses at %d", f.SampleRateHz, Rate)
	}
	hopMs := float64(f.HopLen) * 1000 / Rate
	z := &Featurizer{
		mfcc:    dsp.NewMFCC(Rate, f.FrameLen, f.MelBands, f.Coeffs, f.MinHz, f.MaxHz),
		frame:   f.FrameLen,
		hop:     f.HopLen,
		coeffs:  f.Coeffs,
		minRMS:  s.MinRMS,
		snr:     s.SNR,
		minLen:  max(int(math.Round(float64(s.MinMs)/hopMs)), 2),
		maxLen:  max(int(math.Round(float64(s.MaxMs)/hopMs)), 3),
		hang:    max(int(math.Round(float64(s.HangoverMs)/hopMs)), 1),
		preRoll: segmentPreRoll,
		window:  make([]float64, f.FrameLen),
		floor:   silentFloor,
		scratch: make([]float64, f.FrameLen),
	}
	cap := z.maxLen + z.preRoll + z.hang + 1
	z.feats = make([][]float64, 0, cap)
	for i := 0; i < cap; i++ {
		z.feats = append(z.feats, make([]float64, f.Coeffs))
	}
	z.feats = z.feats[:0]
	z.recent = make([][]float64, z.preRoll)
	for i := range z.recent {
		z.recent[i] = make([]float64, f.Coeffs)
	}
	return z, nil
}

// Reset clears the stream state, for a fresh recording.
func (z *Featurizer) Reset() {
	for i := range z.window {
		z.window[i] = 0
	}
	z.head, z.filled, z.sinceH = 0, 0, 0
	z.floor, z.active, z.silent = silentFloor, false, 0
	z.feats = z.feats[:0]
	z.nRec, z.echoed, z.frames = 0, 0, 0
}

// Active reports whether a segment is open.
func (z *Featurizer) Active() bool { return z.active }

// Write feeds samples, calling segment with each completed segment's
// frames. echo marks the samples as the cabinet's own playback; a segment
// mostly made of such frames is dropped rather than matched. The frames
// slice is only valid during the callback.
func (z *Featurizer) Write(samples []float64, echo bool, segment func(frames [][]float64)) {
	for _, s := range samples {
		z.window[z.head] = s
		z.head++
		if z.head == z.frame {
			z.head = 0
		}
		if z.filled < z.frame {
			z.filled++
		}
		z.sinceH++
		if z.sinceH < z.hop || z.filled < z.frame {
			continue
		}
		z.sinceH = 0
		z.step(echo, segment)
	}
}

func (z *Featurizer) step(echo bool, segment func([][]float64)) {
	n := copy(z.scratch, z.window[z.head:])
	copy(z.scratch[n:], z.window[:z.head])

	sum := 0.0
	for _, x := range z.scratch {
		sum += x * x
	}
	rms := math.Sqrt(sum / float64(z.frame))
	loud := rms > z.minRMS && rms > z.floor*z.snr
	if rms < z.floor {
		z.floor += (rms - z.floor) * 0.25
	} else {
		z.floor += (rms - z.floor) * 0.002
	}

	var dst []float64
	if z.active {
		z.feats = z.feats[:len(z.feats)+1]
		dst = z.feats[len(z.feats)-1]
	} else {
		dst = z.recent[z.nRec%z.preRoll]
		z.nRec++
	}
	z.mfcc.Frame(z.scratch, dst)

	if !z.active {
		if loud && !echo {
			z.open()
		}
		return
	}
	z.frames++
	if echo {
		z.echoed++
	}
	if loud {
		z.silent = 0
	} else {
		z.silent++
	}
	if z.silent >= z.hang || len(z.feats) == cap(z.feats) {
		z.close(segment)
	}
}

// open starts a segment seeded with the pre-roll frames, oldest first.
func (z *Featurizer) open() {
	z.active, z.silent, z.frames, z.echoed = true, 0, 0, 0
	z.feats = z.feats[:0]
	have := min(z.nRec, z.preRoll)
	for i := 0; i < have; i++ {
		src := z.recent[(z.nRec-have+i)%z.preRoll]
		z.feats = z.feats[:len(z.feats)+1]
		copy(z.feats[len(z.feats)-1], src)
	}
}

func (z *Featurizer) close(segment func([][]float64)) {
	z.active = false
	frames := z.feats
	if trim := min(z.silent, len(frames)-1); trim > 0 {
		frames = frames[:len(frames)-trim]
	}
	if z.frames-z.silent < z.minLen || z.echoed*2 > z.frames {
		return
	}
	cepstralMeanSubtract(frames)
	segment(frames)
}

// cepstralMeanSubtract removes each coefficient's mean over the segment in
// place; microphone colour and room gain are additive in the cepstrum.
func cepstralMeanSubtract(frames [][]float64) {
	if len(frames) == 0 {
		return
	}
	n := len(frames[0])
	for c := 0; c < n; c++ {
		mean := 0.0
		for _, f := range frames {
			mean += f[c]
		}
		mean /= float64(len(frames))
		for _, f := range frames {
			f[c] -= mean
		}
	}
}

// dtwBand is the Sakoe-Chiba half-width as a fraction of the longer sequence.
const dtwBand = 0.2

type class struct {
	name      string
	cue       int
	threshold float64
	templates [][][]float64
}

// model is one compiled envelope: its featurizer and its classes.
type model struct {
	feat    *Featurizer
	classes []class
	scratch []float64
}

func compileModel(env *assetfile.Envelope) (*model, error) {
	feat, err := NewFeaturizer(env.Features, env.Segment)
	if err != nil {
		return nil, err
	}
	m := &model{feat: feat}
	for _, c := range env.Classes {
		m.classes = append(m.classes, class{
			name: c.Name, cue: c.Cue, threshold: c.Threshold, templates: c.Templates,
		})
	}
	if len(m.classes) == 0 {
		return nil, fmt.Errorf("mic model %q: no classes", env.Name)
	}
	return m, nil
}

// match scores a segment against every class, returning the best class under
// its threshold, or noHit.
func (m *model) match(frames [][]float64) Hit {
	best := noHit
	best.Score = math.Inf(1)
	for _, c := range m.classes {
		score := m.distance(frames, c.templates)
		if score < c.threshold && score < best.Score {
			best = Hit{Cue: c.cue, Name: c.name, Score: score}
		}
	}
	return best
}

// distance is the segment's DTW distance to the nearest template.
func (m *model) distance(frames [][]float64, templates [][][]float64) float64 {
	best := math.Inf(1)
	for _, t := range templates {
		band := int(dtwBand*float64(max(len(frames), len(t)))) + 1
		var d float64
		d, m.scratch = dsp.DTW(frames, t, band, m.scratch)
		if d < best {
			best = d
		}
	}
	return best
}

// Classifier runs every installed sound model over the cancelled capture.
type Classifier struct {
	models []*model
	hit    Hit
}

// NewClassifier compiles envelopes into a classifier. Envelopes that fail
// to compile are skipped; the count of models kept and the first error are
// returned, and a classifier with no models is nil.
func NewClassifier(envs []*assetfile.Envelope) (*Classifier, int, error) {
	c := &Classifier{}
	var first error
	for _, env := range envs {
		m, err := compileModel(env)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		c.models = append(c.models, m)
	}
	if len(c.models) == 0 {
		return nil, 0, first
	}
	return c, len(c.models), first
}

// Write feeds samples (at Rate) and reports the first class that fired on
// them, or a Hit with Cue -1.
func (c *Classifier) Write(samples []float64, echo bool) Hit {
	c.hit = noHit
	for _, m := range c.models {
		m.feat.Write(samples, echo, func(frames [][]float64) {
			if h := m.match(frames); h.Cue >= 0 && c.hit.Cue < 0 {
				c.hit = h
			}
		})
	}
	return c.hit
}

// Active reports whether any model currently has a segment open.
func (c *Classifier) Active() bool {
	for _, m := range c.models {
		if m.feat.Active() {
			return true
		}
	}
	return false
}
