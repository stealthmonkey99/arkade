// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func micEnv() *Envelope {
	return &Envelope{
		Format: FormatVersion, Kind: KindMic, Name: "commands",
		Generator: Generator{Tool: "micmodel", Version: "1.0.0"},
		MicPayload: MicPayload{
			Features: &MicFeatures{
				SampleRateHz: 14700, FrameLen: 512, HopLen: 147,
				MelBands: 26, Coeffs: 3, MinHz: 100, MaxHz: 6000,
			},
			Segment: &MicSegment{MinMs: 150, MaxMs: 1200, HangoverMs: 200, SNR: 4, MinRMS: 0.02},
			Classes: []MicClass{
				{Name: "come", Cue: 0, Threshold: 0.42, Templates: [][][]float64{
					{{1, 2, 3}, {4, 5, 6}},
					{{1.5, 2.5, 3.5}, {4.5, 5.5, 6.5}},
				}},
				{Name: "sit", Cue: 1, Threshold: 0.5, Templates: [][][]float64{
					{{-1, 0, 1}, {-2, 0, 2}},
				}},
			},
		},
	}
}

func TestMicRoundTrip(t *testing.T) {
	const name = "commands.arkademic.json"
	if kind, ok := KindFromFilename(name); !ok || kind != KindMic {
		t.Fatalf("KindFromFilename(%q) = (%q, %v), want (mic, true)", name, kind, ok)
	}
	want := micEnv()
	data, err := WriteJSON(want)
	if err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	got, err := ReadBytes(name, data)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if !reflect.DeepEqual(got.Features, want.Features) {
		t.Errorf("features = %+v, want %+v", got.Features, want.Features)
	}
	if !reflect.DeepEqual(got.Segment, want.Segment) {
		t.Errorf("segment = %+v, want %+v", got.Segment, want.Segment)
	}
	if !reflect.DeepEqual(got.Classes, want.Classes) {
		t.Errorf("classes = %+v, want %+v", got.Classes, want.Classes)
	}
	for _, absent := range []string{`"channels"`, `"frames"`, `"cels"`, `"render"`} {
		if strings.Contains(string(data), absent) {
			t.Errorf("the encoded model carries %s, which belongs to another kind", absent)
		}
	}
}

func TestValidateMicRefusesStructuralProblems(t *testing.T) {
	cases := []struct {
		name    string
		breakIt func(*Envelope)
		want    string
	}{
		{"no features", func(e *Envelope) { e.Features = nil }, "no features block"},
		{"sample rate is not the analysis rate", func(e *Envelope) { e.Features.SampleRateHz = 16000 }, "want 14700"},
		{"frame length not a power of two", func(e *Envelope) { e.Features.FrameLen = 500 }, "power of two"},
		{"frame length past the cap", func(e *Envelope) { e.Features.FrameLen = 4096 }, "power of two"},
		{"hop of zero", func(e *Envelope) { e.Features.HopLen = 0 }, "hopLen 0"},
		{"hop past the frame", func(e *Envelope) { e.Features.HopLen = 600 }, "hopLen 600"},
		{"no mel bands", func(e *Envelope) { e.Features.MelBands = 0 }, "melBands 0"},
		{"more bands than the cap", func(e *Envelope) { e.Features.MelBands = MaxMicBands + 1 }, "melBands 65"},
		{"more coefficients than bands", func(e *Envelope) { e.Features.MelBands, e.Features.Coeffs = 3, 4 }, "the mel bands they come from"},
		{"more coefficients than the cap", func(e *Envelope) {
			e.Features.Coeffs = MaxMicCoeffs + 1
			e.Features.MelBands = MaxMicCoeffs + 1
		}, "over the 20 cap"},
		{"band inverted", func(e *Envelope) { e.Features.MinHz, e.Features.MaxHz = 6000, 100 }, "want 0 <= minHz < maxHz"},
		{"band past nyquist", func(e *Envelope) { e.Features.MaxHz = 9000 }, "half the sample rate"},

		{"no segment", func(e *Envelope) { e.Segment = nil }, "no segment block"},
		{"segment of no length", func(e *Envelope) { e.Segment.MinMs = 0 }, "minMs 0"},
		{"segment ceiling below its floor", func(e *Envelope) { e.Segment.MaxMs = 100 }, "want longer than minMs"},
		{"negative hangover", func(e *Envelope) { e.Segment.HangoverMs = -1 }, "hangoverMs -1"},
		{"snr below the room", func(e *Envelope) { e.Segment.SNR = 0.5 }, "want 1 or more"},
		{"rms past full scale", func(e *Envelope) { e.Segment.MinRMS = 1.5 }, "want within 0..1"},

		{"no classes", func(e *Envelope) { e.Classes = nil }, "no classes"},
		{"more classes than cues", func(e *Envelope) {
			e.Classes = make([]MicClass, MaxMicClasses+1)
			for i := range e.Classes {
				e.Classes[i] = MicClass{Name: "c" + string(rune('a'+i)), Cue: i, Threshold: 1,
					Templates: [][][]float64{{{1, 2, 3}, {4, 5, 6}}}}
			}
		}, "over the 8 cap"},
		{"class with no name", func(e *Envelope) { e.Classes[0].Name = "" }, "no name"},
		{"class name outside the grammar", func(e *Envelope) { e.Classes[0].Name = "Come Here" }, "lowercase words"},
		{"duplicate class name", func(e *Envelope) { e.Classes[1].Name = e.Classes[0].Name }, "used twice"},
		{"cue past the eight keys", func(e *Envelope) { e.Classes[0].Cue = MaxMicClasses }, "want within 0..7"},
		{"negative cue", func(e *Envelope) { e.Classes[0].Cue = -1 }, "want within 0..7"},
		{"two classes on one cue", func(e *Envelope) { e.Classes[1].Cue = e.Classes[0].Cue }, "both fire cue 0"},
		{"threshold of zero", func(e *Envelope) { e.Classes[0].Threshold = 0 }, "positive match distance"},
		{"class with no templates", func(e *Envelope) { e.Classes[0].Templates = nil }, "no templates"},
		{"more templates than the cap", func(e *Envelope) {
			e.Classes[0].Templates = make([][][]float64, MaxMicTemplates+1)
			for i := range e.Classes[0].Templates {
				e.Classes[0].Templates[i] = [][]float64{{1, 2, 3}, {4, 5, 6}}
			}
		}, "over the 16 cap"},
		{"template of one frame", func(e *Envelope) { e.Classes[0].Templates[0] = [][]float64{{1, 2, 3}} }, "want within 2..400"},
		{"template past the frame cap", func(e *Envelope) {
			frames := make([][]float64, MaxMicTemplateFrames+1)
			for i := range frames {
				frames[i] = []float64{1, 2, 3}
			}
			e.Classes[0].Templates[0] = frames
		}, "want within 2..400"},
		{"frame of the wrong width", func(e *Envelope) { e.Classes[0].Templates[0][1] = []float64{1, 2} }, "want the 3 features declares"},
		{"a coefficient that is not a number", func(e *Envelope) { e.Classes[0].Templates[0][1][2] = math.Inf(1) }, "not a finite number"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := micEnv()
			tc.breakIt(env)
			err := Validate(env, KindMic, 0, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestMicKindMustMatchTheFilename(t *testing.T) {
	env := micEnv()
	data, err := WriteJSON(env)
	if err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if _, err := ReadBytes("commands.arkadehid.json", data); err == nil {
		t.Fatal("a mic envelope named .arkadehid.json was accepted")
	}
	if _, err := WriteBytes("commands.arkadehid.json", env, nil); err == nil {
		t.Fatal("a mic envelope was written under a hid filename")
	}
}
