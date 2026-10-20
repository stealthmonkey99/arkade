// SPDX-License-Identifier: GPL-2.0-or-later

package speech

import (
	"encoding/json"
	"math"
	"os"
	"slices"
	"testing"
)

func TestParamsRoundTrip(t *testing.T) {
	in := Params{Text: "fifteen two", Voice: "bm_george", Speed: 1.3, PitchSemitones: -2, HighpassHz: 150, Peak: 0.8}
	raw, err := in.MarshalParams("f2")
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	json.Unmarshal(raw, &probe)
	if probe["kind"] != "speech" || probe["name"] != "f2" {
		t.Errorf("document %s", raw)
	}
	out, err := UnmarshalParams(raw)
	if err != nil || out == nil || *out != in {
		t.Errorf("round trip: %+v, %v", out, err)
	}
}

func TestBlendRoundTripsAndReachesTheWorker(t *testing.T) {
	in := Params{Text: "fifteen two", Voice: "bm_george", BlendVoice: "bm_fable", BlendWeight: 0.4, Speed: 1.3}
	raw, err := in.MarshalParams("f2")
	if err != nil {
		t.Fatal(err)
	}
	var probe map[string]any
	json.Unmarshal(raw, &probe)
	if probe["blendVoice"] != "bm_fable" || probe["blendWeight"] != 0.4 {
		t.Errorf("document %s", raw)
	}
	out, err := UnmarshalParams(raw)
	if err != nil || out == nil || *out != in {
		t.Fatalf("round trip: %+v, %v", out, err)
	}
	req, _ := json.Marshal(newJob(out.Resolved().voice()))
	if want := `{"voice":"bm_george","speed":1.3,"blend":{"voice":"bm_fable","weight":0.4},"items":null}`; string(req) != want {
		t.Errorf("worker request %s, want %s", req, want)
	}

	plain, _ := Params{Text: "go", Voice: "bm_george", BlendWeight: 0.7}.MarshalParams("go")
	if want := `{"kind":"speech","name":"go","text":"go","voice":"bm_george","blendWeight":0.7}`; string(plain) != want {
		t.Errorf("document %s, want %s", plain, want)
	}
	unblended, _ := Params{Text: "go"}.MarshalParams("go")
	if want := `{"kind":"speech","name":"go","text":"go"}`; string(unblended) != want {
		t.Errorf("an unblended line wrote %s, want %s", unblended, want)
	}
	req, _ = json.Marshal(newJob(Params{Text: "go", BlendWeight: 0.7}.Resolved().voice()))
	if want := `{"voice":"bm_george","speed":1,"items":null}`; string(req) != want {
		t.Errorf("a weight without a second voice reached the worker: %s", req)
	}
}

func TestResolvedClampsTheBlend(t *testing.T) {
	for _, tc := range []struct {
		voice    string
		in, want float64
	}{
		{"bm_fable", 0.25, 0.25}, {"bm_fable", 3, 1}, {"bm_fable", -1, 0}, {"bm_fable", math.NaN(), 0}, {"", 0.5, 0},
	} {
		if got := (Params{BlendVoice: tc.voice, BlendWeight: tc.in}).Resolved().BlendWeight; got != tc.want {
			t.Errorf("%q at %v resolved to %v, want %v", tc.voice, tc.in, got, tc.want)
		}
	}
}

func TestUnmarshalParamsLeavesOtherKindsAlone(t *testing.T) {
	for _, raw := range []string{`{"kind":"vocal","syllables":[]}`, `{"wave":"sine"}`, ``} {
		p, err := UnmarshalParams(json.RawMessage(raw))
		if p != nil || err != nil {
			t.Errorf("%q: %+v, %v", raw, p, err)
		}
	}
	if _, err := UnmarshalParams(json.RawMessage(`{"kind":`)); err == nil {
		t.Error("a broken document was accepted")
	}
}

func TestResolvedFillsDefaultsAndClamps(t *testing.T) {
	got := Params{Speed: 9, Peak: -1, HighpassHz: -5, PitchSemitones: -40}.Resolved()
	want := Params{Voice: DefaultVoice, Speed: MaxSpeed, Peak: DefaultPeak, PitchSemitones: -MaxPitchSemitones}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestShapeTrimsAndNormalizes(t *testing.T) {
	const rate = 16000
	s := make([]float64, rate) // 1 s: silence, 200 ms of tone, silence
	for i := 400 * rate / 1000; i < 600*rate/1000; i++ {
		s[i] = 0.3 * math.Sin(2*math.Pi*440*float64(i)/rate)
	}
	out := Params{Peak: 0.9}.Resolved().shape(s, rate)
	if d := float64(len(out)) / rate; d < 0.2 || d > 0.26 {
		t.Errorf("trimmed to %.3fs, want ~0.24s", d)
	}
	peak := 0.0
	for _, v := range out {
		peak = math.Max(peak, math.Abs(v))
	}
	if math.Abs(peak-0.9) > 1e-9 {
		t.Errorf("peak %.4f, want 0.9", peak)
	}
	if out[0] != 0 || out[len(out)-1] != 0 {
		t.Error("ends are not faded to zero")
	}
}

func TestRendererSpeaksAndCaches(t *testing.T) {
	if os.Getenv("ARKADE_SPEECH_TEST") == "" {
		t.Skip("set ARKADE_SPEECH_TEST=1 to run the Kokoro renderer")
	}
	r := NewRenderer()
	defer r.Close()
	voices, err := r.Voices()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range voices {
		found = found || v.ID == DefaultVoice
	}
	if !found {
		t.Errorf("%s missing from %d voices", DefaultVoice, len(voices))
	}
	p := Params{Text: "a pair for two", HighpassHz: 150}
	a, err := p.Render(r, 44100)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.Render(r, 44100)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("renders of %d and %d samples", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("second render differs at sample %d", i)
		}
	}
}

func TestRendererBlendsTwoVoices(t *testing.T) {
	if os.Getenv("ARKADE_SPEECH_TEST") == "" {
		t.Skip("set ARKADE_SPEECH_TEST=1 to run the Kokoro renderer")
	}
	r := NewRenderer()
	defer r.Close()
	render := func(p Params) []float64 {
		t.Helper()
		p.Text, p.Speed = "fifteen two", 1.3
		s, err := p.Render(r, 24000)
		if err != nil || len(s) == 0 {
			t.Fatalf("%+v: %d samples, %v", p, len(s), err)
		}
		return s
	}
	george := render(Params{Voice: "bm_george"})
	fable := render(Params{Voice: "bm_fable"})
	blend := render(Params{Voice: "bm_george", BlendVoice: "bm_fable", BlendWeight: 0.5})
	if slices.Equal(blend, george) || slices.Equal(blend, fable) {
		t.Error("the half blend renders as one of its voices")
	}
	if !slices.Equal(render(Params{Voice: "bm_george", BlendVoice: "bm_fable"}), george) {
		t.Error("weight 0 is not the first voice byte for byte")
	}
	if _, err := (Params{Text: "go", BlendVoice: "zz_nobody", BlendWeight: 0.5}).Render(r, 24000); err == nil {
		t.Error("an unknown second voice rendered")
	}
}
