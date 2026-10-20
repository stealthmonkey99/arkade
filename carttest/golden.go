// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/goldens"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// ASCII renders a frame as ASCII art plus a legend for non-palette colors.
func ASCII(frame Matrix) string { return render.ASCII(frame) }

// GoldenFrame compares frame with testdata/golden/<name>.txt relative to the
// test's directory. UPDATE_GOLDENS=1 rewrites the file instead.
func GoldenFrame(t *testing.T, name string, frame Matrix) {
	t.Helper()
	goldens.Frame(t, name, frame)
}
