// SPDX-License-Identifier: GPL-2.0-or-later

// Command arkade-lcdboot owns the cabinet's two LCDs from the moment I2C bus
// 1 appears until the engine takes them over, and again while a crashed
// engine waits to restart.
//
// The PCF8574 backpacks power up with every pin high, so the panels glare
// until arkade.service starts. This blanks them, then shows a progress bar
// against the units in -stages on P1 and the address and temperature on P2.
//
// systemd drives both ends: a udev rule starts the unit when the bus exists,
// and arkade.service's ExecStartPre stops it with a SIGTERM that fills the
// bar and exits with the panels lit at 30% for the engine's startup card.
// ExecStopPost reruns this binary with -after-engine to leave a crash card.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/stealthmonkey99/arkade/internal/hardware/pi"
)

const unitName = "arkade-lcdboot.service"

// defaultStages are milestones arkade.service itself waits on, so each lands
// before the handover.
const defaultStages = "network=network.target,sound=sound.target,radio=bluetooth.service,engine=arkade.service"

const (
	frameTick   = 50 * time.Millisecond
	pollTick    = 500 * time.Millisecond
	marqueeTick = 300 * time.Millisecond
	sysTick     = 2 * time.Second
)

type options struct {
	dataDir     string
	stateDir    string
	stages      string
	afterEngine bool
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("arkade-lcdboot: ")
	var o options
	flag.StringVar(&o.dataDir, "data", "data", "engine data directory, for hardware/settings.json")
	flag.StringVar(&o.stateDir, "state-dir", "/run/arkade", "where the engine's exit is left for the crash card")
	flag.StringVar(&o.stages, "stages", defaultStages,
		"boot milestones as name=unit, in the order they come up")
	flag.BoolVar(&o.afterEngine, "after-engine", false, "record the engine's exit (systemd ExecStopPost) and exit")
	flag.Parse()

	if err := run(o); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func logf(format string, args ...any) { log.Printf(format, args...) }

func run(o options) error {
	if o.afterEngine {
		return afterEngine(o)
	}
	stages, err := parseStages(o.stages)
	if err != nil {
		return err
	}
	bus, closer, err := pi.OpenBus()
	if err != nil {
		return err
	}
	defer closer.Close()

	cab := openCabinet(bus, pi.LoadConfig(o.dataDir))
	defer cab.close()
	if !cab.live() {
		return fmt.Errorf("neither LCD answered on bus 1")
	}
	cab.darkenLamps()
	cab.setBacklight(backlightIdle)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if c, fresh := readCrash(o.stateDir, time.Now()); fresh {
		logf("the engine exited %s (%s); showing the crash card", c.status, c.result)
		crashCard(ctx, cab, c)
	} else {
		bootCard(ctx, cab, stages)
	}
	cab.finish(stages[len(stages)-1].name)
	return nil
}

// afterEngine is the ExecStopPost hook. A clean stop does nothing; a failure
// is recorded for a fresh run, started with --no-block because this runs
// inside arkade.service's own stop job.
func afterEngine(o options) error {
	c, failed := crashFromEnv(os.Getenv, time.Now())
	if !failed {
		return nil
	}
	if err := writeCrash(o.stateDir, c); err != nil {
		return err
	}
	return exec.Command("systemctl", "start", "--no-block", unitName).Run()
}

func bootCard(ctx context.Context, cab *cabinet, stages []stage) {
	start := time.Now()
	track := newTracker(stages, start)
	units := startPolling(ctx, unitNames(stages))
	sys := readSystemInfo()

	frames(ctx, start, func(now time.Time, elapsed time.Duration) {
		track.update(units.read(), now)
		if every(elapsed, sysTick) {
			sys = readSystemInfo()
		}
		frac := progressFraction(track.reached, len(stages), now.Sub(track.since))
		cab.setText(0, bootLines(frac, track.stage, elapsed))
		cab.setText(1, statusLines(sys))
	})
}

// crashCard replaces boot progress after an engine death: breathing panels and
// pulsing red buttons, until the engine's restart stops the unit.
func crashCard(ctx context.Context, cab *cabinet, c crash) {
	start := time.Now()
	msg := c.message()
	sys := readSystemInfo()

	frames(ctx, start, func(now time.Time, elapsed time.Duration) {
		if every(elapsed, sysTick) {
			sys = readSystemInfo()
		}
		cab.setText(0, crashLines(msg, elapsed))
		cab.setText(1, statusLines(sys))
		cab.setBacklight(lerpLevel(backlightIdle, backlightBreath, wave(elapsed, breathPeriod)))
		lamp := lerpLevel(0, redLampPeak, wave(elapsed, pulsePeriod))
		cab.setLamp(0, redButton, lamp)
		cab.setLamp(1, redButton, lamp)
	})
}

func frames(ctx context.Context, start time.Time, draw func(now time.Time, elapsed time.Duration)) {
	ticker := time.NewTicker(frameTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			draw(now, now.Sub(start))
		}
	}
}

// every reports whether this frame crosses a multiple of period.
func every(elapsed, period time.Duration) bool {
	return elapsed/period != (elapsed-frameTick)/period
}

// unitPoller polls systemd on its own goroutine so a busy early boot stalls no drawing.
type unitPoller struct {
	mu     sync.Mutex
	active []bool
}

func startPolling(ctx context.Context, units []string) *unitPoller {
	p := &unitPoller{active: make([]bool, len(units))}
	go func() {
		complained := false
		for {
			active, err := unitsActive(ctx, units)
			if err != nil && !complained {
				complained = true
				logf("systemctl is-active: %v", err)
			}
			p.mu.Lock()
			p.active = active
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollTick):
			}
		}
	}()
	return p
}

func (p *unitPoller) read() []bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active
}

// unitsActive asks about every unit in one call. is-active exits non-zero
// whenever any unit is down, so only empty output counts as a failure.
func unitsActive(ctx context.Context, units []string) ([]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", append([]string{"is-active"}, units...)...).Output()
	if len(out) == 0 {
		return make([]bool, len(units)), err
	}
	return parseActive(string(out), len(units)), nil
}
