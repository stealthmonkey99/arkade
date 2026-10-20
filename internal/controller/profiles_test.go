// SPDX-License-Identifier: GPL-2.0-or-later

package controller

import (
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/input"
)

func TestBuiltinProfilesPinTheConvertedTables(t *testing.T) {
	cases := []struct {
		p           Profile
		name        string
		vid, pid    uint16
		directional string
		minLen      int
		buttons     []input.KeyName
	}{
		{Speedlink, "Speedlink", 0x0738, 0x2217, "stick", 3,
			[]input.KeyName{input.KeyJoyA, input.KeyJoyB, input.KeyL1, input.KeyR1}},
		{DragonRise, "DragonRise", 0x0079, 0x0006, "stick", 7,
			[]input.KeyName{
				input.KeyJoyA, input.KeyJoyB, input.KeyJoyC, input.KeyJoyD,
				input.KeyL1, input.KeyR1, input.KeyL2, input.KeyR2,
				input.KeySelect, input.KeyStart, input.KeyJoyAux1, input.KeyJoyAux2}},
		{Buffalo, "Buffalo", 0x0583, 0x2060, "dpad", 4,
			[]input.KeyName{
				input.KeyJoyD, input.KeyJoyC, input.KeyJoyB, input.KeyJoyA,
				input.KeyL1, input.KeyR1, input.KeySelect, input.KeyStart,
				input.KeyTurbo, input.KeyClear}},
		{Sonicon, "Sonicon", 0x10c4, 0x82c0, "stick", 3,
			[]input.KeyName{
				input.KeyJoyC, input.KeyJoyA, input.KeyJoyB, input.KeyJoyD,
				input.KeyL2, input.KeyR2, input.KeyL1, input.KeyR1,
				input.KeySelect, input.KeyStart, input.KeyJoyAux1, input.KeyJoyAux2,
				input.KeyHome}},
		{DualShock, "DualShock", 0x054c, 0x09cc, "dpad", 8,
			[]input.KeyName{
				input.KeyHome, input.KeyEnter,
				input.KeyL1, input.KeyR1, input.KeyL2, input.KeyR2,
				input.KeyStart, input.KeySelect,
				input.KeyJoyC, input.KeyJoyA, input.KeyJoyB, input.KeyJoyD}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.p.Name != tc.name || tc.p.VID != tc.vid || tc.p.PID != tc.pid {
				t.Fatalf("identity = %q %#x/%#x, want %q %#x/%#x", tc.p.Name, tc.p.VID, tc.p.PID, tc.name, tc.vid, tc.pid)
			}
			if tc.p.Directional != tc.directional {
				t.Errorf("directional = %q, want %q", tc.p.Directional, tc.directional)
			}
			if tc.p.Spec == nil {
				t.Fatal("profile has no compiled Spec")
			}
			if got := tc.p.Spec.MinReportLen; got != tc.minLen {
				t.Errorf("MinReportLen = %d, want %d", got, tc.minLen)
			}
			buttons := tc.p.Spec.Buttons()
			if len(buttons) != len(tc.buttons) {
				t.Fatalf("buttons = %v, want %v", buttons, tc.buttons)
			}
			for i := range tc.buttons {
				if buttons[i] != tc.buttons[i] {
					t.Fatalf("button bit %d = %q, want %q (full %v)", i, buttons[i], tc.buttons[i], buttons)
				}
			}
		})
	}
}

func TestSpecSamplesEveryChannel(t *testing.T) {
	env := &assetfile.Envelope{
		Format: assetfile.FormatVersion, Kind: assetfile.KindHID, Name: "spinner",
		Generator: assetfile.Generator{Tool: "t", Version: "1"},
		HIDPayload: assetfile.HIDPayload{
			Channels: []assetfile.Channel{
				{Name: "fire", Kind: assetfile.ChannelButton, Report: assetfile.Report{Byte: 0, Bit: 3}},
				{Name: "wheel", Kind: assetfile.ChannelDelta, Report: assetfile.Report{Byte: 1, Size: 2, Signed: true}},
				{Name: "strip", Kind: assetfile.ChannelBlob, Report: assetfile.Report{Byte: 3, Size: 4}},
			},
			Standard: map[string]string{"fire": "a"},
		},
	}
	spec, err := CompileSpec(env)
	if err != nil {
		t.Fatalf("CompileSpec: %v", err)
	}
	if spec.MinReportLen != 7 {
		t.Fatalf("MinReportLen = %d, want 7", spec.MinReportLen)
	}
	// wheel = 0xFFFE little-endian signed = -2.
	report := []byte{0b1000, 0xfe, 0xff, 1, 2, 3, 4}
	got := map[string]int32{}
	var blob []byte
	spec.Sample(report, func(v ChannelValue) {
		if v.Kind == assetfile.ChannelBlob {
			blob = append([]byte(nil), v.Blob...)
			return
		}
		got[v.Name] = v.Int
	})
	if got["fire"] != 1 || got["wheel"] != -2 {
		t.Errorf("sampled %v, want fire=1 wheel=-2", got)
	}
	if string(blob) != string([]byte{1, 2, 3, 4}) {
		t.Errorf("blob = %v, want [1 2 3 4]", blob)
	}

	spec.Sample(report[:6], func(ChannelValue) { t.Error("short report sampled a channel") })
}

func TestDancePadDrivesThePanel(t *testing.T) {
	env := &assetfile.Envelope{
		Format: assetfile.FormatVersion, Kind: assetfile.KindHID, Name: "dancepad",
		Generator: assetfile.Generator{Tool: "t", Version: "1"},
		HIDPayload: assetfile.HIDPayload{
			Channels: []assetfile.Channel{
				{Name: "pad.up", Kind: assetfile.ChannelButton, Report: assetfile.Report{Byte: 0, Bit: 0}},
				{Name: "pad.down", Kind: assetfile.ChannelButton, Report: assetfile.Report{Byte: 0, Bit: 1}},
				{Name: "pad.left", Kind: assetfile.ChannelButton, Report: assetfile.Report{Byte: 0, Bit: 2}},
				{Name: "pad.right", Kind: assetfile.ChannelButton, Report: assetfile.Report{Byte: 0, Bit: 3}},
				{Name: "pad.start", Kind: assetfile.ChannelButton, Report: assetfile.Report{Byte: 0, Bit: 4}},
			},
			Standard: map[string]string{
				"pad.up": "joy.north", "pad.down": "joy.south",
				"pad.left": "joy.west", "pad.right": "joy.east",
				"pad.start": "a",
			},
		},
	}
	p, err := ProfileFromEnvelope(env)
	if err != nil {
		t.Fatalf("ProfileFromEnvelope: %v", err)
	}
	d := NewDecoder(p.Spec)
	d.Feed([]byte{0}) // prime
	assertEvents(t, d.Feed([]byte{0b0001}), "joy:up", "keydown:up", "keypress:up")
	assertEvents(t, d.Feed([]byte{0b0101}), "joy:up-left", "keydown:left", "keypress:left")
	assertEvents(t, d.Feed([]byte{0b10000}), "keydown:joya", "keypress:joya",
		"joy:center", "keyup:left", "keyup:up")
}
