// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

// sleepLampFadeMs is the lamp fade on a deliberate sleep, timed from DARK
// entry like the goodnight sequence.
const sleepLampFadeMs = 2500

// wakeBloomMs is the ambient motion wake; pressBloomMs, eased out, is a wake
// somebody is waiting on and matches the boot bloom. A wake as long as the
// sleep reads as a slow cabinet, and a ramp that lingers near black reads as
// a missed press.
const (
	wakeBloomMs  = 900
	pressBloomMs = 500
)

// StandbyState is the engine's presence state: ACTIVE while an app is in
// use, ATTRACT once idle, DARK once even attract has gone unwatched.
type StandbyState int

const (
	StateActive StandbyState = iota
	StateAttract
	StateDark
)

func (s StandbyState) String() string {
	switch s {
	case StateAttract:
		return "attract"
	case StateDark:
		return "dark"
	default:
		return "active"
	}
}

// standbyAction is what a standby check tells Tick to do. standby never
// touches the Runtime, so it unit-tests on time alone.
type standbyAction int

const (
	actionNone standbyAction = iota
	actionEnterAttract
	actionEnterDark
	// actionHardDecay reports a hard DARK turning soft; DARK stays DARK.
	actionHardDecay
)

// standby is the presence timer behind the state machine:
//
//   - ACTIVE → ATTRACT after attractAfter with no input.
//   - ATTRACT → DARK after standbyAfter with no presence; motion alone keeps
//     attract alive but never exits it.
//   - ATTRACT/DARK → ACTIVE on any input.
//   - DARK → ATTRACT on motion, unless hard.
//
// attractAfter == 0 disables standby; standbyAfter == 0 lets attract run
// forever. hard marks a deliberate dark (BACK in attract, or SLEEP) that
// motion does not wake until hardDecayAfter passes with no presence; a press
// always wakes. hardDecayAfter == 0 disables hard.
type standby struct {
	attractAfter   time.Duration
	standbyAfter   time.Duration
	hardDecayAfter time.Duration

	state        StandbyState
	hard         bool
	lastInput    time.Time
	lastPresence time.Time
	// lastSignal is refreshed only by a person, a press or a motion reading;
	// lastPresence is also held open by the machine's own housekeeping.
	lastSignal time.Time
}

// newStandby starts ACTIVE as of now, so boot doesn't attract immediately.
func newStandby(attractAfter, standbyAfter, hardDecayAfter time.Duration, now time.Time) *standby {
	return &standby{
		attractAfter:   attractAfter,
		standbyAfter:   standbyAfter,
		hardDecayAfter: hardDecayAfter,
		lastInput:      now,
		lastPresence:   now,
		lastSignal:     now,
	}
}

func (s *standby) enabled() bool { return s.attractAfter > 0 }

// noteInput records an input, which is also presence, and reports whether it
// woke the cabinet.
func (s *standby) noteInput(now time.Time) (wake bool) {
	s.lastInput = now
	s.lastPresence = now
	s.lastSignal = now
	if s.state != StateActive {
		s.state = StateActive
		s.hard = false
		return true
	}
	return false
}

// noteHold refreshes the clocks during a JoyA hold so it can't lapse to dark
// mid-press; unlike noteInput it never wakes.
func (s *standby) noteHold(now time.Time) {
	s.lastInput = now
	s.lastPresence = now
	s.lastSignal = now
}

// holdPresence pushes the ATTRACT to DARK countdown out to now without
// touching the state machine or lastSignal, so a screensaver on screen keeps
// the panel lit while PresenceWithin can still see the room empty.
func (s *standby) holdPresence(now time.Time) { s.lastPresence = now }

// noteMotion records presence without input and wakes a soft DARK to
// ATTRACT. In hard DARK it still refreshes presence, so an occupied room
// stays hard.
func (s *standby) noteMotion(now time.Time) (wakeToAttract bool) {
	s.lastPresence = now
	s.lastSignal = now
	if s.state == StateDark && !s.hard {
		s.state = StateAttract
		return true
	}
	return false
}

// forceAttract enters ATTRACT now, with the input and presence clocks reset
// so the dark countdown starts fresh.
func (s *standby) forceAttract(now time.Time) {
	s.state = StateAttract
	s.lastInput = now
	s.lastPresence = now
	s.hard = false
}

// forceDark enters a hard DARK now, for the shell's SLEEP: the person who
// pressed it is still in front of the sensor and must not wake it.
func (s *standby) forceDark(now time.Time) {
	s.state = StateDark
	s.lastInput = now
	s.lastPresence = now
	s.hard = s.hardDecayAfter > 0
}

// enterHardDark drops ATTRACT to hard DARK on a BACK press. The press proves
// presence, so the decay countdown starts now.
func (s *standby) enterHardDark(now time.Time) {
	s.state = StateDark
	s.lastPresence = now
	s.lastSignal = now
	s.hard = s.hardDecayAfter > 0
}

// step evaluates the timeouts once per tick, after the tick's signals are
// folded in, and reports the transition Tick must apply. allowAttract=false
// holds ACTIVE while the idle clock keeps aging.
func (s *standby) step(now time.Time, allowAttract bool) standbyAction {
	if !s.enabled() {
		return actionNone
	}
	switch s.state {
	case StateActive:
		if allowAttract && now.Sub(s.lastInput) >= s.attractAfter {
			s.state = StateAttract
			return actionEnterAttract
		}
	case StateAttract:
		if s.standbyAfter > 0 && now.Sub(s.lastPresence) >= s.standbyAfter {
			s.state = StateDark
			return actionEnterDark
		}
	case StateDark:
		if s.hard && s.hardDecayAfter > 0 && now.Sub(s.lastPresence) >= s.hardDecayAfter {
			s.hard = false
			return actionHardDecay
		}
	}
	return actionNone
}

// inputRoute is what the standby router decided about one event arriving
// outside ACTIVE.
type inputRoute int

const (
	// routeWake swallows the event and the rest of the drain, and wakes the
	// cabinet.
	routeWake inputRoute = iota
	// routeDrop swallows this event only.
	routeDrop
	// routeStop swallows this event and the rest of the drain; the router
	// has already acted on it.
	routeStop
)

// routeStandbyInput decides what an event does while the cabinet attracts or
// is dark. Every kind of a press is intercepted so the keydown half can't
// reach the wake path when only the keypress should act. In ATTRACT:
//
//   - BACK deepens to hard dark;
//   - JoyA over a scheduled screensaver opens its settings;
//   - JoyA over a game slot starts a hold that stepAttractHold may complete
//     into a launch, a hold released early waking as usual.
//
// Anything else that is a press wakes; releases and mic keys are dropped.
func (rt *Runtime) routeStandbyInput(now time.Time, ev input.Event) inputRoute {
	if rt.standby.state == StateAttract && isBackName(ev.Name) {
		if ev.Kind != input.KindKeypress {
			return routeDrop
		}
		rt.standby.enterHardDark(now)
		rt.Log("info", "standby: hard dark (back pressed)")
		rt.setTransition(transition{kind: transitionDark, fade: true})
		return routeStop
	}
	if slug := rt.screensaverShowingSlug(); slug != "" &&
		rt.standby.state == StateAttract && ev.Name == input.KeyJoyA {
		if ev.Kind != input.KindKeypress {
			return routeDrop
		}
		rt.openScreensaverSettings(now, slug)
		return routeStop
	}
	if rt.standby.state == StateAttract && ev.Name == input.KeyJoyA {
		switch ev.Kind {
		case input.KindKeydown, input.KindKeypress:
			if rt.attractLaunchSlug() != "" {
				if rt.attractHold.begin(now) {
					rt.standby.noteHold(now) // a hold can't lapse to dark
				}
				return routeDrop
			}
		case input.KindKeyup:
			if rt.attractHold.active() {
				rt.leaveStandby(now, resumeParked)
				return routeStop
			}
		}
	}
	if !wakesStandby(ev) {
		return routeDrop
	}
	return routeWake
}

func isBackName(name input.KeyName) bool {
	switch name {
	case input.KeyQuit, input.KeyEscape, input.KeyJoyB:
		return true
	}
	return false
}

// wakesStandby reports whether ev is a press. The release of the press that
// entered standby arrives a moment later and must not wake it straight back up.
func wakesStandby(ev input.Event) bool {
	if input.IsMicKey(ev.Name) {
		// Presence is the radar's job; sound would only add false wakes.
		return false
	}
	return ev.Kind == input.KindKeydown || ev.Kind == input.KindKeypress
}

// stepStandby folds the tick's timeouts into the state machine and applies
// the transition it reports.
func (rt *Runtime) stepStandby(now time.Time) {
	// Boot is exempt from attract: interrupting its pairing prompt abandons
	// the flow mid-pair.
	allowAttract := !rt.bootAppRunning()
	switch rt.standby.step(now, allowAttract) {
	case actionEnterAttract:
		rt.Log("info", fmt.Sprintf("standby: idle %s, starting attract", now.Sub(rt.standby.lastInput).Round(time.Second)))
		rt.suspendCurrent(now)
		rt.notes.quiet = true
		rt.loadStandby(now)
	case actionEnterDark:
		rt.Log("info", fmt.Sprintf("standby: no presence %s, going dark", now.Sub(rt.standby.lastPresence).Round(time.Second)))
		// No goodnight sequence: the room is empty.
		rt.setTransition(transition{kind: transitionDark})
	case actionHardDecay:
		rt.Log("info", "standby: hard dark decayed to soft")
	}
}

// StandbyState reports the current presence state.
func (rt *Runtime) StandbyState() StandbyState { return rt.standby.state }

// StandbyEnabled reports whether standby runs at all. When it doesn't, an
// app backing out all the way should exit rather than call EnterStandby.
func (rt *Runtime) StandbyEnabled() bool { return rt.standby.enabled() }

// StandbyHard reports whether the current DARK ignores motion; always false
// outside StateDark.
func (rt *Runtime) StandbyHard() bool { return rt.standby.hard }

// IdleFor reports the engine-clock time since the last input, the ACTIVE to
// ATTRACT countdown. It runs even with standby disabled.
func (rt *Runtime) IdleFor() time.Duration {
	return rt.Clock.Now().Sub(rt.standby.lastInput)
}

// PresenceFor reports the engine-clock time since the last input or motion,
// the ATTRACT to DARK countdown.
func (rt *Runtime) PresenceFor() time.Duration {
	return rt.Clock.Now().Sub(rt.standby.lastPresence)
}

// PresenceWithin reports whether a person, by press or motion, was detected
// within d. Unlike PresenceFor it is not held open by a screensaver being on
// screen, so a screensaver can notice the room emptying.
func (rt *Runtime) PresenceWithin(d time.Duration) bool {
	return rt.Clock.Now().Sub(rt.standby.lastSignal) <= d
}

// InjectMotion simulates a motion-sensor reading, for hardware with no
// sensor.
func (rt *Runtime) InjectMotion() {
	rt.noteMotionWake(rt.Clock.Now())
}

// WakeFromHardware records a deliberate hardware act, such as a wireless pad
// switching on, folded in next tick. It counts as input, not motion, so it
// wakes a hard dark the way a press does. Safe from any goroutine.
func (rt *Runtime) WakeFromHardware() { rt.hardwareWake.Store(true) }

func (rt *Runtime) takeHardwareWake(now time.Time) {
	if !rt.hardwareWake.Swap(false) {
		return
	}
	if rt.standby.state == StateActive {
		rt.standby.noteInput(now)
		return
	}
	rt.Log("info", "standby: controller connected, waking")
	rt.leaveStandby(now, resumeParked)
}

func (rt *Runtime) pollMotion(now time.Time) {
	ms, ok := rt.Hardware.(hardware.MotionSensor)
	if ok && ms.MotionActive() {
		rt.noteMotionWake(now)
	}
}

func (rt *Runtime) noteMotionWake(now time.Time) {
	if rt.standby.noteMotion(now) {
		rt.darkFade.cancel()
		rt.Log("info", "standby: motion, waking to attract")
		rt.startWakeBloom(wakeBloomMs, EaseInOut)
		rt.loadStandby(now)
	}
}

func (rt *Runtime) setBacklight(on bool) { rt.SetBacklightLevel(onLevel(on)) }

// backlightHeld reports whether a lab preview owns the LCDs, so standby must
// not put their backlight out.
func (rt *Runtime) backlightHeld() bool {
	return rt.labPreview != nil && rt.labPreview.lcdHeld
}

// powerDownNotice tells the host once per dark that the cabinet is going
// down, however many times DARK is re-entered before the next wake.
type powerDownNotice struct {
	fn   func(reason string)
	down bool
}

func (n *powerDownNotice) notify(reason string) {
	if n.down {
		return
	}
	n.down = true
	if n.fn != nil {
		n.fn(reason)
	}
}

func (n *powerDownNotice) wake() { n.down = false }

// parkedApp says what leaving standby does with the app a takeover parked.
type parkedApp int

const (
	// resumeParked continues the parked app where it stopped, or loads the
	// shell when nothing is parked.
	resumeParked parkedApp = iota
	// discardParked tears the parked app down: the caller is loading the app
	// somebody asked for in its place.
	discardParked
)

// leaveStandby brings the cabinet to ACTIVE; every deliberate wake goes
// through it.
func (rt *Runtime) leaveStandby(now time.Time, parked parkedApp) {
	fromDark := rt.standby.state == StateDark
	rt.standby.noteInput(now)
	rt.powerDown.wake()
	rt.attractHold.clear()
	// Mid-goodnight the cabinet is still lit; blooming would dip it to black
	// first.
	bloom := fromDark && !rt.darkFade.running()
	rt.darkFade.cancel()
	if bloom {
		rt.startWakeBloom(pressBloomMs, EaseOut)
	} else {
		rt.hurryWakeBloom()
	}
	rt.clearPanelDim()
	rt.setBacklight(true)
	if parked == discardParked {
		rt.discardSuspended()
		return
	}
	if rt.resumeSuspended(now) {
		return
	}
	rt.Log("info", "standby: input, waking to shell")
	if shell, ok := lookupType(TypeShell); ok {
		rt.notes.quiet = true
		rt.requestLoad(shell)
	}
}

// enterStandbyNow forces ATTRACT as if the idle timer had fired, suspending
// the current app so a wake resumes it.
func (rt *Runtime) enterStandbyNow(now time.Time) {
	rt.standby.forceAttract(now)
	rt.Log("info", "standby: forced attract (shell back)")
	rt.suspendCurrent(now)
	rt.loadStandby(now)
}

// NoteBootPairingWaited tells the engine boot's pairing flow waited for a
// human press. Called by the boot app only.
func (rt *Runtime) NoteBootPairingWaited() { rt.bootPairingWaited = true }

// bootHandsOffToAttract reports whether boot is finishing to an empty room,
// so ATTRACT comes up facing the room and the first press wakes to a shell
// already seated. A pairing wait proves somebody is there.
func (rt *Runtime) bootHandsOffToAttract() bool {
	return rt.bootAppRunning() && rt.standby.enabled() && !rt.bootPairingWaited
}

// TakeBootSettle reports whether the attract load now starting follows the
// boot splash, and clears the flag. Called once from the attract app's Init.
func (rt *Runtime) TakeBootSettle() bool { return rt.notes.takeBootSettle() }

// bootAppRunning reports whether boot is loaded. Boot addresses the room, so
// it is exempt from both attract and the seated orientation.
func (rt *Runtime) bootAppRunning() bool {
	return rt.curCtx != nil && rt.curCtx.Manifest.Type == TypeBoot
}

// enterSleepNow suspends the current app and goes straight to DARK with the
// goodnight sequence, skipping attract.
func (rt *Runtime) enterSleepNow(now time.Time) {
	rt.standby.forceDark(now)
	rt.Log("info", "standby: sleep requested, going dark")
	rt.suspendCurrent(now)
	rt.enterDark(true)
}

// loadAttract requests the attract app, relighting the LCDs in case this is
// a wake from DARK.
func (rt *Runtime) loadAttract() {
	rt.powerDown.wake()
	rt.attractHold.clear()
	rt.setBacklight(true)
	if m, ok := lookupType(TypeAttract); ok {
		rt.requestLoad(m)
	}
}

// darkFade times the goodnight sequence from DARK entry; the zero value is
// no sequence running.
type darkFade struct{ start time.Time }

func (d *darkFade) arm(now time.Time) { d.start = now }
func (d *darkFade) cancel()           { d.start = time.Time{} }
func (d darkFade) running() bool      { return !d.start.IsZero() }

// elapsedMs is how far into the goodnight now is.
func (d darkFade) elapsedMs(now time.Time) float64 { return ms(now.Sub(d.start)) }

// enterDark unloads the app, blanks every peripheral and stops painting.
// fade plays the goodnight sequence instead of an immediate black frame.
// The panel is painted from the renderer, not the compositor, which still
// holds a suspended app's layers.
func (rt *Runtime) enterDark(fade bool) {
	rt.unloadCurrent()
	rt.powerDown.notify("standby")
	if fade {
		rt.FadeLamps(-1, hardware.LampLevels{}, hardware.AllLights, sleepLampFadeMs)
	} else {
		rt.Hardware.SetLights(-1, false)
	}
	rt.Hardware.SetText(-1, "", "")
	if fade {
		rt.FadeBacklight(0, sleepLampFadeMs)
	} else {
		rt.setBacklight(false)
	}
	if rt.labPreview.hasFrames() {
		// endLabPreview blanks the panel when it hands it back.
		return
	}
	if fade {
		rt.darkFade.arm(rt.frames.tickStart)
		return
	}
	rt.Renderer.Paint(blackFrame(rt.Display.Size()), rt.Stats())
}

// stepDarkFade paints one goodnight frame, then black once it has run out.
// No-op unless enterDark armed a fade.
func (rt *Runtime) stepDarkFade(now time.Time) {
	if !rt.darkFade.running() {
		return
	}
	elapsed := rt.darkFade.elapsedMs(now)
	if elapsed >= sleepTotalMs {
		rt.darkFade.cancel()
		rt.Renderer.Paint(blackFrame(rt.Display.Size()), rt.Stats())
		return
	}
	w, h := rt.Display.Size()
	rt.Renderer.Paint(sleepFadeFrame(w, h, elapsed), rt.Stats())
}
