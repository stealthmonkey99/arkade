// SPDX-License-Identifier: GPL-2.0-or-later

package display

import (
	"sync"

	"github.com/stealthmonkey99/arkade/internal/clock"
)

// Renderer receives composited frames. Implementations live in the render
// package.
type Renderer interface {
	Paint(data Matrix, stats clock.FrameTime)
}

// DefaultWidth/DefaultHeight match the 64×64 LED matrix.
const (
	DefaultWidth  = 64
	DefaultHeight = 64
)

// Display owns the layer stacks and composites them into frames for the
// renderer.
//
// Layer order (bottom to top): base fill, app layers, system layers.
type Display struct {
	mu        sync.Mutex
	width     int
	height    int
	renderer  Renderer
	baseLayer *Layer
	sysLayers []*Layer
	appLayers []*Layer
}

// New creates a display with a black base layer.
func New(w, h int, renderer Renderer) *Display {
	fill := Black
	d := &Display{width: w, height: h, renderer: renderer}
	d.baseLayer = NewLayer(w, h, "base", "bg", LayerOptions{Fill: &fill})
	return d
}

// Size returns the display dimensions.
func (d *Display) Size() (w, h int) { return d.width, d.height }

// AddLayer creates a layer. Owner "system" layers stack above app layers.
func (d *Display) AddLayer(owner, label string, opts LayerOptions) *Layer {
	d.mu.Lock()
	defer d.mu.Unlock()
	layer := NewLayer(d.width, d.height, owner, label, opts)
	if owner == "system" {
		d.sysLayers = append(d.sysLayers, layer)
	} else {
		d.appLayers = append(d.appLayers, layer)
	}
	return layer
}

// RemoveLayers removes all app layers for owner; pass label != "" to remove
// only that label.
func (d *Display) RemoveLayers(owner, label string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	keep := d.appLayers[:0]
	for _, l := range d.appLayers {
		if l.Owner == owner && (label == "" || l.Label == label) {
			continue
		}
		keep = append(keep, l)
	}
	d.appLayers = keep
}

func (d *Display) findAppLayer(owner, label string) *Layer {
	for _, l := range d.appLayers {
		if l.Owner == owner && l.Label == label {
			return l
		}
	}
	return nil
}

func (d *Display) findSysLayer(label string) *Layer {
	for _, l := range d.sysLayers {
		if l.Label == label {
			return l
		}
	}
	return nil
}

// AddElements appends elements to an existing app layer.
func (d *Display) AddElements(owner, label string, elements ...Element) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if layer := d.findAppLayer(owner, label); layer != nil {
		for _, e := range elements {
			layer.AddElement(e)
		}
	}
}

// RemoveElements removes elements from an existing app layer.
func (d *Display) RemoveElements(owner, label string, elements ...Element) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if layer := d.findAppLayer(owner, label); layer != nil {
		for _, e := range elements {
			layer.RemoveElement(e)
		}
	}
}

// SetLayerAlpha sets opacity on a layer (system or app).
func (d *Display) SetLayerAlpha(owner, label string, value float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if owner == "system" {
		if l := d.findSysLayer(label); l != nil {
			l.SetOpacity(value)
		}
		return
	}
	if l := d.findAppLayer(owner, label); l != nil {
		l.SetOpacity(value)
	}
}

// SetScrim fills the system "scrim" layer (used by fade-to-color
// transitions), if one exists.
func (d *Display) SetScrim(p Pixel) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l := d.findSysLayer("scrim"); l != nil {
		l.SetFill(p)
	}
}

// Compose flattens base + app + system layers into one matrix.
func (d *Display) Compose(stats clock.FrameTime) Matrix {
	d.mu.Lock()
	defer d.mu.Unlock()

	data := CloneMatrix(d.baseLayer.Paint(stats))

	layers := make([]*Layer, 0, len(d.appLayers)+len(d.sysLayers))
	layers = append(layers, d.appLayers...)
	layers = append(layers, d.sysLayers...)
	for _, layer := range layers {
		painted := layer.Paint(stats)
		for y := 0; y < min(len(painted), len(data)); y++ {
			for x := 0; x < min(len(painted[y]), len(data[y])); x++ {
				data[y][x] = Merge(data[y][x], painted[y][x])
			}
		}
	}
	return data
}

// Paint composites and hands the frame to the renderer.
func (d *Display) Paint(stats clock.FrameTime) {
	d.renderer.Paint(d.Compose(stats), stats)
}
