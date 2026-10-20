// SPDX-License-Identifier: GPL-2.0-or-later

package hardware

// LampLevelMax is the brightest lamp level. Levels are 8 bits; coarser
// back-ends quantize.
const LampLevelMax = 255

// LampLevelABIMax is the brightest lamp level a cartridge can ask for;
// host_set_lamp_levels packs six 4-bit lamps into one u32.
const LampLevelABIMax = 15

// LampLevelFromABI widens a cartridge's 4-bit level to the 8-bit scale,
// clamping above LampLevelABIMax.
func LampLevelFromABI(v uint8) uint8 {
	if v > LampLevelABIMax {
		v = LampLevelABIMax
	}
	return v * 17
}

// BacklightDimmer is optionally implemented by a Suite whose LCD backlight
// takes a level, 0 dark to LampLevelMax full.
type BacklightDimmer interface {
	SetBacklightLevel(level uint8)
}

// BacklightReader is optionally implemented beside BacklightDimmer by a Suite
// that knows its current backlight level, so fades can start from it.
type BacklightReader interface {
	BacklightLevel() uint8
}

// LampLevels is one seat's lamp brightness, 0 to LampLevelMax, indexed in
// LightButtons order.
type LampLevels [6]uint8

func LampLevelsOf(m LightMask) LampLevels {
	var l LampLevels
	for i := range l {
		if m.Has(i) {
			l[i] = LampLevelMax
		}
	}
	return l
}

// Mask reports which lamps are lit; any non-zero level counts.
func (l LampLevels) Mask() LightMask {
	var m LightMask
	for i, v := range l {
		if v > 0 {
			m |= 1 << uint(i)
		}
	}
	return m
}

// Binary reports whether every lamp is dark or full, so it can be latched
// instead of modulated.
func (l LampLevels) Binary() bool {
	for _, v := range l {
		if v != 0 && v != LampLevelMax {
			return false
		}
	}
	return true
}

func (l LampLevels) Clamp() LampLevels {
	for i, v := range l {
		if v > LampLevelMax {
			l[i] = LampLevelMax
		}
	}
	return l
}
