// SPDX-License-Identifier: GPL-2.0-or-later

package scene

import (
	"errors"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/fsm"
	"github.com/stealthmonkey99/arkade/internal/input"
)

type fakeHost struct {
	now    time.Time
	layers map[string]bool
	alpha  map[string]float64
}

func newFakeHost() *fakeHost {
	return &fakeHost{now: time.Unix(0, 0), layers: map[string]bool{}, alpha: map[string]float64{}}
}

func (h *fakeHost) AddLayer(label string, _ []display.Element, opacity float64) {
	h.layers[label] = true
	h.alpha[label] = opacity
}
func (h *fakeHost) RemoveLayer(label string)                  { delete(h.layers, label) }
func (h *fakeHost) SetLayerAlpha(label string, v float64)     { h.alpha[label] = v }
func (h *fakeHost) AddElements(string, ...display.Element)    {}
func (h *fakeHost) RemoveElements(string, ...display.Element) {}
func (h *fakeHost) SetScrim(display.Pixel)                    {}
func (h *fakeHost) PlaySound(string)                          {}
func (h *fakeHost) PlayMusic(string)                          {}
func (h *fakeHost) StopMusic()                                {}
func (h *fakeHost) Log(string)                                {}
func (h *fakeHost) Now() time.Time                            { return h.now }
func (h *fakeHost) advance(d time.Duration)                   { h.now = h.now.Add(d) }
func (h *fakeHost) layer(label string) (present bool, alpha float64) {
	return h.layers[label], h.alpha[label]
}

type stubImpl struct{ initErr error }

func (s *stubImpl) Init() error                                   { return s.initErr }
func (s *stubImpl) Enter()                                        {}
func (s *stubImpl) HandleInput(clock.FrameTime, input.Event) bool { return false }
func (s *stubImpl) Update(clock.FrameTime)                        {}
func (s *stubImpl) Render(clock.FrameTime, RenderReason)          {}
func (s *stubImpl) Teardown()                                     {}

func TestSceneAddLayerIsFadedByTransitions(t *testing.T) {
	h := newFakeHost()
	s := NewScene("a", h, &stubImpl{}, Options{})
	s.AddLayer("a", nil, 0)
	if present, _ := h.layer("a"); !present {
		t.Fatal("AddLayer did not add the layer to the host")
	}
	s.TransitionTo()
	h.advance(DurFast)
	s.Render(clock.FrameTime{}, ReasonTick)
	if s.State() != Active {
		t.Fatalf("state = %v, want active", s.State())
	}
	if _, alpha := h.layer("a"); alpha != 1 {
		t.Errorf("alpha after transition in = %v, want 1", alpha)
	}
}

func TestManagerAddRegistersOnlyAfterInitSucceeds(t *testing.T) {
	h := newFakeHost()
	m := NewManager(h, fsm.NewBuilder().Build("a"))
	if _, err := m.Add("a", &stubImpl{initErr: errors.New("boom")}, Options{}); err == nil {
		t.Fatal("Add with a failing Init returned no error")
	}
	if m.Current() != nil {
		t.Error("a scene whose Init failed is the current scene")
	}
	s, err := m.Add("a", &stubImpl{}, Options{})
	if err != nil {
		t.Fatalf("re-adding after a failed Init: %v", err)
	}
	if m.Current() != s {
		t.Error("the successfully added scene is not current")
	}
}
