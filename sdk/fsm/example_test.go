// SPDX-License-Identifier: GPL-2.0-or-later

package fsm_test

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/sdk/fsm"
)

const (
	stTitle fsm.State = iota
	stPlay
	stOver
)

var names = [...]string{stTitle: "title", stPlay: "play", stOver: "over"}

var legalEdges = []uint16{
	stTitle: 1 << stPlay,
	stPlay:  1 << stOver,
	stOver:  1 << stTitle,
}

func enterScreen(prev, next fsm.State) {
	fmt.Println(names[prev], "->", names[next])
}

func ExampleMachine() {
	var m fsm.Machine
	m.OnEnter(enterScreen)
	m.Edges(legalEdges)
	m.Reset(stTitle)

	m.Go(stPlay)
	m.Tick(500_000)
	fmt.Println("elapsed:", m.Elapsed())
	fmt.Println("play -> title allowed:", m.Go(stTitle))
	m.Go(stOver)
	m.Go(stTitle)
	fmt.Println("blocked:", m.Blocked())
	// Output:
	// title -> title
	// title -> play
	// elapsed: 500000
	// play -> title allowed: false
	// play -> over
	// over -> title
	// blocked: 1
}
