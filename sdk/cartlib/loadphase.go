// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "github.com/stealthmonkey99/arkade/sdk/ui"

// MinSplashMicros is the minimum load-phase duration, in microseconds: the
// splash or [Loader] frame shows for at least this long before the first
// Update. The phase is timed on dtMicros alone, so both hosts draw the
// same frames.
const MinSplashMicros uint64 = 500_000

type loadPhase struct {
	elapsed uint64
	done    bool
}

var load loadPhase

// tick reports whether the game may be ticked, accumulating dt while the
// splash is up.
func (p *loadPhase) tick(dtMicros uint64) bool {
	if p.done {
		return true
	}
	p.elapsed += dtMicros
	return false
}

// draw draws the splash into fb and reports true while the load phase is
// running. Readiness decided here takes effect on the next tick.
func (p *loadPhase) draw(g Game, fb *Framebuffer) bool {
	if p.done {
		return false
	}
	if l, ok := g.(Loader); ok {
		ready := l.LoadFrame(p.elapsed, fb)
		p.done = ready && p.elapsed >= MinSplashMicros
		return true
	}
	drawDefaultSplash(fb, g)
	p.done = p.elapsed >= MinSplashMicros
	return true
}

func drawDefaultSplash(fb *Framebuffer, g Game) {
	ui.Splash{
		W:     FBWidth,
		H:     FBHeight,
		Title: manifestName(g.Manifest()),
	}.Draw(fb)
}

// manifestName scans for the first "name" string by hand, keeping
// encoding/json's reflection out of the TinyGo binary. It returns "" if
// none is found.
func manifestName(manifest []byte) string {
	const key = `"name"`
	i := indexOf(manifest, key)
	if i < 0 {
		return ""
	}
	j := i + len(key)
	for j < len(manifest) && (manifest[j] == ' ' || manifest[j] == ':') {
		j++
	}
	if j >= len(manifest) || manifest[j] != '"' {
		return ""
	}
	j++
	start := j
	for j < len(manifest) && manifest[j] != '"' {
		j++
	}
	return string(manifest[start:j])
}

func indexOf(hay []byte, needle string) int {
	n := len(needle)
	for i := 0; i+n <= len(hay); i++ {
		match := true
		for k := 0; k < n; k++ {
			if hay[i+k] != needle[k] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// DispatchAttract calls the registered Game's [Attractor.Attract] and sends
// the frame to the host. It is the body of a cartridge's cart_attract
// export (see [Attractor]) and does nothing if the Game is not an
// Attractor.
func DispatchAttract(dtMicros uint64) {
	if g, ok := currentGame.(Attractor); ok {
		g.Attract(dtMicros, &frame)
		frame.blit()
	}
}
