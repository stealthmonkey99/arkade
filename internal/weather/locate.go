// SPDX-License-Identifier: GPL-2.0-or-later

package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WhereAmIURL is arkade.build's IP-geolocation endpoint. It is a variable so
// tests can point Locate at an httptest server instead of the network.
var WhereAmIURL = "https://arkade.build/api/whereami"

// CabinetUserAgent signs a cabinet's requests to arkade.build; the whereami
// route answers only to this prefix.
const CabinetUserAgent = "Arkade-Cabinet/1 (+https://arkade.build)"

// locateTimeout bounds a whole lookup.
const locateTimeout = 10 * time.Second

// Location is one resolved fix. Lat/Lon are always set; everything the
// geolocation database could not name is empty.
type Location struct {
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	City     string    `json:"city,omitempty"`
	Region   string    `json:"region,omitempty"`
	Country  string    `json:"country,omitempty"`
	Timezone string    `json:"timezone,omitempty"`
	Source   string    `json:"source,omitempty"`
	At       time.Time `json:"at"`
}

type whereAmIResponse struct {
	Lat      *float64 `json:"lat"`
	Lon      *float64 `json:"lon"`
	City     string   `json:"city"`
	Region   string   `json:"region"`
	Country  string   `json:"country"`
	Timezone string   `json:"timezone"`
	Source   string   `json:"source"`
}

// Locate resolves where this cabinet is from its public IP. httpClient may be
// nil, which uses one with the same 10s bound as the context. It returns an
// error on a non-200 (the endpoint answers 404 when the edge has no fix),
// malformed JSON, or a response carrying no coordinates.
func Locate(ctx context.Context, httpClient *http.Client) (Location, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: locateTimeout}
	}
	ctx, cancel := context.WithTimeout(ctx, locateTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, WhereAmIURL, nil)
	if err != nil {
		return Location{}, err
	}
	req.Header.Set("User-Agent", CabinetUserAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return Location{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Location{}, fmt.Errorf("weather: whereami returned %d", resp.StatusCode)
	}
	var parsed whereAmIResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Location{}, err
	}
	if parsed.Lat == nil || parsed.Lon == nil {
		return Location{}, fmt.Errorf("weather: whereami response carried no coordinates")
	}
	return Location{
		Lat:      *parsed.Lat,
		Lon:      *parsed.Lon,
		City:     parsed.City,
		Region:   parsed.Region,
		Country:  parsed.Country,
		Timezone: parsed.Timezone,
		Source:   parsed.Source,
		At:       time.Now(),
	}, nil
}
