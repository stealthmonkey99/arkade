// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

const (
	minHoldMs     = 10
	maxHoldMs     = 10000
	defaultHoldMs = 120
)

// Normalize repairs env's presentation values in place so every reader
// shows a file the same way. A sprite frame or lamp hold under 10 ms (a
// missing one decodes as 0) becomes 120 ms and one over 10000 ms becomes
// 10000; a lamp fade is clamped to 0..MaxLightFadeMs; an unknown loop mode
// becomes LoopCycle. Only env's own kind is touched, and a second call
// changes nothing. Read, ReadBytes and DecodeSheet apply it after Validate;
// a caller building an envelope by hand applies it before writing.
func Normalize(env *Envelope) {
	switch env.Kind {
	case KindSprite:
		for i := range env.Animations {
			a := &env.Animations[i]
			for j := range a.Frames {
				a.Frames[j].HoldMs = clampHold(a.Frames[j].HoldMs)
			}
			a.Loop = knownLoop(a.Loop)
		}
	case KindLights:
		for i := range env.Frames {
			f := &env.Frames[i]
			f.HoldMs = clampHold(f.HoldMs)
			f.FadeMs = min(max(f.FadeMs, 0), MaxLightFadeMs)
		}
		env.Loop = knownLoop(env.Loop)
	}
}

func clampHold(ms int) int {
	switch {
	case ms < minHoldMs:
		return defaultHoldMs
	case ms > maxHoldMs:
		return maxHoldMs
	}
	return ms
}

func knownLoop(mode string) string {
	switch mode {
	case LoopCycle, LoopOnce, LoopPingPong:
		return mode
	}
	return LoopCycle
}
