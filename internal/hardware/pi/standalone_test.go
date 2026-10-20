// SPDX-License-Identifier: GPL-2.0-or-later

package pi

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

func TestPanelDrawsThroughItsWorker(t *testing.T) {
	bus := &fakeBus{}
	p, err := NewPanel(bus, LCDAddrs()[0], map[rune]hardware.GlyphRows{
		'▏': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10},
		'▲': {0x0e, 0x0e, 0x04, 0x04, 0x04, 0x0e, 0x1f, 0x00},
	}, nil)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	defer p.Close()

	p.SetText("Årkade         ▲", "▏       boot   ")
	p.s.testWorker().flush()

	if got := p.s.window(0); got != "Årkade         ▲" {
		t.Errorf("row 0 = %q", got)
	}
	for _, r := range p.s.substitute(0, "▏▲") {
		if r >= maxCGRAMSlots {
			t.Errorf("custom rune rendered as %#x, want a CGRAM slot", r)
		}
	}
	if len(bus.forAddr(LCDAddrs()[0])) == 0 {
		t.Error("nothing reached the backpack")
	}
}

func TestPanelFailsWithNothingOnTheBus(t *testing.T) {
	bus := &fakeBus{}
	if _, err := NewPanel(bus, 0x55, nil, nil); err == nil {
		t.Error("an empty address should not come back as a panel")
	}
}

func TestPWMHonoursTheSettings(t *testing.T) {
	off := DefaultConfig().PCA9685
	off.Enabled = boolPtr(false)
	pwm, err := NewPWM(&fakeBus{}, off)
	if err != nil || pwm != nil {
		t.Fatalf("a disabled board = %v, %v; want nil, nil", pwm, err)
	}
	if err := pwm.SetChannel(3, 200, false); err != nil {
		t.Errorf("a nil PWM should absorb the call, got %v", err)
	}

	if _, err := NewPWM(&fakeBus{}, DefaultConfig().PCA9685); err == nil {
		t.Error("an empty bus should fail the probe")
	}

	bus := &fakeBus{}
	cfg := DefaultConfig().PCA9685
	bus.plug(cfg.Addr, pcaReset)
	pwm, err = NewPWM(bus, cfg)
	if err != nil {
		t.Fatalf("NewPWM: %v", err)
	}
	if err := pwm.SetChannel(-1, hardware.LampLevelMax, false); err != nil {
		t.Errorf("an unwired channel should be a no-op, got %v", err)
	}
	before := len(chanWrites(bus, cfg.Addr))
	if err := pwm.SetChannel(6, 76, true); err != nil {
		t.Fatalf("SetChannel: %v", err)
	}
	if got := len(chanWrites(bus, cfg.Addr)); got != before+1 {
		t.Errorf("channel writes = %d, want one more than %d", got, before)
	}
}
