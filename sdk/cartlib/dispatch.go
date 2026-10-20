// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

// The cart_* export bodies, shared by the tinygo exports and the host-Go
// Cart* drivers so both toolchains run one lifecycle.

var setup Setup

func dispatchInit() {
	load = loadPhase{}
	cheatTainted = false
	setup.live = true
	currentGame.Init(&setup)
	setup.live = false
}

func dispatchHandleInput(buf []byte) {
	n := DecodeInputRecordsInto(buf, recordBuf[:])
	records := recordBuf[:n]
	if !ownsQuitKeys(currentGame) && quitPressed(records) {
		Exit()
	}
	currentGame.HandleInput(records)
}

func dispatchUpdate(dtMicros uint64) {
	if load.tick(dtMicros) {
		currentGame.Update(dtMicros)
	}
}

func dispatchRender() {
	if !load.draw(currentGame, &frame) {
		currentGame.Render(&frame)
	}
	frame.blit()
}

func dispatchTeardown() {
	if t, ok := currentGame.(Teardowner); ok {
		t.Teardown()
	}
}
