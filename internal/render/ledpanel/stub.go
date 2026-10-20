// SPDX-License-Identifier: GPL-2.0-or-later

//go:build !led || !linux || !cgo

// Package ledpanel drives the physical RGB LED matrix; this stub satisfies
// builds without the `led` tag (desktop, tests) so the CLI can always link
// against the package.
package ledpanel

import (
	"fmt"

	"github.com/stealthmonkey99/arkade/internal/render"
)

// New always errors in non-led builds.
func New(Options) (render.Renderer, error) {
	return nil, fmt.Errorf("ledpanel: built without LED support (make build-pi)")
}
