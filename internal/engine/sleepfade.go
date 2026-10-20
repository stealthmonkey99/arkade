// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Goodnight timeline in milliseconds from entering DARK. Only deliberate
// darks play it; the idle-timeout dark cuts straight to black.
const (
	sleepHintFadeInMs = 900
	sleepHintFullMs   = 1800
	sleepFadeStartMs  = 3000
	sleepFadeMs       = 1200
	sleepTotalMs      = sleepFadeStartMs + sleepFadeMs
)

var sleepLabel = display.Pixel{R: 132, G: 136, B: 144, A: 255}

var sleepZColor = display.Pixel{R: 186, G: 170, B: 255, A: 255}

// Three sizes of '#'-keyed Z.
var sleepZArt = [][]string{
	{
		"#####",
		"...#.",
		"..#..",
		".#...",
		"#####",
	},
	{
		"#######",
		".....#.",
		"....#..",
		"...#...",
		"..#....",
		".#.....",
		"#######",
	},
	{
		"#########",
		"......##.",
		".....##..",
		"....##...",
		"...##....",
		"..##.....",
		".##......",
		"##.......",
		"#########",
	},
}

// Zzz loop geometry, tuned for the 64×64 panel; sleepBlit clips anything else.
const (
	sleepZLoopMs = 2400
	sleepZX      = 20
	sleepZY      = 30
	sleepZRise   = 22
	sleepZDrift  = 14
	sleepZEdge   = 0.2 // the head and tail of a loop, where a Z fades

	sleepHintY    = 42
	sleepHintStep = 9
)

// sleepFadeFrame renders the goodnight at elapsedMs since DARK began: rising
// Zs, the wake hint fading in, then the frame fading to black. elapsedMs >=
// sleepTotalMs renders pure black, which is how stepDarkFade knows it is done.
// Pure function of time.
func sleepFadeFrame(w, h int, elapsedMs float64) display.Matrix {
	out := display.NewMatrix(w, h, display.Black)
	if elapsedMs < 0 {
		elapsedMs = 0
	}

	// Nothing is drawn behind it, so scaling every alpha by fade is the fade to black.
	fade := 1.0
	if elapsedMs > sleepFadeStartMs {
		fade = math.Max(0, 1-(elapsedMs-sleepFadeStartMs)/sleepFadeMs)
	}
	if fade <= 0 {
		return out
	}

	for i, art := range sleepZArt {
		phase := math.Mod(elapsedMs/sleepZLoopMs+float64(i)/float64(len(sleepZArt)), 1)
		alpha := math.Min(1, math.Min(phase, 1-phase)/sleepZEdge)
		sleepBlit(out, keyedSprite(art, sleepZColor),
			sleepZX+int(math.Round(phase*sleepZDrift)),
			sleepZY-int(math.Round(phase*sleepZRise)),
			alpha*fade)
	}

	hint := clamp01((elapsedMs - sleepHintFadeInMs) / (sleepHintFullMs - sleepHintFadeInMs))
	if hint > 0 {
		for i, line := range []string{"PRESS ANY KEY", "TO WAKE ME"} {
			sleepBlit(out, display.TextSprite(line, sleepLabel),
				(w-ui.FontLED{}.TextWidth(line, 1))/2, sleepHintY+i*sleepHintStep, hint*fade)
		}
	}
	return out
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

func keyedSprite(art []string, c display.Pixel) display.Sprite {
	out := make(display.Sprite, len(art))
	for y, line := range art {
		row := make([]display.Pixel, len(line))
		for x, ch := range line {
			if ch == '#' {
				row[x] = c
			}
		}
		out[y] = row
	}
	return out
}

// sleepBlit composites src at (ox,oy) into dst with every pixel's alpha scaled
// by alpha.
func sleepBlit(dst display.Matrix, src display.Sprite, ox, oy int, alpha float64) {
	if alpha <= 0 {
		return
	}
	for y, row := range src {
		dy := oy + y
		if dy < 0 || dy >= len(dst) {
			continue
		}
		for x, p := range row {
			dx := ox + x
			if dx < 0 || dx >= len(dst[dy]) || p.A == 0 {
				continue
			}
			dst[dy][dx] = display.Merge(dst[dy][dx], p.ScaleAlpha(alpha))
		}
	}
}
