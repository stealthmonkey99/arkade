// SPDX-License-Identifier: GPL-2.0-or-later

// Package weather provides a minimal Open-Meteo client, a non-blocking poller
// for keeping one Reading fresh on a background goroutine, and an IP-based
// lookup for cabinets whose operator never typed a latitude.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Sky is the icon class a WMO weather code falls into.
type Sky int

const (
	SkyClear Sky = iota
	SkyPartly
	SkyCloud
	SkyRain
	SkySnow
	SkyFog
	SkyStorm
)

// SkyFor maps a WMO weather code to its icon class. An unrecognized code
// returns SkyCloud.
func SkyFor(code int) Sky {
	switch code {
	case 0:
		return SkyClear
	case 1, 2:
		return SkyPartly
	case 3:
		return SkyCloud
	case 45, 48:
		return SkyFog
	case 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 80, 81, 82:
		return SkyRain
	case 71, 73, 75, 77, 85, 86:
		return SkySnow
	case 95, 96, 99:
		return SkyStorm
	default:
		return SkyCloud
	}
}

// Reading is one observation: the current temperature, the day's high and
// low, the sky class, today's sun times and when it was fetched.
type Reading struct {
	TempNow float64   `json:"tempNow"`
	High    float64   `json:"high"`
	Low     float64   `json:"low"`
	Code    int       `json:"code"`
	Units   string    `json:"units"`
	At      time.Time `json:"at"`
	// Sunrise and Sunset are today's, SunriseNext tomorrow's. All are zero
	// in a reading cached without them.
	Sunrise     time.Time `json:"sunrise"`
	Sunset      time.Time `json:"sunset"`
	SunriseNext time.Time `json:"sunriseNext"`
}

// Sky is Reading's icon class, derived from Code.
func (r Reading) Sky() Sky {
	return SkyFor(r.Code)
}

// Night reports whether at falls outside the reading's daylight; a reading
// with no sun times reads as day.
func (r Reading) Night(at time.Time) bool {
	if r.Sunrise.IsZero() || r.Sunset.IsZero() {
		return false
	}
	return at.Before(r.Sunrise) || at.After(r.Sunset)
}

// NextSunrise is the first sunrise after at, and whether one is known.
func (r Reading) NextSunrise(at time.Time) (time.Time, bool) {
	for _, t := range [...]time.Time{r.Sunrise, r.SunriseNext} {
		if !t.IsZero() && at.Before(t) {
			return t, true
		}
	}
	return time.Time{}, false
}

// DefaultBaseURL is Open-Meteo's keyless forecast endpoint.
const DefaultBaseURL = "https://api.open-meteo.com/v1/forecast"

// Client fetches readings from the Open-Meteo forecast API.
type Client struct {
	BaseURL string       // defaults to DefaultBaseURL when empty
	HTTP    *http.Client // defaults to a 10s-timeout client when nil
}

type apiResponse struct {
	Current struct {
		Temperature2m float64 `json:"temperature_2m"`
		WeatherCode   int     `json:"weather_code"`
	} `json:"current"`
	Daily struct {
		Temperature2mMax []float64 `json:"temperature_2m_max"`
		Temperature2mMin []float64 `json:"temperature_2m_min"`
		Sunrise          []int64   `json:"sunrise"`
		Sunset           []int64   `json:"sunset"`
	} `json:"daily"`
}

// unixAt is days[i] as an instant, or the zero time when the response did
// not carry that day. Open-Meteo unixtime values are GMT+0, so they compare
// against any clock.
func unixAt(days []int64, i int) time.Time {
	if i >= len(days) {
		return time.Time{}
	}
	return time.Unix(days[i], 0)
}

// Fetch requests the current conditions and today's high/low at lat/lon.
// units is "F" or "C"; anything else is treated as "F". It returns an error
// on a non-200 response, malformed JSON, or a response missing the daily
// high/low.
func (c *Client) Fetch(ctx context.Context, lat, lon float64, units string) (Reading, error) {
	baseURL := c.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	tempUnit, unitLabel := "fahrenheit", "F"
	if units == "C" {
		tempUnit, unitLabel = "celsius", "C"
	}

	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(lat, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(lon, 'f', -1, 64))
	q.Set("current", "temperature_2m,weather_code")
	q.Set("daily", "temperature_2m_max,temperature_2m_min,sunrise,sunset")
	q.Set("temperature_unit", tempUnit)
	q.Set("timezone", "auto")
	q.Set("timeformat", "unixtime")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return Reading{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return Reading{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Reading{}, fmt.Errorf("weather: unexpected status %d", resp.StatusCode)
	}

	var parsed apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Reading{}, err
	}

	if len(parsed.Daily.Temperature2mMax) == 0 || len(parsed.Daily.Temperature2mMin) == 0 {
		return Reading{}, fmt.Errorf("weather: response missing daily high/low")
	}

	return Reading{
		TempNow:     parsed.Current.Temperature2m,
		High:        parsed.Daily.Temperature2mMax[0],
		Low:         parsed.Daily.Temperature2mMin[0],
		Code:        parsed.Current.WeatherCode,
		Units:       unitLabel,
		At:          time.Now(),
		Sunrise:     unixAt(parsed.Daily.Sunrise, 0),
		Sunset:      unixAt(parsed.Daily.Sunset, 0),
		SunriseNext: unixAt(parsed.Daily.Sunrise, 1),
	}, nil
}

// Poller keeps one Reading fresh on a background goroutine so a caller on a
// frame loop never blocks on the network.
type Poller struct {
	client   *Client
	lat, lon float64
	units    string
	interval time.Duration

	mu          sync.Mutex
	latest      Reading
	hasLatest   bool
	lastAttempt time.Time
	inFlight    bool
	cancel      context.CancelFunc
	closed      bool

	wg sync.WaitGroup
}

// NewPoller creates a Poller for lat/lon in units. interval is how often a
// fresh fetch is started when Poll is called.
func NewPoller(c *Client, lat, lon float64, units string, interval time.Duration) *Poller {
	return &Poller{
		client:   c,
		lat:      lat,
		lon:      lon,
		units:    units,
		interval: interval,
	}
}

// Seed installs a cached reading as the current one without a fetch. Its At
// counts as the last attempt, so Poll stays quiet until the cache ages out.
func (p *Poller) Seed(r Reading) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.latest = r
	p.hasLatest = true
	p.lastAttempt = r.At
}

// Poll starts a fetch if none is in flight and the last attempt finished at
// least interval ago. It never blocks.
func (p *Poller) Poll(now time.Time) {
	p.mu.Lock()
	if p.closed || p.inFlight || (!p.lastAttempt.IsZero() && now.Sub(p.lastAttempt) < p.interval) {
		p.mu.Unlock()
		return
	}
	p.lastAttempt = now
	p.inFlight = true
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.mu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		r, err := p.client.Fetch(ctx, p.lat, p.lon, p.units)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.inFlight = false
		p.cancel = nil
		if err == nil {
			p.latest = r
			p.hasLatest = true
		}
	}()
}

// Latest returns the most recent good reading and whether there has ever
// been one.
func (p *Poller) Latest() (Reading, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latest, p.hasLatest
}

// Close stops the poller; an in-flight fetch is cancelled. It is safe to
// call more than once.
func (p *Poller) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Unlock()
	p.wg.Wait()
}
