// SPDX-License-Identifier: GPL-2.0-or-later

package render

import (
	"sync"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
)

// CapturedFrame is one recorded paint.
type CapturedFrame struct {
	Data  display.Matrix
	Stats clock.FrameTime
	// Dirty is false when the frame was pixel-identical to its predecessor.
	Dirty bool
}

// Capture is the data-only renderer: it records composited frames instead of
// drawing them, for golden-frame tests and headless runs. It applies the same
// rotation and brightness pipeline as any other renderer.
type Capture struct {
	*Base
	mu     sync.Mutex
	frames []CapturedFrame
	// KeepAll retains every frame; when false only the last frame is kept.
	KeepAll bool
}

// NewCapture builds a capture renderer. keepAll retains the full frame
// history (tests); pass false for long headless runs.
func NewCapture(keepAll bool) *Capture {
	c := &Capture{KeepAll: keepAll}
	c.Base = NewBase(c)
	return c
}

func (c *Capture) Init() error  { return nil }
func (c *Capture) Close() error { return nil }

// PaintImpl records the transformed frame.
func (c *Capture) PaintImpl(data display.Matrix, stats clock.FrameTime, dirty bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	frame := CapturedFrame{Data: display.CloneMatrix(data), Stats: stats, Dirty: dirty}
	if c.KeepAll {
		c.frames = append(c.frames, frame)
	} else {
		c.frames = []CapturedFrame{frame}
	}
}

// LastFrame returns the most recent frame, or nil if nothing painted yet.
func (c *Capture) LastFrame() *CapturedFrame {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.frames) == 0 {
		return nil
	}
	f := c.frames[len(c.frames)-1]
	return &f
}

// Frames returns the recorded history.
func (c *Capture) Frames() []CapturedFrame {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]CapturedFrame(nil), c.frames...)
}

// FrameCount returns how many frames have been recorded.
func (c *Capture) FrameCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.frames)
}

// Reset clears the recorded history.
func (c *Capture) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = nil
}
