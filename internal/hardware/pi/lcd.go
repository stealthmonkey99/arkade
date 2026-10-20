// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// HD44780 commands and the PCF8574 backpack layout: data on the high nibble,
// control on the low bits.
const (
	lcdCmdClear          = 0x01
	lcdCmdEntryMode      = 0x06 // increment cursor, no display shift
	lcdCmdDisplayControl = 0x0c // display on, cursor off, blink off
	lcdCmdFunctionSet    = 0x28 // 4-bit, 2 lines, 5x8 font
	lcdCmdSetCGRAM       = 0x40
	lcdCmdSetDDRAMRow0   = 0x80
	lcdCmdSetDDRAMRow1   = 0xc0

	pcfRS        = 0x01
	pcfEnable    = 0x04
	pcfBacklight = 0x08 // held high except when the level rounds dark
)

// The backlight uses the lamps' 8-bit scale and 4-bit bit-angle modulation.
// backlightHalf is where a level rounds lit for writes that carry the pin as
// a single bit.
const (
	backlightFull = hardware.LampLevelMax
	backlightHalf = (backlightFull + 1) / 2
)

const scrollTick = 250 * time.Millisecond

// scrollLeadIn is the ticks a new or wrapped line holds still: 2.5s.
const scrollLeadIn = 10

// hd44780 is a 16x2 character LCD behind a PCF8574 I2C backpack in 4-bit
// mode. A full redraw is ~136 bus transactions, tens of milliseconds, so once
// a screen is wired up only its screenWorker touches it.
type hd44780 struct {
	bus   I2CBus
	addr  uint16
	sleep func(time.Duration)

	backlight uint8 // 0..backlightFull
}

// newHD44780 runs the 4-bit wake-up and leaves the display cleared and on.
func newHD44780(bus I2CBus, addr uint16, sleep func(time.Duration)) (*hd44780, error) {
	return newHD44780At(bus, addr, sleep, backlightFull)
}

// newHD44780At is newHD44780 with the backlight level every init write
// carries.
func newHD44780At(bus I2CBus, addr uint16, sleep func(time.Duration), backlight uint8) (*hd44780, error) {
	d := &hd44780{bus: bus, addr: addr, sleep: sleep, backlight: backlight}
	d.sleep(50 * time.Millisecond) // power-on settle
	wake := []struct {
		nib  byte
		wait time.Duration
	}{
		{0x3, 4500 * time.Microsecond},
		{0x3, 4500 * time.Microsecond},
		{0x3, 150 * time.Microsecond},
		{0x2, 150 * time.Microsecond},
	}
	for _, w := range wake {
		if err := d.writeNibble(w.nib, false); err != nil {
			return nil, err
		}
		d.sleep(w.wait)
	}
	for _, cmd := range []byte{lcdCmdFunctionSet, lcdCmdDisplayControl} {
		if err := d.command(cmd); err != nil {
			return nil, err
		}
	}
	if err := d.clear(); err != nil {
		return nil, err
	}
	if err := d.command(lcdCmdEntryMode); err != nil {
		return nil, err
	}
	return d, nil
}

// writeNibble carries the backlight as one bit, so a dimmed level rounds to
// its nearest binary state for the write; a full redraw is ~40ms, visible as
// a brief step in a fade.
func (d *hd44780) writeNibble(nib byte, rs bool) error {
	b := nib << 4
	if d.backlight >= backlightHalf {
		b |= pcfBacklight
	}
	if rs {
		b |= pcfRS
	}
	if err := d.bus.Tx(d.addr, []byte{b | pcfEnable}, nil); err != nil {
		return err
	}
	d.sleep(time.Microsecond) // enable pulse width
	if err := d.bus.Tx(d.addr, []byte{b}, nil); err != nil {
		return err
	}
	d.sleep(50 * time.Microsecond) // command settle
	return nil
}

func (d *hd44780) writeByte(b byte, rs bool) error {
	if err := d.writeNibble(b>>4, rs); err != nil {
		return err
	}
	return d.writeNibble(b&0x0f, rs)
}

func (d *hd44780) command(b byte) error { return d.writeByte(b, false) }
func (d *hd44780) data(b byte) error    { return d.writeByte(b, true) }

func (d *hd44780) setBacklight(on bool) error {
	if on {
		return d.setBacklightLevel(backlightFull)
	}
	return d.setBacklightLevel(0)
}

// setBacklightLevel sets the backlight pin and the level carried by later
// writes. A binary level is one bare write; a dimmed one is a BAM frame that
// screenWorker.run repeats every 10ms.
func (d *hd44780) setBacklightLevel(level uint8) error {
	if level > backlightFull {
		level = backlightFull
	}
	changed := d.backlight != level
	d.backlight = level
	if d.backlightDimmed() {
		return d.backlightFrame()
	}
	if !changed {
		return nil
	}
	return d.backlightSlot(0)
}

// backlightDimmed checks the quantized level, so a level that rounds to a
// binary pattern is latched once instead of framed forever.
func (d *hd44780) backlightDimmed() bool {
	q := bamLevel(d.backlight)
	return q > 0 && q < bamLevelMax
}

// backlightFrame emits one 100 Hz BAM frame, sleeping to each slot's deadline
// measured from the frame start so a slow write cannot push the next slot.
func (d *hd44780) backlightFrame() error {
	start := time.Now()
	for slot := 0; slot < bamSlots; slot++ {
		if err := d.backlightSlot(slot); err != nil {
			return err
		}
		d.sleep(time.Until(start.Add(slotEnd(slot))))
	}
	return nil
}

func (d *hd44780) backlightSlot(slot int) error {
	var b byte
	if bamLevel(d.backlight)&(1<<uint(slot)) != 0 {
		b = pcfBacklight
	}
	return d.bus.Tx(d.addr, []byte{b}, nil)
}

func (d *hd44780) clear() error {
	if err := d.command(lcdCmdClear); err != nil {
		return err
	}
	d.sleep(2 * time.Millisecond) // clear needs >1.52ms
	return nil
}

// printLine writes bytes from a row's DDRAM origin; bytes past column 15 land
// off-screen for the marquee.
func (d *hd44780) printLine(row int, text []byte) error {
	addr := byte(lcdCmdSetDDRAMRow0)
	if row == 1 {
		addr = lcdCmdSetDDRAMRow1
	}
	if err := d.command(addr); err != nil {
		return err
	}
	for _, b := range text {
		if err := d.data(b); err != nil {
			return err
		}
	}
	return nil
}

// createChar uploads a 5x8 glyph into a CGRAM slot. The next printLine
// returns the address to DDRAM.
func (d *hd44780) createChar(slot int, pattern [8]byte) error {
	if err := d.command(lcdCmdSetCGRAM | byte(slot)<<3); err != nil {
		return err
	}
	for _, b := range pattern {
		if err := d.data(b); err != nil {
			return err
		}
	}
	return nil
}

type scrollAction int

const (
	scrollHold  scrollAction = iota // index still in the lead-in, no redraw
	scrollDraw                      // redraw both rows at the new offset
	scrollReset                     // wrapped: clear and redraw from the top
	scrollStop                      // nothing overflows anymore
)

// scrollStep advances the marquee index from -scrollLeadIn, one column per
// tick past 0, wrapping once it passes the longest line plus 3.
func scrollStep(line1, line2 string, idx int) (int, scrollAction) {
	longest := utf8.RuneCountInString(line1)
	if n := utf8.RuneCountInString(line2); n > longest {
		longest = n
	}
	if longest <= lcdCols {
		return idx, scrollStop
	}
	idx++
	if idx >= longest+3 {
		return -scrollLeadIn, scrollReset
	}
	if idx <= 0 {
		return idx, scrollHold
	}
	return idx, scrollDraw
}

// screenOps is how a screen's requests reach its device: applied inline by
// syncOps, or queued to the screenWorker that owns the bus.
type screenOps interface {
	text(line1, line2 string)
	backlight(level uint8)
	// marquee starts or stops the scroll ticks; ops without a ticker ignore it.
	marquee(on bool)
}

// syncOps applies each request on the caller's goroutine, one at a time, and
// never scrolls on its own; a test steps the marquee by hand.
type syncOps struct{ s *screen }

func (o syncOps) text(line1, line2 string) {
	o.s.devMu.Lock()
	defer o.s.devMu.Unlock()
	o.s.applyText(line1, line2)
}

func (o syncOps) backlight(level uint8) {
	o.s.devMu.Lock()
	defer o.s.devMu.Unlock()
	o.s.applyBacklightLevel(level)
}

func (syncOps) marquee(bool) {}

// screen layers centering, change detection, the CGRAM glyph cache, the
// marquee and the backlight level on an hd44780.
//
// mu guards lastLines and idx, which the engine goroutine reads, and is never
// held across a device write. dev, cache, cacheGen and shownGlyphs belong to
// the worker while one runs, and to devMu's holder otherwise.
type screen struct {
	dev         *hd44780
	glyphs      *glyphTable
	cache       *lruCache
	cacheGen    uint64    // the glyphs generation cache's bitmaps came from
	shownGlyphs [2][]rune // the custom runes each row's last draw paged in
	logger      func(level, msg string)

	mu        sync.Mutex
	lastLines [2]string
	idx       int

	opsMu sync.Mutex
	ops   screenOps
	devMu sync.Mutex
}

// newScreen draws through glyphs, which may be shared with the other screen of
// a pair; nil means the built-in table alone. It starts synchronous.
func newScreen(dev *hd44780, glyphs *glyphTable, logger func(level, msg string)) *screen {
	s := &screen{dev: dev, glyphs: glyphs, cache: newLRUCache(maxCGRAMSlots), logger: logger}
	s.cacheGen = glyphs.view().gen
	s.ops = syncOps{s}
	return s
}

func (s *screen) currentOps() screenOps {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()
	return s.ops
}

// startWorker hands all device I/O to a goroutine, so setText and
// setBacklightLevel never block the caller on the I2C bus. marquee enables the
// scroll ticker. Idempotent.
func (s *screen) startWorker(marquee bool) {
	s.opsMu.Lock()
	defer s.opsMu.Unlock()
	if _, running := s.ops.(*screenWorker); running {
		return
	}
	s.ops = newScreenWorker(s, marquee)
}

// stopWorker joins the worker, leaving the caller sole owner of the device.
func (s *screen) stopWorker() {
	s.opsMu.Lock()
	w, running := s.ops.(*screenWorker)
	s.ops = syncOps{s}
	s.opsMu.Unlock()
	if running {
		w.stop()
	}
}

// setText writes both lines: text under 16 columns is centered, longer text
// stays left-aligned and scrolls. Unchanged lines are not rewritten unless the
// glyph table has changed since they were drawn.
func (s *screen) setText(line1, line2 string) { s.currentOps().text(line1, line2) }

// applyText updates lastLines and idx under mu, then writes the device
// unlocked so window never blocks on a redraw.
func (s *screen) applyText(line1, line2 string) {
	type draw struct {
		row  int
		text string
	}
	var draws []draw
	_, restyled := s.refreshGlyphs()

	s.mu.Lock()
	for row, text := range [2]string{line1, line2} {
		text = centerShort(text)
		if s.lastLines[row] == text && !restyled {
			continue
		}
		s.lastLines[row] = text
		s.idx = -scrollLeadIn
		draws = append(draws, draw{row, text})
	}
	overflow := utf8.RuneCountInString(s.lastLines[0]) > lcdCols || utf8.RuneCountInString(s.lastLines[1]) > lcdCols
	s.mu.Unlock()

	if len(draws) == 0 {
		return
	}
	for _, d := range draws {
		s.tryDisplay(d.row, d.text)
	}
	s.currentOps().marquee(overflow)
}

// refreshGlyphs reads the glyph table, dropping every CGRAM slot assignment
// when the table has changed since they were made. restyled reports that drop.
func (s *screen) refreshGlyphs() (view glyphView, restyled bool) {
	view = s.glyphs.view()
	if view.gen == s.cacheGen {
		return view, false
	}
	s.cache = newLRUCache(maxCGRAMSlots)
	s.shownGlyphs = [2][]rune{}
	s.cacheGen = view.gen
	return view, true
}

func centerShort(text string) string {
	n := utf8.RuneCountInString(text)
	if n >= lcdCols {
		return text
	}
	return strings.Repeat(" ", (lcdCols-n)/2) + text
}

// step performs one marquee tick; false means nothing is left to scroll.
func (s *screen) step() bool {
	s.mu.Lock()
	idx, act := scrollStep(s.lastLines[0], s.lastLines[1], s.idx)
	s.idx = idx
	s.mu.Unlock()

	switch act {
	case scrollDraw:
		s.tryScroll()
	case scrollReset:
		s.tryReset()
	case scrollStop:
		s.currentOps().marquee(false)
		return false
	}
	return true
}

func (s *screen) tryDisplay(row int, text string) {
	if s.dev == nil {
		return
	}
	if err := s.dev.printLine(row, s.substitute(row, padRight(text, lcdCols))); err != nil {
		s.logger("error", fmt.Sprintf("lcd 0x%x row %d: %v", s.dev.addr, row, err))
	}
}

func (s *screen) tryScroll() {
	if s.dev == nil {
		return
	}
	s.mu.Lock()
	from := s.idx
	lines := s.lastLines
	s.mu.Unlock()
	if from < 0 {
		from = 0
	}
	for row, line := range lines {
		win := padRight(runeWindow(line, from, lcdCols), lcdCols)
		if err := s.dev.printLine(row, s.substitute(row, win)); err != nil {
			s.logger("error", fmt.Sprintf("lcd 0x%x scroll row %d: %v", s.dev.addr, row, err))
		}
	}
}

func (s *screen) tryReset() {
	if s.dev == nil {
		return
	}
	if err := s.dev.clear(); err != nil {
		s.logger("error", fmt.Sprintf("lcd 0x%x clear: %v", s.dev.addr, err))
	}
	s.mu.Lock()
	lines := s.lastLines
	s.mu.Unlock()
	for row, line := range lines {
		s.tryDisplay(row, line)
	}
}

// substitute converts one row's text to LCD bytes, paging custom glyphs into
// CGRAM. Both rows share the 8 slots, so the other row's glyphs stay pinned;
// custom runes past the budget, and other non-ASCII, become '?'.
func (s *screen) substitute(row int, text string) []byte {
	view, _ := s.refreshGlyphs()
	inUse := make(map[rune]bool, maxCGRAMSlots)
	for _, r := range s.shownGlyphs[1-row] {
		if _, ok := s.cache.get(r); ok { // refreshed, so this row's inserts evict around it
			inUse[r] = true
		}
	}
	slots := map[rune]byte{}
	var shown []rune
	for _, r := range text {
		if _, seen := slots[r]; seen {
			continue
		}
		pattern, ok := view.lookup(r)
		if !ok || (!inUse[r] && len(inUse) >= maxCGRAMSlots) {
			continue
		}
		inUse[r] = true
		shown = append(shown, r)
		if slot, ok := s.cache.get(r); ok {
			slots[r] = byte(slot)
			continue
		}
		slot := s.cache.insert(r)
		if err := s.dev.createChar(slot, pattern); err != nil {
			s.logger("error", fmt.Sprintf("lcd 0x%x createChar %d: %v", s.dev.addr, slot, err))
		}
		slots[r] = byte(slot)
	}
	s.shownGlyphs[row] = shown
	out := make([]byte, 0, len(text))
	for _, r := range text {
		if b, ok := slots[r]; ok {
			out = append(out, b)
		} else if r < utf8.RuneSelf {
			out = append(out, byte(r))
		} else {
			out = append(out, '?')
		}
	}
	return out
}

func (s *screen) window(row int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.idx
	if from < 0 {
		from = 0
	}
	return padRight(runeWindow(s.lastLines[row], from, lcdCols), lcdCols)
}

// setBacklight switches the backlight; DDRAM keeps its text while dark.
func (s *screen) setBacklight(on bool) {
	if on {
		s.setBacklightLevel(backlightFull)
		return
	}
	s.setBacklightLevel(0)
}

func (s *screen) setBacklightLevel(level uint8) { s.currentOps().backlight(level) }

// applyBacklightLevel writes one bare byte for a binary level, one BAM frame
// for a dimmed one.
func (s *screen) applyBacklightLevel(level uint8) {
	if s.dev == nil {
		return
	}
	s.logBacklight(s.dev.setBacklightLevel(level))
}

func (s *screen) backlightDimmed() bool { return s.dev != nil && s.dev.backlightDimmed() }

func (s *screen) backlightFrame() {
	if s.dev == nil {
		return
	}
	s.logBacklight(s.dev.backlightFrame())
}

// backlightRound leaves the pin on the frame's top slot, the binary rounding
// of the modulated level, so a stopped worker leaves nothing mid-frame.
func (s *screen) backlightRound() {
	if s.dev == nil || !s.dev.backlightDimmed() {
		return
	}
	s.logBacklight(s.dev.backlightSlot(bamSlots - 1))
}

func (s *screen) logBacklight(err error) {
	if err != nil {
		s.logger("error", fmt.Sprintf("lcd 0x%x backlight: %v", s.dev.addr, err))
	}
}

// close is the teardown for abrupt exits that skipped the power-off sequence:
// stop the worker, clear, and leave the goodbye message. If that message is
// already showing the panel is untouched.
//
// lightBacklight forces the backlight fully on, for a backpack bit that cannot
// hold a dimmed level; a PCA9685 channel keeps its PWM after exit, so callers
// leave it alone.
func (s *screen) close(lightBacklight bool) {
	s.stopWorker()
	if lightBacklight {
		s.setBacklightLevel(backlightFull)
	}
	if s.showing(hardware.PowerOffLine1, hardware.PowerOffLine2) {
		return
	}
	s.devMu.Lock()
	if s.dev != nil {
		if err := s.dev.clear(); err != nil {
			s.logger("error", fmt.Sprintf("lcd 0x%x clear: %v", s.dev.addr, err))
		}
	}
	s.devMu.Unlock()
	s.setText(hardware.PowerOffLine1, hardware.PowerOffLine2)
}

func (s *screen) showing(line1, line2 string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastLines[0] == centerShort(line1) && s.lastLines[1] == centerShort(line2)
}

func padRight(s string, n int) string {
	if pad := n - utf8.RuneCountInString(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func runeWindow(s string, from, n int) string {
	runes := []rune(s)
	if from >= len(runes) {
		return ""
	}
	to := from + n
	if to > len(runes) {
		to = len(runes)
	}
	return string(runes[from:to])
}

type textReq struct{ line1, line2 string }

// screenWorker is the screenOps that owns all device I/O for a screen on one
// goroutine: text redraws, the marquee and backlight modulation. Requests
// coalesce, so a burst while it is busy collapses to the latest value.
type screenWorker struct {
	screen  *screen
	ticking bool // the marquee ticker runs

	reqMu       sync.Mutex
	pendingText *textReq
	pendingBL   *uint8

	animMu sync.Mutex
	animOn bool

	wake     chan struct{}
	flushReq chan chan struct{}
	stopCh   chan struct{}
	done     chan struct{}
}

func newScreenWorker(s *screen, marquee bool) *screenWorker {
	w := &screenWorker{
		screen:   s,
		ticking:  marquee,
		wake:     make(chan struct{}, 1),
		flushReq: make(chan chan struct{}),
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *screenWorker) run() {
	defer close(w.done)
	var tickC <-chan time.Time
	if w.ticking {
		t := time.NewTicker(scrollTick)
		defer t.Stop()
		tickC = t.C
	}
	for {
		if w.screen.backlightDimmed() {
			w.screen.backlightFrame()
			if w.stopping() {
				w.screen.backlightRound()
				return
			}
			w.betweenFrames(tickC)
			continue
		}
		select {
		case <-w.stopCh:
			return
		case <-w.wake:
			w.drain()
		case reply := <-w.flushReq:
			w.drain()
			close(reply)
		case <-tickC:
			w.tick()
		}
	}
}

// betweenFrames does queued work in the gap between modulation frames. It
// never blocks, since a frame must not start late, and a backlight write
// between a nibble's enable-high and enable-low would corrupt the latched
// character.
func (w *screenWorker) betweenFrames(tickC <-chan time.Time) {
	select {
	case <-w.wake:
		w.drain()
	case reply := <-w.flushReq:
		w.drain()
		close(reply)
	case <-tickC:
		w.tick()
	default:
	}
}

func (w *screenWorker) tick() {
	if w.isAnimating() && !w.screen.step() {
		w.setAnimating(false)
	}
}

func (w *screenWorker) stopping() bool {
	select {
	case <-w.stopCh:
		return true
	default:
		return false
	}
}

func (w *screenWorker) drain() {
	w.reqMu.Lock()
	text := w.pendingText
	w.pendingText = nil
	bl := w.pendingBL
	w.pendingBL = nil
	w.reqMu.Unlock()

	if bl != nil {
		w.screen.applyBacklightLevel(*bl)
	}
	if text != nil {
		w.screen.applyText(text.line1, text.line2)
	}
}

func (w *screenWorker) text(line1, line2 string) {
	w.reqMu.Lock()
	w.pendingText = &textReq{line1, line2}
	w.reqMu.Unlock()
	w.signal()
}

func (w *screenWorker) backlight(level uint8) {
	w.reqMu.Lock()
	w.pendingBL = &level
	w.reqMu.Unlock()
	w.signal()
}

func (w *screenWorker) marquee(on bool) { w.setAnimating(on && w.ticking) }

func (w *screenWorker) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *screenWorker) isAnimating() bool {
	w.animMu.Lock()
	defer w.animMu.Unlock()
	return w.animOn
}

func (w *screenWorker) setAnimating(on bool) {
	w.animMu.Lock()
	w.animOn = on
	w.animMu.Unlock()
}

// stop signals the run loop to exit and joins it.
func (w *screenWorker) stop() {
	close(w.stopCh)
	<-w.done
}

// flush blocks until pending requests are drained and the in-flight write has
// finished; for tests.
func (w *screenWorker) flush() {
	reply := make(chan struct{})
	w.flushReq <- reply
	<-reply
}
