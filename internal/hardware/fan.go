// SPDX-License-Identifier: GPL-2.0-or-later

package hardware

// FanMode is the cooling relay's mode: Auto tracks the temperature threshold,
// On and Off pin the relay.
type FanMode string

const (
	FanAuto FanMode = "auto"
	FanOn   FanMode = "on"
	FanOff  FanMode = "off"
)

// CoolingFan is optionally implemented by a Suite with a fan relay whose
// state can be read back; Suite.SetFanConfig is how it is driven.
type CoolingFan interface {
	FanOn() bool
	FanMode() FanMode
}

// ParseFanMode reads a persisted mode string. An unrecognised or empty value
// is Auto with ok false.
func ParseFanMode(s string) (FanMode, bool) {
	switch m := FanMode(s); m {
	case FanAuto, FanOn, FanOff:
		return m, true
	}
	return FanAuto, false
}
