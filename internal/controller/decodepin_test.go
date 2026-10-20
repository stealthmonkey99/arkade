// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/input"
)

// Pins the Report each built-in pad produced from recorded reports.
func TestBuiltinSpecsDecodeRecordedReports(t *testing.T) {
	bt := func(mutate func(r []byte)) []byte {
		r := dualShockBTRest()
		mutate(r)
		return r
	}
	cases := []struct {
		profile Profile
		report  []byte
		want    Report
	}{
		{Speedlink, []byte{0, 128, 128}, Report{}},
		{Speedlink, []byte{0b0001, 128, 128}, Report{Buttons: 1}},
		{Speedlink, []byte{0b1010, 128, 128}, Report{Buttons: 10}},
		{Speedlink, []byte{0, 0, 128}, Report{X: -128}},
		{Speedlink, []byte{0, 0, 0}, Report{X: -128, Y: -128}},
		{Speedlink, []byte{0, 255, 255}, Report{X: 127, Y: 127}},

		{DragonRise, []byte{127, 127, 0, 0, 0, 0x0f, 0}, Report{}},
		{DragonRise, []byte{127, 127, 0, 0, 0, 0x1f, 0}, Report{Buttons: 1}},
		{DragonRise, []byte{127, 127, 0, 0, 0, 0x0f, 0b1}, Report{Buttons: 16}},
		{DragonRise, []byte{0, 255, 0, 0, 0, 0x0f, 0}, Report{X: -127, Y: 128}},
		{DragonRise, []byte{255, 0, 0, 0, 0, 0xff, 0xff}, Report{Buttons: 4095, X: 128, Y: -127}},

		{Buffalo, []byte{128, 128, 0, 0}, Report{}},
		{Buffalo, []byte{132, 124, 0, 0}, Report{}},
		{Buffalo, []byte{0, 128, 0, 0}, Report{X: -13}},
		{Buffalo, []byte{128, 128, 0, 0b100}, Report{Buttons: 64}},
		{Buffalo, []byte{128, 128, 0b1111, 0}, Report{Buttons: 15}},
		{Buffalo, []byte{255, 255, 0xff, 0xff}, Report{Buttons: 1023, X: 13, Y: 13}},

		{Sonicon, []byte{0, 0, 8}, Report{}},
		{Sonicon, []byte{0, 0, 0}, Report{Y: -1}},
		{Sonicon, []byte{0, 0, 2}, Report{X: 2}},
		{Sonicon, []byte{0, 0b10000, 8}, Report{Buttons: 4096}},
		{Sonicon, []byte{0xff, 0xff, 5}, Report{Buttons: 8191, X: -1, Y: 1}},

		{DualShock, []byte{0, 128, 128, 0, 0, 0x08, 0, 0}, Report{HasDpad: true, Dpad: input.JoyCenter}},
		{DualShock, []byte{0, 128, 128, 0, 0, 0x00, 0, 0}, Report{HasDpad: true, Dpad: input.JoyUp}},
		{DualShock, []byte{0, 128, 128, 0, 0, 0x04, 0, 0}, Report{HasDpad: true, Dpad: input.JoyDown}},
		{DualShock, []byte{0, 128, 128, 0, 0, 0x08, 0b10000000, 0}, Report{Buttons: 512, HasDpad: true, Dpad: input.JoyCenter}},
		{DualShock, []byte{0, 255, 128, 0, 0, 0x08, 0b10000000, 0}, Report{Buttons: 512, X: 13, HasDpad: true, Dpad: input.JoyCenter}},
		{DualShock, []byte{0, 0, 0, 0, 0, 0x06, 0xff, 0xff}, Report{Buttons: 1023, X: -13, Y: -13, HasDpad: true, Dpad: input.JoyLeft}},

		{DualShockBT, dualShockBTRest(), Report{HasDpad: true, Dpad: input.JoyCenter}},
		{DualShockBT, bt(func(r []byte) { r[7] |= 0x20 }), Report{Buttons: 512, HasDpad: true, Dpad: input.JoyCenter}},
		{DualShockBT, bt(func(r []byte) { r[9] = 1 }), Report{Buttons: 1, HasDpad: true, Dpad: input.JoyCenter}},
		{DualShockBT, bt(func(r []byte) { r[3], r[4], r[7] = 0, 255, 0 }), Report{X: -13, Y: 13, HasDpad: true, Dpad: input.JoyUp}},
	}
	for _, tc := range cases {
		if got := tc.profile.Spec.Decode(tc.report); got != tc.want {
			t.Errorf("%s %v decoded to %+v, want %+v", tc.profile.Name, tc.report, got, tc.want)
		}
	}
	for _, p := range Profiles {
		if got := p.Spec.Decode(nil); got != (Report{}) {
			t.Errorf("%s decoded an empty report to %+v", p.Name, got)
		}
	}
}
