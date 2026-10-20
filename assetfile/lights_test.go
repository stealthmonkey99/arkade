// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"strings"
	"testing"
)

func TestReadBytesNormalizes(t *testing.T) {
	raw := `{"arkade":1,"kind":"lights","name":"t","generator":{"tool":"t","version":"1"},` +
		`"frames":[{"lamps":{"p1":1},"fadeMs":-5}],"loop":"sideways"}`
	env, err := ReadBytes("t.arkadelights.json", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if f := env.Frames[0]; f.HoldMs != defaultHoldMs || f.FadeMs != 0 || env.Loop != LoopCycle {
		t.Errorf("read frame %+v loop %q, want hold %d, fade 0, loop %q", f, env.Loop, defaultHoldMs, LoopCycle)
	}
}

func lightsEnvelope(frames ...LightFrame) *Envelope {
	return &Envelope{
		Format:    FormatVersion,
		Kind:      KindLights,
		Name:      "t",
		Generator: Generator{Tool: "t", Version: "1"},
		LightsPayload: LightsPayload{
			Frames: frames,
			Loop:   LoopCycle,
		},
	}
}

func TestValidateLightsRefusesStructuralProblems(t *testing.T) {
	tests := []struct {
		name string
		env  *Envelope
		want string
	}{
		{
			name: "no frames",
			env:  lightsEnvelope(),
			want: "no frames",
		},
		{
			name: "a mask past the six lamps",
			env:  lightsEnvelope(LightFrame{Lamps: NewLightLamps(LightMaskPtr(MaxLightMask+1), nil), HoldMs: 100}),
			want: "want within 0..63",
		},
		{
			name: "a negative mask",
			env:  lightsEnvelope(LightFrame{Lamps: NewLightLamps(nil, LightMaskPtr(-1)), HoldMs: 100}),
			want: "want within 0..63",
		},
		{
			name: "a frame naming neither seat",
			env:  lightsEnvelope(LightFrame{HoldMs: 100}),
			want: "names neither seat",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.env, KindLights, 0, 0)
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestValidateLightsRefusesTooManyFrames(t *testing.T) {
	frames := make([]LightFrame, MaxLightFrames+1)
	for i := range frames {
		frames[i] = LightFrame{Lamps: NewLightLamps(LightMaskPtr(0), nil), HoldMs: 100}
	}
	err := Validate(lightsEnvelope(frames...), KindLights, 0, 0)
	if err == nil || !strings.Contains(err.Error(), "over the 256 cap") {
		t.Fatalf("Validate() = %v, want a frame-cap error", err)
	}
}

func TestNormalizeLightsRepairsPresentation(t *testing.T) {
	env := lightsEnvelope(
		LightFrame{Lamps: NewLightLamps(LightMaskPtr(1), nil)},                        // no holdMs at all
		LightFrame{Lamps: NewLightLamps(LightMaskPtr(0), nil), HoldMs: 3},             // below the floor
		LightFrame{Lamps: NewLightLamps(LightMaskPtr(0), nil), HoldMs: maxHoldMs * 2}, // over the ceiling
	)
	env.Loop = "sideways"
	validateThenNormalize(t, env, KindLights, 0, 0)
	want := []int{defaultHoldMs, defaultHoldMs, maxHoldMs}
	for i, w := range want {
		if got := env.Frames[i].HoldMs; got != w {
			t.Errorf("frame %d holdMs = %d, want %d", i, got, w)
		}
	}
	if env.Loop != LoopCycle {
		t.Errorf("loop = %q, want %q", env.Loop, LoopCycle)
	}
}

func TestNormalizeLightsRepairsFade(t *testing.T) {
	env := lightsEnvelope(
		LightFrame{Lamps: NewLightLamps(LightMaskPtr(1), nil), HoldMs: 100},
		LightFrame{Lamps: NewLightLamps(LightMaskPtr(0), nil), HoldMs: 100, FadeMs: -5},
		LightFrame{Lamps: NewLightLamps(LightMaskPtr(0), nil), HoldMs: 100, FadeMs: MaxLightFadeMs * 2},
		LightFrame{Lamps: NewLightLamps(LightMaskPtr(0), nil), HoldMs: 100, FadeMs: 600},
	)
	validateThenNormalize(t, env, KindLights, 0, 0)
	want := []int{0, 0, MaxLightFadeMs, 600}
	for i, w := range want {
		if got := env.Frames[i].FadeMs; got != w {
			t.Errorf("frame %d fadeMs = %d, want %d", i, got, w)
		}
	}
	out, err := WriteBytes("t.arkadelights.json", env, nil)
	if err != nil {
		t.Fatalf("WriteBytes() = %v", err)
	}
	if n := strings.Count(string(out), `"fadeMs"`); n != 2 {
		t.Errorf("fadeMs encoded %d times, want 2 (hard cuts omit it):\n%s", n, out)
	}
}

func TestLightLampsDistinguishDarkFromUnnamed(t *testing.T) {
	env := lightsEnvelope(LightFrame{Lamps: NewLightLamps(LightMaskPtr(0), nil), HoldMs: 100})
	out, err := WriteBytes("t.arkadelights.json", env, nil)
	if err != nil {
		t.Fatalf("WriteBytes() = %v", err)
	}
	if !strings.Contains(string(out), `"p1": 0`) {
		t.Errorf("an explicitly dark seat did not survive the encoding:\n%s", out)
	}
	if strings.Contains(string(out), `"p2"`) {
		t.Errorf("an unnamed seat was encoded anyway:\n%s", out)
	}
	back, err := ReadBytes("t.arkadelights.json", out)
	if err != nil {
		t.Fatalf("ReadBytes() = %v", err)
	}
	if m, ok := back.Frames[0].Lamps.Mask(0); !ok || m != 0 {
		t.Errorf("p1 read back as (%d, %v), want (0, true)", m, ok)
	}
	if _, ok := back.Frames[0].Lamps.Mask(1); ok {
		t.Errorf("p2 read back as named, want unnamed")
	}
}

func TestKindFromFilenameKnowsLights(t *testing.T) {
	if k, ok := KindFromFilename("assets/lights/pulse.arkadelights.json"); !ok || k != KindLights {
		t.Fatalf("KindFromFilename() = (%q, %v), want (lights, true)", k, ok)
	}
	if _, ok := KindFromFilename("pulse.arkadelights.png"); ok {
		t.Fatal("KindFromFilename accepted a lights envelope in a PNG")
	}
}
