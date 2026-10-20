// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Peripherals records the host_set_text, host_set_button_lights,
// host_set_lights, player-strategy and host_exit calls a scripted journey
// makes, in order. Its JSON form is the golden file any conformant host
// checks its own recording against.
type Peripherals struct {
	SetText      []PeripheralText     `json:"setText"`
	ButtonLights []PeripheralButtons  `json:"buttonLights"`
	LightSets    []PeripheralLights   `json:"lightSets"`
	Strategy     []PeripheralStrategy `json:"strategy"`
	ExitCalls    int                  `json:"exitCalls"`
}

// PeripheralText is one host_set_text call. Player is the seat argument as
// the host received it: 0 or 1, or -1 for both seats.
type PeripheralText struct {
	Player int    `json:"player"`
	Line1  string `json:"line1"`
	Line2  string `json:"line2"`
}

// PeripheralButtons is one host_set_button_lights call.
type PeripheralButtons struct {
	Player int             `json:"player"`
	Lights map[string]bool `json:"lights"`
	Only   bool            `json:"only"`
}

// PeripheralLights is one host_set_lights call.
type PeripheralLights struct {
	Player int  `json:"player"`
	On     bool `json:"on"`
}

// PeripheralStrategy is one player-strategy call. Op is "push", "pop",
// "lock", "unlock", "switch" or "lockseat"; Kind is meaningful only for
// "push", Mode only for "switch", Seat only for "lockseat".
type PeripheralStrategy struct {
	Op   string `json:"op"`
	Kind uint32 `json:"kind,omitempty"`
	Mode uint32 `json:"mode,omitempty"`
	Seat int    `json:"seat,omitempty"`
}

// RecordedPeripherals snapshots h's peripheral calls in oracle form.
func RecordedPeripherals(h *FakeHost) Peripherals {
	h.Mu.Lock()
	defer h.Mu.Unlock()
	var p Peripherals
	for _, c := range h.LCDTexts {
		p.SetText = append(p.SetText, PeripheralText{Player: c.Player, Line1: c.Line1, Line2: c.Line2})
	}
	for _, c := range h.ButtonLights {
		p.ButtonLights = append(p.ButtonLights, PeripheralButtons{Player: c.Player, Lights: c.Lights, Only: c.Only})
	}
	for _, c := range h.LightSets {
		p.LightSets = append(p.LightSets, PeripheralLights{Player: c.Player, On: c.On})
	}
	for _, c := range h.StrategyCalls {
		p.Strategy = append(p.Strategy, PeripheralStrategy{Op: c.Op, Kind: uint32(c.Kind), Mode: uint32(c.Mode), Seat: c.Seat})
	}
	p.ExitCalls = h.Exits
	return p
}

// GoldenPeripherals checks h's recorded peripheral calls against the JSON
// golden at path. UPDATE_GOLDENS=1 rewrites the file instead.
func GoldenPeripherals(t *testing.T, h *FakeHost, path string) {
	t.Helper()
	got := RecordedPeripherals(h)
	if os.Getenv("UPDATE_GOLDENS") != "" {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("regenerated %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var want Peripherals
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	if len(got.SetText) != len(want.SetText) {
		t.Fatalf("host_set_text calls = %d, want %d\ngot:  %+v\nwant: %+v", len(got.SetText), len(want.SetText), got.SetText, want.SetText)
	}
	for i := range want.SetText {
		if got.SetText[i] != want.SetText[i] {
			t.Errorf("host_set_text[%d] = %+v, want %+v", i, got.SetText[i], want.SetText[i])
		}
	}
	if len(got.ButtonLights) != len(want.ButtonLights) {
		t.Fatalf("host_set_button_lights calls = %d, want %d\ngot:  %+v\nwant: %+v", len(got.ButtonLights), len(want.ButtonLights), got.ButtonLights, want.ButtonLights)
	}
	for i, w := range want.ButtonLights {
		g := got.ButtonLights[i]
		if g.Player != w.Player || g.Only != w.Only || !reflect.DeepEqual(g.Lights, w.Lights) {
			t.Errorf("host_set_button_lights[%d] = %+v, want %+v", i, g, w)
		}
	}
	if len(got.LightSets) != len(want.LightSets) {
		t.Fatalf("host_set_lights calls = %d, want %d\ngot:  %+v\nwant: %+v", len(got.LightSets), len(want.LightSets), got.LightSets, want.LightSets)
	}
	for i := range want.LightSets {
		if got.LightSets[i] != want.LightSets[i] {
			t.Errorf("host_set_lights[%d] = %+v, want %+v", i, got.LightSets[i], want.LightSets[i])
		}
	}
	if len(got.Strategy) != len(want.Strategy) {
		t.Fatalf("player-strategy calls = %d, want %d\ngot:  %+v\nwant: %+v", len(got.Strategy), len(want.Strategy), got.Strategy, want.Strategy)
	}
	for i := range want.Strategy {
		if got.Strategy[i] != want.Strategy[i] {
			t.Errorf("player-strategy[%d] = %+v, want %+v", i, got.Strategy[i], want.Strategy[i])
		}
	}
	if got.ExitCalls != want.ExitCalls {
		t.Errorf("host_exit calls = %d, want %d", got.ExitCalls, want.ExitCalls)
	}
}
