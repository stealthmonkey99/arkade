// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// EncodeAsset renders samples as a .arkadesound.wav file's bytes:
// Encode's WAV with meta's envelope in an "arkd" chunk. name must follow
// the <stem>.arkadesound.wav rule or the envelope is refused.
func EncodeAsset(name string, samples []float64, sampleRate int, meta assetfile.SoundMeta) ([]byte, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("sfxlib: %s: nothing to write, the sound is empty", name)
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sfxlib: %s: sample rate %d must be positive", name, sampleRate)
	}
	env, err := assetfile.NewSoundEnvelope(meta, assetfile.Render{
		SampleRate: sampleRate,
		Channels:   1,
		Bits:       16,
	})
	if err != nil {
		return nil, err
	}
	return assetfile.WriteBytes(name, env, Encode(samples, sampleRate))
}

// WriteAssetFile encodes samples as EncodeAsset does and writes them to
// path with mode 0o644.
func WriteAssetFile(path string, samples []float64, sampleRate int, meta assetfile.SoundMeta) error {
	data, err := EncodeAsset(filepath.Base(path), samples, sampleRate, meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SoundFileName applies the sound naming rule to a stem: "lost" and
// "lost.wav" both become "lost.arkadesound.wav". It is idempotent.
func SoundFileName(stem string) string {
	base := filepath.Base(stem)
	if kind, ok := assetfile.KindFromFilename(base); ok && kind == assetfile.KindSound {
		return base
	}
	for {
		ext := filepath.Ext(base)
		if ext == "" {
			break
		}
		base = base[:len(base)-len(ext)]
	}
	return base + ".arkade" + string(assetfile.KindSound) + ".wav"
}
