// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"fmt"
	"sort"
	"time"
)

// TypeScreensaver is a hidden native app the standby machine may show in
// place of the attract reel while that app's schedule has a window open.
// Several may be registered; pickScreensaver decides which, if any, runs.
const TypeScreensaver AppType = "screensaver"

// screensaverPickInterval is how often ATTRACT re-evaluates which screensaver should show.
const screensaverPickInterval = time.Second

// scheduleCacheTTL bounds how often a screensaver's settings file is read; InvalidateSchedules bypasses it.
const scheduleCacheTTL = time.Minute

// screensaverSettings is the one key the engine reads from a screensaver's settings.json; the rest
// belongs to the app.
type screensaverSettings struct {
	Schedule Schedule `json:"schedule"`
}

// screensaverPicker is ATTRACT's choice of what to show: the screensavers'
// schedules, cached; the slug the last pick settled on and when; and an
// operator's pin that outranks every schedule. Engine goroutine only.
type screensaverPicker struct {
	schedules     map[string]Schedule
	schedulesRead time.Time
	// showing is the slug loadStandby chose last, "" for the attract reel.
	showing  string
	pickedAt time.Time
	pin      string
}

func (p *screensaverPicker) invalidate() { p.schedules = nil }

// schedule returns m's schedule, reading it through load at most once per
// scheduleCacheTTL and falling back to the manifest's DefaultSchedule when
// the stored one is zero. A load error is reported once and the default used.
func (p *screensaverPicker) schedule(m Manifest, now time.Time, load func(slug string) (Schedule, error)) (Schedule, error) {
	if p.schedules == nil || now.Sub(p.schedulesRead) >= scheduleCacheTTL {
		p.schedules = make(map[string]Schedule)
		p.schedulesRead = now
	}
	if s, ok := p.schedules[m.Slug]; ok {
		return s, nil
	}
	sc, err := load(m.Slug)
	if sc == (Schedule{}) {
		sc = m.DefaultSchedule
	}
	p.schedules[m.Slug] = sc
	return sc, err
}

// due reports whether the periodic re-pick is owed.
func (p screensaverPicker) due(now time.Time) bool {
	return now.Sub(p.pickedAt) >= screensaverPickInterval
}

// saverCandidate is a screensaver whose window is open and whose gate
// agrees, with the priority its schedule claims.
type saverCandidate struct {
	m        Manifest
	priority int
}

// bestScreensaver picks among open candidates: the highest priority wins,
// ties settled by slug. ok=false means show the attract reel.
func bestScreensaver(cands []saverCandidate) (Manifest, bool) {
	var best saverCandidate
	found := false
	for _, c := range cands {
		if found && (c.priority < best.priority || (c.priority == best.priority && c.m.Slug > best.m.Slug)) {
			continue
		}
		best, found = c, true
	}
	return best.m, found
}

// InvalidateSchedules drops the cached screensaver schedules so the next pick re-reads them.
func (rt *Runtime) InvalidateSchedules() { rt.savers.invalidate() }

// screensaverSchedule returns a screensaver's stored schedule through the
// picker's cache, logging a read that fails.
func (rt *Runtime) screensaverSchedule(m Manifest, now time.Time) Schedule {
	sc, err := rt.savers.schedule(m, now, func(slug string) (Schedule, error) {
		var st screensaverSettings
		err := rt.Store.Read(slug, "", &st)
		return st.Schedule, err
	})
	if err != nil {
		rt.Log("warn", fmt.Sprintf("screensaver: %s schedule: %v", m.Slug, err))
	}
	return sc
}

// screensaverManifests returns every registered TypeScreensaver manifest.
func screensaverManifests() []Manifest {
	regMu.Lock()
	defer regMu.Unlock()
	var out []Manifest
	for _, m := range registry {
		if m.Type == TypeScreensaver {
			out = append(out, m)
		}
	}
	return out
}

// Screensavers returns every registered TypeScreensaver manifest sorted by slug; Apps() omits them
// because they are Hidden.
func Screensavers() []Manifest {
	out := screensaverManifests()
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// pickScreensaver reports which screensaver should show at now: a ShowScreensaver pin, else the
// bestScreensaver of the open, gate-approved schedules. ok=false means show the attract reel.
func (rt *Runtime) pickScreensaver(now time.Time) (Manifest, bool) {
	if m, ok := rt.pinnedScreensaver(); ok {
		return m, true
	}
	var cands []saverCandidate
	for _, m := range screensaverManifests() {
		sc := rt.screensaverSchedule(m, now)
		if !sc.Contains(now) {
			continue
		}
		if m.ScreensaverGate != nil && !m.ScreensaverGate(rt, now) {
			continue
		}
		cands = append(cands, saverCandidate{m: m, priority: sc.Priority})
	}
	return bestScreensaver(cands)
}

// loadStandby loads what ATTRACT should show: the scheduled screensaver, else the attract reel. Every
// route into ATTRACT goes through here.
func (rt *Runtime) loadStandby(now time.Time) {
	rt.savers.pickedAt = now
	m, ok := rt.pickScreensaver(now)
	if !ok {
		rt.savers.showing = ""
		rt.loadAttract()
		return
	}
	rt.savers.showing = m.Slug
	rt.powerDown.wake()
	rt.attractHold.clear()
	rt.setBacklight(true)
	rt.Log("info", fmt.Sprintf("standby: screensaver %s", m.Slug))
	rt.requestLoad(m)
}

// stepScreensaver keeps ATTRACT showing the right thing. A scheduled screensaver holds the presence
// clock so the ATTRACT to DARK timeout cannot fire under it; the pick is re-evaluated once a second and
// a change restarts the standby countdown.
func (rt *Runtime) stepScreensaver(now time.Time) {
	if rt.standby.state != StateAttract {
		rt.savers.pin = "" // a forced show lasts exactly as long as its attract
		return
	}
	if rt.screensaverShowingSlug() != "" {
		rt.standby.holdPresence(now)
	}
	if !rt.savers.due(now) {
		return
	}
	rt.savers.pickedAt = now
	pick := ""
	if m, ok := rt.pickScreensaver(now); ok {
		pick = m.Slug
	}
	if pick == rt.savers.showing {
		return
	}
	rt.standby.forceAttract(now)
	rt.loadStandby(now)
}

// screensaverShowingSlug returns the slug of the scheduled screensaver on screen, or "".
func (rt *Runtime) screensaverShowingSlug() string {
	if rt.curCtx != nil && rt.curCtx.Manifest.Type == TypeScreensaver {
		return rt.curCtx.Manifest.Slug
	}
	return ""
}

// ShowScreensaver shows slug's screensaver now, ignoring its schedule and gate, and pins it against the
// once-a-second re-pick. The pin drops on any wake, hard dark, or when the screensaver exits. Safe from
// any goroutine: the request applies at the top of the next tick, latest wins; an unregistered or
// non-screensaver slug is refused immediately.
func (rt *Runtime) ShowScreensaver(slug string) error {
	m, err := Lookup(slug)
	if err != nil {
		return err
	}
	if m.Type != TypeScreensaver {
		return fmt.Errorf("%s is not a screensaver", slug)
	}
	rt.requests.screensaverShow.Set(slug)
	return nil
}

// enterScreensaverShow applies one drained ShowScreensaver request on the engine goroutine, pinning the
// pick before loadStandby reads it. From ATTRACT nothing new is parked.
func (rt *Runtime) enterScreensaverShow(now time.Time, slug string) {
	if !rt.standby.enabled() {
		rt.Log("warn", fmt.Sprintf("screensaver: show %q ignored: standby is disabled", slug))
		return
	}
	if m, err := Lookup(slug); err != nil || m.Type != TypeScreensaver {
		rt.Log("warn", fmt.Sprintf("screensaver: show %q ignored: not a registered screensaver", slug))
		return
	}
	rt.savers.pin = slug
	rt.standby.forceAttract(now)
	rt.Log("info", fmt.Sprintf("screensaver: show %s, ignoring its schedule", slug))
	if rt.curCtx == nil || (rt.curCtx.Manifest.Type != TypeAttract && rt.curCtx.Manifest.Type != TypeScreensaver) {
		rt.suspendCurrent(now)
	}
	rt.loadStandby(now)
}

// pinnedScreensaver returns the screensaver ShowScreensaver forced, dropping a pin whose app was
// unregistered.
func (rt *Runtime) pinnedScreensaver() (Manifest, bool) {
	if rt.savers.pin == "" {
		return Manifest{}, false
	}
	m, err := Lookup(rt.savers.pin)
	if err != nil || m.Type != TypeScreensaver {
		rt.savers.pin = ""
		return Manifest{}, false
	}
	return m, true
}

// dropScreensaverPinOnExit releases a forced screensaver that quits on its own so the picker decides
// what follows.
func (rt *Runtime) dropScreensaverPinOnExit() {
	if rt.savers.pin != "" && rt.screensaverShowingSlug() == rt.savers.pin {
		rt.savers.pin = ""
	}
}

// openScreensaverSettings wakes to ACTIVE and loads the showing screensaver as a foreground app flagged
// to open its settings (the green button). It discards whatever standby parked, as attract's
// hold-to-launch does.
func (rt *Runtime) openScreensaverSettings(now time.Time, slug string) {
	m, err := Lookup(slug)
	if err != nil {
		rt.Log("error", fmt.Sprintf("standby: screensaver %q not found: %v", slug, err))
		rt.leaveStandby(now, resumeParked)
		return
	}
	rt.leaveStandby(now, discardParked)
	rt.notes.settingsOpen = slug
	rt.Log("info", fmt.Sprintf("standby: %s settings (green)", slug))
	rt.requestLoad(m)
}

// setPanelDim takes the panel to pct percent, below MinPanelBrightness if asked. The floor guards
// against losing the menu in the dark; a screensaver ends on any button, so it is exempt. clearPanelDim
// restores the brightness remembered on the first dim.
func (rt *Runtime) setPanelDim(pct int) {
	rt.panel.dim(pct, rt.PanelBrightness())
	rt.applyPanelBrightness()
	rt.hwShadow.setPanelBrightness(rt.panel.pct)
}

// clearPanelDim restores the operator's brightness after a dim. Idempotent.
func (rt *Runtime) clearPanelDim() {
	if restore, ok := rt.panel.clearDim(); ok {
		rt.SetPanelBrightness(restore)
	}
}

// SetPanelDim dims the panel to pct percent (1..100) while this app stays loaded, ignoring the
// MinPanelBrightness floor. The runtime restores the operator's brightness when the app unloads or the
// cabinet wakes.
func (c *AppContext) SetPanelDim(pct int) { c.RT.setPanelDim(pct) }

// OpenedForSettings reports whether this load came from the green button while this screensaver was
// showing, the cue to open its settings rather than the screensaver view.
func (c *AppContext) OpenedForSettings() bool { return c.openedForSettings }

// ExitToScreensaver returns to ATTRACT with this screensaver reloaded and pinned by ShowScreensaver
// until the attract ends. Settings must already be written. Only a TypeScreensaver app with standby
// enabled can; any other caller gets Exit().
func (c *AppContext) ExitToScreensaver() {
	if c.Manifest.Type != TypeScreensaver {
		c.RT.Log("warn", fmt.Sprintf("%s: ExitToScreensaver from a %s app, exiting to the shell", c.Manifest.Slug, c.Manifest.Type))
		c.Exit()
		return
	}
	if !c.RT.StandbyEnabled() {
		c.Exit()
		return
	}
	if err := c.RT.ShowScreensaver(c.Manifest.Slug); err != nil {
		c.RT.Log("error", fmt.Sprintf("%s: ExitToScreensaver: %v", c.Manifest.Slug, err))
		c.Exit()
	}
}

// InvalidateSchedules makes the engine re-read every screensaver's schedule on its next pick.
// WriteStorage already does this; call it directly when a schedule changes another way.
func (c *AppContext) InvalidateSchedules() { c.RT.InvalidateSchedules() }
