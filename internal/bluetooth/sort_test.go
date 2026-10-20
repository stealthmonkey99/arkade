// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import "testing"

func names(devs []Device) []string {
	out := make([]string, len(devs))
	for i, d := range devs {
		out[i] = DisplayName(d)
	}
	return out
}

func TestRankerTiersControllerThenNamedThenAnonymous(t *testing.T) {
	var r Ranker
	devs := []Device{
		{Address: "AA:01", Name: "Speaker"},
		{Address: "0A:1B:2C:3D:4E:5F", Name: "0A-1B-2C-3D-4E-5F"},
		{Address: "AA:03", Name: "Lamp-BT"},
		{Address: "AA:04"},
		{Address: "AA:05", Name: "Wireless Controller", Input: true},
		{Address: "AA:06", Name: "Television"},
	}
	r.Sort(devs)

	if DisplayName(devs[0]) != "Wireless Controller" {
		t.Errorf("first row is %q, want the controller", DisplayName(devs[0]))
	}
	got := names(devs)[1:4]
	want := []string{"Speaker", "Lamp-BT", "Television"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("named tier = %v, want %v", got, want)
			break
		}
	}
	for _, d := range devs[4:] {
		if HasRealName(d) {
			t.Errorf("%q sorted into the anonymous tier", DisplayName(d))
		}
	}
}

func TestAMacShapedAliasIsNotAName(t *testing.T) {
	cases := []struct {
		name string
		dev  Device
		want bool
	}{
		{"dashed alias", Device{Address: "0A:1B:2C:3D:4E:5F", Name: "0A-1B-2C-3D-4E-5F"}, false},
		{"same spelling", Device{Address: "0A:1B:2C:3D:4E:5F", Name: "0A:1B:2C:3D:4E:5F"}, false},
		{"lower case", Device{Address: "0A:1B:2C:3D:4E:5F", Name: "0a-1b-2c-3d-4e-5f"}, false},
		{"no name at all", Device{Address: "0A:1B:2C:3D:4E:5F"}, false},
		{"a real name", Device{Address: "0A:1B:2C:3D:4E:5F", Name: "Speaker"}, true},
	}
	for _, tc := range cases {
		if got := HasRealName(tc.dev); got != tc.want {
			t.Errorf("%s: HasRealName = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRowsDoNotMoveAsDevicesArrive(t *testing.T) {
	var r Ranker
	first := []Device{
		{Address: "AA:01", Name: "Speaker"},
		{Address: "AA:02", Name: "Lamp-BT"},
	}
	r.Sort(first)
	order := names(first)

	second := []Device{
		{Address: "AA:03", Name: "Television"},
		{Address: "AA:02", Name: "Lamp-BT"},
		{Address: "AA:04"},
		{Address: "AA:01", Name: "Speaker"},
	}
	r.Sort(second)

	got := names(second)
	for i := range order {
		if got[i] != order[i] {
			t.Fatalf("row %d moved from %q to %q as new devices arrived", i, order[i], got[i])
		}
	}
	if got[2] != "Television" {
		t.Errorf("the new named device landed at %q, want it appended to its tier", got[2])
	}
}

func TestSignalDriftCannotReorderAnything(t *testing.T) {
	var r Ranker
	devs := []Device{
		{Address: "AA:01", Name: "Alpha", RSSI: -80},
		{Address: "AA:02", Name: "Bravo", RSSI: -40},
	}
	r.Sort(devs)
	order := names(devs)

	devs[0].RSSI, devs[1].RSSI = -40, -80
	r.Sort(devs)
	if got := names(devs); got[0] != order[0] || got[1] != order[1] {
		t.Errorf("order changed from %v to %v on signal alone", order, got)
	}
}

func TestADeviceThatBlinksKeepsItsPlace(t *testing.T) {
	var r Ranker
	all := []Device{
		{Address: "AA:01", Name: "Speaker"},
		{Address: "AA:02", Name: "Lamp-BT"},
		{Address: "AA:03", Name: "Television"},
	}
	r.Sort(all)

	gone := []Device{all[0], all[2]}
	r.Sort(gone)
	back := []Device{all[2], all[1], all[0]}
	r.Sort(back)

	if got := names(back); got[1] != "Lamp-BT" {
		t.Errorf("a device that blinked came back at %v, want it in its original place", got)
	}
}

func TestDisplayNameFallsBackToAddress(t *testing.T) {
	if got := DisplayName(Device{Address: "AA:BB", Name: "Pad"}); got != "Pad" {
		t.Errorf("DisplayName = %q, want the name", got)
	}
	if got := DisplayName(Device{Address: "AA:BB"}); got != "AA:BB" {
		t.Errorf("DisplayName = %q, want the address", got)
	}
}
