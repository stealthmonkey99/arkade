// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"math"

	"github.com/stealthmonkey99/arkade/internal/hardware"
)

// lampFade is one lamp's ramp. It animates the device value only: the mirror
// jumps to the target when the fade is asked for, so a resume replay lands on
// the end state. The browser host uses the same arithmetic on the ABI's 4-bit
// scale rather than the hardware's 8-bit one; parity is asserted on ABI calls,
// not per-tick device writes.
type lampFade struct {
	from, to       float64
	elapsed, total uint64 // microseconds
	ease           Ease
	active         bool
}

// Ease is the shape a fade takes between its two levels; Linear is the zero
// value.
type Ease uint8

const (
	Linear Ease = iota
	// EaseOut arrives fast and settles.
	EaseOut
	// EaseInOut eases at both ends, for ramps long enough that a hard start
	// or stop would read as a switch.
	EaseInOut
)

func (e Ease) at(p float64) float64 {
	p = clamp01(p)
	switch e {
	case EaseOut:
		return 1 - math.Pow(1-p, 3)
	case EaseInOut:
		return p * p * (3 - 2*p)
	}
	return p
}

func (f lampFade) value() float64 {
	if f.total == 0 {
		return f.to
	}
	if f.ease == Linear {
		// Not routed through Ease.at: the boot bloom was tuned on these numbers.
		return f.from + (f.to-f.from)*float64(f.elapsed)/float64(f.total)
	}
	return f.from + (f.to-f.from)*f.ease.at(float64(f.elapsed)/float64(f.total))
}

func (f lampFade) level() uint8 {
	v := int(math.Round(f.value()))
	return uint8(min(hardware.LampLevelMax, max(0, v)))
}

// shownLocked is the mirror with each fading lamp's in-flight value in place
// of its target.
func (s *shadowHardware) shownLocked(p int) hardware.LampLevels {
	out := s.levels[p]
	for i := range out {
		if f := s.fades[p][i]; f.active {
			out[i] = f.level()
		}
	}
	return out
}

func (s *shadowHardware) shownBacklightLocked() uint8 {
	if s.backlightFade.active {
		return s.backlightFade.level()
	}
	return s.backlight
}

// fadeBacklight ramps the LCD backlight to level over fadeMs; fadeMs <= 0 is
// instant.
func (s *shadowHardware) fadeBacklight(level uint8, fadeMs int, ease Ease) {
	level = min(level, hardware.LampLevelMax)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backlightTouched = true
	shown := s.shownBacklightLocked()
	s.backlight = level
	if fadeMs <= 0 || shown == level {
		s.backlightFade = lampFade{}
	} else {
		s.backlightFade = lampFade{
			from:   float64(shown),
			to:     float64(level),
			total:  uint64(fadeMs) * 1000,
			ease:   ease,
			active: true,
		}
	}
	s.pushBacklightLocked()
}

// fadeLamps ramps the lamps named by touch on the addressed seats (index -1
// = both) to levels over fadeMs; fadeMs <= 0 is instant. Other lamps are
// untouched.
func (s *shadowHardware) fadeLamps(index int, levels hardware.LampLevels, touch hardware.LightMask, fadeMs int, ease Ease) {
	levels = levels.Clamp()
	s.mu.Lock()
	defer s.mu.Unlock()
	for p := range s.levels {
		if !covered(index, p) {
			continue
		}
		shown := s.shownLocked(p)
		for i := range s.levels[p] {
			if !touch.Has(i) {
				continue
			}
			s.levels[p][i] = levels[i]
			if fadeMs <= 0 || shown[i] == levels[i] {
				s.fades[p][i] = lampFade{}
				continue
			}
			s.fades[p][i] = lampFade{
				from:   float64(shown[i]),
				to:     float64(levels[i]),
				total:  uint64(fadeMs) * 1000,
				ease:   ease,
				active: true,
			}
		}
	}
	s.pushLocked(index)
}

// bloomPctLocked is the wake bloom's scale, 100 when none is running.
func (s *shadowHardware) bloomPctLocked() int {
	if !s.bloom.active {
		return 100
	}
	return int(math.Round(s.bloom.value()))
}

// startBloom ramps the scale over every lamp and the backlight up to full
// over fadeMs; fadeMs <= 0 lands at full immediately.
func (s *shadowHardware) startBloom(fadeMs int, ease Ease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fadeMs <= 0 {
		s.bloom = lampFade{}
		s.repushLocked()
		return
	}
	// Not bloomPctLocked, which answers 100 when none is running.
	from := 0.0
	if s.bloom.active {
		from = s.bloom.value()
	}
	s.bloom = lampFade{
		from:   from,
		to:     100,
		total:  uint64(fadeMs) * 1000,
		ease:   ease,
		active: true,
	}
	s.repushLocked()
}

// endBloom drops a ramp in flight so a button press mid-wake is answered at
// once.
func (s *shadowHardware) endBloom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.bloom.active {
		return
	}
	s.bloom = lampFade{}
	s.repushLocked()
}

// stepFades advances every fade by dtMicros and writes whatever moved. The
// delta is the tick's, never wall clock, so replays reproduce fades exactly.
func (s *shadowHardware) stepFades(dtMicros uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// The bloom scales what the other fades push, so it advances first and
	// re-pushes last.
	blooming := s.bloom.active
	if blooming {
		s.bloom.elapsed += dtMicros
		if s.bloom.elapsed >= s.bloom.total {
			s.bloom = lampFade{}
		}
	}
	if s.backlightFade.active {
		s.backlightFade.elapsed += dtMicros
		if s.backlightFade.elapsed >= s.backlightFade.total {
			s.backlightFade = lampFade{}
		}
		s.pushBacklightLocked()
	}
	for p := range s.fades {
		moved := false
		for i := range s.fades[p] {
			f := &s.fades[p][i]
			if !f.active {
				continue
			}
			f.elapsed += dtMicros
			if f.elapsed >= f.total {
				*f = lampFade{}
			}
			moved = true
		}
		if moved {
			s.pushLocked(p)
		}
	}
	if blooming {
		s.repushLocked()
	}
}
