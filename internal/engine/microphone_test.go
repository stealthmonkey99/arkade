// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
	"github.com/stealthmonkey99/arkade/internal/audio"
	"github.com/stealthmonkey99/arkade/internal/render"
)

type calibrationMic struct{ calibrations int }

func (m *calibrationMic) SetSessionModels([]*assetfile.Envelope) {}
func (m *calibrationMic) Calibrate()                             { m.calibrations++ }

func TestPlayStingerCalibratesTheMicOnTheStinger(t *testing.T) {
	mic := &calibrationMic{}
	rec := audio.NewRecorder()
	rt, err := New(Options{Renderer: render.NewCapture(false), DataDir: t.TempDir(), Audio: rec, Mic: mic})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.PlayStinger(); err != nil {
		t.Fatal(err)
	}
	if mic.calibrations != 1 {
		t.Errorf("Calibrate called %d times, want 1", mic.calibrations)
	}
	evs := rec.Events()
	if len(evs) != 1 || evs[0].Owner != "system" || evs[0].Label != "stinger" || evs[0].Bytes == 0 {
		t.Errorf("audio events %+v, want one stinger under the system owner", evs)
	}
}

func TestPlayStingerWithoutAMic(t *testing.T) {
	rt, err := New(Options{Renderer: render.NewCapture(false), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.PlayStinger(); err != nil {
		t.Fatal(err)
	}
}
