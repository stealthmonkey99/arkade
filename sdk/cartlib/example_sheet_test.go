// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib_test

import (
	"fmt"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// sheetTB stands in for the *testing.T a real test passes to hosttest.New.
type sheetTB struct {
	testing.TB
	cleanups []func()
}

func (e *sheetTB) Helper()          {}
func (e *sheetTB) Cleanup(f func()) { e.cleanups = append(e.cleanups, f) }

type sheetGame struct {
	idle *ui.Sprite
	jump *ui.Sprite
}

var sheetManifest = []byte(`{"abi":1,"slug":"hero","name":"hero"}`)

func (g *sheetGame) Manifest() []byte { return sheetManifest }

func (g *sheetGame) HandleInput(records []cartlib.InputRecord) {}

func (g *sheetGame) Init(s *cartlib.Setup) {
	sheet := s.LoadSheet("assets/hero.arkadesprite.png")
	g.idle = sheet.Cel("idle")
	g.jump = sheet.Cel("jump")
}

func (g *sheetGame) Update(dtMicros uint64) {}

func (g *sheetGame) Render(fb *cartlib.Framebuffer) { fb.Clear(0, 0, 0) }

func ExampleSheet_Cel() {
	tb := &sheetTB{}
	defer func() {
		for _, f := range tb.cleanups {
			f()
		}
	}()

	h := hosttest.New(tb) // in a test: hosttest.New(t)
	h.AddCel("assets/hero.arkadesprite.png", "idle", 3, 2, make([]byte, 3*2*4))

	g := &sheetGame{}
	h.Init(g)

	fmt.Printf("idle: %dx%d\n", g.idle.W, g.idle.H)
	fmt.Println("jump is nil:", g.jump == nil)
	// Output:
	// idle: 3x2
	// jump is nil: true
}
