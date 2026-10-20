// SPDX-License-Identifier: GPL-2.0-or-later

package boot

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/controller"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/hardware"
	"github.com/stealthmonkey99/arkade/internal/input"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

// stubEnum presents scripted HID devices whose reads block forever.
type stubEnum struct {
	mu      sync.Mutex
	present []controller.DeviceInfo
}

func (e *stubEnum) plug(info controller.DeviceInfo) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.present = append(e.present, info)
}

func (e *stubEnum) Enumerate() []controller.DeviceInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]controller.DeviceInfo(nil), e.present...)
}

func (e *stubEnum) Open(string) (controller.Device, error) {
	return &blockedDevice{ch: make(chan struct{})}, nil
}

type blockedDevice struct {
	ch   chan struct{}
	once sync.Once
}

func (d *blockedDevice) Read() ([]byte, error) {
	<-d.ch
	return nil, errors.New("closed")
}

func (d *blockedDevice) Close() error {
	d.once.Do(func() { close(d.ch) })
	return nil
}

func pressFrom(rig *testkit.Rig, id string, index int, name input.KeyName) {
	rig.RT.Input.Push(input.Event{
		Kind: input.KindKeypress,
		Name: name,
		Source: input.PlayerSource{
			Type: input.SourceController,
			ID:   id,
			Slot: input.SlotOf(index),
			Seat: input.SeatOf(index),
		},
	})
}

func TestBootWithoutRegistrySkipsPairing(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{})
	rig.Tick(80) // 2500ms splash at 30fps is 75 ticks, plus the done tick

	// No shell is registered here, so boot's exit quits the engine.
	var s settings
	if err := rig.RT.Store.Read("boot", "", &s); err != nil {
		t.Fatal(err)
	}
	if s.Player1 != "virtual-0" || s.Player2 != "virtual-1" {
		t.Errorf("headless boot should pair virtual controllers, got %+v", s)
	}
}

func TestBootPairingFlowClaimsSlots(t *testing.T) {
	enum := &stubEnum{}
	enum.plug(controller.DeviceInfo{VID: controller.Speedlink.VID, PID: controller.Speedlink.PID, Path: "usb-A"})
	enum.plug(controller.DeviceInfo{VID: controller.Buffalo.VID, PID: controller.Buffalo.PID, Path: "usb-B"})

	var mgr *controller.Manager
	rig := testkit.NewRig(t, testkit.Options{
		MakeControllers: func(h *input.Handler) *controller.Manager {
			mgr = controller.NewManager(h, enum, nil)
			return mgr
		},
	})

	rig.Tick(80)
	rig.Golden("boot-pair-player-1")

	// Pairing is first-press-wins: usb-B presses first and claims player 1.
	pressFrom(rig, "usb-B", 1, input.KeyJoyA)
	rig.Tick(2)
	pressFrom(rig, "usb-A", 0, input.KeyJoyA)
	rig.Tick(3)

	var s settings
	if err := rig.RT.Store.Read("boot", "", &s); err != nil {
		t.Fatal(err)
	}
	if s.Player1 != "usb-B" || s.Player2 != "usb-A" {
		t.Fatalf("first press should claim player 1: %+v", s)
	}

	for _, c := range mgr.List() {
		want := map[string]input.Seat{"usb-B": input.SeatP1, "usb-A": input.SeatP2}[c.ID]
		if c.Seat != want {
			t.Errorf("%s should play %v, got %v", c.ID, want, c.Seat)
		}
	}
}

// seedSettings writes a stored pairing into dataDir.
func seedSettings(t *testing.T, dataDir, p1, p2 string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dataDir, "boot"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"player1":"` + p1 + `","player2":"` + p2 + `"}`)
	if err := os.WriteFile(filepath.Join(dataDir, "boot", "settings.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBootLateControllerPairsSilently(t *testing.T) {
	dataDir := t.TempDir()
	seedSettings(t, dataDir, "usb-A", "usb-B")

	enum := &stubEnum{}
	var mgr *controller.Manager
	rig := testkit.NewRig(t, testkit.Options{
		DataDir: dataDir,
		MakeControllers: func(h *input.Handler) *controller.Manager {
			mgr = controller.NewManager(h, enum, nil)
			return mgr
		},
	})

	rig.Tick(80)
	if rig.RT.CurrentSlug() != "boot" {
		t.Fatalf("boot should still be waiting for player 1, got %q", rig.RT.CurrentSlug())
	}

	// The manager's 2s hot-plug ticker runs on the wall clock, so wait it out.
	enum.plug(controller.DeviceInfo{VID: controller.Speedlink.VID, PID: controller.Speedlink.PID, Path: "usb-A"})
	enum.plug(controller.DeviceInfo{VID: controller.Buffalo.VID, PID: controller.Buffalo.PID, Path: "usb-B"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(mgr.Controllers()) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if len(mgr.Controllers()) < 2 {
		t.Fatal("hot-plug never attached the late controllers")
	}

	rig.Tick(5)
	var s settings
	if err := rig.RT.Store.Read("boot", "", &s); err != nil {
		t.Fatal(err)
	}
	if s.Player1 != "usb-A" || s.Player2 != "usb-B" {
		t.Errorf("late-attached controllers should pair silently, got %+v", s)
	}
}

func TestBootInterruptedTeardownKeepsStoredPairing(t *testing.T) {
	dataDir := t.TempDir()
	seedSettings(t, dataDir, "usb-A", "usb-B")
	rig := testkit.NewRig(t, testkit.Options{DataDir: dataDir, StartApp: "boot"})

	stored := func() settings {
		t.Helper()
		var s settings
		if err := rig.RT.Store.Read("boot", "", &s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	// An unfinished boot with empty settings must not write.
	a := &app{ctx: &engine.AppContext{Manifest: engine.Manifest{Slug: "boot"}, RT: rig.RT}}
	if err := a.Teardown(); err != nil {
		t.Fatal(err)
	}
	if s := stored(); s.Player1 != "usb-A" || s.Player2 != "usb-B" {
		t.Errorf("interrupted boot must not rewrite stored pairing, got %+v", s)
	}

	a.completed = true
	a.settings = settings{Player1: "usb-C", Player2: "usb-D"}
	if err := a.Teardown(); err != nil {
		t.Fatal(err)
	}
	if s := stored(); s.Player1 != "usb-C" || s.Player2 != "usb-D" {
		t.Errorf("completed boot should persist its pairing, got %+v", s)
	}
}

func TestBootRemembersPairedControllers(t *testing.T) {
	dataDir := t.TempDir()
	enum := &stubEnum{}
	enum.plug(controller.DeviceInfo{VID: controller.Speedlink.VID, PID: controller.Speedlink.PID, Path: "usb-A"})
	enum.plug(controller.DeviceInfo{VID: controller.Buffalo.VID, PID: controller.Buffalo.PID, Path: "usb-B"})
	mk := func(h *input.Handler) *controller.Manager { return controller.NewManager(h, enum, nil) }

	rig := testkit.NewRig(t, testkit.Options{DataDir: dataDir, MakeControllers: mk})
	rig.Tick(80)
	pressFrom(rig, "usb-A", 0, input.KeyJoyA)
	rig.Tick(2)
	pressFrom(rig, "usb-B", 1, input.KeyJoyA)
	rig.Tick(3)

	// Second boot: both still attached, so pairing is silent.
	rig2 := testkit.NewRig(t, testkit.Options{DataDir: dataDir, MakeControllers: mk})
	rig2.Tick(80)
	var s settings
	if err := rig2.RT.Store.Read("boot", "", &s); err != nil {
		t.Fatal(err)
	}
	if s.Player1 != "usb-A" || s.Player2 != "usb-B" {
		t.Errorf("remembered pairing should survive reboot, got %+v", s)
	}
	if rig2.Capture.FrameCount() == 0 {
		t.Error("expected frames painted")
	}
}

func TestBootPlaysTheStinger(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{})
	rig.Tick(20) // past stingerAtMs at 30fps
	for _, ev := range rig.Audio.Events() {
		if ev.Action == "play" && ev.Label == "stinger" {
			return
		}
	}
	t.Fatalf("no stinger in the boot's audio events: %+v", rig.Audio.Events())
}

func TestBootBloomSchedule(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{FPS: 100})
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}

	const msPerTick = 10
	fadeTicks := bloomFadeMs / msPerTick
	starts := make([]int, len(hardware.LightButtons))
	for i := range starts {
		starts[i] = (stingerAtMs + bloomVoice(i)*bloomStepMs) / msPerTick
	}

	ticked := 0
	for i, start := range starts {
		// Up to this lamp's start tick, it and every later lamp are dark.
		rig.Tick(start - 1 - ticked)
		ticked = start - 1
		for seat := 0; seat < 2; seat++ {
			for j := i; j < len(starts); j++ {
				if got := hw.LampLevels(seat)[j]; got != 0 {
					t.Errorf("seat %d lamp %d (%s) lit at tick %d, before its start (tick %d)",
						seat, j, hardware.LightButtons[j], ticked, starts[j])
				}
			}
		}
	}

	for i, start := range starts {
		done := start + fadeTicks
		rig.Tick(done - ticked)
		ticked = done
		for seat := 0; seat < 2; seat++ {
			if got := hw.LampLevels(seat)[i]; got != hardware.LampLevelMax {
				t.Errorf("seat %d lamp %d (%s) = %d at tick %d (%dms), want full (%d)",
					seat, i, hardware.LightButtons[i], got, ticked, ticked*msPerTick, hardware.LampLevelMax)
			}
		}
	}

	// Tick 130 (1300ms): every lamp is full.
	rig.Tick(130 - ticked)
	want := hardware.LampLevels{hardware.LampLevelMax, hardware.LampLevelMax, hardware.LampLevelMax, hardware.LampLevelMax, hardware.LampLevelMax, hardware.LampLevelMax}
	for seat := 0; seat < 2; seat++ {
		if got := hw.LampLevels(seat); got != want {
			t.Errorf("seat %d lamps at 1300ms = %+v, want %+v", seat, got, want)
		}
	}
}

func TestBootBacklightFadeSchedule(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{FPS: 100})
	hw, ok := rig.RT.RawHardware().(*hardware.Virtual)
	if !ok {
		t.Fatalf("RawHardware is %T, want *hardware.Virtual", rig.RT.RawHardware())
	}

	const msPerTick = 10
	start := stingerAtMs / msPerTick

	// Init must not touch the backlight; the startup card is still showing.
	rig.Tick(1)
	if got := hw.BacklightLevel(); got != hardware.LampLevelMax {
		t.Fatalf("backlight = %d after Init, want the level the hardware reports (%d) left alone", got, hardware.LampLevelMax)
	}

	// The cabinet hands over at the card's 30%; the fade starts from there.
	const handover = hardware.LampLevelMax * 30 / 100
	rig.RT.SetBacklightLevel(handover)
	rig.Tick(start - 1)
	if got := hw.BacklightLevel(); got != handover {
		t.Fatalf("backlight = %d at tick %d (%dms), want the handover level %d held until the stinger", got, start, stingerAtMs, handover)
	}

	// The fade lands on tick 130, where the bloom finishes.
	prev := uint8(handover)
	rose := false
	for tick := start + 1; tick <= 130; tick++ {
		rig.Tick(1)
		got := hw.BacklightLevel()
		if got < prev {
			t.Fatalf("backlight fell from %d to %d at tick %d", prev, got, tick)
		}
		if got > prev {
			rose = true
		}
		prev = got
	}
	if !rose {
		t.Errorf("backlight never rose during the %dms fade", backlightFadeMs)
	}
	if prev != hardware.LampLevelMax {
		t.Errorf("backlight at 1300ms = %d, want full (%d)", prev, hardware.LampLevelMax)
	}
}
