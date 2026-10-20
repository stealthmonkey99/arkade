// SPDX-License-Identifier: GPL-2.0-or-later

package input

import (
	"os"
	"sync"

	"golang.org/x/term"
)

// Keyboard reads raw terminal input and emits Events. Besides plain keys it
// simulates arcade buttons: i/o/k/l map to JOY A/B/C/D, laid out like the
// panel where A/B sit above C/D, and 1/2 pick the simulated player.
type Keyboard struct {
	handler  *Handler
	oldState *term.State
	quit     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	// simIndex is -1 for a plain keyboard, else the simulated player slot.
	simIndex int
}

func NewKeyboard(h *Handler) *Keyboard {
	return &Keyboard{handler: h, quit: make(chan struct{}), done: make(chan struct{}), simIndex: 0}
}

func (k *Keyboard) SimIndex() int { return k.simIndex }

func (k *Keyboard) Start() error {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		state, err := term.MakeRaw(fd)
		if err != nil {
			return err
		}
		k.oldState = state
	}
	go k.readLoop()
	return nil
}

// Stop restores the terminal and stops the read loop. It is idempotent
// because callers stop both explicitly and via defer.
func (k *Keyboard) Stop() {
	k.stopOnce.Do(func() {
		close(k.quit)
		if k.oldState != nil {
			_ = term.Restore(int(os.Stdin.Fd()), k.oldState)
			k.oldState = nil
		}
	})
}

func (k *Keyboard) readLoop() {
	defer close(k.done)
	buf := make([]byte, 64)
	for {
		select {
		case <-k.quit:
			return
		default:
		}
		// Stdin reads block, so quit takes effect only after the next read.
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		k.parse(buf[:n])
	}
}

func (k *Keyboard) parse(b []byte) {
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x03: // ctrl-c
			k.emit(Event{Kind: KindKeypress, Name: "c", Ctrl: true})
			i++
		case c == 0x04: // ctrl-d
			k.emit(Event{Kind: KindKeypress, Name: "d", Ctrl: true})
			i++
		case c == 0x1b:
			consumed := k.parseEscape(b[i:])
			i += consumed
		case c == '\r' || c == '\n':
			k.emit(Event{Kind: KindKeypress, Name: KeyEnter})
			i++
		case c == ' ':
			k.emit(Event{Kind: KindKeypress, Name: KeySpace})
			i++
		case c >= 'A' && c <= 'Z':
			k.emitNamed(KeyName(c+32), true)
			i++
		case c >= 'a' && c <= 'z':
			k.emitNamed(KeyName(c), false)
			i++
		case c >= '0' && c <= '9':
			k.handleDigit(c)
			i++
		default:
			i++
		}
	}
}

func (k *Keyboard) parseEscape(b []byte) int {
	if len(b) == 1 || b[1] != '[' && b[1] != 'O' {
		k.emit(Event{Kind: KindKeypress, Name: KeyEscape})
		return 1
	}
	arrows := map[byte]KeyName{'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft, 'H': KeyHome, 'F': KeyEnd}
	if len(b) >= 3 {
		if name, ok := arrows[b[2]]; ok {
			k.emitDirection(name, false)
			return 3
		}
		if len(b) >= 6 && b[2] == '1' && b[3] == ';' {
			if name, ok := arrows[b[5]]; ok {
				k.emitDirection(name, b[4] == '2')
				return 6
			}
		}
	}
	return len(b)
}

func (k *Keyboard) handleDigit(c byte) {
	switch c {
	case '1':
		k.simIndex = 0
	case '2':
		k.simIndex = 1
	case '0':
		k.simIndex = -1
	}
}

func (k *Keyboard) emitNamed(name KeyName, shift bool) {
	if k.simIndex >= 0 {
		switch name {
		case "i":
			name = KeyJoyA
		case "o":
			name = KeyJoyB
		case "k":
			name = KeyJoyC
		case "l":
			name = KeyJoyD
		}
	}
	k.emit(Event{Kind: KindKeypress, Name: name, Shift: shift})
}

func (k *Keyboard) emitDirection(name KeyName, shift bool) {
	k.emit(Event{Kind: KindKeypress, Name: name, Shift: shift})
}

func (k *Keyboard) emit(ev Event) {
	if k.simIndex >= 0 {
		ev.Source = PlayerSource{
			Type: SourceController,
			ID:   VirtualControllerID(k.simIndex),
			Slot: SlotOf(k.simIndex),
			Seat: SeatOf(k.simIndex),
		}
	} else {
		ev.Source = PlayerSource{Type: SourceKeyboard, ID: "keyboard"}
	}
	k.handler.Push(ev)
}
