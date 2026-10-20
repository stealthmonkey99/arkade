// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import "testing"

func TestVocalParamsRoundTrip(t *testing.T) {
	want := testPhrase()
	raw, err := want.MarshalParams("hello")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalVocalParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("a vocal document did not read back as a vocal")
	}
	requireSameSamples(t, want.Render(SampleRate44100), got.Render(SampleRate44100))
}

func TestVocalParamsZeroesSurvive(t *testing.T) {
	v := DefaultVocal()
	v.Peak = 0
	v.Syllables = []Syllable{{Hz: 200, Phones: []Phone{{Ms: 40, F1Hz: 300, F2Hz: 1000, Nasal: 1}}}}
	raw, err := v.MarshalParams("")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnmarshalVocalParams(raw)
	if err != nil || got == nil {
		t.Fatalf("read back %v, %v", got, err)
	}
	s := got.Syllables[0]
	if s.AtMs != 0 || s.Gain != 0 || got.Peak != 0 {
		t.Errorf("zero-valued syllable fields moved: at %v gain %v peak %v", s.AtMs, s.Gain, got.Peak)
	}
	if p := s.Phones[0]; p.Open != 0 || p.GlideMs != 0 || p.Nasal != 1 {
		t.Errorf("phone came back as %+v", p)
	}
}

func TestVocalParamsIgnoresOtherGenerators(t *testing.T) {
	for _, raw := range []string{`{}`, `{"kind":"patch","durationS":0.2,"layers":[]}`, `{"hz":880,"ms":60}`, `null`} {
		got, err := UnmarshalVocalParams([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if got != nil {
			t.Fatalf("%s: read back as a vocal", raw)
		}
	}
	if _, err := UnmarshalVocalParams([]byte(`{"kind":"vocal","syllables":[{"hz":"loud"}]}`)); err == nil {
		t.Fatal("an unreadable vocal document was accepted")
	}
}
