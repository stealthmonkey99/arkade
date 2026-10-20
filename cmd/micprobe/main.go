// SPDX-License-Identifier: GPL-2.0-or-later

// Command micprobe is the microphone's bench instrument: it runs the
// cabinet's capture pipeline with nothing attached to it and prints what
// the detectors decide, or replays a recording through them.
//
//	micprobe -device plughw:0,0 -meter           # live, with a level meter
//	micprobe -dump room.wav -secs 20             # live, keeping the capture
//	micprobe -file room.wav                      # replay a recording
//	micprobe -file word.wav -model x.arkademic.json
//
// Replay is exact — every stage is a pure function of its samples — so a
// threshold can be tuned against a recording instead of a person standing
// at the cabinet. Live mode has no playback reference, so there is nothing
// to cancel; the peripherals lab is where cancellation is watched.
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/mic"
)

func main() {
	cfg := mic.DefaultConfig()

	device := flag.String("device", "plughw:0,0", "ALSA capture device")
	file := flag.String("file", "", "replay a 16-bit WAV instead of capturing")
	dump := flag.String("dump", "", "live: write the analysed capture (at the analysis rate) to this WAV")
	secs := flag.Int("secs", 0, "live: stop after N seconds (0 = until Ctrl-C)")
	meter := flag.Bool("meter", false, "live level meter")
	modelPath := flag.String("model", "", "a <name>.arkademic.json sound model to run alongside the detector")

	flag.Float64Var(&cfg.MinRMS, "min-rms", cfg.MinRMS, "absolute level floor, 0..1")
	flag.Float64Var(&cfg.SNR, "snr", cfg.SNR, "required level above the tracked noise floor")
	flag.Float64Var(&cfg.Tonality, "tonality", cfg.Tonality, "whistle purity, 0..1")
	flag.Float64Var(&cfg.WhistleMinHz, "min-hz", cfg.WhistleMinHz, "low edge of the whistle band")
	flag.Float64Var(&cfg.WhistleMaxHz, "max-hz", cfg.WhistleMaxHz, "high edge of the whistle band")
	flag.IntVar(&cfg.HoldFrames, "hold", cfg.HoldFrames, "consecutive frames a sustained event needs")
	flag.IntVar(&cfg.RefractoryMS, "refractory", cfg.RefractoryMS, "per-event dead time, ms")
	flag.BoolVar(&cfg.EnableClap, "clap", cfg.EnableClap, "enable the clap detector")
	flag.BoolVar(&cfg.EnableBlow, "blow", cfg.EnableBlow, "enable the blow detector")
	flag.Parse()

	var models []*assetfile.Envelope
	if *modelPath != "" {
		env, err := assetfile.Read(*modelPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "micprobe:", err)
			os.Exit(1)
		}
		models = append(models, env)
	}

	var err error
	if *file != "" {
		err = replay(cfg, *file, models)
	} else {
		err = live(cfg, *device, *dump, *secs, *meter, models)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "micprobe:", err)
		os.Exit(1)
	}
}

// replay runs a recording through the detector at the file's own rate and
// through the sound model when the file is at the analysis rate.
func replay(cfg mic.Config, path string, models []*assetfile.Envelope) error {
	samples, rate, err := mic.ReadWAVFile(path)
	if err != nil {
		return err
	}
	if rate != cfg.SampleRate {
		fmt.Fprintf(os.Stderr, "note: %s is %d Hz; the detector analyses at that rate, the sound model needs %d\n", path, rate, mic.Rate)
		cfg.SampleRate = rate
	}
	spf := float64(cfg.FrameLen) / float64(rate)
	det := mic.NewDetector(cfg)
	counts := map[mic.Event]int{}
	var peak, floor float64
	frames := 0
	det.WriteFloat(samples, func(f mic.Frame) {
		frames++
		floor = f.Floor
		peak = math.Max(peak, f.RMS)
		if f.Event != mic.EventNone {
			counts[f.Event]++
			fmt.Println(eventLine(float64(f.Index)*spf, f))
		}
	})
	if len(models) > 0 && rate == mic.Rate {
		cls, _, err := mic.NewClassifier(models)
		if err != nil {
			return err
		}
		for off := 0; off < len(samples); off += cfg.FrameLen {
			end := min(off+cfg.FrameLen, len(samples))
			if h := cls.Write(samples[off:end], false); h.Cue >= 0 {
				fmt.Printf("%7.2fs  cue %d %q  score %.3f\n", float64(end)/float64(rate), h.Cue, h.Name, h.Score)
			}
		}
	}
	summary(float64(frames)*spf, counts, peak, floor)
	return nil
}

func live(cfg mic.Config, device, dump string, secs int, meter bool, models []*assetfile.Envelope) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
		case <-ctx.Done():
		}
		cancel()
	}()
	if secs > 0 {
		go func() {
			select {
			case <-time.After(time.Duration(secs) * time.Second):
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	cap := mic.NewCapture(mic.Options{
		Device: device, Detector: cfg, NoCancel: true,
		Log: func(s string) { fmt.Fprintln(os.Stderr, s) },
	})
	if len(models) > 0 {
		cap.SetSessionModels(models)
	}
	var dumped []float64
	counts := map[mic.Event]int{}
	var peak, floor float64
	spf := float64(cfg.FrameLen) / float64(mic.Rate)
	frames := 0
	cap.Listen(func(b mic.Block) {
		frames++
		floor = b.Floor
		peak = math.Max(peak, b.RMS)
		if dump != "" {
			dumped = append(dumped, b.Raw...)
		}
		if b.Event != mic.EventNone {
			counts[b.Event]++
			if meter {
				fmt.Print("\r\033[K")
			}
			fmt.Println(eventLine(float64(b.Seq)*spf, mic.Frame{
				RMS: b.RMS, Tonality: b.Tonality, Event: b.Event, PitchHz: b.PeakHz,
			}))
		}
		if b.Cue >= 0 {
			if meter {
				fmt.Print("\r\033[K")
			}
			fmt.Printf("%7.2fs  cue %d %q  score %.3f\n", float64(b.Seq)*spf, b.Cue, b.CueName, b.CueScore)
		}
		if meter && b.Seq%4 == 0 {
			fmt.Printf("\r\033[K%6.1fs %s %s  floor %s  peak %5.0f Hz  ton %.2f",
				float64(b.Seq)*spf, bar(b.RMS), dbfs(b.RMS), dbfs(b.Floor), b.PeakHz, b.Tonality)
		}
	})

	fmt.Printf("listening on %s — whistle, clap, blow. Ctrl-C to stop.\n", device)
	if err := cap.Run(ctx); err != nil {
		return err
	}
	if meter {
		fmt.Print("\r\033[K")
	}
	if dump != "" {
		if err := mic.WriteWAVFile(dump, dumped, mic.Rate); err != nil {
			return err
		}
		fmt.Printf("wrote %s (%.1fs at %d Hz)\n", dump, float64(len(dumped))/mic.Rate, mic.Rate)
	}
	summary(float64(frames)*spf, counts, peak, floor)
	return nil
}

func eventLine(t float64, f mic.Frame) string {
	line := fmt.Sprintf("%7.2fs  %-8s", t, f.Event)
	if f.Event == mic.EventWhistle {
		line += fmt.Sprintf(" %5.0f Hz", f.PitchHz)
	} else {
		line += "         "
	}
	return line + fmt.Sprintf("  rms %s  tonality %.2f", dbfs(f.RMS), f.Tonality)
}

func summary(secs float64, counts map[mic.Event]int, peak, floor float64) {
	fmt.Printf("\n%.1fs analysed — whistle %d, clap %d, blow %d\n",
		secs, counts[mic.EventWhistle], counts[mic.EventClap], counts[mic.EventBlow])
	fmt.Printf("peak %s, noise floor %s (headroom %.1f dB)\n", dbfs(peak), dbfs(floor), db(peak)-db(floor))
}

func db(v float64) float64 {
	if v <= 1e-9 {
		return -90
	}
	return 20 * math.Log10(v)
}

func dbfs(v float64) string { return fmt.Sprintf("%6.1f dB", db(v)) }

// bar renders a 24-cell meter spanning -60..0 dBFS.
func bar(v float64) string {
	cells := min(max(int((db(v)+60)/60*24), 0), 24)
	return "[" + strings.Repeat("#", cells) + strings.Repeat(".", 24-cells) + "]"
}
