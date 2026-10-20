// SPDX-License-Identifier: GPL-2.0-or-later

package shell

// control is a tile the shell owns, as opposed to an app: its name and glyph,
// what the LCD says while it is focused, how the footer and legend describe
// green, and what green does.
type control struct {
	label  string
	glyph  []string // 8×8 art, '#'-keyed
	fill   []string // a second-colour overlay on the glyph, or nil
	legend string   // the 16-column shelf legend; "" reads GREEN = OPEN
	verb   string   // the footer's green verb; "" reads OPEN
	hint   string   // the footer's stick hint on a slider tile
	// slider is the panel the stick adjusts in place on the shelf and green opens.
	slider *levelPanel
	lcd    func(a *app) (line1, line2 string)
	open   func(a *app, e entry)
}

func (e entry) control() *control { return controls[e.kind] }

func controlEntry(kind entryKind) entry {
	return entry{kind: kind, label: controls[kind].label}
}

func fixedLCD(line1, line2 string) func(*app) (string, string) {
	return func(*app) (string, string) { return line1, line2 }
}

func openPanel(p *levelPanel) func(*app, entry) {
	return func(a *app, _ entry) { a.push(p) }
}

var controls = map[entryKind]*control{
	entrySearch: {
		label: "SEARCH",
		glyph: []string{
			".###....",
			"#...#...",
			"#...#...",
			"#...#...",
			".###....",
			"....##..",
			".....##.",
			"......##",
		},
		lcd:  fixedLCD("SEARCH", "find any game"),
		open: func(a *app, _ entry) { a.openSearch() },
	},
	entryVolume: {
		label: "VOLUME",
		glyph: []string{
			".....#..",
			"....##.#",
			"..####..",
			"#.####.#",
			"..####..",
			"....##.#",
			".....#..",
			"........",
		},
		legend: "<> ADJUSTS VOL",
		hint:   "VOL",
		slider: volumePanel,
		lcd:    volumeTileLCD,
		open:   openPanel(volumePanel),
	},
	entryBrightness: {
		label: "BRIGHT",
		glyph: []string{
			"...##...",
			".#....#.",
			"..####..",
			"#.####.#",
			"#.####.#",
			"..####..",
			".#....#.",
			"...##...",
		},
		legend: "<> ADJUSTS BRT",
		hint:   "BRT",
		slider: brightnessPanel,
		lcd:    brightnessTileLCD,
		open:   openPanel(brightnessPanel),
	},
	entryWifi: {
		label: "JOIN WIFI",
		glyph: []string{
			"..####..",
			".#....#.",
			"#......#",
			"..####..",
			".#....#.",
			"........",
			"...##...",
			"...##...",
		},
		lcd:  fixedLCD("JOIN WIFI", "scan to connect"),
		open: func(a *app, _ entry) { a.openWifi() },
	},
	entryBluetooth: {
		label: "BLUETOOTH",
		glyph: []string{
			"...##...",
			"...###..",
			"#..#..#.",
			".##.##..",
			"..###...",
			".##.##..",
			"#..#..#.",
			"...###..",
		},
		lcd:  fixedLCD("BLUETOOTH", "pads and seats"),
		open: func(a *app, e entry) { a.launch(e.m) },
	},
	entryTemp: {
		label: "TEMP",
		glyph: []string{
			"..###...",
			"..#.#...",
			"..#.#...",
			"..#.#...",
			"..#.#...",
			".#...#..",
			".#...#..",
			"..###...",
		},
		fill: []string{
			"........",
			"........",
			"........",
			"...#....",
			"...#....",
			"..###...",
			"..###...",
			"........",
		},
		lcd:  fanTileLCD,
		open: openPanel(fanPanel),
	},
	entrySleep: {
		label: "SLEEP",
		glyph: []string{
			"######..",
			"....#...",
			"..#.....",
			"######..",
			".....###",
			"......#.",
			".....###",
			"........",
		},
		legend: "GREEN = SLEEP",
		verb:   "SLEEP",
		lcd:    fixedLCD("SLEEP", "rest the screen"),
		open:   func(a *app, _ entry) { a.ctx.EnterSleep() },
	},
	entryShutdown: {
		label: "SHUT DOWN",
		glyph: []string{
			"...##...",
			"...##...",
			".#.##.#.",
			"#..##..#",
			"#......#",
			"#......#",
			".#....#.",
			"..####..",
		},
		legend: "GREEN = SHUTDOWN",
		verb:   "SHUTDOWN",
		lcd:    fixedLCD("SHUT DOWN", "power off"),
		open:   func(a *app, _ entry) { a.push(confirmOff) },
	},
}
