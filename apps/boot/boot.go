// SPDX-License-Identifier: GPL-2.0-or-later

// Package boot is the startup app: a splash followed by controller pairing.
// A remembered controller that is still attached pairs silently; otherwise a
// pair-player graphic shows until a controller presses a button and claims
// the slot. Without a controller registry only the splash runs.
package boot

import (
	"embed"
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/element"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
)

//go:embed assets
var assetsFS embed.FS

// splashMs is long enough for the 1.8s stinger to finish with the marquee chase.
const splashMs = 2500

type settings struct {
	Player1 string `json:"player1,omitempty"`
	Player2 string `json:"player2,omitempty"`
}

type app struct {
	ctx       *engine.AppContext
	queue     input.Queue
	machine   *fsm.FSM
	settings  settings
	splash    *element.SpriteEl
	stung     bool
	bloomNext int
	completed bool // pairing reached "done"; Teardown persists only then
}

func init() {
	engine.Register(engine.Manifest{
		Slug:   "boot",
		Name:   "Årkade",
		Type:   engine.TypeBoot,
		Hidden: true,
		New:    func(ctx *engine.AppContext) engine.App { return &app{ctx: ctx} },
	})
}

func bootMachine() *fsm.FSM {
	return fsm.NewBuilder().
		AddTransition("splash", "next", "check-player-1").
		AddTransition("check-player-1", "next", "check-player-2").
		AddTransition("check-player-1", "wait", "wait-for-player-1").
		AddTransition("wait-for-player-1", "next", "check-player-2").
		AddTransition("check-player-2", "next", "done").
		AddTransition("check-player-2", "wait", "wait-for-player-2").
		AddTransition("wait-for-player-2", "next", "done").
		AddTransition("done", "next", "exited").
		Build("splash")
}

func (a *app) Init() error {
	a.machine = bootMachine()
	if err := a.ctx.ReadStorage("", &a.settings); err != nil {
		a.ctx.Log(fmt.Sprintf("settings read failed: %v", err))
	}

	a.ctx.SetLights(input.BothSeats, false)
	a.ctx.SetText(input.BothSeats, "Årkade", "©2024 M. Bielich")

	w, h := a.ctx.RT.Display.Size()
	a.splash = element.NewSprite(element.SpriteOptions{Data: splashFrame(w, h, 0)})
	a.ctx.AddLayer("splash", []display.Element{a.splash}, 1)

	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{
			input.Key(input.KeyUp), input.Key(input.KeyDown),
			input.Key(input.KeyLeft), input.Key(input.KeyRight),
			input.Key(input.KeyEnter), input.Key(input.KeySpace),
			input.Key(input.KeyJoyA), input.Key(input.KeyJoyB),
			input.Key(input.KeyJoyC), input.Key(input.KeyJoyD),
			input.Key(input.KeyJoyAux1), input.Key(input.KeyJoyAux2),
			input.Key(input.KeyStart),
		},
		Queue: &a.queue,
	}})
	return nil
}

func (a *app) pairingEnabled() bool { return a.ctx.RT.Controllers != nil }

// attachedNow queries the live registry each time: USB enumeration can lag
// startup by seconds.
func (a *app) attachedNow(id string) bool {
	if input.IsVirtualControllerID(id) {
		return true
	}
	if a.ctx.RT.Controllers == nil {
		return false
	}
	for _, c := range a.ctx.RT.Controllers.List() {
		if c.ID == id {
			return true
		}
	}
	return false
}

func (a *app) HandleInput(stats clock.FrameTime) {
	events := a.queue.Drain()
	switch a.machine.State() {
	case "splash":
		if len(events) > 0 {
			a.advanceFromSplash()
		}
	case "wait-for-player-1":
		a.handlePairPress(events, input.SeatP1)
	case "wait-for-player-2":
		a.handlePairPress(events, input.SeatP2)
	}
}

func (a *app) handlePairPress(events []input.Event, seat input.Seat) {
	for _, ev := range events {
		if ev.Source.Type != input.SourceController || ev.Source.ID == "" {
			continue
		}
		a.ctx.Log(fmt.Sprintf("controller '%s' pressed, pairing player %d", ev.Source.ID, seat.Number()))
		if seat == input.SeatP1 {
			a.settings.Player1 = ev.Source.ID
		} else {
			a.settings.Player2 = ev.Source.ID
		}
		a.ctx.RemoveLayer("info")
		a.ctx.SetLights(input.BothSeats, false)
		a.ctx.RT.Renderer.VirtualRotate(display.Upright)
		_ = a.machine.Dispatch("next")
		return
	}
}

func (a *app) Update(stats clock.FrameTime) {
	switch a.machine.State() {
	case "splash":
		if !a.stung && stats.DurationMs >= stingerAtMs {
			a.stung = true
			a.playStinger()
			a.ctx.RT.FadeBacklight(hardware.LampLevelMax, backlightFadeMs)
		}
		w, h := a.ctx.RT.Display.Size()
		a.splash.SetData(splashFrame(w, h, stats.DurationMs))
		a.stepBloom(stats)
		if stats.DurationMs >= splashMs {
			a.advanceFromSplash()
		}
	case "check-player-1":
		a.checkPlayer(input.SeatP1, a.settings.Player1, display.CounterClockwise)
	case "check-player-2":
		a.checkPlayer(input.SeatP2, a.settings.Player2, display.Clockwise)
	case "wait-for-player-1":
		a.recheckRemembered(input.SeatP1, a.settings.Player1)
	case "wait-for-player-2":
		a.recheckRemembered(input.SeatP2, a.settings.Player2)
	case "done":
		if a.ctx.RT.Controllers != nil {
			a.ctx.RT.Controllers.PairSeat(a.settings.Player1, input.SeatP1)
			a.ctx.RT.Controllers.PairSeat(a.settings.Player2, input.SeatP2)
		}
		a.ctx.Log("all done with the bootstrap, exiting...")
		a.completed = true
		_ = a.machine.Dispatch("next")
		a.ctx.SetLights(input.BothSeats, true)
		a.ctx.Exit()
	}
}

// advanceFromSplash keeps the splash layer up: removing it paints black frames
// into the transition.
func (a *app) advanceFromSplash() {
	_ = a.machine.Dispatch("next")
	if !a.pairingEnabled() {
		a.settings.Player1 = input.VirtualControllerID(input.SeatP1.Index())
		a.settings.Player2 = input.VirtualControllerID(input.SeatP2.Index())
		_ = a.machine.Dispatch("next")
		_ = a.machine.Dispatch("next")
	}
}

// recheckRemembered claims a remembered controller that attaches late.
func (a *app) recheckRemembered(seat input.Seat, storedID string) {
	if storedID == "" || !a.attachedNow(storedID) {
		return
	}
	a.ctx.Log(fmt.Sprintf("controller '%s' attached late for player %d, continuing", storedID, seat.Number()))
	a.ctx.RemoveLayer("info")
	a.ctx.SetLights(input.BothSeats, false)
	a.ctx.RT.Renderer.VirtualRotate(display.Upright)
	_ = a.machine.Dispatch("next")
}

func (a *app) checkPlayer(seat input.Seat, storedID string, rotate display.Orientation) {
	if storedID != "" && a.attachedNow(storedID) {
		a.ctx.Log(fmt.Sprintf("found controller '%s' for player %d, continuing", storedID, seat.Number()))
		_ = a.machine.Dispatch("next")
		return
	}
	a.ctx.Log(fmt.Sprintf("no controller for player %d, waiting for input...", seat.Number()))
	// someone is at the cabinet: hand off to the shell, not the screensaver
	a.ctx.RT.NoteBootPairingWaited()
	a.ctx.SetLights(input.SeatsOf(seat), true)
	_ = a.machine.Dispatch("wait")
	a.ctx.RT.Renderer.VirtualRotate(rotate)
	graphic, err := element.SpriteFromPNG(assetsFS, fmt.Sprintf("assets/pair-player-%d.png", seat.Number()), element.Options{})
	if err != nil {
		a.ctx.Log(fmt.Sprintf("pair graphic: %v", err))
		return
	}
	a.ctx.RemoveLayer("splash")
	a.ctx.AddLayer("info", []display.Element{graphic}, 1)
}

func (a *app) Render(clock.FrameTime, engine.RenderReason) {}

func (a *app) Teardown() error {
	// An interrupted boot must not overwrite the remembered pairing with its
	// half-empty copy.
	if !a.completed {
		return nil
	}
	return a.ctx.WriteStorage("", a.settings)
}

// stingerAtMs is late enough that a cold boot's capture process is open
// before the first note.
const stingerAtMs = 500

// The boot bloom lights one colour group per chord voice, staggered like the
// splash's rings.
const (
	bloomVoices = 5
	bloomStepMs = 60
	bloomFadeMs = 550
)

// bloomVoice maps a LightButtons index to its chord voice; the two white aux
// lamps share the last voice.
func bloomVoice(light int) int { return min(light, bloomVoices-1) }

// the startup card leaves the backlight at 30%
const backlightFadeMs = 800

func (a *app) stepBloom(stats clock.FrameTime) {
	for a.bloomNext < bloomVoices &&
		stats.DurationMs >= float64(stingerAtMs+a.bloomNext*bloomStepMs) {
		var levels hardware.LampLevels
		var touch hardware.LightMask
		for light := range hardware.LightButtons {
			if bloomVoice(light) != a.bloomNext {
				continue
			}
			levels[light] = hardware.LampLevelMax
			touch |= 1 << uint(light)
		}
		a.ctx.FadeLampsEased(input.BothSeats, levels, touch, bloomFadeMs, engine.EaseOut)
		a.bloomNext++
	}
}

// playStinger also calibrates the microphone.
func (a *app) playStinger() {
	if err := a.ctx.RT.PlayStinger(); err != nil {
		a.ctx.Log(fmt.Sprintf("stinger: %v", err))
	}
}
