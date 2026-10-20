// Fixture: a cartridge that breaks every GOLDEN RULE check_goldenrules.go
// decides. The fail path. One violation per rule, so a test can assert
// that each is reported and that nothing else is invented.
package main

import (
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/sdk/cartlib"
)

const manifestJSON = `{"abi":1,"slug":"rulebreaker","name":"Rule Breaker","description":"A fixture that breaks every rule.","version":"1.0.0","players":[1]}`

var manifestBytes = []byte(manifestJSON)

// RULE 1: a package-level initializer that does work.
var startedAt = buildClock()

var mu sync.Mutex

// RULE 1: no func init() at all.
func init() {
	tileNames[0] = "floor"
}

type breakerGame struct{}

func (breakerGame) Manifest() []byte { return manifestBytes }

func (breakerGame) Init(cols, rows int) {}

func (breakerGame) HandleInput(records []cartlib.InputRecord) {}

func (breakerGame) Update(dtMicros uint64) {
	// RULE 3: a wall clock.
	since := time.Since(startedAt)
	// RULE 4: math/rand instead of cartlib.RandomU64.
	roll := rand.Intn(6)
	// RULE 8: a libm transcendental on a per-frame path.
	wobble := math.Sin(float64(roll))
	// RULE 5: a goroutine, a channel and a select.
	done := make(chan int)
	go func() { done <- 1 }()
	select {
	case <-done:
	default:
	}
	// RULE 6: reflection-backed formatting.
	_, _ = fmt.Println(since, wobble)
	_, _ = json.Marshal(tileNames)
	_, _ = crand.Read(nil)
	_ = mu
}

func (breakerGame) Render(fb *cartlib.Framebuffer) {}

func buildClock() time.Time { return time.Now() }

var game breakerGame

var _ = cartlib.Run(game)

// RULE 1: main must stay empty.
func main() {
	println("booting")
}
