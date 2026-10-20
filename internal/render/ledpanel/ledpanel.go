// SPDX-License-Identifier: GPL-2.0-or-later

//go:build led && linux && cgo

// Package ledpanel drives the physical RGB LED matrix through hzeller's
// rpi-rgb-led-matrix C library. It builds only with the `led` tag on Linux
// (the Makefile's build-pi target); options.go holds the API both builds share.
package ledpanel

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/rpi-rgb-led-matrix/include
#cgo LDFLAGS: -L${SRCDIR}/../../../third_party/rpi-rgb-led-matrix/lib -lrgbmatrix -lstdc++ -lm -lpthread
#include <stdlib.h>
#include "led-matrix-c.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/render"
)

// Panel implements render.Renderer over the LED matrix.
type Panel struct {
	*render.Base
	matrix *C.struct_RGBLedMatrix
	canvas *C.struct_LedCanvas
	opts   Options
}

// New opens the matrix. It must run as root and the process stays root, for
// HID, I2C and ALSA access.
func New(opts Options) (render.Renderer, error) {
	if opts.Cols == 0 {
		opts.Cols = 64
	}
	if opts.Rows == 0 {
		opts.Rows = 64
	}
	if opts.HardwareMapping == "" {
		opts.HardwareMapping = "regular"
	}
	if opts.GPIOSlowdown == 0 {
		opts.GPIOSlowdown = 3
	}
	p := &Panel{opts: opts}
	p.Base = render.NewBase(p)
	return p, nil
}

// Init creates the matrix and offscreen canvas.
func (p *Panel) Init() error {
	var mo C.struct_RGBLedMatrixOptions
	mo.rows = C.int(p.opts.Rows)
	mo.cols = C.int(p.opts.Cols)
	mo.chain_length = 1
	mo.parallel = 1
	mo.brightness = 100 // software brightness is applied by render.Base
	mapping := C.CString(p.opts.HardwareMapping)
	defer C.free(unsafe.Pointer(mapping))
	mo.hardware_mapping = mapping

	var ro C.struct_RGBLedRuntimeOptions
	ro.gpio_slowdown = C.int(p.opts.GPIOSlowdown)
	// Keep root for the controllers, I2C and audio; the C wrapper copies only a non-zero value, so -1 is "off".
	ro.drop_privileges = -1

	p.matrix = C.led_matrix_create_from_options_and_rt_options(&mo, &ro)
	if p.matrix == nil {
		return fmt.Errorf("ledpanel: matrix init failed (running as root?)")
	}
	p.canvas = C.led_matrix_create_offscreen_canvas(p.matrix)
	return nil
}

// PaintImpl writes the frame to the offscreen canvas and swaps on vsync.
// Unchanged frames skip the write entirely — the previous canvas stays up.
func (p *Panel) PaintImpl(data display.Matrix, stats clock.FrameTime, dirty bool) {
	if p.matrix == nil || !dirty {
		return
	}
	for y, row := range data {
		for x, px := range row {
			C.led_canvas_set_pixel(p.canvas, C.int(x), C.int(y), C.uint8_t(px.R), C.uint8_t(px.G), C.uint8_t(px.B))
		}
	}
	p.canvas = C.led_matrix_swap_on_vsync(p.matrix, p.canvas)
}

// Close shuts the matrix down (blanks the panel).
func (p *Panel) Close() error {
	if p.matrix != nil {
		C.led_matrix_delete(p.matrix)
		p.matrix = nil
	}
	return nil
}
