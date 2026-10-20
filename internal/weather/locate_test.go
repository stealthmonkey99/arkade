// SPDX-License-Identifier: GPL-2.0-or-later

package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// stubWhereAmI points Locate at a local server for the rest of the test, so
// nothing here ever reaches arkade.build.
func stubWhereAmI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	prev := WhereAmIURL
	WhereAmIURL = srv.URL
	t.Cleanup(func() {
		WhereAmIURL = prev
		srv.Close()
	})
}

func TestLocateParsesAFix(t *testing.T) {
	stubWhereAmI(t, func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua != CabinetUserAgent {
			t.Errorf("User-Agent = %q, want %q", ua, CabinetUserAgent)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"lat":37.77,"lon":-122.42,"city":"San Francisco",` +
			`"region":"California","country":"US","timezone":"America/Los_Angeles",` +
			`"source":"cloudflare-ip"}`))
	})

	before := time.Now()
	got, err := Locate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if got.Lat != 37.77 || got.Lon != -122.42 {
		t.Errorf("coordinates = %v,%v, want 37.77,-122.42", got.Lat, got.Lon)
	}
	if got.City != "San Francisco" || got.Region != "California" || got.Country != "US" {
		t.Errorf("place = %+v, want San Francisco, California, US", got)
	}
	if got.Timezone != "America/Los_Angeles" || got.Source != "cloudflare-ip" {
		t.Errorf("timezone/source = %q/%q", got.Timezone, got.Source)
	}
	if got.At.Before(before) {
		t.Errorf("At = %v, want it stamped at the fetch", got.At)
	}
}

func TestLocateFieldsBeyondCoordinatesAreOptional(t *testing.T) {
	stubWhereAmI(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"lat":51.5,"lon":-0.13}`))
	})

	got, err := Locate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if got.City != "" || got.Source != "" {
		t.Errorf("unnamed fields should stay empty, got %+v", got)
	}
	if got.Lat != 51.5 || got.Lon != -0.13 {
		t.Errorf("coordinates = %v,%v, want 51.5,-0.13", got.Lat, got.Lon)
	}
}

func TestLocateRejections(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"no fix at the edge", http.StatusNotFound, `{"error":"location unavailable"}`},
		{"server error", http.StatusBadGateway, `nope`},
		{"garbage body", http.StatusOK, `<html>captive portal</html>`},
		{"truncated json", http.StatusOK, `{"lat":37.7`},
		{"no coordinates", http.StatusOK, `{"city":"Nowhere"}`},
		{"lat without lon", http.StatusOK, `{"lat":37.77}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubWhereAmI(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := Locate(context.Background(), nil)
			if err == nil {
				t.Fatalf("Locate = %+v, want an error", got)
			}
		})
	}
}

func TestLocateHonoursACancelledContext(t *testing.T) {
	stubWhereAmI(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"lat":1,"lon":2}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Locate(ctx, nil); err == nil {
		t.Fatal("a cancelled context should fail the lookup")
	}
}
