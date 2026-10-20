// SPDX-License-Identifier: GPL-2.0-or-later

package morning

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/stealthmonkey99/arkade/apps/attract"
	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/storage"
	"github.com/stealthmonkey99/arkade/internal/testkit"
	"github.com/stealthmonkey99/arkade/internal/weather"
)

// The manual clock's epoch is 2024-01-01T00:00:00Z, a Monday.
const (
	morningOffset = 6*time.Hour + 42*time.Minute
	testLat       = 37.77
	testLon       = -122.42
)

// Pixels that separate "6:42" from "6:43" in the crossfade test: at row 6 of
// the last digit (x=34, top at timeY), the 2 lights column 2 and the 3 column 5.
const (
	lastDigitX = 34
	digitRow6Y = timeY + 6
	onlyTwoX   = lastDigitX + 2
	onlyThreeX = lastDigitX + 5
)

func canned(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"current":{"temperature_2m":54.3,"weather_code":2},` +
		`"daily":{"temperature_2m_max":[68.1],"temperature_2m_min":[51.4]}}`))
}

// stubWeather points the poller at a local server for the rest of the test.
// A nil handler always fails, keeping a live fetch from replacing the cache.
func stubWeather(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusInternalServerError)
		}
	}
	srv := httptest.NewServer(handler)
	prev := weatherBaseURL
	weatherBaseURL = srv.URL
	t.Cleanup(func() {
		weatherBaseURL = prev
		srv.Close()
	})
}

// TestMain answers every unstubbed location lookup with a 404 so no test reaches the network.
func TestMain(m *testing.M) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"location unavailable"}`))
	}))
	weather.WhereAmIURL = srv.URL
	code := m.Run()
	srv.Close()
	os.Exit(code)
}

// stubWhereAmI points the location lookup at a local server and counts requests. A nil handler answers 404.
func stubWhereAmI(t *testing.T, handler http.HandlerFunc) *atomic.Int64 {
	t.Helper()
	var hits atomic.Int64
	if handler == nil {
		handler = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"location unavailable"}`))
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	prev := weather.WhereAmIURL
	weather.WhereAmIURL = srv.URL
	t.Cleanup(func() {
		weather.WhereAmIURL = prev
		srv.Close()
	})
	return &hits
}

func cannedWhereAmI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"lat":37.77,"lon":-122.42,"city":"San Francisco",` +
		`"region":"California","country":"US","timezone":"America/Los_Angeles",` +
		`"source":"cloudflare-ip"}`))
}

func seedSettings(t *testing.T, dir string, s settings) {
	t.Helper()
	if err := storage.New(dir).Write(Slug, "", s); err != nil {
		t.Fatalf("seeding settings: %v", err)
	}
}

func seedWeather(t *testing.T, dir string, r weather.Reading) {
	t.Helper()
	if err := storage.New(dir).Write(Slug, weatherLabel, r); err != nil {
		t.Fatalf("seeding weather cache: %v", err)
	}
}

// withLocation adds the manual location the dashboard would write.
func withLocation(s settings) settings {
	lat, lon := testLat, testLon
	s.Lat, s.Lon = &lat, &lon
	s.Location = locationManual
	return s
}

// newFaceRig boots the face directly and settles it at 06:42 past the crossfade.
func newFaceRig(t *testing.T, dir string) *testkit.Rig {
	t.Helper()
	rig := testkit.NewRig(t, testkit.Options{StartApp: Slug, DataDir: dir})
	rig.Clock.Advance(morningOffset)
	rig.TickFor("2100ms")
	return rig
}

func TestMorningFaceWithWeatherGolden(t *testing.T) {
	stubWeather(t, nil)
	dir := t.TempDir()
	seedSettings(t, dir, withLocation(defaultSettings()))
	// A cache stamped at open is fresh, so nothing is fetched.
	seedWeather(t, dir, weather.Reading{
		TempNow: 54, High: 68, Low: 51, Code: 2, Units: "F",
		At: clock.NewManualClock().Now().Add(morningOffset),
	})

	rig := newFaceRig(t, dir)
	rig.Golden("face-weather")

	if got := rig.RT.PanelBrightness(); got != defaultSettings().Brightness {
		t.Errorf("face should dim the panel to its brightness setting, got %d", got)
	}
}

// nightReading is a clear pre-dawn sky, sunrise 38 minutes after the rig's 06:42.
func nightReading() weather.Reading {
	epoch := clock.NewManualClock().Now()
	return weather.Reading{
		TempNow: 41, High: 66, Low: 39, Code: 0, Units: "F",
		At:          epoch.Add(morningOffset),
		Sunrise:     epoch.Add(7*time.Hour + 20*time.Minute),
		Sunset:      epoch.Add(19 * time.Hour),
		SunriseNext: epoch.Add(31*time.Hour + 20*time.Minute),
	}
}

func TestMorningNightFaceGolden(t *testing.T) {
	stubWeather(t, nil)
	dir := t.TempDir()
	seedSettings(t, dir, withLocation(defaultSettings()))
	seedWeather(t, dir, nightReading())

	// The jump to 06:42 outlasts the hold, so the line has already handed over once.
	rig := newFaceRig(t, dir)
	if !sunriseLineUp(rig) {
		t.Fatal("a dark morning should have rotated to the sunrise line")
	}
	rig.Golden("face-night")
}

// sunriseLineUp reports whether the sunrise has the lower line, by looking for
// its unbroken seven-pixel horizon (forecast glyphs never join more than three).
func sunriseLineUp(rig *testkit.Rig) bool {
	lit := 0
	for x := 0; x < panelW; x++ {
		if p := rig.PixelAt(x, outlookY+smallGlyphH-1); p.R|p.G|p.B != 0 {
			lit++
			if lit >= sunriseMarkW {
				return true
			}
			continue
		}
		lit = 0
	}
	return false
}

func TestMorningOutlookRotatesOnlyWhileTheSunIsDown(t *testing.T) {
	stubWeather(t, nil)
	dir := t.TempDir()
	seedSettings(t, dir, withLocation(defaultSettings()))
	seedWeather(t, dir, nightReading())
	rig := newFaceRig(t, dir)

	for i, wantRise := range []bool{false, true} {
		rig.Clock.Advance(outlookHold)
		rig.TickFor("2100ms")
		if got := sunriseLineUp(rig); got != wantRise {
			t.Errorf("turn %d: sunrise line up = %v, want %v", i, got, wantRise)
		}
	}

	// Past sunrise the forecast keeps the line.
	rig.Clock.Advance(time.Hour)
	rig.TickFor("2100ms")
	for i := 0; i < 3; i++ {
		rig.Clock.Advance(outlookHold)
		rig.TickFor("2100ms")
		if sunriseLineUp(rig) {
			t.Fatalf("the sunrise line came back after dawn, pass %d", i)
		}
	}
}

func TestMorningRefetchWaitsForTheRoom(t *testing.T) {
	var hits atomic.Int64
	stubWeather(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		canned(w, r)
	})
	dir := t.TempDir()
	seedSettings(t, dir, withLocation(defaultSettings()))
	seedWeather(t, dir, weather.Reading{
		TempNow: 54, High: 68, Low: 51, Code: 2, Units: "F",
		At: clock.NewManualClock().Now().Add(morningOffset),
	})

	rig := newFaceRig(t, dir)
	rig.Clock.Advance(weatherInterval + time.Minute)
	rig.TickFor("500ms")
	if got := hits.Load(); got != 0 {
		t.Fatalf("a room nobody has walked through should not refetch, made %d requests", got)
	}

	rig.RT.InjectMotion()
	waitFor(t, rig, "the refetch somebody walking in earns", func() bool {
		return hits.Load() > 0
	})
}

func TestMorningFaceWithoutWeatherGolden(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings()) // no lat/lon: clock only
	rig := newFaceRig(t, dir)
	rig.Golden("face-no-weather")
}

func TestMorningFaceBlanksThePeripherals(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	rig := newFaceRig(t, dir)

	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatal("rig hardware should be the virtual suite")
	}
	for _, name := range hardware.LightButtons {
		if hw.ButtonLit(0, name) || hw.ButtonLit(1, name) {
			t.Errorf("the face should leave every lamp out, %s is lit", name)
		}
	}
	if l1, l2 := hw.Lines(0); l1 != "" || l2 != "" {
		t.Errorf("the face should blank the LCDs, got %q / %q", l1, l2)
	}
	if got := hw.BacklightLevel(); got != 0 {
		t.Errorf("the face should take the backlight to 0, got %d", got)
	}
}

func TestMorningMinuteCrossfade(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	rig := newFaceRig(t, dir)

	lit := func(x, y int) uint8 { return rig.PixelAt(x, y).R }
	two, three := lit(onlyTwoX, digitRow6Y), lit(onlyThreeX, digitRow6Y)
	if two == 0 || three != 0 {
		t.Fatalf("expected 6:42 settled before the turn, got 2=%d 3=%d", two, three)
	}

	rig.Clock.Advance(time.Minute)
	rig.Tick(1)
	rig.TickFor("700ms")
	mid2, mid3 := lit(onlyTwoX, digitRow6Y), lit(onlyThreeX, digitRow6Y)
	if mid2 == 0 || mid2 >= two {
		t.Errorf("the old minute should still be fading, got %d (was %d)", mid2, two)
	}
	if mid3 == 0 || mid3 >= two {
		t.Errorf("the new minute should be part way in, got %d (full is %d)", mid3, two)
	}

	rig.TickFor("1500ms")
	if got := lit(onlyTwoX, digitRow6Y); got != 0 {
		t.Errorf("the old minute should be gone, got %d", got)
	}
	if got := lit(onlyThreeX, digitRow6Y); got != two {
		t.Errorf("the new minute should be at full strength %d, got %d", two, got)
	}
}

func TestMorningShipsItsScheduleWithoutWritingIt(t *testing.T) {
	m, err := engine.Lookup(Slug)
	if err != nil {
		t.Fatal(err)
	}
	if want := defaultSettings().Schedule; m.DefaultSchedule != want {
		t.Errorf("manifest DefaultSchedule = %+v, want %+v", m.DefaultSchedule, want)
	}

	dir := t.TempDir()
	newFaceRig(t, dir) // no settings file at all
	var got *settings
	if err := storage.New(dir).Read(Slug, "", &got); err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if got != nil {
		t.Errorf("first run wrote settings %+v; the manifest default should stand in for them", *got)
	}
}

func TestMorningStaleMarker(t *testing.T) {
	stubWeather(t, nil)
	dir := t.TempDir()
	seedSettings(t, dir, withLocation(defaultSettings()))
	// Four hours old, past staleAfter.
	seedWeather(t, dir, weather.Reading{
		TempNow: 54, High: 68, Low: 51, Code: 2, Units: "F",
		At: clock.NewManualClock().Now().Add(morningOffset - 4*time.Hour),
	})
	rig := newFaceRig(t, dir)

	// The O's top-left pixel, in an 11px-wide string right-aligned to staleRight.
	if got := rig.PixelAt(staleRight-10, staleY); got.A == 0 {
		t.Error("a reading older than three hours should carry the stale marker")
	}
}

func TestMorningCachesAGoodReading(t *testing.T) {
	stubWeather(t, canned)
	dir := t.TempDir()
	seedSettings(t, dir, withLocation(defaultSettings()))

	rig := newFaceRig(t, dir)
	var got weather.Reading
	for i := 0; i < 200; i++ {
		rig.Tick(1)
		if err := storage.New(dir).Read(Slug, weatherLabel, &got); err != nil {
			t.Fatalf("reading the weather cache: %v", err)
		}
		if !got.At.IsZero() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got.At.IsZero() {
		t.Fatal("a good fetch should have been written to the cache")
	}
	if got.TempNow != 54.3 || got.High != 68.1 || got.Low != 51.4 || got.Code != 2 {
		t.Errorf("cached the wrong reading: %+v", got)
	}
	if got.Units != "F" {
		t.Errorf("cache should record the unit it was fetched in, got %q", got.Units)
	}
}

// waitFor ticks the rig until done reports true, giving background goroutines real time.
func waitFor(t *testing.T, rig *testkit.Rig, what string, done func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		rig.Tick(1)
		if done() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestMorningAutoLocationCachesTheFixAndDrawsTheWeather(t *testing.T) {
	var asked struct {
		lat, lon string
	}
	stubWeather(t, func(w http.ResponseWriter, r *http.Request) {
		asked.lat, asked.lon = r.URL.Query().Get("latitude"), r.URL.Query().Get("longitude")
		canned(w, r)
	})
	hits := stubWhereAmI(t, cannedWhereAmI)

	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings()) // auto, and no cached fix
	rig := newFaceRig(t, dir)

	var fix weather.Location
	waitFor(t, rig, "the location cache", func() bool {
		if err := storage.New(dir).Read(Slug, locationLabel, &fix); err != nil {
			t.Fatalf("reading the location cache: %v", err)
		}
		return !fix.At.IsZero()
	})
	if fix.Lat != testLat || fix.Lon != testLon {
		t.Errorf("cached fix = %v,%v, want %v,%v", fix.Lat, fix.Lon, testLat, testLon)
	}
	if fix.City != "San Francisco" || fix.Source != "cloudflare-ip" {
		t.Errorf("cached fix lost its provenance: %+v", fix)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("the lookup should run once per load, ran %d times", got)
	}

	var reading weather.Reading
	waitFor(t, rig, "the weather cache", func() bool {
		if err := storage.New(dir).Read(Slug, weatherLabel, &reading); err != nil {
			t.Fatalf("reading the weather cache: %v", err)
		}
		return !reading.At.IsZero()
	})
	if asked.lat != "37.77" || asked.lon != "-122.42" {
		t.Errorf("the forecast was fetched for %q,%q, want the resolved fix", asked.lat, asked.lon)
	}
	if !weatherRowDrawn(rig) {
		t.Error("the weather row should be on the face once the fix resolves")
	}
}

// weatherRowDrawn reports whether anything is lit in the conditions band.
func weatherRowDrawn(rig *testkit.Rig) bool {
	for y := condY; y < condY+weatherIconPx; y++ {
		for x := 0; x < panelW; x++ {
			if p := rig.PixelAt(x, y); p.R|p.G|p.B != 0 {
				return true
			}
		}
	}
	return false
}

func TestMorningAutoLocationPrefersAFreshCache(t *testing.T) {
	hits := stubWhereAmI(t, cannedWhereAmI)
	stubWeather(t, nil)

	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	if err := storage.New(dir).Write(Slug, locationLabel, weather.Location{
		Lat: 51.5, Lon: -0.13, City: "London", Source: "cloudflare-ip",
		At: clock.NewManualClock().Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seeding the location cache: %v", err)
	}

	rig := newFaceRig(t, dir)
	rig.TickFor("500ms")
	if got := hits.Load(); got != 0 {
		t.Errorf("a fix an hour old should be used as is, but the lookup ran %d times", got)
	}
}

func TestMorningAutoLocationRefetchesAStaleCache(t *testing.T) {
	hits := stubWhereAmI(t, cannedWhereAmI)
	stubWeather(t, nil)

	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	stale := clock.NewManualClock().Now().Add(-locationTTL - time.Hour)
	if err := storage.New(dir).Write(Slug, locationLabel, weather.Location{
		Lat: 51.5, Lon: -0.13, City: "London", At: stale,
	}); err != nil {
		t.Fatalf("seeding the location cache: %v", err)
	}

	rig := newFaceRig(t, dir)
	var fix weather.Location
	waitFor(t, rig, "the refreshed fix", func() bool {
		if err := storage.New(dir).Read(Slug, locationLabel, &fix); err != nil {
			t.Fatalf("reading the location cache: %v", err)
		}
		return fix.City == "San Francisco"
	})
	if got := hits.Load(); got != 1 {
		t.Errorf("a fix past its day should be refetched once, ran %d times", got)
	}
}

func TestMorningManualLocationIgnoresTheLookup(t *testing.T) {
	var asked url.Values
	stubWeather(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query()
		canned(w, r)
	})
	hits := stubWhereAmI(t, cannedWhereAmI)

	dir := t.TempDir()
	seedSettings(t, dir, withLocation(defaultSettings()))
	rig := newFaceRig(t, dir)

	var reading weather.Reading
	waitFor(t, rig, "the weather cache", func() bool {
		if err := storage.New(dir).Read(Slug, weatherLabel, &reading); err != nil {
			t.Fatalf("reading the weather cache: %v", err)
		}
		return !reading.At.IsZero()
	})
	if got := hits.Load(); got != 0 {
		t.Errorf("manual mode should never ask where it is, asked %d times", got)
	}
	if asked.Get("latitude") != "37.77" {
		t.Errorf("manual mode should fetch for its own coordinates, got %q", asked.Get("latitude"))
	}
	var cachedFix weather.Location
	if err := storage.New(dir).Read(Slug, locationLabel, &cachedFix); err != nil {
		t.Fatalf("reading the location cache: %v", err)
	}
	if !cachedFix.At.IsZero() {
		t.Error("manual mode should leave no automatic fix behind")
	}
}

func TestMorningCoordinatesWithoutTheKeyReadAsManual(t *testing.T) {
	var asked url.Values
	stubWeather(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query()
		canned(w, r)
	})
	hits := stubWhereAmI(t, cannedWhereAmI)

	dir := t.TempDir()
	if err := storage.New(dir).Write(Slug, "", map[string]any{
		"schedule":   defaultSettings().Schedule,
		"brightness": 15,
		"units":      "F",
		"lat":        testLat,
		"lon":        testLon,
	}); err != nil {
		t.Fatalf("seeding legacy settings: %v", err)
	}

	rig := newFaceRig(t, dir)
	var reading weather.Reading
	waitFor(t, rig, "the weather cache", func() bool {
		if err := storage.New(dir).Read(Slug, weatherLabel, &reading); err != nil {
			t.Fatalf("reading the weather cache: %v", err)
		}
		return !reading.At.IsZero()
	})
	if got := hits.Load(); got != 0 {
		t.Errorf("a file with coordinates is manual, but the lookup ran %d times", got)
	}
	if asked.Get("latitude") != "37.77" {
		t.Errorf("the legacy coordinates should still drive the fetch, got %q", asked.Get("latitude"))
	}
}

func TestMorningLocationModeDefaults(t *testing.T) {
	cases := []struct {
		stored    string
		hasCoords bool
		want      string
	}{
		{"", false, locationAuto},
		{"", true, locationManual},
		{"auto", false, locationAuto},
		{"auto", true, locationAuto},
		{"manual", false, locationManual},
		{"manual", true, locationManual},
		{"nonsense", false, locationAuto},
		{"nonsense", true, locationManual},
	}
	for _, c := range cases {
		if got := locationMode(c.stored, c.hasCoords); got != c.want {
			t.Errorf("locationMode(%q, %v) = %q, want %q", c.stored, c.hasCoords, got, c.want)
		}
	}
}

func TestMorningFailedLookupDrawsTheClockAlone(t *testing.T) {
	stubWhereAmI(t, nil) // the endpoint's 404: no fix at the edge
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())

	rig := newFaceRig(t, dir)
	rig.TickFor("500ms")
	if weatherRowDrawn(rig) {
		t.Error("an unresolved location should leave the weather row off entirely")
	}
	var fix weather.Location
	if err := storage.New(dir).Read(Slug, locationLabel, &fix); err != nil {
		t.Fatalf("reading the location cache: %v", err)
	}
	if !fix.At.IsZero() {
		t.Error("a failed lookup should cache nothing")
	}
}

type stubApp struct{}

func newStub(*engine.AppContext) engine.App                 { return stubApp{} }
func (stubApp) Init() error                                 { return nil }
func (stubApp) HandleInput(clock.FrameTime)                 {}
func (stubApp) Update(clock.FrameTime)                      {}
func (stubApp) Render(clock.FrameTime, engine.RenderReason) {}
func (stubApp) Teardown() error                             { return nil }

// newSettingsRig reaches settings the real way: idle into ATTRACT inside the window, then green.
func newSettingsRig(t *testing.T, dir string) *testkit.Rig {
	t.Helper()
	for _, m := range []engine.Manifest{
		{Slug: "test/idle", Name: "Idle", Type: engine.TypeApp, Hidden: true, New: newStub},
		{Slug: "test/shell", Name: "Shell", Type: engine.TypeShell, Hidden: true, New: newStub},
	} {
		engine.Register(m)
		slug := m.Slug
		t.Cleanup(func() { engine.Unregister(slug) })
	}

	rig := testkit.NewRig(t, testkit.Options{
		StartApp: "test/idle", DataDir: dir, AttractAfter: time.Second,
	})
	rig.Clock.Advance(morningOffset)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != Slug {
		t.Fatalf("expected the scheduled screensaver to take over attract, got %q", got)
	}
	rig.Press(input.KeyJoyA)
	rig.TickFor("600ms") // the scene only takes input once its transition lands
	if got := rig.RT.CurrentSlug(); got != Slug {
		t.Fatalf("green should reload morning for settings, got %q", got)
	}
	return rig
}

func TestMorningSettingsSceneEditsAndSaves(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	rig := newSettingsRig(t, dir)

	// Right twice steps 06:00 to 06:30.
	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyRight, false)
	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(2)

	rig.PressKeyboard(input.KeyDown, false)
	rig.PressKeyboard(input.KeyDown, false)
	rig.Tick(1)
	if hw, ok := rig.RT.RawHardware().(*hardware.Virtual); ok {
		if !hw.ButtonLit(0, "joya") || !hw.ButtonLit(0, "joyb") {
			t.Error("the DAYS row should light both green and red")
		}
	}
	rig.Press(input.KeyJoyA)
	rig.Tick(1)

	rig.Press(input.KeyJoyB)
	rig.Tick(2)

	var got settings
	if err := storage.New(dir).Read(Slug, "", &got); err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if got.Schedule.From != "06:30" {
		t.Errorf("FROM should have stepped to 06:30, got %q", got.Schedule.From)
	}
	if got.Schedule.Days[0] {
		t.Error("green on the DAYS row should have turned Sunday off")
	}
	if got.Schedule.To != "09:00" || !got.Schedule.Enabled {
		t.Errorf("untouched rows should be unchanged, got %+v", got.Schedule)
	}
	if got := rig.RT.CurrentSlug(); got != Slug {
		t.Errorf("red should have gone back to the screensaver, got %q", got)
	}
}

func TestMorningSettingsRedReturnsToTheScreensaver(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	rig := newSettingsRig(t, dir)

	// Disabling the window leaves only the pin to bring it back.
	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(1)

	for i := 0; i < 4; i++ {
		rig.PressKeyboard(input.KeyDown, false)
		rig.Tick(1)
	}
	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(1)

	rig.Press(input.KeyJoyB)
	rig.Tick(2)

	if got := rig.RT.CurrentSlug(); got != Slug {
		t.Fatalf("red should have landed back on the screensaver, got %q", got)
	}
	if got := rig.RT.StandbyState(); got != engine.StateAttract {
		t.Errorf("the return should be to ATTRACT, got %v", got)
	}
	if got := rig.RT.PanelBrightness(); got != defaultSettings().Brightness+brightStep {
		t.Errorf("the reloaded face should read the brightness just saved, got %d", got)
	}

	var saved settings
	if err := storage.New(dir).Read(Slug, "", &saved); err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if saved.Schedule.Enabled {
		t.Fatal("the test needs the window shut to prove the pin holds")
	}

	// Any other button wakes to the shell and unpins.
	rig.Press(input.KeyEnter)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "test/shell" {
		t.Fatalf("a press should wake to the shell, got %q", got)
	}
	rig.Clock.Advance(2 * time.Second)
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got == Slug {
		t.Error("the pin should not outlive the wake")
	}
}

func TestMorningSettingsTogglesTheLocationMode(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	rig := newSettingsRig(t, dir)

	rig.PressKeyboard(input.KeyUp, false)
	rig.PressKeyboard(input.KeyRight, false)
	rig.Tick(2)
	rig.Press(input.KeyJoyB)
	rig.Tick(2)

	var doc map[string]json.RawMessage
	if err := storage.New(dir).Read(Slug, "", &doc); err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if got := string(doc["location"]); got != `"manual"` {
		t.Errorf("stored location mode = %s, want \"manual\"", got)
	}
	if _, ok := doc["schedule"]; !ok {
		t.Error("toggling the location must not drop the schedule")
	}
}

func TestMorningSettingsLampsOffTheDaysRow(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, defaultSettings())
	rig := newSettingsRig(t, dir)

	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatal("rig hardware should be the virtual suite")
	}
	if !hw.ButtonLit(0, "joyb") {
		t.Error("red saves from every row, so it should always be lit")
	}
	if hw.ButtonLit(0, "joya") {
		t.Error("green does nothing on the ENABLED row and should be dark")
	}
	rig.Golden("settings")
}

func TestClockAndDateFormatting(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2024, 9, 15, h, m, 0, 0, time.UTC) }
	cases := []struct {
		now            time.Time
		digits, suffix string
	}{
		{at(6, 43), "6:43", "AM"},
		{at(0, 5), "12:05", "AM"},
		{at(12, 0), "12:00", "PM"},
		{at(13, 9), "1:09", "PM"},
	}
	for _, c := range cases {
		d, s := formatClock(c.now)
		if d != c.digits || s != c.suffix {
			t.Errorf("formatClock(%v) = %q %q, want %q %q", c.now, d, s, c.digits, c.suffix)
		}
	}
	if got := formatDate(at(6, 43)); got != "SUN SEP 15" {
		t.Errorf("formatDate = %q", got)
	}
}

func TestBigDigitMetrics(t *testing.T) {
	// 7+2+7+7 glyph columns and three 2px gaps.
	if got := bigTextWidth("6:42"); got != 29 {
		t.Errorf("bigTextWidth(6:42) = %d, want 29", got)
	}
	for r, rows := range bigGlyphs {
		if len(rows) != bigGlyphH {
			t.Errorf("glyph %q has %d rows, want %d", r, len(rows), bigGlyphH)
		}
		for i, row := range rows {
			if len(row) != len(rows[0]) {
				t.Errorf("glyph %q row %d is %d wide, want %d", r, i, len(row), len(rows[0]))
			}
		}
	}
}

func TestWeatherIconsAreSquare(t *testing.T) {
	for _, set := range []map[weather.Sky][]string{weatherIcons, nightIcons} {
		for sky, art := range set {
			if len(art) != weatherIconPx {
				t.Errorf("sky %d has %d rows, want %d", sky, len(art), weatherIconPx)
			}
			for i, row := range art {
				if len(row) != weatherIconPx {
					t.Errorf("sky %d row %d is %d wide, want %d", sky, i, len(row), weatherIconPx)
				}
			}
		}
	}
	for sky := range weatherIcons {
		for _, night := range []bool{false, true} {
			got := weatherIcon(sky, night, display.White, display.White, display.White)
			if len(got) != weatherIconPx {
				t.Errorf("sky %d (night=%v) rasterized to %d rows", sky, night, len(got))
			}
		}
	}
}
