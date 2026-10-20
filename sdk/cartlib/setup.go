// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

// Setup is the receiver for the calls only [Game.Init] may make: loading
// sounds and sprite sheets, binding input and output channels, and defining
// LCD glyphs. Each one allocates in the guest, asks the host for a handle it
// hands out only during cart_init, or both, so none of them belongs in
// Update, HandleInput or Render, and the type system rather than a comment
// keeps them out: they are methods of Setup, cartlib passes the one live
// Setup to Init, and nothing else ever produces one.
//
//	func (g *myGame) Init(s *cartlib.Setup) {
//	    g.chime = s.LoadSound("assets/chime.wav")
//	    g.hero = s.LoadSheet("assets/hero.arkadesprite.png").Cel("idle")
//	    g.wheel = s.BindDelta(cartlib.P1 + "spinner")
//	}
//
// The Setup is dead once Init returns. The zero Setup is dead too, so a
// cartridge cannot build a live one. A call through a dead Setup, or through
// a [Sheet] it loaded, returns that call's failure value (0, the zero Sheet,
// a nil cel, an unbound Channel, false); under the plain Go toolchain it also
// panics, so a cartridge's `go test` and sdk/cartlib/hosttest catch the
// misuse, and on the cabinet it logs at [LogError], where the host's log
// and carttest's FakeHost.Logs show it. Keeping a *Setup in a field to use
// later is therefore a bug the tests find, not an escape hatch.
//
// The canvas size is not a Setup concern: it is [FBWidth] by [FBHeight],
// constants, which is why Init takes no cols and rows.
type Setup struct {
	live bool
}

// ok reports whether s may act, logging or panicking per toolchain when not.
func (s *Setup) ok(stale string) bool {
	if s != nil && s.live {
		return true
	}
	staleSetup(stale)
	return false
}

// LoadSound registers the sound at a bundle-relative path such as
// "assets/chime.wav" and returns its handle for [PlaySound] and
// [StopSound]. It returns 0 when the load fails, including in a bare .wasm
// build with no bundle; every sound call treats handle 0 as a silent no-op.
func (s *Setup) LoadSound(path string) uint32 {
	if !s.ok("cartlib: Setup.LoadSound after Init returned; the sound was not loaded") {
		return 0
	}
	return host.LoadSound(path)
}

// LoadSheet loads the sprite sheet at a bundle-relative path such as
// "assets/hero.arkadesprite.png" (docs/ASSETS.md, "Sprites from the
// bundle"). Nothing is copied into the guest until [Sheet.Cel]. A failed
// load, including a bare .wasm build with no bundle, returns the zero
// [Sheet], whose Cel returns nil, and the host logs why.
func (s *Setup) LoadSheet(path string) Sheet {
	if !s.ok("cartlib: Setup.LoadSheet after Init returned; the sheet was not loaded") {
		return Sheet{}
	}
	return Sheet{handle: host.LoadSheet(path), setup: s}
}

// BindButton binds a two-state channel such as a panel button or a pad
// arrow. A bind fails when nothing on this cabinet offers name, or offers
// it as another kind; see [Channel].
func (s *Setup) BindButton(name string) Channel {
	return s.bind(name, kindButton, "cartlib: Setup.BindButton after Init returned; the channel is unbound")
}

// BindAxis binds a positional channel such as a stick axis, a trigger or a
// light-gun coordinate, read as an absolute position in int16 range; the
// cabinet's own stick reads -1, 0 or +1.
func (s *Setup) BindAxis(name string) Channel {
	return s.bind(name, kindAxis, "cartlib: Setup.BindAxis after Init returned; the channel is unbound")
}

// BindDelta binds a relative channel such as a spinner, trackball or
// encoder, whose motion the host accumulates between Updates.
func (s *Setup) BindDelta(name string) Channel {
	return s.bind(name, kindDelta, "cartlib: Setup.BindDelta after Init returned; the channel is unbound")
}

// BindBlob binds a channel whose value is a fixed-length run of bytes, up
// to [MaxBlobBytes], declared by the device's profile.
func (s *Setup) BindBlob(name string) Channel {
	return s.bind(name, kindBlob, "cartlib: Setup.BindBlob after Init returned; the channel is unbound")
}

func (s *Setup) bind(name string, kind channelKind, stale string) Channel {
	if !s.ok(stale) {
		return Channel{}
	}
	h := host.InputBind(name, uint32(kind))
	if h < 0 {
		return Channel{}
	}
	return Channel{handle: h, bound: true}
}

// BindOutput binds an output channel by name. Reserved: it fails on every
// current host, returning the zero [Output], whose Set does nothing.
func (s *Setup) BindOutput(name string) Output {
	if !s.ok("cartlib: Setup.BindOutput after Init returned; the output is unbound") {
		return Output{}
	}
	h := host.OutputBind(name)
	if h < 0 {
		return Output{}
	}
	return Output{handle: h, bound: true}
}

// DefineLCDGlyph makes the cabinet's LCDs draw r as rows while this
// cartridge is loaded, overriding the engine's built-in glyph for r if
// there is one. Definitions end when the cartridge unloads.
//
// It reports whether the definition was accepted, with the same answer on
// every host, including one with no LCD. It returns false when:
//
//   - r is below U+0080: the LCD's ROM draws ASCII.
//   - r is an emoji or East Asian wide rune: U+1F000 to U+1FFFF, the
//     emoji-presentation symbols, variation selectors, the zero-width
//     joiner, CJK, Hangul, kana, the fullwidth forms, and everything from
//     U+20000 up. A glyph rune must be a plain single-width monochrome
//     character; ♥, ☺, ✔, arrows, shades and the private-use area are
//     fine.
//   - r is not a code point: a surrogate, a noncharacter, or past U+10FFFF.
//   - [MaxCartGlyphs] distinct runes are already defined. Redefining a rune
//     always succeeds.
//
// Each LCD can show at most eight distinct custom glyphs at once, built-in
// ones included; a ninth on the same screen draws as '?'.
func (s *Setup) DefineLCDGlyph(r rune, rows GlyphRows) bool {
	if !s.ok("cartlib: Setup.DefineLCDGlyph after Init returned; the glyph was not defined") {
		return false
	}
	return host.DefineLcdGlyph(int32(r), packGlyphRows(rows)) != 0
}
