// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	btlib "github.com/stealthmonkey99/arkade/internal/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

const btRows = listSlots

const btRefresh = 900 * time.Millisecond

const doneLingerMs = 1800 * time.Millisecond

type btPhase int

const (
	btScanning btPhase = iota // discovery running, nothing found yet
	btListing                 // devices on screen, cursor live
	btPairing                 // a pairing is in flight
	btDone                    // it worked, or it didn't; message says which
)

// scanScene pairs a wireless controller. The LCD shows the selected device's
// full name, which does not fit the panel's ~10 characters per row.
//
// Pairing runs on its own goroutine (BlueZ times out after 30 seconds) so the
// frame loop never blocks.
type scanScene struct {
	a   *app
	bus *bus

	header    *element.Label
	rows      []*element.SpriteEl
	spin      *element.SpriteEl
	spinPhase int
	spinMs    float64

	// mu guards everything the pairing goroutine touches.
	mu      sync.Mutex
	phase   btPhase
	devices []btlib.Device
	cursor  int
	message string

	lastPoll time.Time
	cancel   context.CancelFunc
	hold     ui.HoldRepeat
	// discovery requested; Enter cannot ask because the bus may still be connecting
	started bool
	// keeps rows stable as the list grows
	rank   btlib.Ranker
	doneAt time.Time
}

func (s *scanScene) Init() error {
	s.header = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 2, OffsetY: 2},
		Text:    "BLUETOOTH",
	})
	elements := []display.Element{s.header}
	s.rows = make([]*element.SpriteEl, btRows)
	for i := range s.rows {
		s.rows[i] = element.NewSprite(element.SpriteOptions{Options: element.Options{OffsetX: 2, OffsetY: listTop + i*rowH}, Data: blank(rowW, rowH)})
		elements = append(elements, s.rows[i])
	}
	s.spin = element.NewSprite(element.SpriteOptions{Options: element.Options{OffsetX: 26, OffsetY: 26}, Data: blank(spinnerSize, spinnerSize)})
	s.spin.SetVisible(false)
	elements = append(elements, s.spin)
	s.a.Ctx.AddLayer(modeAdd, elements, 0)
	return nil
}

func (s *scanScene) Enter() {
	s.mu.Lock()
	s.phase = btScanning
	s.devices = nil
	s.cursor = 0
	s.message = ""
	s.mu.Unlock()

	s.started = false
	s.hold.Reset()
	s.rank = btlib.Ranker{}
	s.lastPoll = time.Time{}
	s.bus.open()
}

// beginScan reports a cabinet with no radio outright rather than showing a
// list that stays empty.
func (s *scanScene) beginScan() {
	if s.started {
		return
	}
	switch state, _ := s.bus.snapshot(); state {
	case busMissing:
		s.started = true
		s.setDone("no bluetooth on this cabinet")
		return
	case busReady:
	default:
		return // still connecting
	}
	bt := s.bus.live()
	if bt == nil {
		return
	}
	s.started = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		if err := bt.StartScan(ctx); err != nil {
			// the LCD gets 16 characters; the journal gets the whole error
			s.logf("bluetooth: scan failed: %v", err)
			s.setDone(shortErr(err))
		}
	}()
}

// Teardown stops discovering. The app, not the scene, closes the pairer.
func (s *scanScene) Teardown() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.started = false
	if bt := s.bus.live(); bt != nil {
		go bt.StopScan()
	}
}

func (s *scanScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeAdd {
		return false
	}
	// a held stick is one joystickchange, not a stream of presses
	if ev.Kind == input.KindJoystickChange {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.hold.Hold(element.StickDir(ev.Position))
		return true
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != btListing || len(s.devices) == 0 {
		return false
	}
	switch ev.Name {
	case input.KeyUp, input.KeyDown:
		s.stepLocked(ev.Name)
		return true
	case input.KeyJoyA, input.KeyEnter:
		s.startPairLocked()
		return true
	}
	return false
}

func (s *scanScene) stepLocked(dir input.KeyName) {
	switch dir {
	case input.KeyUp:
		if s.cursor > 0 {
			s.cursor--
		}
	case input.KeyDown:
		if s.cursor < len(s.devices)-1 {
			s.cursor++
		}
	}
}

// tickHold also advances the spinner while a pairing runs.
func (s *scanScene) tickHold(stats clock.FrameTime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == btPairing {
		s.spinMs += stats.ElapsedMs
		for s.spinMs >= spinnerStepMs {
			s.spinPhase = (s.spinPhase + 1) % len(spinnerRing)
			s.spinMs -= spinnerStepMs
		}
	}
	if s.phase != btListing {
		return
	}
	for n := s.hold.Update(element.ElapsedMicros(stats)); n > 0; n-- {
		s.stepLocked(element.Key(s.hold.Held()))
	}
}

func (s *scanScene) startPairLocked() {
	bt := s.bus.live()
	if bt == nil {
		return
	}
	dev := s.devices[s.cursor]
	s.phase = btPairing
	s.message = "pairing…"
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		err := bt.Pair(ctx, dev.Address)
		if err != nil {
			s.logf("bluetooth: pairing %s: %v", dev.Address, err)
			s.setDone(shortErr(err))
			return
		}
		s.logf("bluetooth: paired %s (%s)", dev.Address, btlib.DisplayName(dev))
		s.setDone("paired: " + btlib.DisplayName(dev))
	}()
}

// logf writes to the journal; nil-safe on the app so tests need no engine.
func (s *scanScene) logf(format string, args ...any) {
	if s.a == nil {
		return
	}
	s.a.Ctx.Log(fmt.Sprintf(format, args...))
}

func (s *scanScene) setDone(msg string) {
	s.mu.Lock()
	s.phase = btDone
	s.message = msg
	s.doneAt = time.Now()
	s.mu.Unlock()
}

func (s *scanScene) Update(stats clock.FrameTime) {
	s.tickHold(stats)
	s.mu.Lock()
	phase, doneAt := s.phase, s.doneAt
	s.mu.Unlock()
	if phase == btDone && !doneAt.IsZero() && time.Since(doneAt) >= doneLingerMs {
		s.mu.Lock()
		s.doneAt = time.Time{}
		s.mu.Unlock()
		if err := s.a.SceneMgr.Transition("back"); err != nil {
			s.a.Ctx.Log(fmt.Sprintf("bluetooth: returning from the scan: %v", err))
		}
		return
	}
	if phase == btScanning || phase == btListing {
		s.bus.open()
		s.beginScan()
		now := time.Now()
		if now.Sub(s.lastPoll) >= btRefresh {
			s.lastPoll = now
			s.bus.poll()
		}
		s.refresh()
	}
	s.paint()
}

// refresh re-reads the device list, keeping the cursor on the same device
// rather than the same row, since the list reorders as signal settles.
func (s *scanScene) refresh() {
	if err := s.bus.takeErr(); err != nil {
		s.logf("bluetooth: listing devices: %v", err)
		s.setDone(shortErr(err))
		return
	}
	_, devs := s.bus.snapshot()
	s.rank.Sort(devs)
	s.mu.Lock()
	defer s.mu.Unlock()
	var wasOn string
	if s.cursor < len(s.devices) {
		wasOn = s.devices[s.cursor].Address
	}
	s.devices = devs
	if len(devs) > 0 && s.phase == btScanning {
		s.phase = btListing
	}
	s.cursor = 0
	for i, d := range devs {
		if d.Address == wasOn {
			s.cursor = i
			break
		}
	}
}

// wantLamps lights green only when the cursor is on a pairable device; red
// always backs out.
func (s *scanScene) wantLamps(phase btPhase, devs []btlib.Device, cursor int) lampSet {
	if phase == btListing && cursor >= 0 && cursor < len(devs) {
		return lampSet{a: true, b: true}
	}
	return lampSet{b: true}
}

func (s *scanScene) paint() {
	s.mu.Lock()
	phase, devs, cursor, message := s.phase, s.devices, s.cursor, s.message
	s.mu.Unlock()
	s.a.Ctx.SetButtonLights(input.BothSeats, s.wantLamps(phase, devs, cursor).lights(), true)

	switch phase {
	case btScanning:
		s.header.SetText("SCANNING…")
	case btPairing:
		s.header.SetText("PAIRING…")
	case btDone:
		s.header.SetText("BLUETOOTH")
	default:
		s.header.SetText(fmt.Sprintf("FOUND %d", len(devs)))
	}

	locked := phase == btPairing
	s.spin.SetVisible(locked)
	if locked {
		setArt(s.spin, spinnerArt(s.spinPhase, display.FromUI(ui.ButtonBlue)))
	}

	start := 0
	if cursor >= btRows {
		start = cursor - btRows + 1
	}
	for i := range s.rows {
		idx := start + i
		if idx >= len(devs) {
			setArt(s.rows[i], blank(rowW, rowH))
			continue
		}
		art := scanRowArt(devs[idx], idx == cursor && !locked)
		if locked {
			art = dimSprite(art, 35)
		}
		setArt(s.rows[i], art)
	}

	lcd1, lcd2 := btLCD(phase, devs, cursor, message)
	s.a.Ctx.SetText(input.BothSeats, lcd1, lcd2)
}

func (s *scanScene) Render(clock.FrameTime, scene.RenderReason) {}

func btLCD(phase btPhase, devs []btlib.Device, cursor int, message string) (string, string) {
	switch phase {
	case btScanning:
		return "Scanning…", "hold SHARE+PS"
	case btPairing, btDone:
		return message, ""
	}
	if cursor >= len(devs) {
		return "No devices", "hold SHARE+PS"
	}
	d := devs[cursor]
	// the buttons carry no letters, so the hint names the colour
	state := "GREEN = PAIR"
	switch {
	case d.Connected:
		state = "connected"
	case d.Paired:
		state = "paired"
	}
	return btlib.DisplayName(d), state
}

// shortErr trims an error to fit the 16-column LCD.
func shortErr(err error) string {
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "bluetooth: ")
	if i := strings.Index(msg, ":"); i > 0 && i < len(msg)-1 {
		msg = msg[:i]
	}
	if len(msg) > 16 {
		msg = msg[:16]
	}
	return msg
}
