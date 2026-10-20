// SPDX-License-Identifier: GPL-2.0-or-later

// Package speech bakes spoken lines into samples with Kokoro-82M, an
// Apache-2.0 neural voice that runs offline on a CPU. A cartridge's
// generator calls Render and commits the result as assets; nothing here
// runs on the cabinet. Renderer keeps one worker warm for an editor that
// speaks line after line, and Params is the "speech" document a sound
// asset's recipe carries (sdk/docs/reference-sound.md, "The speech
// params").
//
// The model runs under Node (render.mjs beside this file), so the first
// use needs `npm ci` in this directory. The model weights are fetched
// once from a pinned revision, hash-checked, and cached under
// $ARKADE_SPEECH_CACHE (default ~/.cache/arkade-speech).
//
// Output is byte-identical run to run on one machine. Across CPU
// architectures the float math may differ in the low bits, so compare
// rebuilt clips by ear or with `assetdiff -tolerance`, not by hash;
// ./rerender re-speaks committed speech assets for that comparison.
package speech

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/stealthmonkey99/arkade/tools/sfxlib"
)

// ModelRate is the sample rate Kokoro renders at.
const ModelRate = 24000

// Voice names a Kokoro voice ("bm_george") and its pace, where 1 is the
// model's default speed and 1.3 is 30% faster. A non-empty Blend mixes a
// second voice in at BlendWeight, from 0 (Name alone) to 1 (Blend alone);
// see Params. Blend "" ignores BlendWeight. A voice the model does not
// ship, or a BlendWeight outside [0, 1], fails the render.
type Voice struct {
	Name        string
	Speed       float64
	Blend       string
	BlendWeight float64
}

type jobItem struct {
	Text string `json:"text"`
	Out  string `json:"out"`
}

type jobBlend struct {
	Voice  string  `json:"voice"`
	Weight float64 `json:"weight"`
}

// job is one render.mjs request; see the protocol at the top of it.
type job struct {
	Voice string    `json:"voice"`
	Speed float64   `json:"speed"`
	Blend *jobBlend `json:"blend,omitempty"`
	Items []jobItem `json:"items"`
}

func newJob(v Voice) job {
	j := job{Voice: v.Name, Speed: v.Speed}
	if v.Blend != "" {
		j.Blend = &jobBlend{Voice: v.Blend, Weight: v.BlendWeight}
	}
	return j
}

// Render speaks each text in v and returns mono samples in [-1,1] at
// sampleRate, one slice per text, in order.
func Render(v Voice, texts []string, sampleRate int) ([][]float64, error) {
	if err := Installed(); err != nil {
		return nil, err
	}
	dir := Dir()
	tmp, err := os.MkdirTemp("", "arkade-speech")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	job := newJob(v)
	for i, t := range texts {
		job.Items = append(job.Items, jobItem{t, filepath.Join(tmp, fmt.Sprintf("%d.f32", i))})
	}
	in, err := json.Marshal(job)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("node", filepath.Join(dir, "render.mjs"))
	cmd.Stdin = bytes.NewReader(in)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("speech: render.mjs: %w", err)
	}

	out := make([][]float64, len(texts))
	for i, it := range job.Items {
		b, err := os.ReadFile(it.Out)
		if err != nil {
			return nil, err
		}
		s := make([]float64, len(b)/4)
		for j := range s {
			s[j] = float64(math.Float32frombits(binary.LittleEndian.Uint32(b[4*j:])))
		}
		out[i] = Resample(s, ModelRate, sampleRate)
	}
	return out, nil
}

// Resample converts s from one sample rate to another; see
// sfxlib.Resample.
func Resample(s []float64, from, to int) []float64 {
	return sfxlib.Resample(s, from, to)
}
