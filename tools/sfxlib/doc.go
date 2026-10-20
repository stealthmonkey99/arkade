// SPDX-License-Identifier: GPL-2.0-or-later

// Package sfxlib synthesizes the short sound effects the Årkade cabinet
// plays, offline and deterministically. A cue is described as a Patch or
// Vocal (data that can be reopened and re-rendered), rendered, checked
// with AssertSpectrum, and written as an asset file carrying its recipe.
// Cues the grammars cannot express are hand-written from the exported
// primitives. This is host-only build-time code, so it may use math.Sin
// and friends freely.
package sfxlib

// SampleRate44100 is the default rate for tests and most generators; every
// function takes the rate explicitly.
const SampleRate44100 = 44100
