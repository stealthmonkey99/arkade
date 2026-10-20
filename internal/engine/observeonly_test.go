// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/clock"
	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// The lab preview, the flight recorder and the peripherals lab promise to
// observe a running game without changing it. Each test below plays one
// script twice, observer off and on, and requires the same frames, the same
// cartridge random draws and the same engine RNG position afterwards.

const noiseAppSlug = "games/noise"

// noiseApp stands in for a cartridge: it draws host_random_u64 every frame,
// paints the draw, and reports the input records it receives.
type noiseApp struct {
	ctx   *engine.AppContext
	draws *[]uint64
	keys  *[]input.KeyName
}

func (a *noiseApp) Init() error {
	a.ctx.AddKeyMap([]*input.Mapping{{
		Conditions: []input.Condition{input.Key(input.KeyJoyA), input.Key(input.KeyJoyC)},
		Trigger: func(ev input.Event) {
			*a.keys = append(*a.keys, ev.Name)
			a.ctx.RT.RecordCartridgeInput(uint64(len(ev.Name)))
		},
	}})
	return nil
}
func (a *noiseApp) HandleInput(clock.FrameTime) {}
func (a *noiseApp) Update(clock.FrameTime) {
	v := a.ctx.RT.CartRandomU64()
	*a.draws = append(*a.draws, v)
	a.ctx.RemoveLayer("bg")
	a.ctx.AddFilledLayer("bg", display.Pixel{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: 255}, nil, 1)
}
func (a *noiseApp) Render(clock.FrameTime, engine.RenderReason) {}
func (a *noiseApp) Teardown() error                             { return nil }

type noiseRun struct {
	frames  []display.Matrix
	draws   []uint64
	keys    []input.KeyName
	rngTail int64
}

// playNoise runs the script with observe called once, at tick 5, so each
// test can switch its observer on mid-game. skip reports frames the
// observer is allowed to own (a frame preview on the panel).
func playNoise(t *testing.T, opts testkit.Options, observe func(*testkit.Rig), skip func(*testkit.Rig) bool) noiseRun {
	t.Helper()
	var run noiseRun
	engine.Register(engine.Manifest{
		Slug: noiseAppSlug, Name: "Noise", Type: engine.TypeGame, Hidden: true,
		WasmSHA256: "0123456789abcdef",
		New: func(ctx *engine.AppContext) engine.App {
			return &noiseApp{ctx: ctx, draws: &run.draws, keys: &run.keys}
		},
	})
	defer engine.Unregister(noiseAppSlug)
	opts.StartApp = noiseAppSlug
	rig := testkit.NewRig(t, opts)
	for tick := range 60 {
		switch tick {
		case 5:
			if observe != nil {
				observe(rig)
			}
		case 10, 25:
			rig.Press(input.KeyJoyA)
		case 18:
			rig.PressAs(input.KeyJoyC, 1)
		}
		rig.Tick(1)
		if skip != nil && skip(rig) {
			run.frames = append(run.frames, nil)
			continue
		}
		run.frames = append(run.frames, rig.Frame())
	}
	run.rngTail = rig.RT.Rand.Int63()
	return run
}

func requireSameRun(t *testing.T, off, on noiseRun) {
	t.Helper()
	if !reflect.DeepEqual(off.draws, on.draws) {
		t.Errorf("cartridge random draws differ with the observer on:\noff %v\non  %v", off.draws, on.draws)
	}
	if !reflect.DeepEqual(off.keys, on.keys) {
		t.Errorf("input reaching the game differs with the observer on: off %v, on %v", off.keys, on.keys)
	}
	if off.rngTail != on.rngTail {
		t.Error("the engine RNG ended in a different place with the observer on")
	}
	for i := range off.frames {
		if on.frames[i] == nil || off.frames[i] == nil {
			continue
		}
		if !reflect.DeepEqual(off.frames[i], on.frames[i]) {
			t.Errorf("frame %d differs with the observer on", i)
		}
	}
}

func TestFlightRecorderIsObserveOnly(t *testing.T) {
	off := playNoise(t, testkit.Options{}, nil, nil)
	on := playNoise(t, testkit.Options{FlightDir: t.TempDir()}, func(rig *testkit.Rig) {
		if !rig.RT.FlightRecording() {
			t.Fatal("setup: the recorder is not recording the session")
		}
	}, nil)
	requireSameRun(t, off, on)
}

func TestPeriphObservationIsObserveOnly(t *testing.T) {
	off := playNoise(t, testkit.Options{}, nil, nil)
	on := playNoise(t, testkit.Options{}, func(rig *testkit.Rig) {
		t.Cleanup(rig.RT.WatchPeriph())
	}, nil)
	requireSameRun(t, off, on)
}

func TestLabPreviewIsObserveOnly(t *testing.T) {
	off := playNoise(t, testkit.Options{}, nil, nil)
	sawPreview := false
	on := playNoise(t, testkit.Options{}, func(rig *testkit.Rig) {
		rig.RT.RequestLabPreview(engine.LabPreview{
			Frames:  []engine.LabPreviewFrame{{Data: solidFrame(t, rig.RT, display.Red)}},
			LCD:     &engine.LabPreviewLCD{Lines: [2][2]string{{"PREVIEW", ""}, {"PREVIEW", ""}}},
			Timeout: engine.MinLabPreviewTimeout,
		})
	}, func(rig *testkit.Rig) bool {
		active := rig.RT.LabPreviewActive()
		sawPreview = sawPreview || active
		return active
	})
	if !sawPreview {
		t.Fatal("setup: the preview never took the panel")
	}
	requireSameRun(t, off, on)
}

func TestLightShowIsObserveOnly(t *testing.T) {
	off := playNoise(t, testkit.Options{}, nil, nil)
	on := playNoise(t, testkit.Options{}, func(rig *testkit.Rig) {
		rig.RT.RequestLightShow(engine.LightShow{
			Steps:   []engine.LightStep{{Seats: seats(1, 2), Hold: 100 * time.Millisecond}},
			Loop:    true,
			Timeout: time.Second,
		})
	}, nil)
	requireSameRun(t, off, on)
}
