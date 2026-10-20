// SPDX-License-Identifier: GPL-2.0-or-later

package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// The sun times are Open-Meteo's unixtime format: 2024-09-15 06:52 and
// 19:14 Pacific, then the next morning's 06:53.
const (
	cannedSunrise     = 1726408320
	cannedSunset      = 1726452840
	cannedSunriseNext = 1726494780
)

const canned = `{"current":{"temperature_2m":54.3,"weather_code":2},
 "daily":{"temperature_2m_max":[68.1],"temperature_2m_min":[51.4],
 "sunrise":[1726408320,1726494780],"sunset":[1726452840,1726539180]}}`

func TestFetch(t *testing.T) {
	cases := []struct {
		units    string
		wantUnit string
		wantTemp string
	}{
		{"F", "fahrenheit", "F"},
		{"C", "celsius", "C"},
	}

	for _, tc := range cases {
		t.Run(tc.units, func(t *testing.T) {
			var gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				w.Write([]byte(canned))
			}))
			defer srv.Close()

			c := &Client{BaseURL: srv.URL}
			r, err := c.Fetch(context.Background(), 45.5, -122.6, tc.units)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}

			if r.TempNow != 54.3 {
				t.Errorf("TempNow = %v, want 54.3", r.TempNow)
			}
			if r.High != 68.1 {
				t.Errorf("High = %v, want 68.1", r.High)
			}
			if r.Low != 51.4 {
				t.Errorf("Low = %v, want 51.4", r.Low)
			}
			if r.Code != 2 {
				t.Errorf("Code = %v, want 2", r.Code)
			}
			if r.Units != tc.wantTemp {
				t.Errorf("Units = %v, want %v", r.Units, tc.wantTemp)
			}
			if r.At.IsZero() {
				t.Error("At is zero")
			}
			if !r.Sunrise.Equal(time.Unix(cannedSunrise, 0)) ||
				!r.Sunset.Equal(time.Unix(cannedSunset, 0)) ||
				!r.SunriseNext.Equal(time.Unix(cannedSunriseNext, 0)) {
				t.Errorf("sun times = %v/%v/%v", r.Sunrise, r.Sunset, r.SunriseNext)
			}

			q, err := url.ParseQuery(gotQuery)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			if got := q.Get("latitude"); got != "45.5" {
				t.Errorf("latitude = %q, want 45.5", got)
			}
			if got := q.Get("longitude"); got != "-122.6" {
				t.Errorf("longitude = %q, want -122.6", got)
			}
			if got := q.Get("current"); got != "temperature_2m,weather_code" {
				t.Errorf("current = %q", got)
			}
			if got := q.Get("daily"); got != "temperature_2m_max,temperature_2m_min,sunrise,sunset" {
				t.Errorf("daily = %q", got)
			}
			if got := q.Get("timeformat"); got != "unixtime" {
				t.Errorf("timeformat = %q, want unixtime", got)
			}
			if got := q.Get("temperature_unit"); got != tc.wantUnit {
				t.Errorf("temperature_unit = %q, want %q", got, tc.wantUnit)
			}
			if got := q.Get("timezone"); got != "auto" {
				t.Errorf("timezone = %q, want auto", got)
			}
		})
	}
}

func TestFetchErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		code int
	}{
		{"server error", canned, http.StatusInternalServerError},
		{"malformed json", `{"current":`, http.StatusOK},
		{"empty daily", `{"current":{"temperature_2m":1,"weather_code":0},"daily":{"temperature_2m_max":[],"temperature_2m_min":[]}}`, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := &Client{BaseURL: srv.URL}
			if _, err := c.Fetch(context.Background(), 0, 0, "F"); err == nil {
				t.Error("Fetch: want error, got nil")
			}
		})
	}
}

func TestSkyFor(t *testing.T) {
	cases := []struct {
		code int
		want Sky
	}{
		{0, SkyClear},
		{1, SkyPartly},
		{2, SkyPartly},
		{3, SkyCloud},
		{45, SkyFog},
		{48, SkyFog},
		{51, SkyRain},
		{53, SkyRain},
		{55, SkyRain},
		{56, SkyRain},
		{57, SkyRain},
		{61, SkyRain},
		{63, SkyRain},
		{65, SkyRain},
		{66, SkyRain},
		{67, SkyRain},
		{80, SkyRain},
		{81, SkyRain},
		{82, SkyRain},
		{71, SkySnow},
		{73, SkySnow},
		{75, SkySnow},
		{77, SkySnow},
		{85, SkySnow},
		{86, SkySnow},
		{95, SkyStorm},
		{96, SkyStorm},
		{99, SkyStorm},
		{-1, SkyCloud},
		{1000, SkyCloud},
	}

	for _, tc := range cases {
		if got := SkyFor(tc.code); got != tc.want {
			t.Errorf("SkyFor(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}

	r := Reading{Code: 95}
	if got := r.Sky(); got != SkyStorm {
		t.Errorf("Reading.Sky() = %v, want SkyStorm", got)
	}
}

func TestReadingSunTimes(t *testing.T) {
	rise := time.Unix(cannedSunrise, 0)
	set := time.Unix(cannedSunset, 0)
	next := time.Unix(cannedSunriseNext, 0)
	r := Reading{Sunrise: rise, Sunset: set, SunriseNext: next}

	cases := []struct {
		name     string
		at       time.Time
		night    bool
		wantRise time.Time
		haveRise bool
	}{
		{"before dawn", rise.Add(-time.Hour), true, rise, true},
		{"midday", rise.Add(6 * time.Hour), false, next, true},
		{"after dusk", set.Add(time.Hour), true, next, true},
	}
	for _, c := range cases {
		if got := r.Night(c.at); got != c.night {
			t.Errorf("%s: Night = %v, want %v", c.name, got, c.night)
		}
		got, ok := r.NextSunrise(c.at)
		if ok != c.haveRise || !got.Equal(c.wantRise) {
			t.Errorf("%s: NextSunrise = %v,%v, want %v,%v", c.name, got, ok, c.wantRise, c.haveRise)
		}
	}

	// A reading cached before the sun times existed must not claim a night.
	old := Reading{TempNow: 54}
	if old.Night(rise.Add(-time.Hour)) {
		t.Error("a reading with no sun times should read as day")
	}
	if _, ok := old.NextSunrise(rise); ok {
		t.Error("a reading with no sun times should offer no sunrise")
	}
	// Past tomorrow's sunrise there is nothing left in the reading to promise.
	if _, ok := r.NextSunrise(next.Add(time.Minute)); ok {
		t.Error("a reading should not offer a sunrise already behind it")
	}
}

func TestPoller(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Write([]byte(canned))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	p := NewPoller(c, 45.5, -122.6, "F", 100*time.Millisecond)
	t.Cleanup(p.Close)

	// Seeded well past the interval, so the seed counts as a stale attempt
	// and the first Poll is due.
	seeded := Reading{TempNow: 10, High: 20, Low: 0, Code: 0, Units: "F", At: time.Now().Add(-time.Hour)}
	p.Seed(seeded)

	if r, ok := p.Latest(); !ok || r != seeded {
		t.Fatalf("Latest before poll = %+v, %v; want seeded", r, ok)
	}

	now := time.Now()
	p.Poll(now)

	deadline := time.Now().Add(2 * time.Second)
	for {
		r, _ := p.Latest()
		if r.TempNow == 54.3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for fetched reading")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests after first poll = %d, want 1", got)
	}

	p.Poll(now.Add(10 * time.Millisecond))
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("requests after early poll = %d, want 1", got)
	}

	p.Poll(now.Add(200 * time.Millisecond))
	deadline = time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&requests) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for second fetch")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPollerFailurePreservesLatest(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL}
	p := NewPoller(c, 0, 0, "F", 10*time.Millisecond)
	t.Cleanup(p.Close)

	seeded := Reading{TempNow: 42, Units: "F", At: time.Now().Add(-time.Hour)}
	p.Seed(seeded)

	now := time.Now()
	p.Poll(now)

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&requests) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for first attempt")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	if r, ok := p.Latest(); !ok || r != seeded {
		t.Fatalf("Latest after failed poll = %+v, %v; want seeded intact", r, ok)
	}
}

func TestPollerFreshSeedSuppressesFetch(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Write([]byte(canned))
	}))
	defer srv.Close()

	p := NewPoller(&Client{BaseURL: srv.URL}, 0, 0, "F", time.Hour)
	t.Cleanup(p.Close)

	now := time.Now()
	p.Seed(Reading{TempNow: 42, Units: "F", At: now.Add(-time.Minute)})
	p.Poll(now)
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Fatalf("a seed younger than the interval should suppress the fetch, got %d requests", got)
	}

	p.Poll(now.Add(2 * time.Hour))
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&requests) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the fetch once the seed aged out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
