// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stealthmonkey99/arkade/internal/bluetooth"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
	"github.com/stealthmonkey99/arkade/sdk/ui"
)

const listRefresh = 1200 * time.Millisecond

// row is a paired device or the add tile at the bottom.
type row struct {
	dev   bluetooth.Device
	seat  input.Seat
	live  bool // attached to the engine right now, not merely BT-connected
	isAdd bool
}

type devicesScene struct {
	a   *app
	bus *bus

	header  *element.Label
	rows    []*element.SpriteEl
	hint    *element.SpriteEl
	overlay *element.SpriteEl

	state    busState
	list     []row
	cursor   int
	lastPoll time.Time
	rank     bluetooth.Ranker
	confirm  string // StableID whose drop is being confirmed on screen
	// written by the connect and forget goroutines, read by the frame loop
	msg struct {
		sync.Mutex
		text  string
		until time.Time
	}
	hold ui.HoldRepeat
}

func (s *devicesScene) Init() error {
	s.header = element.NewLabel(element.LabelOptions{
		Options: element.Options{OffsetX: 2, OffsetY: 2},
		Text:    "BLUETOOTH",
	})
	elements := []display.Element{s.header}
	// listSlots includes the add tile.
	s.rows = make([]*element.SpriteEl, listSlots)
	for i := range s.rows {
		s.rows[i] = element.NewSprite(element.SpriteOptions{Options: element.Options{OffsetX: 2, OffsetY: listTop + i*rowH}, Data: blank(rowW, rowH)})
		elements = append(elements, s.rows[i])
	}
	// Hints are coloured discs because the buttons carry no letters.
	s.hint = element.NewSprite(element.SpriteOptions{Options: element.Options{OffsetX: 2, OffsetY: hintTop}, Data: blank(rowW, hintH)})
	elements = append(elements, s.hint)
	// Added last to sit over the list.
	s.overlay = element.NewSprite(element.SpriteOptions{Options: element.Options{OffsetX: 4, OffsetY: 14}, Data: blank(confirmW, confirmH)})
	s.overlay.SetVisible(false)
	elements = append(elements, s.overlay)
	s.a.Ctx.AddLayer(modeDevices, elements, 0)
	return nil
}

func (s *devicesScene) Enter() {
	s.confirm = ""
	s.hold.Reset()
	s.lastPoll = time.Time{}
	s.bus.open()
	s.refresh()
}

// Teardown leaves the bus to the app.
func (s *devicesScene) Teardown() {}

// refresh merges the bus snapshot (paired in BlueZ) with the controller
// manager (readable by the engine); a pad can be paired but not live.
func (s *devicesScene) refresh() {
	state, devs := s.bus.snapshot()
	s.state = state
	if state != busReady {
		// No add tile: a scan could only report the same failure.
		s.list = nil
		s.cursor = 0
		return
	}
	s.rank.Sort(devs)

	live := map[string]input.Seat{}
	if m := s.a.Ctx.RT.Controllers; m != nil {
		for _, at := range m.List() {
			live[at.StableID] = at.Seat
		}
	}

	var list []row
	for _, d := range devs {
		if !d.Paired {
			continue
		}
		r := row{dev: d, seat: input.NoSeat}
		id := normalizeID(d.Address)
		if seat, ok := live[id]; ok {
			r.live, r.seat = true, seat
		} else if seat, ok := s.a.settings.Seats[id]; ok {
			r.seat = input.SeatOf(seat)
		}
		list = append(list, r)
	}
	list = append(list, row{isAdd: true})

	wasOn := s.selectedID()
	s.list = list
	s.cursor = min(s.cursor, len(list)-1)
	if wasOn != "" {
		for i, r := range list {
			if !r.isAdd && normalizeID(r.dev.Address) == wasOn {
				s.cursor = i
				break
			}
		}
	}
}

func (s *devicesScene) selectedID() string {
	if s.cursor < 0 || s.cursor >= len(s.list) || s.list[s.cursor].isAdd {
		return ""
	}
	return normalizeID(s.list[s.cursor].dev.Address)
}

func (s *devicesScene) HandleInput(_ clock.FrameTime, ev input.Event) bool {
	if s.a.SceneMgr.State() != modeDevices {
		return false
	}
	// A held stick arrives as one joystickchange, not a stream of presses.
	if ev.Kind == input.KindJoystickChange {
		s.hold.Hold(element.StickDir(ev.Position))
		return true
	}
	if ev.Kind != input.KindKeypress {
		return false
	}
	return s.handleKey(ev.Name)
}

// handleKey is split from HandleInput for tests.
func (s *devicesScene) handleKey(name input.KeyName) bool {
	switch name {
	case input.KeyUp, input.KeyDown:
		s.step(name)
		return true
	case input.KeyJoyC:
		// a button, not a direction: the engine remaps a pad's directions by seat
		s.cycleSeat()
		return true
	case input.KeyJoyA, input.KeyEnter:
		if s.confirm != "" {
			s.dropConfirmed()
			return true
		}
		s.activate()
		return true
	case input.KeyJoyB, input.KeyEscape:
		// falls through to the app's back handler unless a drop is pending
		if s.confirm != "" {
			s.confirm = ""
			return true
		}
		return false
	case input.KeyJoyD:
		s.askDrop()
		return true
	}
	return false
}

func (s *devicesScene) cycleSeat() {
	if s.cursor < 0 || s.cursor >= len(s.list) || s.list[s.cursor].isAdd {
		return
	}
	next := input.SeatP1
	if s.list[s.cursor].seat == input.SeatP1 {
		next = input.SeatP2
	}
	s.setSeat(next)
}

func (s *devicesScene) setSeat(seat input.Seat) {
	id := s.selectedID()
	if id == "" {
		return
	}
	s.a.rememberSeat(id, seat)
	if m := s.a.Ctx.RT.Controllers; m != nil {
		for _, at := range m.List() {
			if at.StableID == id {
				m.AssignSeat(at.ID, seat)
			}
		}
	}
	s.confirm = ""
	s.say(fmt.Sprintf("= PLAYER %d", seat.Number()))
	s.refresh()
}

// activate is the green button: open the add screen on the add tile, or
// reconnect a pad that has wandered off.
func (s *devicesScene) activate() {
	if s.cursor >= 0 && s.cursor < len(s.list) && s.list[s.cursor].isAdd {
		_ = s.a.SceneMgr.Transition(modeAdd)
		return
	}
	id := s.selectedID()
	bt := s.bus.live()
	if id == "" || bt == nil {
		return
	}
	dev := s.list[s.cursor].dev
	s.say("connecting…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := bt.Pair(ctx, dev.Address); err != nil {
			s.a.Ctx.Log(fmt.Sprintf("bluetooth: connecting %s: %v", dev.Address, err))
			s.say("press its button")
			return
		}
		s.say("connected")
	}()
}

func (s *devicesScene) askDrop() {
	if id := s.selectedID(); id != "" {
		s.confirm = id
	}
}

func (s *devicesScene) dropConfirmed() {
	id := s.confirm
	s.confirm = ""
	if id == "" || s.cursor >= len(s.list) || s.list[s.cursor].isAdd {
		return
	}
	dev := s.list[s.cursor].dev
	s.a.forgetSeat(id)
	s.say("forgotten")
	if bt := s.bus.live(); bt != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := bt.Forget(ctx, dev.Address); err != nil {
				s.a.Ctx.Log(fmt.Sprintf("bluetooth: forgetting %s: %v", dev.Address, err))
				s.say("could not forget")
				return
			}
			// repoll so the dropped row disappears promptly
			s.bus.poll()
		}()
	}
	s.refresh()
}

// say puts a line on the LCD for a few seconds. Safe from any goroutine.
func (s *devicesScene) say(msg string) {
	s.msg.Lock()
	defer s.msg.Unlock()
	s.msg.text = msg
	s.msg.until = time.Now().Add(3 * time.Second)
}

// saying is the unexpired message, or "".
func (s *devicesScene) saying() string {
	s.msg.Lock()
	defer s.msg.Unlock()
	if s.msg.text == "" || !time.Now().Before(s.msg.until) {
		return ""
	}
	return s.msg.text
}

func (s *devicesScene) Update(stats clock.FrameTime) {
	s.tickHold(stats)
	s.bus.open()
	if now := time.Now(); now.Sub(s.lastPoll) >= listRefresh {
		s.lastPoll = now
		s.bus.poll()
	}
	s.refresh()
	s.paint()
}

func (s *devicesScene) tickHold(stats clock.FrameTime) {
	// the cursor is what the confirmation is about
	if s.confirm != "" {
		return
	}
	for n := s.hold.Update(element.ElapsedMicros(stats)); n > 0; n-- {
		s.step(element.Key(s.hold.Held()))
	}
}

func (s *devicesScene) step(dir input.KeyName) {
	switch dir {
	case input.KeyUp:
		if s.cursor > 0 {
			s.cursor--
			s.confirm = ""
		}
	case input.KeyDown:
		if s.cursor < len(s.list)-1 {
			s.cursor++
			s.confirm = ""
		}
	}
}

// wantLamps lights exactly the buttons that act in the current state.
func (s *devicesScene) wantLamps() lampSet {
	switch {
	case s.confirm != "":
		return lampSet{a: true, b: true}
	case s.cursor >= 0 && s.cursor < len(s.list) && s.list[s.cursor].isAdd:
		// no device to seat or drop
		return lampSet{a: true, b: true}
	case s.cursor >= 0 && s.cursor < len(s.list):
		r := s.list[s.cursor]
		// green wakes a sleeping pad; dark when already live
		return lampSet{a: !r.live, b: true, c: true, d: true}
	default:
		return lampSet{b: true}
	}
}

func (s *devicesScene) paint() {
	s.a.Ctx.SetButtonLights(input.BothSeats, s.wantLamps().lights(), true)
	s.header.SetText("BLUETOOTH")
	start := 0
	if s.cursor >= listSlots {
		start = s.cursor - listSlots + 1
	}
	asking := s.confirm != ""
	// a ready bus always yields the add tile, so empty means no bus
	note := ""
	if len(s.list) == 0 {
		note, _, _ = statusNote(s.state)
	}
	for i := range s.rows {
		if note != "" {
			art := blank(rowW, rowH)
			if i == 1 {
				paste(art, display.TextSprite(note, display.Pixel{R: 150, G: 150, B: 150, A: 255}), 8, 1)
			}
			setArt(s.rows[i], art)
			continue
		}
		idx := start + i
		if idx >= len(s.list) {
			setArt(s.rows[i], blank(rowW, rowH))
			continue
		}
		art := rowArt(s.list[idx], idx == s.cursor && !asking)
		if asking {
			art = dimSprite(art, 35)
		}
		setArt(s.rows[i], art)
	}
	setArt(s.hint, s.hintRow())
	s.overlay.SetVisible(asking)
	if asking {
		setArt(s.overlay, confirmArt(bluetooth.DisplayName(s.list[s.cursor].dev), confirmW, confirmH))
	}

	l1, l2 := s.lcd()
	s.a.Ctx.SetText(input.BothSeats, l1, l2)
}

func (s *devicesScene) Render(clock.FrameTime, scene.RenderReason) {}

func (s *devicesScene) hintRow() display.Sprite {
	label := ui.Color{R: 170, G: 170, B: 170, A: 255}
	// the confirmation carries its own YES/NO
	if s.confirm != "" || len(s.list) == 0 {
		return blank(rowW, ui.HintRowHeight)
	}
	// two hints is all 64 pixels hold; RED back is omitted
	hints := []ui.Hint{
		{Text: "SEAT", Color: ui.ButtonYellow},
		{Text: "DROP", Color: ui.ButtonBlue},
	}
	if s.cursor >= 0 && s.cursor < len(s.list) && s.list[s.cursor].isAdd {
		hints = []ui.Hint{{Text: "SCAN", Color: ui.ButtonGreen}}
	}
	row := ui.HintRow{LabelColor: label, Hints: hints}
	return display.Draw(row.Width(), ui.HintRowHeight, row)
}

// chordHint teaches the in-game seat chord from the screen where seats are set.
const chordHint = "SEL+R1=P1 L1=P2"

// statusNote is the panel line and two LCD lines shown when there is no list,
// distinguishing a missing radio from one still being checked.
func statusNote(state busState) (panel, lcd1, lcd2 string) {
	if state == busMissing {
		return "NO RADIO", "No Bluetooth", "radio not found"
	}
	return "CHECKING…", "Checking…", "looking for radio"
}

func (s *devicesScene) lcd() (string, string) {
	if len(s.list) == 0 {
		_, l1, l2 := statusNote(s.state)
		return l1, l2
	}
	if s.confirm != "" && s.cursor < len(s.list) {
		return "Drop " + bluetooth.DisplayName(s.list[s.cursor].dev) + "?", "GREEN yes  RED no"
	}
	if msg := s.saying(); msg != "" {
		return msg, chordHint
	}
	if s.cursor < 0 || s.cursor >= len(s.list) {
		return "Bluetooth", chordHint
	}
	r := s.list[s.cursor]
	if r.isAdd {
		return "Add a controller", "GREEN = SCAN"
	}
	name := bluetooth.DisplayName(r.dev)
	switch {
	case r.live && r.seat.Valid():
		return name, fmt.Sprintf("plays P%d YEL=SEAT", r.seat.Number())
	case r.live:
		return name, "YELLOW = seat"
	case r.seat.Valid():
		return name, fmt.Sprintf("P%d, asleep", r.seat.Number())
	default:
		return name, "asleep  GREEN=wake"
	}
}

// normalizeID lower-cases to match StableID: hidapi reports the MAC in lower
// case, BlueZ in upper.
func normalizeID(address string) string {
	return lower(address)
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + ('a' - 'A')
		}
	}
	return string(out)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	if b < 0 {
		return 0
	}
	return b
}
