// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib_test

import (
	"fmt"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

// loadSoundTB stands in for the *testing.T a real test passes to hosttest.New.
type loadSoundTB struct {
	testing.TB
	cleanups []func()
}

func (e *loadSoundTB) Helper()          {}
func (e *loadSoundTB) Cleanup(f func()) { e.cleanups = append(e.cleanups, f) }

type loadSoundGame struct {
	chime uint32
}

var loadSoundManifest = []byte(`{"abi":1,"slug":"chimes","name":"chimes"}`)

func (g *loadSoundGame) Manifest() []byte { return loadSoundManifest }

func (g *loadSoundGame) HandleInput(records []cartlib.InputRecord) {}

func (g *loadSoundGame) Init(s *cartlib.Setup) {
	g.chime = s.LoadSound("assets/chime.wav")
}

func (g *loadSoundGame) Update(dtMicros uint64) {}

func (g *loadSoundGame) Render(fb *cartlib.Framebuffer) { fb.Clear(0, 0, 0) }

func ExampleSetup_LoadSound() {
	tb := &loadSoundTB{}
	defer func() {
		for _, f := range tb.cleanups {
			f()
		}
	}()

	h := hosttest.New(tb) // in a test: hosttest.New(t)
	g := &loadSoundGame{}
	h.Init(g)

	fmt.Println("chime handle:", g.chime)
	fmt.Println("never loaded:", h.SoundID("assets/never-loaded.wav"))
	// Output:
	// chime handle: 1
	// never loaded: 0
}
