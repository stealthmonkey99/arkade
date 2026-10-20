// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// Stand-in apps register per test and unregister on cleanup; the app registry is process-global and TestEngineQuitKeyStopsLoop needs no shell registered.

// testAttractSlug is the attract stand-in sleep_test.go registers at init().
// lookupType(TypeAttract) returns the first match in the registry, so this
// suite reuses it rather than adding a second that would never be chosen.
const testAttractSlug = "test/idle-attract"

func TestScheduleContains(t *testing.T) {
	// 2024-01-01 is a Monday; 2024-01-06 a Saturday.
	at := func(day, hour, min int) time.Time {
		return time.Date(2024, 1, day, hour, min, 0, 0, time.UTC)
	}
	weekdays := [7]bool{false, true, true, true, true, true, false}

	cases := []struct {
		name string
		sc   Schedule
		now  time.Time
		want bool
	}{
		{"inside a normal window", Schedule{Enabled: true, From: "06:00", To: "09:00", Days: EveryDay()}, at(1, 7, 30), true},
		{"at the opening minute", Schedule{Enabled: true, From: "06:00", To: "09:00", Days: EveryDay()}, at(1, 6, 0), true},
		{"at the closing minute", Schedule{Enabled: true, From: "06:00", To: "09:00", Days: EveryDay()}, at(1, 9, 0), false},
		{"before a normal window", Schedule{Enabled: true, From: "06:00", To: "09:00", Days: EveryDay()}, at(1, 5, 59), false},
		{"disabled", Schedule{From: "06:00", To: "09:00", Days: EveryDay()}, at(1, 7, 30), false},
		{"no days set", Schedule{Enabled: true, From: "06:00", To: "09:00"}, at(1, 7, 30), false},
		{"day not selected", Schedule{Enabled: true, From: "06:00", To: "09:00", Days: weekdays}, at(6, 7, 30), false},
		{"zero-length window", Schedule{Enabled: true, From: "06:00", To: "06:00", Days: EveryDay()}, at(1, 6, 0), false},
		{"unparseable window", Schedule{Enabled: true, From: "6:00", To: "9am", Days: EveryDay()}, at(1, 7, 30), false},

		{"wrap, late evening", Schedule{Enabled: true, From: "22:00", To: "02:00", Days: EveryDay()}, at(1, 23, 0), true},
		{"wrap, small hours", Schedule{Enabled: true, From: "22:00", To: "02:00", Days: EveryDay()}, at(2, 1, 0), true},
		{"wrap, after it closes", Schedule{Enabled: true, From: "22:00", To: "02:00", Days: EveryDay()}, at(2, 2, 0), false},
		{"wrap, mid-afternoon", Schedule{Enabled: true, From: "22:00", To: "02:00", Days: EveryDay()}, at(2, 15, 0), false},
		// The day check applies to the day the window STARTED: Friday night's
		// window is open at 01:00 on Saturday, and Saturday night's isn't.
		{"wrap, Friday night runs into Saturday", Schedule{Enabled: true, From: "22:00", To: "02:00", Days: weekdays}, at(6, 1, 0), true},
		{"wrap, Saturday night doesn't open", Schedule{Enabled: true, From: "22:00", To: "02:00", Days: weekdays}, at(6, 23, 0), false},
		{"window ending at midnight", Schedule{Enabled: true, From: "22:00", To: "00:00", Days: EveryDay()}, at(1, 23, 59), true},
	}
	for _, c := range cases {
		if got := c.sc.Contains(c.now); got != c.want {
			t.Errorf("%s: Contains(%v) = %v, want %v", c.name, c.now, got, c.want)
		}
	}
}

func TestScheduleStorageShape(t *testing.T) {
	raw, err := json.Marshal(screensaverSettings{Schedule: Schedule{
		Enabled: true, From: "06:00", To: "09:00",
		Days:     [7]bool{false, true, true, true, true, true, false},
		Priority: 10,
	}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"schedule":{"enabled":true,"from":"06:00","to":"09:00","days":[false,true,true,true,true,true,false],"priority":10}}`
	if string(raw) != want {
		t.Errorf("settings JSON shape changed:\n got %s\nwant %s", raw, want)
	}
}

func TestPickScreensaverHighestPriorityWins(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", 0, 0)
	registerSaver(t, "test/saver-a", nil)
	registerSaver(t, "test/saver-b", nil)
	open := Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()}
	rig.writeSchedule("test/saver-a", withPriority(open, 1))
	rig.writeSchedule("test/saver-b", withPriority(open, 5))

	m, ok := rig.rt.pickScreensaver(rig.clk.Now())
	if !ok || m.Slug != "test/saver-b" {
		t.Fatalf("expected the higher priority to win, got %q ok=%v", m.Slug, ok)
	}

	// Equal priorities are settled by slug.
	rig.writeSchedule("test/saver-b", withPriority(open, 1))
	m, ok = rig.rt.pickScreensaver(rig.clk.Now())
	if !ok || m.Slug != "test/saver-a" {
		t.Fatalf("expected the tie to break by slug, got %q ok=%v", m.Slug, ok)
	}
}

func TestPickScreensaverGateVetoes(t *testing.T) {
	allow := true
	rig := newSaverRig(t, "test/foreground", 0, 0)
	registerSaver(t, "test/saver-gated", func(*Runtime, time.Time) bool { return allow })
	rig.writeSchedule("test/saver-gated", Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()})

	if _, ok := rig.rt.pickScreensaver(rig.clk.Now()); !ok {
		t.Fatal("expected an open window with a passing gate to be eligible")
	}
	allow = false
	if m, ok := rig.rt.pickScreensaver(rig.clk.Now()); ok {
		t.Fatalf("expected the gate to veto, got %q", m.Slug)
	}
}

func TestPickScreensaverNoneEligible(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", 0, 0)
	registerSaver(t, "test/saver-closed", nil)
	rig.writeSchedule("test/saver-closed", Schedule{Enabled: true, From: "18:00", To: "20:00", Days: EveryDay()})

	if m, ok := rig.rt.pickScreensaver(rig.clk.Now()); ok {
		t.Fatalf("expected no pick outside every window, got %q", m.Slug)
	}
	// ...and the standby entry falls back to the attract reel.
	rig.rt.loadStandby(rig.clk.Now())
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != testAttractSlug {
		t.Fatalf("expected the attract reel with nothing eligible, got %q", got)
	}
}

func TestScreensaverSwapsAtWindowBoundaries(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Minute, 0)
	registerSaver(t, "test/saver-morning", nil)
	rig.writeSchedule("test/saver-morning", Schedule{Enabled: true, From: "06:00", To: "09:00", Days: EveryDay()})

	// 05:59: idle long enough to attract, but the window hasn't opened.
	rig.jump(5*time.Hour + 59*time.Minute)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != testAttractSlug {
		t.Fatalf("expected the attract reel before the window opens, got %q", got)
	}

	// Past 06:00 the once-a-second re-pick swaps the screensaver in.
	rig.jump(2 * time.Minute)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != "test/saver-morning" {
		t.Fatalf("expected the screensaver once its window opened, got %q", got)
	}
	if got := rig.rt.StandbyState(); got != StateAttract {
		t.Fatalf("a screensaver swap must stay in ATTRACT, got %v", got)
	}

	// Past 09:00 it swaps back out, with the standby countdown restarted.
	rig.jump(3 * time.Hour)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != testAttractSlug {
		t.Fatalf("expected the attract reel once the window closed, got %q", got)
	}
	if got := rig.rt.PresenceFor(); got > time.Second {
		t.Fatalf("expected fresh standby timers after the swap, presence is %v old", got)
	}
}

func TestScreensaverSuppressesDark(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Second, 10*time.Second)
	registerSaver(t, "test/saver-night", nil)
	rig.writeSchedule("test/saver-night", Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()})

	rig.jump(2 * time.Second)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != "test/saver-night" {
		t.Fatalf("expected the screensaver to take over attract, got %q", got)
	}

	// Well past standbyAfter with no presence at all: a scheduled screensaver
	// stays up regardless.
	for i := 0; i < 3; i++ {
		rig.jump(30 * time.Second)
	}
	if got := rig.rt.StandbyState(); got != StateAttract {
		t.Fatalf("expected the screensaver to suppress DARK, got %v", got)
	}
	if got := rig.rt.CurrentSlug(); got != "test/saver-night" {
		t.Fatalf("expected the screensaver still loaded, got %q", got)
	}

	// Close its window and the suppression goes with it.
	rig.writeSchedule("test/saver-night", Schedule{Enabled: false, From: "00:00", To: "12:00", Days: EveryDay()})
	rig.jump(2 * time.Second)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != testAttractSlug {
		t.Fatalf("expected the attract reel after the window closed, got %q", got)
	}
	rig.jump(11 * time.Second)
	if got := rig.rt.StandbyState(); got != StateDark {
		t.Fatalf("expected plain attract to go DARK on lost presence, got %v", got)
	}
}

func TestScreensaverGreenOpensSettings(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Second, 0)
	rec := registerSaver(t, "test/saver-green", nil)
	rig.writeSchedule("test/saver-green", Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()})

	rig.jump(2 * time.Second)
	rig.tick(1)
	if rec.settings {
		t.Fatal("a standby load must not claim it was opened for settings")
	}

	rig.press(input.KeyJoyA)
	rig.tick(2)
	if got := rig.rt.StandbyState(); got != StateActive {
		t.Fatalf("expected green to wake to ACTIVE, got %v", got)
	}
	if got := rig.rt.CurrentSlug(); got != "test/saver-green" {
		t.Fatalf("expected green to reload the screensaver itself, got %q", got)
	}
	if !rec.settings {
		t.Fatal("expected OpenedForSettings on the green-button load")
	}
	if rec.inits != 2 {
		t.Fatalf("expected the screensaver re-loaded for settings, got %d inits", rec.inits)
	}
	if got := rig.rt.IdleFor(); got > time.Second {
		t.Fatalf("expected standby timers reset by the wake, idle is %v", got)
	}
}

func TestScreensaverOtherButtonWakesToShell(t *testing.T) {
	registerApp(t, Manifest{Slug: "test/saver-shell", Name: "Shell", Type: TypeShell, Hidden: true, New: newStub})
	rig := newSaverRig(t, "test/saver-shell", time.Second, 0)
	rec := registerSaver(t, "test/saver-any", nil)
	rig.writeSchedule("test/saver-any", Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()})

	rig.jump(2 * time.Second)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != "test/saver-any" {
		t.Fatalf("expected the screensaver showing, got %q", got)
	}

	rig.press(input.KeyEnter)
	rig.tick(1)
	if got := rig.rt.StandbyState(); got != StateActive {
		t.Fatalf("expected any button to wake to ACTIVE, got %v", got)
	}
	if got := rig.rt.CurrentSlug(); got != "test/saver-shell" {
		t.Fatalf("expected a wake to the shell, got %q", got)
	}
	if rec.settings {
		t.Fatal("only green opens the screensaver's settings")
	}
}

func TestScreensaverSettingsReturnToTheScreensaver(t *testing.T) {
	registerApp(t, Manifest{Slug: "test/saver-back-shell", Name: "Shell", Type: TypeShell, Hidden: true, New: newStub})
	rig := newSaverRig(t, "test/saver-back-shell", time.Second, 0)
	rec := registerSaver(t, "test/saver-back", nil)
	rig.writeSchedule("test/saver-back", Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()})

	rig.jump(2 * time.Second)
	rig.tick(1)
	rig.press(input.KeyJoyA)
	rig.tick(2)
	if !rec.settings {
		t.Fatal("green should have opened the screensaver's settings")
	}

	rig.press(input.KeyJoyB)
	rig.tick(2)
	if got := rig.rt.CurrentSlug(); got != "test/saver-back" {
		t.Fatalf("red should have gone back to the screensaver, got %q", got)
	}
	if got := rig.rt.StandbyState(); got != StateAttract {
		t.Errorf("the return should be to ATTRACT, got %v", got)
	}
	if rec.settings {
		t.Error("the return is an ordinary screensaver load, not another settings one")
	}
	if rec.from != "23:00" {
		t.Errorf("the reloaded screensaver should read the window just saved, got %q", rec.from)
	}
	if got := rig.rt.PresenceFor(); got > time.Second {
		t.Errorf("expected fresh standby timers on the return, presence is %v old", got)
	}

	// Its window is shut now, so only the pin is keeping it up — and the pin
	// ends at the next wake.
	rig.jump(5 * time.Second)
	if got := rig.rt.CurrentSlug(); got != "test/saver-back" {
		t.Fatalf("the pin should outlast the closed window, got %q", got)
	}
	rig.press(input.KeyEnter)
	rig.tick(2)
	if got := rig.rt.CurrentSlug(); got != "test/saver-back-shell" {
		t.Fatalf("a press should wake to the shell, got %q", got)
	}
	rig.jump(2 * time.Second)
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != testAttractSlug {
		t.Errorf("the pin should not outlive the wake, got %q", got)
	}
}

func TestExitToScreensaverFromAnOrdinaryAppExits(t *testing.T) {
	registerApp(t, Manifest{Slug: "test/plain-shell", Name: "Shell", Type: TypeShell, Hidden: true, New: newStub})
	rig := newSaverRig(t, "test/foreground", 0, 0)

	rig.rt.curCtx.ExitToScreensaver()
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != "test/plain-shell" {
		t.Fatalf("a non-screensaver should exit to the shell, got %q", got)
	}
}

func TestScreensaverPanelDimRestoredOnWake(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Second, 0)
	rec := registerSaver(t, "test/saver-dim", nil)
	rec.dimPct = 2 // below MinPanelBrightness on purpose
	rig.writeSchedule("test/saver-dim", Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()})

	rig.jump(2 * time.Second)
	rig.tick(1)
	if got := rig.rt.PanelBrightness(); got != 2 {
		t.Fatalf("expected the screensaver's dim to go below the operator floor, got %d", got)
	}

	rig.press(input.KeyEnter)
	rig.tick(2)
	if got := rig.rt.PanelBrightness(); got != 100 {
		t.Fatalf("expected the operator brightness restored on wake, got %d", got)
	}
}

func TestShowScreensaverIgnoresTheSchedule(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Minute, 0)
	rec := registerSaver(t, "test/saver-shut", nil)
	registerSaver(t, "test/saver-open", nil)
	rig.writeSchedule("test/saver-shut", Schedule{Enabled: true, From: "18:00", To: "20:00", Days: EveryDay()})
	rig.writeSchedule("test/saver-open", Schedule{Enabled: true, From: "00:00", To: "12:00", Days: EveryDay()})

	if err := rig.rt.ShowScreensaver("test/saver-shut"); err != nil {
		t.Fatalf("ShowScreensaver: %v", err)
	}
	rig.tick(1)
	if got := rig.rt.StandbyState(); got != StateAttract {
		t.Fatalf("expected a forced show to enter ATTRACT from ACTIVE, got %v", got)
	}
	if got := rig.rt.CurrentSlug(); got != "test/saver-shut" {
		t.Fatalf("expected the asked-for screensaver, got %q", got)
	}
	if got := rig.rt.SuspendedSlug(); got != "test/foreground" {
		t.Fatalf("expected the running app parked, suspended is %q", got)
	}

	// The once-a-second re-pick would normally swap in the screensaver whose
	// window IS open; the pin outranks it.
	rig.jump(2 * time.Second)
	if got := rig.rt.CurrentSlug(); got != "test/saver-shut" {
		t.Fatalf("expected the pin to survive the re-pick, got %q", got)
	}
	if rec.inits != 1 {
		t.Fatalf("expected the pinned screensaver loaded once, got %d inits", rec.inits)
	}
}

func TestShowScreensaverUnpinsOnWake(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Minute, 0)
	registerSaver(t, "test/saver-shut", nil)
	rig.writeSchedule("test/saver-shut", Schedule{Enabled: true, From: "18:00", To: "20:00", Days: EveryDay()})

	if err := rig.rt.ShowScreensaver("test/saver-shut"); err != nil {
		t.Fatalf("ShowScreensaver: %v", err)
	}
	rig.tick(1)

	rig.press(input.KeyEnter)
	rig.tick(1)
	if got := rig.rt.StandbyState(); got != StateActive {
		t.Fatalf("expected any button to wake the forced screensaver, got %v", got)
	}
	if got := rig.rt.CurrentSlug(); got != "test/foreground" {
		t.Fatalf("expected the parked app resumed, got %q", got)
	}

	// Idling back into attract now picks normally, and nothing qualifies.
	rig.jump(time.Minute + time.Second)
	rig.tick(1) // the attract load lands at the top of the tick after the timeout
	if got := rig.rt.StandbyState(); got != StateAttract {
		t.Fatalf("expected the idle timeout to attract again, got %v", got)
	}
	if got := rig.rt.CurrentSlug(); got != testAttractSlug {
		t.Fatalf("expected the wake to have dropped the pin, got %q", got)
	}
}

func TestShowScreensaverThenSettingsReturnsToTheScreensaver(t *testing.T) {
	registerApp(t, Manifest{Slug: "test/shown-shell", Name: "Shell", Type: TypeShell, Hidden: true, New: newStub})
	rig := newSaverRig(t, "test/shown-shell", time.Minute, 0)
	rec := registerSaver(t, "test/saver-shown", nil)
	rig.writeSchedule("test/saver-shown", Schedule{Enabled: true, From: "18:00", To: "20:00", Days: EveryDay()})

	if err := rig.rt.ShowScreensaver("test/saver-shown"); err != nil {
		t.Fatalf("ShowScreensaver: %v", err)
	}
	rig.tick(1)
	if got := rig.rt.CurrentSlug(); got != "test/saver-shown" {
		t.Fatalf("expected the forced show, got %q", got)
	}

	rig.press(input.KeyJoyA)
	rig.tick(2)
	if !rec.settings {
		t.Fatal("green should open the settings of a forced show too")
	}

	rig.press(input.KeyJoyB)
	rig.tick(2)
	if got := rig.rt.CurrentSlug(); got != "test/saver-shown" {
		t.Fatalf("red should have gone back to the screensaver, got %q", got)
	}
	if got := rig.rt.StandbyState(); got != StateAttract {
		t.Errorf("the return should be to ATTRACT, got %v", got)
	}
	if rec.from != "23:00" {
		t.Errorf("the reloaded screensaver should read the window just saved, got %q", rec.from)
	}
}

func TestShowScreensaverRejectsUnknownAndNonScreensaver(t *testing.T) {
	rig := newSaverRig(t, "test/foreground", time.Minute, 0)

	if err := rig.rt.ShowScreensaver("test/ghost"); err == nil {
		t.Error("expected an error for an unregistered slug")
	}
	if err := rig.rt.ShowScreensaver("test/foreground"); err == nil {
		t.Error("expected an error for an app that isn't a screensaver")
	}
	rig.tick(1)
	if got := rig.rt.StandbyState(); got != StateActive {
		t.Fatalf("a refused show must not touch the standby state, got %v", got)
	}
}

// saverRecord is what the test-only screensaver reports back about its loads;
// from is the schedule window the last Init read off disk.
type saverRecord struct {
	inits    int
	settings bool
	dimPct   int
	from     string
}

type saverApp struct {
	rec      *saverRecord
	ctx      *AppContext
	settings screensaverSettings
}

func (a *saverApp) Init() error {
	a.rec.inits++
	a.rec.settings = a.ctx.OpenedForSettings()
	_ = a.ctx.ReadStorage("", &a.settings)
	a.rec.from = a.settings.Schedule.From
	if a.rec.dimPct > 0 {
		a.ctx.SetPanelDim(a.rec.dimPct)
	}
	if a.ctx.OpenedForSettings() {
		a.ctx.AddKeyMap([]*input.Mapping{{
			Conditions: []input.Condition{input.Key(input.KeyJoyB)},
			Trigger:    func(input.Event) { a.save() },
		}})
	}
	return nil
}

// save stands in for the real screensavers' settings screens: red edits the
// window — shut, so only the pin can bring the screensaver back — writes it
// and returns to the screensaver itself.
func (a *saverApp) save() {
	a.settings.Schedule.Enabled = false
	a.settings.Schedule.From = "23:00"
	if err := a.ctx.WriteStorage("", a.settings); err != nil {
		panic(err)
	}
	a.ctx.ExitToScreensaver()
}

func (a *saverApp) HandleInput(clock.FrameTime)          {}
func (a *saverApp) Update(clock.FrameTime)               {}
func (a *saverApp) Render(clock.FrameTime, RenderReason) {}
func (a *saverApp) Teardown() error                      { return nil }

// stubApp stands in for the apps the standby machine moves between but this
// suite never looks inside: the attract reel, an idle foreground app, a shell.
type stubApp struct{}

func newStub(*AppContext) App                        { return stubApp{} }
func (stubApp) Init() error                          { return nil }
func (stubApp) HandleInput(clock.FrameTime)          {}
func (stubApp) Update(clock.FrameTime)               {}
func (stubApp) Render(clock.FrameTime, RenderReason) {}
func (stubApp) Teardown() error                      { return nil }

func registerApp(t *testing.T, m Manifest) {
	t.Helper()
	Register(m)
	t.Cleanup(func() { Unregister(m.Slug) })
}

func registerSaver(t *testing.T, slug string, gate func(*Runtime, time.Time) bool) *saverRecord {
	t.Helper()
	rec := &saverRecord{}
	registerApp(t, Manifest{
		Slug: slug, Name: slug, Type: TypeScreensaver, Hidden: true,
		ScreensaverGate: gate,
		New:             func(ctx *AppContext) App { return &saverApp{rec: rec, ctx: ctx} },
	})
	return rec
}

func withPriority(sc Schedule, p int) Schedule {
	sc.Priority = p
	return sc
}

type saverRig struct {
	t   *testing.T
	rt  *Runtime
	clk *clock.ManualClock
}

// newSaverRig builds a runtime on the manual clock, whose epoch is
// 2024-01-01 00:00 UTC — a Monday — so schedule windows are reached by
// advancing from midnight.
func newSaverRig(t *testing.T, startApp string, attractAfter, standbyAfter time.Duration) *saverRig {
	t.Helper()
	registerApp(t, Manifest{Slug: "test/foreground", Name: "Foreground", Type: TypeApp, Hidden: true, New: newStub})
	clk := clock.NewManualClock()
	rt, err := New(Options{
		Renderer:     render.NewCapture(false),
		Clock:        clk,
		DataDir:      t.TempDir(),
		StartApp:     startApp,
		RandSeed:     42,
		AttractAfter: attractAfter,
		StandbyAfter: standbyAfter,
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	if err := rt.Init(); err != nil {
		t.Fatalf("engine.Init: %v", err)
	}
	return &saverRig{t: t, rt: rt, clk: clk}
}

func (r *saverRig) writeSchedule(slug string, sc Schedule) {
	r.t.Helper()
	if err := r.rt.Store.Write(slug, "", screensaverSettings{Schedule: sc}); err != nil {
		r.t.Fatalf("writing %s schedule: %v", slug, err)
	}
	r.rt.InvalidateSchedules()
}

func (r *saverRig) tick(n int) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		r.clk.Advance(r.rt.FrameBudget())
		if err := r.rt.Tick(); err != nil {
			r.t.Fatalf("tick: %v", err)
		}
	}
}

// jump moves the clock forward by d and runs a single tick, the way a test
// crosses a schedule boundary without simulating every frame in between.
func (r *saverRig) jump(d time.Duration) {
	r.t.Helper()
	r.clk.Advance(d)
	r.tick(1)
}

func (r *saverRig) press(name input.KeyName) {
	r.rt.Input.Push(input.Event{
		Kind:   input.KindKeypress,
		Name:   name,
		Source: input.PlayerSource{Type: input.SourceController},
	})
}
