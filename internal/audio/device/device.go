// SPDX-License-Identifier: GPL-2.0-or-later

// Package device connects an audio.Mixer to the system's audio output via
// oto (CoreAudio on macOS, ALSA on Linux). It is the only package that imports
// oto, so tests of the mixing core never link against system audio.
//
// On macOS with Go 1.23, binaries linking purego need
// `-ldflags=-linkmode=external` or dyld aborts with "missing LC_UUID load command".
package device

import (
	"fmt"
	"io"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/stealthmonkey99/arkade/internal/audio"
)

// Output is a running audio device player. It retains the context so a
// finalizer doesn't tear down the device mid-playback.
type Output struct {
	ctx    *oto.Context
	player *oto.Player
}

// Open configures the device for the mixer's format and starts playback. It
// returns an error when no audio device is available.
func Open(m *audio.Mixer) (*Output, error) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   m.SampleRate(),
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
		BufferSize:   m.DeviceBuffer(),
	})
	if err != nil {
		return nil, fmt.Errorf("audio device: %w", err)
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("audio device: not ready after 5s")
	}
	player := ctx.NewPlayer(m.Reader())
	// oto defaults to 0.5s of read-ahead, so a new sound queues behind
	// buffered silence and lands well after the on-screen event.
	const playerBufferMs = 40
	const bytesPerFrame = 2 * 4
	player.SetBufferSize(m.SampleRate() * playerBufferMs / 1000 * bytesPerFrame)
	player.Play()
	return &Output{ctx: ctx, player: player}, nil
}

// Close stops playback.
func (o *Output) Close() error {
	if o.player != nil {
		return o.player.Close()
	}
	return nil
}

var _ io.Closer = (*Output)(nil)
