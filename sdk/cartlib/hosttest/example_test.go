// SPDX-License-Identifier: GPL-2.0-or-later

package hosttest_test

import (
	"fmt"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
	"github.com/stealthmonkey99/arkade/sdk/cartlib/hosttest"
)

var manifest = []byte(`{"abi":1,"slug":"beep","name":"Beep"}`)

type beep struct {
	chime   uint32
	presses int
}

func (g *beep) Manifest() []byte { return manifest }

func (g *beep) Init(s *cartlib.Setup) { g.chime = s.LoadSound("assets/chime.wav") }

func (g *beep) HandleInput(records []cartlib.InputRecord) {
	for _, r := range records {
		if r.IsPress() && r.Key == cartlib.KeyJoyA {
			g.presses++
			cartlib.PlaySound(g.chime, 1)
		}
	}
}

func (g *beep) Update(dtMicros uint64) {
	cartlib.SetText(cartlib.SeatP1, "PRESSES", fmt.Sprint(g.presses))
}

func (g *beep) Render(fb *cartlib.Framebuffer) {
	fb.Clear(0, 0, 0)
	if g.presses > 0 {
		fb.SetPixel(0, 0, 255, 0, 0)
	}
}

// exampleTB stands in for the *testing.T a real test passes to hosttest.New.
type exampleTB struct {
	testing.TB
	cleanups []func()
}

func (e *exampleTB) Helper()          {}
func (e *exampleTB) Cleanup(f func()) { e.cleanups = append(e.cleanups, f) }

func ExampleNew() {
	tb := &exampleTB{}
	defer func() {
		for _, f := range tb.cleanups {
			f()
		}
	}()

	h := hosttest.New(tb) // in a test: hosttest.New(t)
	h.Init(&beep{})
	h.AdvancePastSplash()
	h.Input(hosttest.Press(cartlib.SeatP1, cartlib.KeyJoyA)...)
	h.Tick(33_333)

	line1, line2, _ := h.LCD(cartlib.SeatP1)
	fmt.Println(line1, line2)
	fmt.Println(h.Sounds)
	fmt.Println(h.Pixel(0, 0))
	// Output:
	// PRESSES 1
	// [{play 1 1}]
	// {255 0 0 255}
}
