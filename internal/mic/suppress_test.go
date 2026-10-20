// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import "testing"

func TestSuppressionSilencesEvents(t *testing.T) {
	in := cat(silence(200), sine(1800, 0.25, 800), silence(200))

	d := NewDetector(DefaultConfig())
	d.Suppress(true)
	var got []Frame
	d.Write(in, func(f Frame) {
		if f.Event != EventNone {
			got = append(got, f)
		}
	})
	if len(got) != 0 {
		t.Fatalf("suppressed detector still fired %d events: %+v", len(got), got)
	}

	if n := count(collect(t, DefaultConfig(), in), EventWhistle); n != 1 {
		t.Fatalf("control: unsuppressed run gave %d whistles, want 1", n)
	}
}

func TestSuppressionLiftDoesNotFireHeldProgress(t *testing.T) {
	cfg := DefaultConfig()
	d := NewDetector(cfg)

	tone := sine(1800, 0.25, 1000)
	half := len(tone) / 2

	d.Suppress(true)
	var during []Frame
	d.Write(tone[:half], func(f Frame) {
		if f.Event != EventNone {
			during = append(during, f)
		}
	})
	if len(during) != 0 {
		t.Fatalf("events fired while suppressed: %+v", during)
	}

	d.Suppress(false)
	fired := -1
	d.Write(tone[half:], func(f Frame) {
		if f.Event == EventWhistle && fired < 0 {
			fired = f.Index
		}
	})
	if fired < 0 {
		t.Fatal("no whistle after suppression lifted")
	}
	firstAfter := half / cfg.FrameLen
	if fired-firstAfter < cfg.HoldFrames-1 {
		t.Fatalf("whistle fired %d frames after release, want at least %d — "+
			"held-over progress leaked across the boundary", fired-firstAfter, cfg.HoldFrames-1)
	}
}

func TestSynthesisedToneIsPurerThanAHumanWhistle(t *testing.T) {
	var pureTonality float64
	d := NewDetector(DefaultConfig())
	d.Write(sine(1550, 0.3, 400), func(f Frame) {
		if f.Tonality > pureTonality {
			pureTonality = f.Tonality
		}
	})

	human := sine(1550, 0.3, 400)
	wob := noise(0.03, 400, 0, 0)
	for i := range human {
		v := int(human[i]) + int(wob[i%len(wob)])
		human[i] = int16(max(-32768, min(32767, v)))
	}
	var humanTonality float64
	d2 := NewDetector(DefaultConfig())
	d2.Write(human, func(f Frame) {
		if f.Tonality > humanTonality {
			humanTonality = f.Tonality
		}
	})

	if pureTonality < humanTonality {
		t.Fatalf("a synthesised tone scored %.3f and a human-like one %.3f — "+
			"if effects are now LESS tonal than people, a threshold could "+
			"separate them and suppression may no longer be the only fix",
			pureTonality, humanTonality)
	}
	t.Logf("synthesised %.3f vs human-like %.3f — no threshold separates them",
		pureTonality, humanTonality)
}

func TestClapAndBlowAreOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.EnableClap || cfg.EnableBlow {
		t.Fatal("clap/blow must ship disabled until tuned against a real recording")
	}
	got := collect(t, cfg, cat(silence(500), noise(0.6, 40, 0, 0), silence(300),
		noise(0.5, 700, 180, 80), silence(300)))
	for _, f := range got {
		if f.Event == EventClap || f.Event == EventBlow {
			t.Fatalf("%s fired despite being disabled", f.Event)
		}
	}
}
