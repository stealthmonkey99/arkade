// SPDX-License-Identifier: GPL-2.0-or-later

package attract

import "math"

// ambientFadeMs is how long each side of an ambient transition takes. Views
// fade through black rather than crossfade, since superimposed wordmark and
// figure art reads as noise.
const ambientFadeMs = 350

// bootSettleMs is how long the wordmark eases from the splash's final
// brightness down to ambientDim after boot.
const bootSettleMs = 1200

// tailAttenuate scales level down to 0 across the slot's last ambientFadeMs.
// dur <= 0 returns level unchanged.
func tailAttenuate(rem, dur, level float64) float64 {
	if dur <= 0 {
		return level
	}
	if tail := dur - ambientFadeMs; rem >= tail {
		return level * (1 - (rem-tail)/ambientFadeMs)
	}
	return level
}

// fadeEdges holds level through the middle of a dur-long slot, fading
// linearly from and to black over ambientFadeMs at each end. Pure in rem (ms
// into the slot) and dur.
func fadeEdges(rem, dur, level float64) float64 {
	if dur > 0 && rem < ambientFadeMs {
		return level * rem / ambientFadeMs
	}
	return tailAttenuate(rem, dur, level)
}

// wordmarkOpacity is the wordmark's opacity. On the first lap it does not
// fade up from black, which would flash in a dark room on idle entry; with
// bootSettle it starts at full splash brightness and eases down to ambientDim
// over bootSettleMs. Later laps fade like any ambient edge.
func wordmarkOpacity(rem, dur float64, firstLap, bootSettle bool) float64 {
	if !firstLap {
		return fadeEdges(rem, dur, ambientDim)
	}
	if bootSettle && rem < bootSettleMs {
		return tailAttenuate(rem, dur, 1-(1-ambientDim)*easeOutCubic(rem/bootSettleMs))
	}
	return tailAttenuate(rem, dur, ambientDim)
}

func easeOutCubic(t float64) float64 {
	t = math.Max(0, math.Min(1, t))
	return 1 - math.Pow(1-t, 3)
}
