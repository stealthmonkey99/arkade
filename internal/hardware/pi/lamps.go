// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"fmt"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// Bit-angle modulation: a 100 Hz frame of four slots of 1, 2, 4 and 8 units,
// a lamp on in slot k iff bit k of its level is set. Sigma-delta flickered on
// the cabinet.
const (
	bamSlots    = 4
	bamLevelMax = 1<<bamSlots - 1
	bamFrame    = 10 * time.Millisecond
	bamUnit     = bamFrame / bamLevelMax
)

// bamLevel quantizes a hardware level to four bits. A lit lamp never rounds
// to dark.
func bamLevel(level uint8) uint8 {
	q := level >> 4
	if q == 0 && level > 0 {
		q = 1
	}
	return q
}

// lampWriter owns the expander's 16 output latches, the fan's included, as
// 4-bit BAM levels. Binary states cost one transaction; anything dimmed runs
// the frame loop.
type lampWriter struct {
	mux    *mcp23017
	logger func(level, msg string)
	sleep  func(time.Duration)

	mu      sync.Mutex
	levels  [2 * muxPinsPerPort]uint8
	running bool

	wake   chan struct{}
	stopCh chan struct{}
	done   chan struct{}
}

func newLampWriter(mux *mcp23017, logger func(level, msg string), sleep func(time.Duration)) *lampWriter {
	return &lampWriter{
		mux:    mux,
		logger: logger,
		sleep:  sleep,
		wake:   make(chan struct{}, 1),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// start hands the latches to a goroutine; before it, sets apply inline.
func (w *lampWriter) start() {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()
	go w.run()
}

// setLamps replaces one seat's six lamps: pins 0-5 on port A for seat 0, 8-13
// on port B for seat 1.
func (w *lampWriter) setLamps(seat int, levels hardware.LampLevels) {
	if seat < 0 || seat > 1 {
		return
	}
	base := seat * muxPinsPerPort
	w.set(func(l *[2 * muxPinsPerPort]uint8) {
		for i, v := range levels {
			l[base+i] = bamLevel(v)
		}
	})
}

func (w *lampWriter) setPin(pin int, on bool) {
	if pin < 0 || pin >= 2*muxPinsPerPort {
		w.logger("error", fmt.Sprintf("lamps: pin %d out of range", pin))
		return
	}
	w.set(func(l *[2 * muxPinsPerPort]uint8) {
		l[pin] = 0
		if on {
			l[pin] = bamLevelMax
		}
	})
}

func (w *lampWriter) set(mutate func(*[2 * muxPinsPerPort]uint8)) {
	w.mu.Lock()
	mutate(&w.levels)
	running := w.running
	w.mu.Unlock()
	if !running {
		w.apply()
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *lampWriter) apply() {
	if levels, binary := w.snapshot(); binary {
		w.writeSlot(levels, 0)
		return
	}
	w.frame()
}

func (w *lampWriter) run() {
	defer close(w.done)
	for {
		levels, binary := w.snapshot()
		if !binary {
			w.frame()
			if w.stopping() {
				w.shutdown()
				return
			}
			continue
		}
		w.writeSlot(levels, 0)
		select {
		case <-w.stopCh:
			w.shutdown()
			return
		case <-w.wake:
		}
	}
}

// frame emits one 100 Hz BAM frame, sleeping to each slot's deadline
// measured from the frame start so a slow write cannot push the next slot.
func (w *lampWriter) frame() {
	start := time.Now()
	levels, _ := w.snapshot()
	for slot := 0; slot < bamSlots; slot++ {
		w.writeSlot(levels, slot)
		w.sleep(time.Until(start.Add(slotEnd(slot))))
	}
}

// shutdown leaves the latches on the frame's top slot, lighting the lamps at
// level 128 and above.
func (w *lampWriter) shutdown() {
	levels, _ := w.snapshot()
	w.writeSlot(levels, bamSlots-1)
}

func (w *lampWriter) stop() {
	w.mu.Lock()
	running := w.running
	w.running = false
	w.mu.Unlock()
	if !running {
		return
	}
	close(w.stopCh)
	<-w.done
}

func (w *lampWriter) stopping() bool {
	select {
	case <-w.stopCh:
		return true
	default:
		return false
	}
}

func (w *lampWriter) snapshot() (levels [2 * muxPinsPerPort]uint8, binary bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	binary = true
	for _, v := range w.levels {
		if v != 0 && v != bamLevelMax {
			binary = false
			break
		}
	}
	return w.levels, binary
}

func (w *lampWriter) writeSlot(levels [2 * muxPinsPerPort]uint8, slot int) {
	a, b := slotLatches(levels, slot)
	if err := w.mux.writeLatches(a, b); err != nil {
		w.logger("error", fmt.Sprintf("lamps: latch write: %v", err))
	}
}

// slotLatches projects quantized levels onto the two ports for one BAM slot.
// Slot 0 is also the binary projection.
func slotLatches(levels [2 * muxPinsPerPort]uint8, slot int) (a, b byte) {
	ports := [2]byte{}
	for pin, level := range levels {
		if level&(1<<uint(slot)) != 0 {
			ports[pin/muxPinsPerPort] |= 1 << uint(pin%muxPinsPerPort)
		}
	}
	return ports[0], ports[1]
}

// slotEnd is the offset from the frame start at which slot ends, at 1, 3, 7
// and 15 units.
func slotEnd(slot int) time.Duration {
	return bamUnit * time.Duration(1<<uint(slot+1)-1)
}
