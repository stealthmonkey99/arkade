// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import "strings"

// HID caps. A profile is untrusted input, so every count and width a host
// acts on is bounded here and both hosts refuse the same file.
const (
	// MaxBlobBytes is the largest blob a channel may declare, matching the
	// ABI's host_input_blob cap.
	MaxBlobBytes = 256
	// MaxProfileChannels caps one profile's channel list.
	MaxProfileChannels = 128
	// MaxChannelNameBytes caps a channel name.
	MaxChannelNameBytes = 64
	// MaxReportByte caps a report field's byte offset.
	MaxReportByte = 1023
	// MaxAxisBytes caps the byte form's width for an axis or a delta; four
	// bytes is the int32 a delta carries.
	MaxAxisBytes = 4
)

// The transports Device.Bus may name. An empty Bus matches any transport.
const (
	BusUSB       = "usb"
	BusBluetooth = "bluetooth"
)

// Standard panel names a profile's `standard` block may alias a channel onto.
// They are seat-relative: the channel's own seat decides which seat they
// land on. joy.* is the panel stick; dpad.* is a second directional beside
// it, such as a gamepad d-pad.
const (
	StdJoyX = "joy.x"
	StdJoyY = "joy.y"

	StdJoyNorth = "joy.north"
	StdJoySouth = "joy.south"
	StdJoyEast  = "joy.east"
	StdJoyWest  = "joy.west"

	StdDpadX     = "dpad.x"
	StdDpadY     = "dpad.y"
	StdDpadNorth = "dpad.north"
	StdDpadSouth = "dpad.south"
	StdDpadEast  = "dpad.east"
	StdDpadWest  = "dpad.west"
)

var standardAxes = map[string]bool{
	StdJoyX: true, StdJoyY: true, StdDpadX: true, StdDpadY: true,
}

// StandardButtons are the button-valued standard names: the stick and dpad
// directions, the six panel buttons a/b/c/d/aux1/aux2, then controller-only
// keys, which a cartridge must be ready to go without.
var StandardButtons = []string{
	StdJoyNorth, StdJoySouth, StdJoyEast, StdJoyWest,
	StdDpadNorth, StdDpadSouth, StdDpadEast, StdDpadWest,
	"a", "b", "c", "d", "aux1", "aux2",
	"l1", "r1", "l2", "r2",
	"select", "start", "turbo", "clear", "home", "enter",
}

// StandardAxes are the axis-valued standard names.
var StandardAxes = []string{StdJoyX, StdJoyY, StdDpadX, StdDpadY}

var standardButtonSet = func() map[string]bool {
	m := make(map[string]bool, len(StandardButtons))
	for _, n := range StandardButtons {
		m[n] = true
	}
	return m
}()

// StandardName reports whether name is in the standard panel vocabulary,
// and which of the two shapes it has.
func StandardName(name string) (isAxis bool, ok bool) {
	if standardAxes[name] {
		return true, true
	}
	return false, standardButtonSet[name]
}

// validChannelName allows lowercase letters, digits and . _ -, with no empty
// segment. The p1. and p2. prefixes are refused because the host adds them
// from the channel's seat.
func validChannelName(name string) bool {
	if name == "" || len(name) > MaxChannelNameBytes {
		return false
	}
	if strings.HasPrefix(name, "p1.") || strings.HasPrefix(name, "p2.") {
		return false
	}
	for _, seg := range strings.Split(name, ".") {
		if seg == "" {
			return false
		}
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}
