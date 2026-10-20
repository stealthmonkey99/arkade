// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import "fmt"

// Severity says what a Warning costs the cartridge. Load never fails over
// one; rcartpack refuses to pack an Undeliverable one.
type Severity int

const (
	// Cosmetic: missing title art or manual. The game itself is unaffected.
	Cosmetic Severity = iota
	// Undeliverable: the bundle names or carries an asset the host cannot
	// mount, so the game degrades at runtime without failing.
	Undeliverable
)

// Category names the part of the bundle a Warning is about.
type Category int

const (
	CategoryArt Category = iota // manifest icon or logo
	CategoryManual
	CategorySprite // a .arkadesprite.png anywhere in the bundle
	CategorySound  // a .arkadesound.wav anywhere in the bundle
	CategoryGlyph  // assets.glyphs
	CategoryHID    // assets.hid
	CategoryMic    // assets.mic
	CategoryLights // assets.lights
)

// Warning is one non-fatal problem Load found in a bundle.
type Warning struct {
	Category Category
	Severity Severity
	// Path is the bundle path the warning is about, or "" when it concerns a
	// whole assets list.
	Path    string
	Message string
}

func (w Warning) String() string { return w.Message }

func warnf(c Category, s Severity, path, format string, args ...any) Warning {
	return Warning{Category: c, Severity: s, Path: path, Message: fmt.Sprintf(format, args...)}
}
