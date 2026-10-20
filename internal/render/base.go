// SPDX-License-Identifier: GPL-2.0-or-later

// Package render provides frame renderers: an ANSI console renderer for
// desktop play and a data-only Capture renderer for tests and headless runs.
// The LED matrix renderer is render/ledpanel.
package render

import (
	"math"
	"sync"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// Renderer is the full engine-facing contract.
type Renderer interface {
	display.Renderer
	Init() error
	Close() error
	RotateLeft()
	RotateRight()
	Rotate180()
	VirtualRotate(o display.Orientation)
	SetBaseOrientation(o display.Orientation)
	Orientation() display.Orientation
	BaseOrientation() display.Orientation
	VirtualOrientation() display.Orientation
	DimUp()
	DimDown()
	Brightness() float64
	SetBrightness(float64)
}

// Impl is what a concrete renderer implements; Base handles rotation,
// brightness and frame diffing before delegating.
type Impl interface {
	// PaintImpl receives the transformed frame; dirty is false when the
	// frame is pixel-identical to the previous one.
	PaintImpl(data display.Matrix, stats clock.FrameTime, dirty bool)
}

// orientationStack is the three rotations a frame passes through, each a
// quarter-turn count that composes additively with the others.
type orientationStack struct {
	// physical is the panel's mounting, moved by the RotateLeft/Right
	// hotkeys.
	physical display.Orientation
	// base is the engine-driven default "top" edge: the room orientation
	// while idle, the seated one during play. Unlike virtual it survives the
	// VirtualRotate(Upright) every app unload makes.
	base display.Orientation
	// virtual is a player strategy's rotation, e.g. facing player 2.
	virtual display.Orientation
}

func (o orientationStack) Combined() display.Orientation {
	return display.Orientation((int(o.physical) + int(o.base) + int(o.virtual)) % 4)
}

// Base carries the shared renderer state.
type Base struct {
	mu          sync.Mutex
	impl        Impl
	orientation orientationStack
	brightness  float64
	lastFrame   display.Matrix
	dirty       bool
	// OnChange fires after rotation/brightness updates so the engine can
	// remap input directions.
	OnChange func(orientation, virtual display.Orientation, brightness float64)
}

// NewBase wires a Base to its concrete impl.
func NewBase(impl Impl) *Base {
	return &Base{impl: impl, brightness: 1, dirty: true}
}

// update applies change under the lock, marks the next frame dirty when it
// reports a change, and then notifies outside the lock.
func (b *Base) update(change func() bool) {
	b.mu.Lock()
	if !change() {
		b.mu.Unlock()
		return
	}
	b.dirty = true
	physical, virtual, brightness := b.orientation.physical, b.orientation.virtual, b.brightness
	b.mu.Unlock()
	if b.OnChange != nil {
		b.OnChange(physical, virtual, brightness)
	}
}

func (b *Base) rotate(amount int) {
	b.update(func() bool {
		b.orientation.physical = display.Orientation((int(b.orientation.physical) + amount + 4) % 4)
		return true
	})
}

func (b *Base) RotateLeft()  { b.rotate(-1) }
func (b *Base) RotateRight() { b.rotate(1) }
func (b *Base) Rotate180()   { b.rotate(2) }

// VirtualRotate applies a rotation on behalf of player strategies (e.g.
// facing the board toward player 2) without moving the physical setting.
func (b *Base) VirtualRotate(o display.Orientation) {
	b.update(func() bool { b.orientation.virtual = o; return true })
}

// SetBaseOrientation sets the engine-driven default orientation, an
// absolute value that leaves the physical and virtual layers alone. Setting
// the current value is a no-op, so the engine may call it every tick.
func (b *Base) SetBaseOrientation(o display.Orientation) {
	b.update(func() bool {
		if b.orientation.base == o {
			return false
		}
		b.orientation.base = o
		return true
	})
}

func (b *Base) Orientation() display.Orientation {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.orientation.physical
}

func (b *Base) BaseOrientation() display.Orientation {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.orientation.base
}

func (b *Base) VirtualOrientation() display.Orientation {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.orientation.virtual
}

func (b *Base) dim(amount float64) {
	b.update(func() bool {
		b.brightness = math.Max(0, math.Min(1, b.brightness+amount))
		return true
	})
}

func (b *Base) DimUp()   { b.dim(0.1) }
func (b *Base) DimDown() { b.dim(-0.1) }

func (b *Base) Brightness() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.brightness
}

// SetBrightness sets the panel brightness directly to v (clamped to 0..1),
// unlike DimUp/DimDown's relative steps.
func (b *Base) SetBrightness(v float64) {
	b.update(func() bool {
		b.brightness = math.Max(0, math.Min(1, v))
		return true
	})
}

func (b *Base) applyBrightness(data display.Matrix) display.Matrix {
	factor := b.brightness
	if factor > 0.99 {
		return data
	}
	out := make(display.Matrix, len(data))
	for y, row := range data {
		newRow := make([]display.Pixel, len(row))
		for x, p := range row {
			newRow[x] = display.Pixel{
				R: uint8(math.Round(float64(p.R) * factor)),
				G: uint8(math.Round(float64(p.G) * factor)),
				B: uint8(math.Round(float64(p.B) * factor)),
				A: p.A,
			}
		}
		out[y] = newRow
	}
	return out
}

// Paint transforms and forwards a frame to the impl.
func (b *Base) Paint(data display.Matrix, stats clock.FrameTime) {
	b.mu.Lock()
	dirtyFrame := b.dirty || !display.MatrixEquals(b.lastFrame, data)
	b.lastFrame = display.CloneMatrix(data)
	b.dirty = false
	combined := b.orientation.Combined()
	brightness := b.applyBrightness(data)
	b.mu.Unlock()

	b.impl.PaintImpl(display.Rotate(brightness, combined), stats, dirtyFrame)
}
