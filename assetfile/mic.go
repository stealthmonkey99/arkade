// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

// Mic caps. A model is untrusted input, so every count a host acts on per
// captured sound is bounded here and both hosts refuse the same file.
const (
	// MicRateHz is the one analysis rate a model may use: the cabinet
	// decimates its 44.1 kHz capture by three.
	MicRateHz = 14700
	// MaxMicClasses caps a model's class list; a class's cue indexes the
	// eight mic.cue keys.
	MaxMicClasses = 8
	// MaxMicTemplates caps one class's template list.
	MaxMicTemplates = 16
	// MaxMicTemplateFrames caps one template's frame count, and with it the
	// cost of one match; 400 frames at a 10 ms hop is four seconds.
	MaxMicTemplateFrames = 400
	// MaxMicCoeffs caps the cepstral coefficients per frame.
	MaxMicCoeffs = 20
	// MaxMicBands caps the mel filter bank, which bounds the coefficients.
	MaxMicBands = 64
)

// MicFeatures is the front end a host must run to compare a captured sound
// against this model's templates. A template only matches under the front end
// that produced it, so it travels in the file.
type MicFeatures struct {
	SampleRateHz int     `json:"sampleRateHz"` // must be MicRateHz; see validateMic
	FrameLen     int     `json:"frameLen"`
	HopLen       int     `json:"hopLen"`
	MelBands     int     `json:"melBands"`
	Coeffs       int     `json:"coeffs"`
	MinHz        float64 `json:"minHz"`
	MaxHz        float64 `json:"maxHz"`
}

// MicSegment is the endpointer: candidate length bounds, hangover after the
// sound ends, and the level above the room that makes a candidate.
type MicSegment struct {
	MinMs      int     `json:"minMs"`
	MaxMs      int     `json:"maxMs"`
	HangoverMs int     `json:"hangoverMs"`
	SNR        float64 `json:"snr"`
	MinRMS     float64 `json:"minRms"`
}

// MicClass is one recognisable sound: its cue key, the match distance it
// accepts, and its templates, indexed [template][frame][coefficient] with
// each frame Coeffs long.
type MicClass struct {
	Name      string        `json:"name"`
	Cue       int           `json:"cue"`
	Threshold float64       `json:"threshold"`
	Templates [][][]float64 `json:"templates"`
}
