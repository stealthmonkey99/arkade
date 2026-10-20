// SPDX-License-Identifier: GPL-2.0-or-later

// Package audio defines the sound service API and its two backends: the
// Mixer, which decodes WAVs and plays them through audio/device, and the
// Recorder, a data-only double for tests and for cabinets without sound.
package audio

import "sync"

// LoopForever requests infinite looping.
const LoopForever = -1

// Service plays and stops named sounds. Owner is the app slug; label
// distinguishes concurrent sounds.
type Service interface {
	// Play starts a sound. loop is the total play count: 0 plays once,
	// n>0 plays n times, LoopForever plays until stopped.
	Play(owner, label, file string, loop int)
	// Stop stops sounds for owner; empty label stops all of them.
	Stop(owner, label string)
	// PreloadOwner decodes an app's sounds ahead of first play.
	PreloadOwner(owner string)
	// LoadBytes decodes WAV data and caches it under owner/file, for sounds
	// with no path a Resolver could address (go:embed assets).
	LoadBytes(owner, file string, data []byte) error
	// PlayBytes decodes wav and starts it immediately under owner/label,
	// attenuated by gain (linear, clamped to 0..1, on top of the master
	// volume), without caching it the way LoadBytes would. It plays once;
	// stop it early with Stop(owner, label). A non-nil error means wav did
	// not decode and nothing is playing.
	PlayBytes(owner, label string, wav []byte, gain float64) error
}

// Event is one recorded audio call.
type Event struct {
	Action string // "play" or "stop"
	Owner  string
	Label  string
	File   string
	Loop   int
	// Bytes is the WAV byte count of a PlayBytes call, else 0.
	Bytes int
	// Gain is the unclamped PlayBytes gain, else 0.
	Gain float64
}

// Recorder is the data-only audio backend: it records calls and tracks
// which looping sounds would still be playing.
type Recorder struct {
	mu     sync.Mutex
	events []Event
	active map[string]Event
}

// NewRecorder builds an empty recorder.
func NewRecorder() *Recorder {
	return &Recorder{active: map[string]Event{}}
}

func key(owner, label string) string { return owner + "\x00" + label }

// PreloadOwner does nothing: the Recorder decodes nothing.
func (r *Recorder) PreloadOwner(string) {}

// LoadBytes does nothing and never fails: Play records a call whether or
// not its file was registered.
func (r *Recorder) LoadBytes(string, string, []byte) error { return nil }

func (r *Recorder) Play(owner, label, file string, loop int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev := Event{Action: "play", Owner: owner, Label: label, File: file, Loop: loop}
	r.events = append(r.events, ev)
	if loop != 0 {
		r.active[key(owner, label)] = ev
	}
}

// PlayBytes records the call without decoding, and never fails.
func (r *Recorder) PlayBytes(owner, label string, wav []byte, gain float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, Event{
		Action: "play", Owner: owner, Label: label,
		Bytes: len(wav), Gain: gain,
	})
	return nil
}

func (r *Recorder) Stop(owner, label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, Event{Action: "stop", Owner: owner, Label: label})
	if label == "" {
		for k, ev := range r.active {
			if ev.Owner == owner {
				delete(r.active, k)
			}
		}
		return
	}
	delete(r.active, key(owner, label))
}

// Events returns the recorded call history.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

// Playing reports whether a looping sound is still active.
func (r *Recorder) Playing(owner, label string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.active[key(owner, label)]
	return ok
}

var _ Service = (*Recorder)(nil)

// Reset clears history and active sounds.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
	r.active = map[string]Event{}
}
