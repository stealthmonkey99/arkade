// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"strings"
	"testing"
)

func hidEnv(channels []Channel, standard map[string]string) *Envelope {
	return &Envelope{
		Format: FormatVersion, Kind: KindHID, Name: "t",
		Generator: Generator{Tool: "t", Version: "1"},
		HIDPayload: HIDPayload{
			Channels: channels,
			Standard: standard,
		},
	}
}

func TestValidateHIDReportForms(t *testing.T) {
	cases := []struct {
		name string
		ch   Channel
		want string
	}{
		{"button with a size", Channel{Name: "x", Kind: ChannelButton, Report: Report{Byte: 0, Size: 2}}, "takes no size"},
		{"button wider than a bit", Channel{Name: "x", Kind: ChannelButton, Report: Report{Byte: 0, Bits: 2}}, "a button is one bit"},
		{"bit past the byte", Channel{Name: "x", Kind: ChannelAxis, Report: Report{Byte: 0, Bit: 6, Bits: 4}}, "runs past byte"},
		{"bits and size together", Channel{Name: "x", Kind: ChannelAxis, Report: Report{Byte: 0, Bits: 4, Size: 2}}, "either a run of bits or a run of bytes"},
		{"signed bit field", Channel{Name: "x", Kind: ChannelAxis, Report: Report{Byte: 0, Bits: 4, Signed: true}}, "unsigned"},
		{"axis too wide", Channel{Name: "x", Kind: ChannelAxis, Report: Report{Byte: 0, Size: 8}}, "want within 1..4"},
		{"byte offset out of range", Channel{Name: "x", Kind: ChannelAxis, Report: Report{Byte: 5000}}, "want within 0..1023"},
		{"bit out of range", Channel{Name: "x", Kind: ChannelButton, Report: Report{Byte: 0, Bit: 9}}, "want within 0..7"},
		{"negative divisor", Channel{Name: "x", Kind: ChannelAxis, Report: Report{Byte: 0, Divide: -1}}, "positive divisor"},
		{"blob with a bias", Channel{Name: "x", Kind: ChannelBlob, Report: Report{Byte: 0, Size: 4, Bias: 1}}, "copies bytes"},
		{"blob over the cap", Channel{Name: "x", Kind: ChannelBlob, Report: Report{Byte: 0, Size: MaxBlobBytes + 1}}, "1..256"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(hidEnv([]Channel{tc.ch}, nil), KindHID, 0, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateHIDChannelNames(t *testing.T) {
	for _, bad := range []string{"Pad.Up", "pad up", "pad..up", ".pad", "pad.", "p1.spinner", "p2.a", strings.Repeat("x", MaxChannelNameBytes+1)} {
		env := hidEnv([]Channel{{Name: bad, Kind: ChannelButton, Report: Report{Byte: 0}}}, nil)
		if err := Validate(env, KindHID, 0, 0); err == nil {
			t.Errorf("channel name %q was accepted", bad)
		}
	}
	for _, good := range []string{"a", "pad.up", "pad_up", "pad-up", "axis2"} {
		env := hidEnv([]Channel{{Name: good, Kind: ChannelButton, Report: Report{Byte: 0}}}, nil)
		if err := Validate(env, KindHID, 0, 0); err != nil {
			t.Errorf("channel name %q was refused: %v", good, err)
		}
	}
}

func TestValidateHIDStandardBlock(t *testing.T) {
	btn := Channel{Name: "b", Kind: ChannelButton, Report: Report{Byte: 0}}
	ax := Channel{Name: "x", Kind: ChannelAxis, Report: Report{Byte: 1}}
	blob := Channel{Name: "p", Kind: ChannelBlob, Report: Report{Byte: 2, Size: 4}}
	cases := []struct {
		name     string
		channels []Channel
		standard map[string]string
		want     string
	}{
		{"unknown target", []Channel{btn}, map[string]string{"b": "joy.northwest"}, "not a standard panel name"},
		{"button onto an axis", []Channel{btn}, map[string]string{"b": "joy.x"}, "is an axis"},
		{"axis onto a panel button", []Channel{ax}, map[string]string{"x": "a"}, "needs a button channel"},
		{"blob aliased at all", []Channel{blob}, map[string]string{"p": "a"}, "no panel equivalent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(hidEnv(tc.channels, tc.standard), KindHID, 0, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate error = %v, want it to mention %q", err, tc.want)
			}
		})
	}

	ok := hidEnv([]Channel{
		{Name: "square", Kind: ChannelButton, Report: Report{Byte: 0, Bit: 0}},
		{Name: "l3", Kind: ChannelButton, Report: Report{Byte: 0, Bit: 1}},
	}, map[string]string{"square": "c", "l3": "c"})
	if err := Validate(ok, KindHID, 0, 0); err != nil {
		t.Errorf("two aliases onto one panel button were refused: %v", err)
	}
}

func TestStandardNameVocabulary(t *testing.T) {
	for _, n := range StandardAxes {
		if isAxis, ok := StandardName(n); !ok || !isAxis {
			t.Errorf("StandardName(%q) = (%v, %v), want (true, true)", n, isAxis, ok)
		}
	}
	for _, n := range StandardButtons {
		if isAxis, ok := StandardName(n); !ok || isAxis {
			t.Errorf("StandardName(%q) = (%v, %v), want (false, true)", n, isAxis, ok)
		}
	}
	if _, ok := StandardName("joy.middle"); ok {
		t.Error(`StandardName("joy.middle") reported known`)
	}
}
