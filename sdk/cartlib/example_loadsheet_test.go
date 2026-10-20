// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib_test

import (
	"fmt"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

// loadSheetTB stands in for the *testing.T a real test passes to hosttest.New.
type loadSheetTB struct {
	testing.TB
	cleanups []func()
}

func (e *loadSheetTB) Helper()          {}
func (e *loadSheetTB) Cleanup(f func()) { e.cleanups = append(e.cleanups, f) }

type loadSheetGame struct {
	hero, missing cartlib.Sheet
}

var loadSheetManifest = []byte(`{"abi":1,"slug":"hero","name":"hero"}`)

func (g *loadSheetGame) Manifest() []byte { return loadSheetManifest }

func (g *loadSheetGame) HandleInput(records []cartlib.InputRecord) {}

func (g *loadSheetGame) Init(s *cartlib.Setup) {
	g.hero = s.LoadSheet("assets/hero.arkadesprite.png")
	g.missing = s.LoadSheet("assets/nothing.arkadesprite.png")
}

func (g *loadSheetGame) Update(dtMicros uint64) {}

func (g *loadSheetGame) Render(fb *cartlib.Framebuffer) { fb.Clear(0, 0, 0) }

func ExampleSetup_LoadSheet() {
	tb := &loadSheetTB{}
	defer func() {
		for _, f := range tb.cleanups {
			f()
		}
	}()

	h := hosttest.New(tb) // in a test: hosttest.New(t)
	h.AddCel("assets/hero.arkadesprite.png", "idle", 2, 2, make([]byte, 2*2*4))

	g := &loadSheetGame{}
	h.Init(g)

	fmt.Println("hero loaded:", g.hero.Ok())
	fmt.Println("missing loaded:", g.missing.Ok())
	// Output:
	// hero loaded: true
	// missing loaded: false
}
