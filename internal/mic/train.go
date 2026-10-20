// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/dsp"
)

// TrainOptions describes a labelled sample set: dir/<class>/*.wav are a
// class's positive clips (one utterance or sound each), and dir/_noise/*.wav
// — any underscore-prefixed directory — are negatives. WAVs must be 16-bit
// PCM at Rate or at CaptureRate, which is decimated to Rate through the
// filter the cabinet captures with.
type TrainOptions struct {
	Dir       string
	Name      string   // envelope name, e.g. "commands"
	Classes   []string // class names in cue order; nil = every non-underscore subdirectory, sorted
	Templates int      // max templates per class, default defaultTemplates
	Features  *assetfile.MicFeatures
	Segment   *assetfile.MicSegment
	Generator assetfile.Generator
}

// DefaultFeatures is the front end the cabinet's own models are built with:
// 512-sample frames at Rate (34.8 ms) hopped every 10 ms, 13 coefficients
// over 26 mel bands spanning the band a word or a whistle lives in.
func DefaultFeatures() *assetfile.MicFeatures {
	return &assetfile.MicFeatures{
		SampleRateHz: Rate,
		FrameLen:     512,
		HopLen:       147,
		MelBands:     26,
		Coeffs:       13,
		MinHz:        100,
		MaxHz:        6000,
	}
}

// DefaultSegment is the endpointer a short spoken command wants: at least
// 150 ms of sound, at most 1.2 s, closed by 200 ms of quiet.
func DefaultSegment() *assetfile.MicSegment {
	return &assetfile.MicSegment{MinMs: 150, MaxMs: 1200, HangoverMs: 200, SNR: 4, MinRMS: 0.01}
}

const defaultTemplates = 5

// loneClipDistance stands in for a class's leave-one-out distance when there
// is only one clip; generous enough to accept a second recording.
const loneClipDistance = 0.6

// Report is what the trainer learned about a sample set, printable and
// JSON-serializable for the lab.
type Report struct {
	Classes   []ClassReport `json:"classes"`
	Confusion [][]int       `json:"confusion"` // rows = true class (last row = negatives), cols = predicted (last col = "none")
	Labels    []string      `json:"labels"`    // class names, then "none"
	Clips     int           `json:"clips"`
	Skipped   []string      `json:"skipped,omitempty"`
}

// ClassReport is one class's clips, what was kept, and the margin between the
// positives it accepts and the negatives it nearly accepted.
type ClassReport struct {
	Name       string  `json:"name"`
	Cue        int     `json:"cue"`
	Clips      int     `json:"clips"`
	Templates  int     `json:"templates"`
	Threshold  float64 `json:"threshold"`
	Accepted   int     `json:"accepted"`   // positives under threshold, leave-one-out
	FalseAlarm int     `json:"falseAlarm"` // negatives (other classes and noise) under threshold
	PosMedian  float64 `json:"posMedian"`
	PosMax     float64 `json:"posMax"`
	NegMin     float64 `json:"negMin"` // -1 when the set has no negatives to measure
}

// Train builds a sound model from a labelled sample set, returning the
// envelope and what it learned. The envelope is validated before it is
// returned, so a trainer can never write a file the cabinet refuses.
func Train(o TrainOptions) (*assetfile.Envelope, *Report, error) {
	if o.Features == nil {
		o.Features = DefaultFeatures()
	}
	if o.Segment == nil {
		o.Segment = DefaultSegment()
	}
	if o.Templates <= 0 {
		o.Templates = defaultTemplates
	}
	if o.Templates > assetfile.MaxMicTemplates {
		o.Templates = assetfile.MaxMicTemplates
	}
	feat, err := NewFeaturizer(o.Features, o.Segment)
	if err != nil {
		return nil, nil, err
	}
	set, err := loadSet(o.Dir, o.Classes, feat, false)
	if err != nil {
		return nil, nil, err
	}
	for i, name := range set.classes {
		if len(set.pos[i]) == 0 {
			return nil, nil, fmt.Errorf("mic train: class %q has no usable clip", name)
		}
	}

	classes := make([]assetfile.MicClass, len(set.classes))
	within := make([][][]float64, len(set.classes))
	chosen := make([][]int, len(set.classes))
	var scratch []float64
	for k, name := range set.classes {
		within[k], scratch = pairwise(set.pos[k], scratch)
		chosen[k] = selectTemplates(within[k], o.Templates)
		tmpl := make([][][]float64, len(chosen[k]))
		for i, ci := range chosen[k] {
			tmpl[i] = set.pos[k][ci].frames
		}
		classes[k] = assetfile.MicClass{Name: name, Cue: k, Templates: tmpl}
	}

	all, trueClass := set.flatten()
	scores := make([][]float64, len(all))
	for i := range scores {
		scores[i] = make([]float64, len(set.classes))
	}
	for k := range set.classes {
		for i, c := range all {
			if trueClass[i] == k {
				scores[i][k] = leaveOneOut(within[k], chosen[k], set.posIndex(i))
				continue
			}
			scores[i][k], scratch = nearest(c.frames, classes[k].Templates, scratch)
		}
	}
	for k := range classes {
		classes[k].Threshold = threshold(columnSplit(scores, trueClass, k))
	}

	rep := set.report(classes, scores, trueClass)
	env := &assetfile.Envelope{
		Format:    assetfile.FormatVersion,
		Kind:      assetfile.KindMic,
		Name:      o.Name,
		Generator: o.Generator,
		MicPayload: assetfile.MicPayload{
			Features: o.Features,
			Segment:  o.Segment,
			Classes:  classes,
		},
	}
	if err := assetfile.Validate(env, assetfile.KindMic, 0, 0); err != nil {
		return nil, rep, err
	}
	return env, rep, nil
}

// Evaluate scores a held-out sample set, laid out as Train's, against an
// envelope's own thresholds by the live match path.
func Evaluate(env *assetfile.Envelope, dir string) (*Report, error) {
	m, err := compileModel(env)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(m.classes))
	for i, c := range m.classes {
		names[i] = c.name
	}
	set, err := loadSet(dir, names, m.feat, true)
	if err != nil {
		return nil, err
	}

	classes := make([]assetfile.MicClass, len(m.classes))
	for i, c := range m.classes {
		classes[i] = assetfile.MicClass{Name: c.name, Cue: c.cue, Threshold: c.threshold, Templates: c.templates}
	}
	all, trueClass := set.flatten()
	scores := make([][]float64, len(all))
	for i, c := range all {
		scores[i] = make([]float64, len(classes))
		for k := range classes {
			scores[i][k] = m.distance(c.frames, classes[k].Templates)
		}
	}
	return set.report(classes, scores, trueClass), nil
}

// Segments returns every segment a clip's samples (at Rate) yield under an
// envelope's front end, cepstral-mean-subtracted as the matcher sees them.
func Segments(env *assetfile.Envelope, samples []float64) [][][]float64 {
	feat, err := NewFeaturizer(env.Features, env.Segment)
	if err != nil {
		return nil
	}
	var out [][][]float64
	feat.Write(samples, false, func(frames [][]float64) {
		out = append(out, copyFrames(frames))
	})
	return out
}

type clip struct {
	path   string
	frames [][]float64
}

type sampleSet struct {
	classes []string
	pos     [][]clip
	neg     []clip
	skipped []string
	clips   int
}

// loadSet reads dir's labelled clips into segments. extraNegatives folds a
// non-underscore directory that is not one of classes into the negatives.
func loadSet(dir string, classes []string, feat *Featurizer, extraNegatives bool) (*sampleSet, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var subdirs []string
	for _, e := range entries {
		if e.IsDir() {
			subdirs = append(subdirs, e.Name())
		}
	}
	slices.Sort(subdirs)

	if classes == nil {
		for _, name := range subdirs {
			if !strings.HasPrefix(name, "_") {
				classes = append(classes, name)
			}
		}
		if len(classes) == 0 {
			return nil, fmt.Errorf("mic train: %s has no class directory", dir)
		}
	}
	set := &sampleSet{classes: classes, pos: make([][]clip, len(classes))}

	for k, name := range classes {
		set.pos[k], err = set.loadDir(filepath.Join(dir, name), feat)
		if err != nil {
			return nil, err
		}
	}
	for _, name := range subdirs {
		if !strings.HasPrefix(name, "_") && (!extraNegatives || slices.Contains(classes, name)) {
			continue
		}
		if !strings.HasPrefix(name, "_") {
			set.skipped = append(set.skipped, fmt.Sprintf("%s: not a class in the model, scored as negatives", name))
		}
		clips, err := set.loadDir(filepath.Join(dir, name), feat)
		if err != nil {
			return nil, err
		}
		set.neg = append(set.neg, clips...)
	}
	return set, nil
}

func (s *sampleSet) loadDir(dir string, feat *Featurizer) ([]clip, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".wav") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)

	var out []clip
	for _, name := range names {
		path := filepath.Join(dir, name)
		samples, err := ReadClipFile(path)
		if err != nil {
			return nil, err
		}
		s.clips++
		frames := longestSegment(feat, samples)
		if len(frames) == 0 {
			s.skipped = append(s.skipped, path+": no segment above the gate")
			continue
		}
		if len(frames) > assetfile.MaxMicTemplateFrames {
			s.skipped = append(s.skipped, fmt.Sprintf("%s: segment of %d frames truncated to %d", path, len(frames), assetfile.MaxMicTemplateFrames))
			frames = frames[:assetfile.MaxMicTemplateFrames]
		}
		out = append(out, clip{path: path, frames: frames})
	}
	return out, nil
}

// flatten lays the set out as one clip list — every class in order, then the
// negatives — with each clip's true class, or -1.
func (s *sampleSet) flatten() ([]clip, []int) {
	var all []clip
	var labels []int
	for k := range s.classes {
		for _, c := range s.pos[k] {
			all = append(all, c)
			labels = append(labels, k)
		}
	}
	for _, c := range s.neg {
		all = append(all, c)
		labels = append(labels, -1)
	}
	return all, labels
}

// posIndex maps a flattened index back to the clip's position within its own
// class, which is what the within-class distance matrix is indexed by.
func (s *sampleSet) posIndex(flat int) int {
	for k := range s.classes {
		if flat < len(s.pos[k]) {
			return flat
		}
		flat -= len(s.pos[k])
	}
	return -1
}

func (s *sampleSet) report(classes []assetfile.MicClass, scores [][]float64, trueClass []int) *Report {
	rep := &Report{Clips: s.clips, Skipped: s.skipped}
	for k, c := range classes {
		cr := ClassReport{
			Name: c.Name, Cue: c.Cue, Clips: len(s.pos[k]),
			Templates: len(c.Templates), Threshold: c.Threshold,
			NegMin: math.Inf(1),
		}
		var pos []float64
		for i := range scores {
			d := scores[i][k]
			if trueClass[i] == k {
				pos = append(pos, d)
				if d < c.Threshold {
					cr.Accepted++
				}
				continue
			}
			cr.NegMin = math.Min(cr.NegMin, d)
			if d < c.Threshold {
				cr.FalseAlarm++
			}
		}
		slices.Sort(pos)
		if len(pos) > 0 {
			cr.PosMedian, cr.PosMax = percentile(pos, 50), percentile(pos, 100)
		}
		if math.IsInf(cr.NegMin, 1) {
			cr.NegMin = -1
		}
		rep.Classes = append(rep.Classes, cr)
	}

	n := len(classes)
	rep.Labels = make([]string, 0, n+1)
	for _, c := range classes {
		rep.Labels = append(rep.Labels, c.Name)
	}
	rep.Labels = append(rep.Labels, "none")
	rep.Confusion = make([][]int, n+1)
	for i := range rep.Confusion {
		rep.Confusion[i] = make([]int, n+1)
	}
	for i := range scores {
		row := trueClass[i]
		if row < 0 {
			row = n
		}
		col := classify(scores[i], classes)
		if col < 0 {
			col = n
		}
		rep.Confusion[row][col]++
	}
	return rep
}

// classify is model.match's rule over precomputed distances: the lowest score
// under its own class's threshold, or -1.
func classify(scores []float64, classes []assetfile.MicClass) int {
	best, bestScore := -1, math.Inf(1)
	for k, c := range classes {
		if scores[k] < c.Threshold && scores[k] < bestScore {
			best, bestScore = k, scores[k]
		}
	}
	return best
}

// ReadClipFile reads a lab clip as analysis samples: a WAV at Rate as it
// stands, one at CaptureRate decimated through the filter the cabinet
// captures with, and any other rate refused by name.
func ReadClipFile(path string) ([]float64, error) {
	samples, rate, err := ReadWAVFile(path)
	if err != nil {
		return nil, err
	}
	switch rate {
	case Rate:
		return samples, nil
	case CaptureRate:
		d := dsp.NewDecimator(Decimation, decimatorTaps)
		return d.Process(samples, make([]float64, 0, len(samples)/Decimation+1)), nil
	}
	return nil, fmt.Errorf("%s: %d Hz, want %d (the analysis rate) or %d (the capture rate)", path, rate, Rate, CaptureRate)
}

// longestSegment returns the longest segment the featurizer finds in a clip,
// after normalising its peak so recording loudness does not decide whether
// the gate opens.
func longestSegment(feat *Featurizer, samples []float64) [][]float64 {
	feat.Reset()
	peak := 0.0
	for _, s := range samples {
		peak = math.Max(peak, math.Abs(s))
	}
	// Silence either side lets the gate close on a tightly cut clip.
	pad := Rate / 2
	scaled := make([]float64, len(samples)+2*pad)
	if peak > 0 {
		for i, s := range samples {
			scaled[pad+i] = s * 0.5 / peak
		}
	}
	samples = scaled
	var best [][]float64
	feat.Write(samples, false, func(frames [][]float64) {
		if len(frames) > len(best) {
			best = copyFrames(frames)
		}
	})
	return best
}

func copyFrames(frames [][]float64) [][]float64 {
	out := make([][]float64, len(frames))
	for i, f := range frames {
		out[i] = slices.Clone(f)
	}
	return out
}

func dtwDistance(a, b [][]float64, scratch []float64) (float64, []float64) {
	band := int(dtwBand*float64(max(len(a), len(b)))) + 1
	return dsp.DTW(a, b, band, scratch)
}

func nearest(frames [][]float64, templates [][][]float64, scratch []float64) (float64, []float64) {
	best := math.Inf(1)
	for _, t := range templates {
		var d float64
		d, scratch = dtwDistance(frames, t, scratch)
		best = math.Min(best, d)
	}
	return best, scratch
}

func pairwise(clips []clip, scratch []float64) ([][]float64, []float64) {
	d := make([][]float64, len(clips))
	for i := range d {
		d[i] = make([]float64, len(clips))
	}
	for i := range clips {
		for j := i + 1; j < len(clips); j++ {
			var v float64
			v, scratch = dtwDistance(clips[i].frames, clips[j].frames, scratch)
			d[i][j], d[j][i] = v, v
		}
	}
	return d, scratch
}

// selectTemplates picks up to n clips: the one with the smallest total
// distance to its classmates, then repeatedly the clip farthest from those
// already picked.
func selectTemplates(d [][]float64, n int) []int {
	if len(d) == 0 {
		return nil
	}
	best, bestSum := 0, math.Inf(1)
	for i := range d {
		sum := 0.0
		for j := range d[i] {
			sum += d[i][j]
		}
		if sum < bestSum {
			best, bestSum = i, sum
		}
	}
	chosen := []int{best}
	for len(chosen) < min(n, len(d)) {
		far, farDist := -1, -1.0
		for i := range d {
			if slices.Contains(chosen, i) {
				continue
			}
			near := math.Inf(1)
			for _, c := range chosen {
				near = math.Min(near, d[i][c])
			}
			if near > farDist {
				far, farDist = i, near
			}
		}
		if far < 0 {
			break
		}
		chosen = append(chosen, far)
	}
	slices.Sort(chosen)
	return chosen
}

// leaveOneOut is clip i's distance to the nearest template that is not the
// clip itself, falling back to the nearest other clip, then to
// loneClipDistance.
func leaveOneOut(d [][]float64, chosen []int, i int) float64 {
	best := math.Inf(1)
	for _, t := range chosen {
		if t != i {
			best = math.Min(best, d[i][t])
		}
	}
	if !math.IsInf(best, 1) {
		return best
	}
	for j := range d {
		if j != i {
			best = math.Min(best, d[i][j])
		}
	}
	if math.IsInf(best, 1) {
		return loneClipDistance
	}
	return best
}

// columnSplit is class k's distance column split into the clips that belong
// to it and everything else.
func columnSplit(scores [][]float64, trueClass []int, k int) (pos, neg []float64) {
	for i := range scores {
		if trueClass[i] == k {
			pos = append(pos, scores[i][k])
		} else {
			neg = append(neg, scores[i][k])
		}
	}
	slices.Sort(pos)
	slices.Sort(neg)
	return pos, neg
}

// threshold sits halfway between the positives' 90th percentile and the
// negatives' 10th, clamped to 0.6..1.5 of that 90th percentile and then held
// below the nearest negative whenever the negatives are not already well
// inside the positives (negMin above 0.8 of the 90th percentile).
func threshold(pos, neg []float64) float64 {
	p90 := percentile(pos, 90)
	if !(p90 > 0) {
		p90 = loneClipDistance
	}
	n10 := percentile(neg, 10)
	if math.IsInf(n10, 1) {
		n10 = 2 * p90
	}
	t := (p90 + n10) / 2
	t = math.Max(t, 0.6*p90)
	t = math.Min(t, 1.5*p90)
	if negMin := percentile(neg, 0); negMin > 0.8*p90 {
		t = math.Min(t, 0.999*negMin)
	}
	return math.Max(t, 1e-6)
}

// percentile is the nearest-rank percentile of an already sorted slice, +Inf
// when there is nothing to rank.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.Inf(1)
	}
	i := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	return sorted[min(max(i, 0), len(sorted)-1)]
}
