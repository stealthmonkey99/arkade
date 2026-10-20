// SPDX-License-Identifier: GPL-2.0-or-later

package display

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/clock"
)

// stubElement is a minimal element for layer tests.
type stubElement struct {
	sprite  Sprite
	x, y    int
	visible bool
	dirty   bool
}

func (s *stubElement) Render(clock.FrameTime) Sprite { return s.sprite }
func (s *stubElement) Visible() bool                 { return s.visible }
func (s *stubElement) IsDirty() bool                 { return s.dirty }
func (s *stubElement) MarkClean()                    { s.dirty = false }
func (s *stubElement) OffsetX() int                  { return s.x }
func (s *stubElement) OffsetY() int                  { return s.y }
func (s *stubElement) Width() int {
	if len(s.sprite) == 0 {
		return 0
	}
	return len(s.sprite[0])
}
func (s *stubElement) Height() int { return len(s.sprite) }

func stats() clock.FrameTime { return clock.FrameTime{} }

func TestLayerPaintsElementAtOffset(t *testing.T) {
	el := &stubElement{sprite: Sprite{{Red}}, x: 2, y: 1, visible: true, dirty: true}
	layer := NewLayer(4, 3, "test", "l1", LayerOptions{Elements: []Element{el}})
	out := layer.Paint(stats())
	if out[1][2] != Red {
		t.Errorf("expected red at (2,1), got %+v", out[1][2])
	}
	if out[0][0] != Transparent {
		t.Errorf("expected transparent background, got %+v", out[0][0])
	}
}

func TestLayerClipsOutOfBounds(t *testing.T) {
	el := &stubElement{sprite: Sprite{{Red, Red}, {Red, Red}}, x: 3, y: 2, visible: true, dirty: true}
	layer := NewLayer(4, 3, "test", "l1", LayerOptions{Elements: []Element{el}})
	out := layer.Paint(stats()) // must not panic
	if out[2][3] != Red {
		t.Errorf("in-bounds corner should paint, got %+v", out[2][3])
	}
}

func TestLayerInvisibleElementSkipped(t *testing.T) {
	el := &stubElement{sprite: Sprite{{Red}}, visible: false, dirty: true}
	layer := NewLayer(2, 2, "test", "l1", LayerOptions{Elements: []Element{el}})
	out := layer.Paint(stats())
	if out[0][0] != Transparent {
		t.Errorf("invisible element must not paint, got %+v", out[0][0])
	}
	if el.dirty {
		t.Error("invisible element should still be marked clean")
	}
}

func TestLayerCacheReusedWhenClean(t *testing.T) {
	el := &stubElement{sprite: Sprite{{Red}}, visible: true, dirty: true}
	layer := NewLayer(2, 2, "test", "l1", LayerOptions{Elements: []Element{el}})
	first := layer.Paint(stats())
	// element now clean; mutate sprite silently — cache should mask it
	el.sprite = Sprite{{Blue}}
	second := layer.Paint(stats())
	if !MatrixEquals(first, second) {
		t.Error("clean layer should return cached render")
	}
	// dirty flag invalidates
	el.dirty = true
	third := layer.Paint(stats())
	if third[0][0] != Blue {
		t.Errorf("dirty element should repaint, got %+v", third[0][0])
	}
}

func TestLayerOpacityScalesAlpha(t *testing.T) {
	el := &stubElement{sprite: Sprite{{White}}, visible: true, dirty: true}
	half := 0.5
	layer := NewLayer(1, 1, "test", "l1", LayerOptions{Elements: []Element{el}, Opacity: &half})
	out := layer.Paint(stats())
	if out[0][0].A != 127 {
		t.Errorf("expected alpha 127 at 50%% opacity, got %d", out[0][0].A)
	}
}

func TestDisplayComposesLayersInOrder(t *testing.T) {
	cap := &captureRenderer{}
	d := New(2, 2, cap)
	white := White
	d.AddLayer("app1", "bg", LayerOptions{Fill: &white})
	sys := Red
	d.AddLayer("system", "overlay", LayerOptions{Fill: &sys})
	d.Paint(stats())
	if cap.last[0][0] != Red {
		t.Errorf("system layer should be on top, got %+v", cap.last[0][0])
	}
}

type captureRenderer struct{ last Matrix }

func (c *captureRenderer) Paint(data Matrix, _ clock.FrameTime) { c.last = data }
