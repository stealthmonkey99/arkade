// Fixture: a minimal cartridge that breaks no GOLDEN RULE. The pass path
// for check_goldenrules.go. Never built — testdata is invisible to the go
// command — so it only has to parse.
package main

import "github.com/stealthmonkey99/arkade/sdk/cartlib"

const manifestJSON = `{"abi":1,"slug":"tidy","name":"Tidy","description":"A clean fixture cartridge.","version":"1.0.0","players":[1]}`

var manifestBytes = []byte(manifestJSON)

type tidyGame struct{}

func (tidyGame) Manifest() []byte { return manifestBytes }

// Init is where every bit of setup lives — the whole point of rule 1.
func (tidyGame) Init(cols, rows int) {
	elapsed = 0
	seed = cartlib.RandomU64()
}

func (tidyGame) HandleInput(records []cartlib.InputRecord) {}

func (tidyGame) Update(dtMicros uint64) { elapsed += dtMicros }

func (tidyGame) Render(fb *cartlib.Framebuffer) {}

var (
	elapsed uint64
	seed    uint64
)

var game tidyGame

// The one blessed package-level initializer: the registration line.
var _ = cartlib.Run(game)

// A type assertion, not a call — this must not be mistaken for work.
var _ cartlib.Game = tidyGame{}

func main() {}
