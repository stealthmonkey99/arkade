// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Back (KeyJoyB) at the shell must end Run with a nil error. Uses the real
// Run loop rather than Tick.

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/render"
)

func TestShellQuitKeyEndsEngineCleanly(t *testing.T) {
	rt, err := engine.New(engine.Options{
		Renderer: render.NewCapture(false),
		Audio:    audio.NewRecorder(),
		DataDir:  t.TempDir(),
		StartApp: "shell",
		RandSeed: 42,
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- rt.Run() }()

	time.Sleep(100 * time.Millisecond) // let Init and the first render settle
	rt.Input.Push(input.Event{
		Kind:   input.KindKeypress,
		Name:   input.KeyJoyB,
		Source: input.PlayerSource{Type: input.SourceKeyboard},
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected Run to return nil after JOY B at the shell, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after JOY B at the shell")
	}
}
