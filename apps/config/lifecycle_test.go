// SPDX-License-Identifier: GPL-2.0-or-later

package config_test

// Only Run surfaces the abort error, so these tests drive the real pacing loop;
// the sleeps clear the ~500ms scene fade.

import (
	"testing"
	"time"

	_ "github.com/stealthmonkey99/arkade/apps/config"
	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/render"
)

func runConfigScripted(t *testing.T, script func(rt *engine.Runtime)) error {
	t.Helper()
	rt, err := engine.New(engine.Options{
		Renderer: render.NewCapture(false),
		Audio:    audio.NewRecorder(),
		DataDir:  t.TempDir(),
		StartApp: "apps/config",
		RandSeed: 42,
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- rt.Run() }()
	script(rt)

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not quit in time")
		return nil
	}
}

func pushKey(rt *engine.Runtime, name input.KeyName) {
	rt.Input.Push(input.Event{
		Kind:   input.KindKeypress,
		Name:   name,
		Source: input.PlayerSource{Type: input.SourceKeyboard},
	})
}

func TestConfigShutdownQuitsEngineCleanly(t *testing.T) {
	err := runConfigScripted(t, func(rt *engine.Runtime) {
		time.Sleep(700 * time.Millisecond) // ride out the menu fade-in
		for i := 0; i < 5; i++ {           // SHUTDOWN is the 6th item
			pushKey(rt, input.KeyDown)
		}
		pushKey(rt, input.KeyEnter)        // select SHUTDOWN
		time.Sleep(700 * time.Millisecond) // ride out the confirm scene's fade-in
		pushKey(rt, input.KeyEnter)        // confirm
	})
	if err != nil {
		t.Fatalf("expected a clean shutdown (nil Run error), got %v", err)
	}
}

func TestConfigCrashTestAbortsEngine(t *testing.T) {
	err := runConfigScripted(t, func(rt *engine.Runtime) {
		time.Sleep(700 * time.Millisecond)
		for i := 0; i < 6; i++ { // CRASH TEST is the 7th item
			pushKey(rt, input.KeyDown)
		}
		pushKey(rt, input.KeyEnter)
		time.Sleep(700 * time.Millisecond)
		pushKey(rt, input.KeyEnter)
	})
	if err == nil {
		t.Fatal("expected CRASH TEST to make Run return an error")
	}
	if got, want := err.Error(), "crash test requested from config app"; got != want {
		t.Errorf("Run error = %q, want %q", got, want)
	}
}
