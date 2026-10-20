// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
)

// maxBundleMicModels bounds the per-sound matching work.
const maxBundleMicModels = rcart.MaxMountedMicModels

// MicModelSink is optionally implemented by a Host with a microphone. It
// receives a cartridge's bundled sound models, whose matches arrive as
// mic.cueN keypresses; without it no cue ever fires.
type MicModelSink interface {
	// SetSessionMicModels replaces the session's models and receives nil
	// at teardown.
	SetSessionMicModels(models []*assetfile.Envelope)
}

// micRegistry holds one cartridge's models so teardown can drop them.
type micRegistry struct {
	host   bundleReader
	models []*assetfile.Envelope
}

func newMicRegistry(host bundleReader) *micRegistry { return &micRegistry{host: host} }

func (r *micRegistry) sink() MicModelSink {
	s, _ := r.host.(MicModelSink)
	return s
}

func (r *micRegistry) loadBundle(paths []string) {
	for _, p := range paths {
		if len(r.models) >= maxBundleMicModels {
			hostWarnf(r.host, "mic model %q: refused, already holding %d models", p, maxBundleMicModels)
			continue
		}
		if env, ok := readBundleEnvelope(r.host, "mic model", p, assetfile.KindMic); ok {
			r.models = append(r.models, env)
		}
	}
	if sink := r.sink(); sink != nil && len(r.models) > 0 {
		sink.SetSessionMicModels(r.models)
	}
}

func (r *micRegistry) dropBundle() {
	if sink := r.sink(); sink != nil && len(r.models) > 0 {
		sink.SetSessionMicModels(nil)
	}
	r.models = nil
}
