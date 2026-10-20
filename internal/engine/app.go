// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/scene"
)

// RenderReason says why a scene is being rendered ("init" on first paint).
type RenderReason = scene.RenderReason

// App is the lifecycle contract every app/game implements. The engine
// drives scenes automatically when Scenes() returns a manager, so most
// apps only add app-level logic on top.
type App interface {
	// Init builds layers, scenes and key mappings.
	Init() error
	// HandleInput drains the app's input queues.
	HandleInput(stats clock.FrameTime)
	// Update advances app logic (scenes update first).
	Update(stats clock.FrameTime)
	// Render refreshes elements before compositing (scenes render first).
	Render(stats clock.FrameTime, reason RenderReason)
	// Teardown persists state; engine removes layers/audio afterwards.
	Teardown() error
}

// SceneApp is implemented by apps that own a scene manager; the engine
// forwards input/update/render to scenes before the app's own methods.
type SceneApp interface {
	Scenes() *scene.Manager
}

// Resumer is implemented by an App that wants to hear when the engine brings
// it back from a standby suspension. Resumed runs on the engine goroutine
// during the wake, after the app's LCD text and lamps have been replayed and
// before its next HandleInput. The frame clock is shifted past the gap, so
// DurationMs does not jump; a control the app saw held before the suspension
// should be dropped here, since its release arrived while the app was parked.
type Resumer interface {
	Resumed()
}

// AppContext is the per-app handle on runtime services, scoped to the
// app's manifest (layer ownership, storage keys, audio owner). It also
// implements scene.Host.
type AppContext struct {
	Manifest Manifest
	RT       *Runtime

	keyMaps           [][]*input.Mapping
	attached          []*input.MappingSet
	pushedStrategies  int
	openedForSettings bool  // the one load a screensaver gets when green is pressed while it shows
	rawDirOverride    *bool // replaces Manifest.RawDirections when set
}

// rawDirections reports whether controller directions reach this app
// unremapped; false with no app loaded.
func (c *AppContext) rawDirections() bool {
	switch {
	case c == nil:
		return false
	case c.rawDirOverride != nil:
		return *c.rawDirOverride
	default:
		return c.Manifest.RawDirections
	}
}

// AddLayer creates an app-owned display layer at the given opacity.
func (c *AppContext) AddLayer(label string, elements []display.Element, opacity float64) {
	c.RT.Display.AddLayer(c.Manifest.Slug, label, display.LayerOptions{
		Elements: elements,
		Opacity:  &opacity,
	})
}

// AddFilledLayer creates an app-owned layer with a background fill.
func (c *AppContext) AddFilledLayer(label string, fill display.Pixel, elements []display.Element, opacity float64) {
	c.RT.Display.AddLayer(c.Manifest.Slug, label, display.LayerOptions{
		Fill:     &fill,
		Elements: elements,
		Opacity:  &opacity,
	})
}

// RemoveLayer removes one app-owned layer.
func (c *AppContext) RemoveLayer(label string) {
	c.RT.Display.RemoveLayers(c.Manifest.Slug, label)
}

// SetLayerAlpha adjusts one app-owned layer's opacity.
func (c *AppContext) SetLayerAlpha(label string, value float64) {
	c.RT.Display.SetLayerAlpha(c.Manifest.Slug, label, value)
}

// AddElements appends elements to an app-owned layer.
func (c *AppContext) AddElements(label string, elements ...display.Element) {
	c.RT.Display.AddElements(c.Manifest.Slug, label, elements...)
}

// RemoveElements removes elements from an app-owned layer.
func (c *AppContext) RemoveElements(label string, elements ...display.Element) {
	c.RT.Display.RemoveElements(c.Manifest.Slug, label, elements...)
}

// SetScrim fills the system scrim layer.
func (c *AppContext) SetScrim(p display.Pixel) { c.RT.Display.SetScrim(p) }

// PlaySound plays a one-shot sound owned by this app.
func (c *AppContext) PlaySound(file string) {
	c.RT.Audio.Play(c.Manifest.Slug, "", file, 0)
}

// PlayMusic starts looping music (stops any previous music first).
func (c *AppContext) PlayMusic(file string) {
	c.StopMusic()
	c.RT.Audio.Play(c.Manifest.Slug, "music", file, -1)
}

// StopMusic stops this app's music.
func (c *AppContext) StopMusic() {
	c.RT.Audio.Stop(c.Manifest.Slug, "music")
}

// Log writes to the engine log (visible in the console renderer tail).
func (c *AppContext) Log(msg string) {
	c.RT.Log("log", fmt.Sprintf("[%s] %s", c.Manifest.Name, msg))
}

// Now returns engine time (manual clock under test).
func (c *AppContext) Now() time.Time { return c.RT.Clock.Now() }

// ReadStorage loads this app's stored JSON (label "" = settings).
func (c *AppContext) ReadStorage(label string, out any) error {
	return c.RT.Store.Read(c.Manifest.Slug, label, out)
}

// WriteStorage saves this app's JSON (label "" = settings). A screensaver's
// write also invalidates the schedule cache so edits apply at the next pick.
func (c *AppContext) WriteStorage(label string, in any) error {
	if c.Manifest.Type == TypeScreensaver {
		c.RT.InvalidateSchedules()
	}
	return c.RT.Store.Write(c.Manifest.Slug, label, in)
}

// AddKeyMap pushes a mapping set for this app (removed on teardown).
func (c *AppContext) AddKeyMap(set []*input.Mapping) {
	c.keyMaps = append(c.keyMaps, set)
	c.attached = append(c.attached, c.RT.Input.AddMappings(set))
}

// detachKeyMaps takes every set this app pushed off the input stack.
func (c *AppContext) detachKeyMaps() {
	for _, set := range c.attached {
		c.RT.Input.RemoveMappings(set)
	}
	c.attached = nil
}

// reattachKeyMaps pushes the app's sets back in their original order.
func (c *AppContext) reattachKeyMaps() {
	for _, set := range c.keyMaps {
		c.attached = append(c.attached, c.RT.Input.AddMappings(set))
	}
}

// Exit requests unloading this app (engine exit for the shell itself).
func (c *AppContext) Exit() { c.RT.requestUnload(false) }

// EnterStandby hands off to the attract screensaver now, as an idle timeout
// would. The app is suspended, not unloaded, so an input wake resumes it.
// With standby disabled it exits instead.
func (c *AppContext) EnterStandby() {
	if !c.RT.StandbyEnabled() {
		c.Exit()
		return
	}
	c.RT.enterStandbyNow(c.RT.Clock.Now())
}

// EnterSleep skips the screensaver and goes straight to dark standby: lamps
// and LCDs blank at once, then the goodnight sequence runs on the LED panel.
// The app is suspended first and must not draw. Motion does not wake it (the
// person who pressed sleep is still standing there); a button press does.
// With standby disabled it exits instead.
func (c *AppContext) EnterSleep() {
	if !c.RT.StandbyEnabled() {
		c.Exit()
		return
	}
	c.RT.enterSleepNow(c.RT.Clock.Now())
}

// SetLCDBacklight switches the character-LCD backlight directly, bypassing
// the standby state machine's own backlight calls.
func (c *AppContext) SetLCDBacklight(on bool) { c.RT.setBacklight(on) }

// Launch requests switching to another app by slug.
func (c *AppContext) Launch(slug string) error {
	m, err := Lookup(slug)
	if err != nil {
		return err
	}
	c.RT.requestLoad(m)
	return nil
}

// LaunchManifest requests switching to another app.
func (c *AppContext) LaunchManifest(m Manifest) { c.RT.requestLoad(m) }

// PushPlayerStrategy activates a player strategy; teardown pops any left over.
func (c *AppContext) PushPlayerStrategy(s PlayerStrategy) {
	c.pushedStrategies++
	c.RT.PushPlayerStrategy(s)
}

// PopPlayerStrategy removes this app's most recent strategy.
func (c *AppContext) PopPlayerStrategy() {
	if c.pushedStrategies > 0 {
		c.pushedStrategies--
		c.RT.PopPlayerStrategy()
	}
}

// SwitchPlayer flips the turn on a TwoPlayerTurnBased strategy.
func (c *AppContext) SwitchPlayer(toPrimary ...bool) { c.RT.SwitchPlayer(toPrimary...) }

// SetButtonLights updates the addressed seats' individual arcade button
// lights; only=true turns unmentioned buttons off. Like every peripheral
// write, a value the device already shows is dropped, so an app may write
// every frame without caching.
func (c *AppContext) SetButtonLights(seats input.Seats, lights hardware.ButtonLights, only bool) {
	if index, ok := seats.Index(); ok {
		c.RT.Hardware.SetButtonLights(index, lights, only)
	}
}

// SetLights switches the addressed seats' whole button sets on or off.
func (c *AppContext) SetLights(seats input.Seats, on bool) {
	if index, ok := seats.Index(); ok {
		c.RT.Hardware.SetLights(index, on)
	}
}

// SetLampLevels sets the addressed seats' lamps to brightness levels, 0 to
// hardware.LampLevelMax. Deduped against the device.
func (c *AppContext) SetLampLevels(seats input.Seats, levels hardware.LampLevels) {
	if index, ok := seats.Index(); ok {
		c.RT.Hardware.SetLampLevels(index, levels)
	}
}

// FadeLampsEased ramps the addressed seats' lamps named by touch to levels
// over fadeMs along ease; see Runtime.FadeLamps.
func (c *AppContext) FadeLampsEased(seats input.Seats, levels hardware.LampLevels, touch hardware.LightMask, fadeMs int, ease Ease) {
	if index, ok := seats.Index(); ok {
		c.RT.FadeLampsEased(index, levels, touch, fadeMs, ease)
	}
}

// SetText writes the addressed seats' character LCDs, two lines each.
// Deduped against the device, so an app may rewrite its HUD every frame.
func (c *AppContext) SetText(seats input.Seats, line1, line2 string) {
	if index, ok := seats.Index(); ok {
		c.RT.Hardware.SetText(index, line1, line2)
	}
}

// RejectKey signals an unusable keypress (rings the bell).
func (c *AppContext) RejectKey() { c.RT.RingBell() }

var _ scene.Host = (*AppContext)(nil)
