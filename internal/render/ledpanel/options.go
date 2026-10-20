// SPDX-License-Identifier: GPL-2.0-or-later

package ledpanel

import "github.com/stealthmonkey99/arkade/internal/render"

// Options configures the physical panel.
type Options struct {
	Cols int
	Rows int
	// HardwareMapping is the wiring profile ("regular", "adafruit-hat",
	// "adafruit-hat-pwm").
	HardwareMapping string
	// GPIOSlowdown compensates for fast Pis (typically 2 or 3).
	GPIOSlowdown int
}

// Both builds, the cgo panel and the stub, must export this New.
var _ func(Options) (render.Renderer, error) = New
