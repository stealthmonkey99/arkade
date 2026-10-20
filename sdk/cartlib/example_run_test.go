// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib_test

import (
	"fmt"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

// runTB stands in for the *testing.T a real test passes to hosttest.New.
type runTB struct {
	testing.TB
	cleanups []func()
}

func (e *runTB) Helper()          {}
func (e *runTB) Cleanup(f func()) { e.cleanups = append(e.cleanups, f) }

type runGame struct {
	updates int
}

var runManifest = []byte(`{"abi":1,"slug":"counter","name":"counter"}`)

func (g *runGame) Manifest() []byte { return runManifest }

func (g *runGame) HandleInput(records []cartlib.InputRecord) {}

func (g *runGame) Init(s *cartlib.Setup) {}

func (g *runGame) Update(dtMicros uint64) { g.updates++ }

func (g *runGame) Render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)
	cartlib.DrawUint(fb, 1, 1, uint32(g.updates), 1, 255, 255, 255)
}

var runCart = &runGame{}
var _ = cartlib.Run(runCart)

func ExampleRun() {
	tb := &runTB{}
	defer func() {
		for _, f := range tb.cleanups {
			f()
		}
	}()

	h := hosttest.New(tb) // in a test: hosttest.New(t)
	h.Init(runCart)
	h.AdvancePastSplash()
	h.Tick(16_667)
	h.Tick(16_667)

	fmt.Println("updates:", runCart.updates)
	fmt.Println("digit drawn:", h.Pixel(1, 1) != cartlib.Color{A: 255})
	// Output:
	// updates: 2
	// digit drawn: true
}
