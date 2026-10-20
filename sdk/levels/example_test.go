// SPDX-License-Identifier: GPL-2.0-or-later

package levels_test

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/sdk/levels"
)

// demoPack is the shape the level generator emits: composite literals of
// constants only.
var demoPack = levels.Pack{
	Name: "demo",
	Legend: levels.Legend{
		Runes: "# @",
		Names: []string{"wall", "floor", "start"},
	},
	Levels: []levels.Level{{
		Name: "L01", W: 4, H: 3,
		Cells: "####" +
			"#@ #" +
			"####",
		Layers: []levels.Layer{
			{Name: "hazard", W: 4, H: 3, Cells: "...." + "..~." + "...."},
		},
		Records: []levels.Record{
			{Kind: "coin", X: 2, Y: 1, Args: levels.Args{{Kind: levels.KindInt, Int: 10}}},
		},
		Meta: []levels.Entry{{Key: "par", Int: 7}},
	}},
}

func ExamplePack() {
	lv := &demoPack.Levels[0]

	x, y, _ := lv.Find('@')
	fmt.Println("start:", x, y)

	par, _ := lv.Int("par")
	fmt.Println("par:", par)

	hazard := lv.Layer("hazard")
	fmt.Printf("hazard at 2,1: %c\n", hazard.At(2, 1))

	coin := lv.Record("coin")
	value, _ := coin.Args.Int(0)
	fmt.Println("coin at", coin.X, coin.Y, "worth", value)

	_, ok := lv.Str("par")
	fmt.Println("par as a string:", ok)
	// Output:
	// start: 1 1
	// par: 7
	// hazard at 2,1: ~
	// coin at 2 1 worth 10
	// par as a string: false
}
