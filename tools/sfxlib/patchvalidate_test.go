// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"strings"
	"testing"
)

// TestValidateDutyRangeIncludesZero pins the message to what is accepted:
// 0 is the plain square, 1 is refused.
func TestValidateDutyRangeIncludesZero(t *testing.T) {
	if err := (Patch{Duration: 0.05, Layers: []Layer{{Wave: WaveSquare, Duty: 0}}}).Validate(); err != nil {
		t.Errorf("duty 0 refused: %v", err)
	}
	err := (Patch{Duration: 0.05, Layers: []Layer{{Wave: WaveSquare, Duty: 1}}}).Validate()
	if err == nil || !strings.Contains(err.Error(), "[0,1)") {
		t.Errorf("duty 1: err = %v, want it refused as outside [0,1)", err)
	}
}

func TestValidateRefusesWhatTheDocumentRefuses(t *testing.T) {
	cases := map[string]Layer{
		"negative slide":       {SlideTo: -5},
		"negative slideS":      {SlideS: -1},
		"negative slide delay": {SlideDelayS: -1},
		"rate beside curve":    {SlideRate: 4, SlideCurve: 2},
		"duty of one":          {Duty: 1},
		"start level, no rise": {Env: Env{Kind: EnvADSR, StartLevel: 0.5}},
		"start level on const": {Env: Env{Kind: EnvConst, StartLevel: 0.5, AttackS: 0.1}},
		"wood partial":         {Partials: []Partial{{Ratio: 2, Wave: WaveWood}}},
		"vibrato both widths":  {Vibrato: &Vibrato{Hz: 5, Depth: 0.02, DepthHz: 3}},
		"square vibrato":       {Vibrato: &Vibrato{Hz: 5, Depth: 0.02, Wave: WaveSquare}},
		"beforeEnv after post": {Filters: []Filter{{Kind: FilterLowpass, Hz: 800}, {Kind: FilterLowpass, Hz: 400, BeforeEnv: true}}},
	}
	for name, l := range cases {
		t.Run(name, func(t *testing.T) {
			p := Patch{Duration: 0.05, Layers: []Layer{l}}
			if err := p.Validate(); err == nil {
				t.Fatal("Validate accepted it")
			}
			defer func() {
				if recover() == nil {
					t.Error("Render rendered a patch Validate refuses")
				}
			}()
			p.Render(SampleRate44100)
		})
	}
}
