// SPDX-License-Identifier: GPL-2.0-or-later

package morning

import (
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/weather"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

// Big digit font metrics: 11 rows tall, digits 7 columns wide, colon 2.
const (
	bigGlyphH      = 11
	bigGlyphGap    = 2
	meridiemGap    = 3
	meridiemDrop   = 6
	smallGlyphH    = 5
	smallAdvance   = 4
	weatherIconPx  = 11
	sunriseMarkW   = 7
	sunriseMarkGap = 2
	condGap        = 4
	condDrop       = 3 // centres the five-row temperature against the eleven-row icon
)

var bigGlyphs = map[rune][]string{
	'0': {".#####.", "##...##", "##...##", "##...##", "##...##", "##...##", "##...##", "##...##", "##...##", "##...##", ".#####."},
	'1': {"...##..", "..###..", ".####..", "...##..", "...##..", "...##..", "...##..", "...##..", "...##..", "...##..", ".######"},
	'2': {".#####.", "##...##", ".....##", ".....##", "....##.", "...##..", "..##...", ".##....", "##.....", "##.....", "#######"},
	'3': {".#####.", "##...##", ".....##", ".....##", "..####.", ".....##", ".....##", ".....##", ".....##", "##...##", ".#####."},
	'4': {"....##.", "...###.", "..####.", ".##.##.", "##..##.", "##..##.", "#######", "....##.", "....##.", "....##.", "....##."},
	'5': {"#######", "##.....", "##.....", "##.....", "######.", ".....##", ".....##", ".....##", ".....##", "##...##", ".#####."},
	'6': {".#####.", "##...##", "##.....", "##.....", "######.", "##...##", "##...##", "##...##", "##...##", "##...##", ".#####."},
	'7': {"#######", ".....##", ".....##", "....##.", "....##.", "...##..", "...##..", "..##...", "..##...", "..##...", "..##..."},
	'8': {".#####.", "##...##", "##...##", "##...##", ".#####.", "##...##", "##...##", "##...##", "##...##", "##...##", ".#####."},
	'9': {".#####.", "##...##", "##...##", "##...##", "##...##", ".######", ".....##", ".....##", ".....##", "##...##", ".#####."},
	':': {"..", "..", "..", "##", "##", "..", "..", "##", "##", "..", ".."},
}

// bigTextWidth ignores runes the font doesn't cover.
func bigTextWidth(s string) int {
	w := 0
	for _, r := range s {
		g, ok := bigGlyphs[r]
		if !ok {
			continue
		}
		if w > 0 {
			w += bigGlyphGap
		}
		w += len(g[0])
	}
	return w
}

// smallWidth is ui.Font3x5's rendered width for an n-character string.
func smallWidth(n int) int {
	if n <= 0 {
		return 0
	}
	return n*smallAdvance - 1
}

type inkRun struct {
	text   string
	ink    display.Pixel
	degree bool
}

func (r inkRun) advance() int {
	a := len(r.text) * smallAdvance
	if r.degree {
		a += degreeW + 1
	}
	return a
}

func smallSprite(s string, c display.Pixel) display.Sprite {
	return smallRuns(inkRun{text: s, ink: c})
}

// smallRuns sets runs end to end as one sprite, so a multi-ink line fades as one element.
func smallRuns(runs ...inkRun) display.Sprite {
	w := -1
	for _, r := range runs {
		w += r.advance()
	}
	if w <= 0 {
		return display.Sprite{{}}
	}
	cv, sprite := display.NewSpriteCanvas(w, smallGlyphH)
	x := 0
	for _, r := range runs {
		ui.Font3x5{}.DrawText(cv, x, 0, r.text, 1, display.ToUI(r.ink))
		if r.degree {
			drawDegree(sprite, x+len(r.text)*smallAdvance, 0, r.ink)
		}
		x += r.advance()
	}
	return sprite
}

// sunriseMark is a dome half out of the horizon, five rows to sit on the small font's baseline.
var sunriseMark = []string{
	".......",
	".......",
	"..SSS..",
	".SSSSS.",
	"HHHHHHH",
}

// degreeW is a hand-drawn 2x2 block: Font3x5 walks bytes, so "°" would draw
// nothing yet advance two cells.
const degreeW = 2

func drawDegree(sprite display.Sprite, x, y int, ink display.Pixel) {
	for dy := range degreeW {
		for dx := range degreeW {
			sprite[y+dy][x+dx] = ink
		}
	}
}

// conditionsSprite sets the sky icon and temperature side by side as one
// sprite. A nil icon leaves the temperature alone.
func conditionsSprite(icon display.Sprite, value, unit string, ink display.Pixel) display.Sprite {
	x := 0
	if len(icon) > 0 {
		x = weatherIconPx + condGap
	}
	w := x + smallWidth(len(value)) + 1 + degreeW
	if unit != "" {
		w += 1 + smallWidth(len(unit))
	}
	cv, sprite := display.NewSpriteCanvas(w, weatherIconPx)
	for y, row := range icon {
		copy(sprite[y], row)
	}

	font := ui.Font3x5{}
	font.DrawText(cv, x, condDrop, value, 1, display.ToUI(ink))
	x += smallWidth(len(value)) + 1
	drawDegree(sprite, x, condDrop, ink)
	if unit != "" {
		font.DrawText(cv, x+degreeW+1, condDrop, unit, 1, display.ToUI(ink))
	}
	return sprite
}

func sunriseSprite(at string, sun, horizon, text display.Pixel) display.Sprite {
	cv, sprite := display.NewSpriteCanvas(
		sunriseMarkW+sunriseMarkGap+smallWidth(len(at)), smallGlyphH)
	for y, row := range sunriseMark {
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case 'S':
				sprite[y][x] = sun
			case 'H':
				sprite[y][x] = horizon
			}
		}
	}
	ui.Font3x5{}.DrawText(cv, sunriseMarkW+sunriseMarkGap, 0, at, 1, display.ToUI(text))
	return sprite
}

// clockFace is "H:MM" in the big font with the meridiem in 3x5 beside it.
type clockFace struct {
	element.Base
	digits   string
	meridiem string
	color    display.Pixel
}

func newClockFace(opts element.Options, c display.Pixel) *clockFace {
	f := &clockFace{color: c}
	f.InitBase(opts)
	f.Impl = f.renderImpl
	return f
}

func (f *clockFace) SetTime(digits, meridiem string) {
	if f.digits == digits && f.meridiem == meridiem {
		return
	}
	f.digits, f.meridiem = digits, meridiem
	f.MarkDirty()
}

func (f *clockFace) renderImpl(clock.FrameTime) display.Sprite {
	dw := bigTextWidth(f.digits)
	if dw == 0 {
		return display.Sprite{{}}
	}
	mw := smallWidth(len(f.meridiem))
	total := dw
	if mw > 0 {
		total += meridiemGap + mw
	}
	out := display.NewMatrix(total, bigGlyphH, display.Transparent)

	x := 0
	for _, r := range f.digits {
		g, ok := bigGlyphs[r]
		if !ok {
			continue
		}
		if x > 0 {
			x += bigGlyphGap
		}
		for y, row := range g {
			for i := 0; i < len(row); i++ {
				if row[i] == '#' {
					out[y][x+i] = f.color
				}
			}
		}
		x += len(g[0])
	}
	if mw > 0 {
		cv := &display.SpriteCanvas{Sprite: out}
		ui.Font3x5{}.DrawText(cv, dw+meridiemGap, meridiemDrop, f.meridiem, 1, display.ToUI(f.color))
	}
	return out
}

// Weather icons are 11x11: S is sun/lightning, C cloud/rain/snow; night variants add M for moon.
var weatherIcons = map[weather.Sky][]string{
	weather.SkyClear: {
		".....S.....",
		"..S.....S..",
		"...........",
		"....SSS....",
		"...SSSSS...",
		"S..SSSSS..S",
		"...SSSSS...",
		"....SSS....",
		"...........",
		"..S.....S..",
		".....S.....",
	},
	weather.SkyPartly: {
		".S.....S...",
		"...SSS.....",
		"..SSSSS....",
		"..SSSSS....",
		"...SSS.....",
		".S.....S...",
		"....CCC....",
		"..CCCCCCC..",
		".CCCCCCCCC.",
		"..CCCCCCC..",
		"...........",
	},
	weather.SkyCloud: {
		"...........",
		"...........",
		"....CCC....",
		"..CCCCCCC..",
		".CCCCCCCCC.",
		"CCCCCCCCCCC",
		".CCCCCCCCC.",
		"...........",
		"...........",
		"...........",
		"...........",
	},
	weather.SkyRain: {
		"...........",
		"....CCC....",
		"..CCCCCCC..",
		".CCCCCCCCC.",
		"..CCCCCCC..",
		"...........",
		"..C..C..C..",
		"..C..C..C..",
		"...........",
		".C..C..C...",
		".C..C..C...",
	},
	weather.SkySnow: {
		"...........",
		"....CCC....",
		"..CCCCCCC..",
		".CCCCCCCCC.",
		"..CCCCCCC..",
		"...........",
		"..C..C..C..",
		"...........",
		".C..C..C...",
		"...........",
		"..C..C..C..",
	},
	weather.SkyFog: {
		"...........",
		"...........",
		".CCCCCCCCC.",
		"...........",
		"CCCCCCCCC..",
		"...........",
		"..CCCCCCCCC",
		"...........",
		".CCCCCCCCC.",
		"...........",
		"...........",
	},
	weather.SkyStorm: {
		"...........",
		"....CCC....",
		"..CCCCCCC..",
		".CCCCCCCCC.",
		"..CCCCCCC..",
		"...........",
		".....SS....",
		"....SS.....",
		"...SSSSS...",
		".....SS....",
		"....SS.....",
	},
}

// nightIcons replaces the sun with a crescent for the two skies that show one.
var nightIcons = map[weather.Sky][]string{
	weather.SkyClear: {
		".....MMMM..",
		"...MMMMM...",
		"...MMMM....",
		"..MMMM.....",
		"..MMMM.....",
		"..MMMM.....",
		"..MMMM.....",
		"..MMMM.....",
		"...MMMM....",
		"...MMMMM...",
		".....MMMM..",
	},
	weather.SkyPartly: {
		"....MM.....",
		"...MMM.....",
		"..MM.......",
		"..MM.......",
		"...MMM.....",
		"....MM.....",
		"....CCC....",
		"..CCCCCCC..",
		".CCCCCCCCC.",
		"..CCCCCCC..",
		"...........",
	},
}

func weatherIcon(sky weather.Sky, night bool, sun, cloud, moon display.Pixel) display.Sprite {
	art, ok := weatherIcons[sky]
	if !ok {
		art = weatherIcons[weather.SkyCloud]
	}
	if night {
		if dark, has := nightIcons[sky]; has {
			art = dark
		}
	}
	out := display.NewMatrix(weatherIconPx, weatherIconPx, display.Transparent)
	for y, row := range art {
		for x := 0; x < len(row); x++ {
			switch row[x] {
			case 'S':
				out[y][x] = sun
			case 'C':
				out[y][x] = cloud
			case 'M':
				out[y][x] = moon
			}
		}
	}
	return out
}
