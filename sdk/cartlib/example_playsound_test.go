// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib_test

import (
	"fmt"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

// playSoundTB stands in for the *testing.T a real test passes to hosttest.New.
type playSoundTB struct {
	testing.TB
	cleanups []func()
}

func (e *playSoundTB) Helper()          {}
func (e *playSoundTB) Cleanup(f func()) { e.cleanups = append(e.cleanups, f) }

type playSoundGame struct {
	chime uint32
}

var playSoundManifest = []byte(`{"abi":1,"slug":"chimes","name":"chimes"}`)

func (g *playSoundGame) Manifest() []byte { return playSoundManifest }

func (g *playSoundGame) HandleInput(records []cartlib.InputRecord) {}

func (g *playSoundGame) Init(s *cartlib.Setup) {
	g.chime = s.LoadSound("assets/chime.wav")
}

func (g *playSoundGame) Update(dtMicros uint64) {
	cartlib.PlaySound(g.chime, 2)
}

func (g *playSoundGame) Render(fb *cartlib.Framebuffer) { fb.Clear(0, 0, 0) }

func ExamplePlaySound() {
	tb := &playSoundTB{}
	defer func() {
		for _, f := range tb.cleanups {
			f()
		}
	}()

	h := hosttest.New(tb) // in a test: hosttest.New(t)
	g := &playSoundGame{}
	h.Init(g)
	h.AdvancePastSplash()
	h.Tick(16_667)

	for _, c := range h.Sounds {
		fmt.Printf("%s id=%d loops=%d\n", c.Op, c.ID, c.Loops)
	}
	fmt.Println("playing:", h.Playing(g.chime))
	// Output:
	// play id=1 loops=2
	// playing: true
}
