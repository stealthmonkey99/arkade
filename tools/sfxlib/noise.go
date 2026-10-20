// SPDX-License-Identifier: GPL-2.0-or-later

package sfxlib

// NewRNG returns a seeded, deterministic generator of uniform [0,1)
// floats. It is for host-side asset generation only, not cartridge runtime.
func NewRNG(seed uint64) func() float64 {
	s := seed
	return func() float64 {
		s += 0x9E3779B97F4A7C15
		z := s
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		z ^= z >> 31
		return float64(z%1_000_000) / 1_000_000
	}
}

// NoiseSample returns one uniform sample in [-1,1) from rng.
func NoiseSample(rng func() float64) float64 {
	return (rng() - 0.5) * 2
}
