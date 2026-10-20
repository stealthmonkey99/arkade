// SPDX-License-Identifier: GPL-2.0-or-later

package main

// Check is one question preflight can answer about a cartridge. Run records
// findings on the Result with r.fail, r.warn, r.info and r.skip.
type Check struct {
	// ID is the report column name and JSON key; keep it stable.
	ID string
	// Title is the one-line human description.
	Title string
	// Rule cites the canon this check enforces.
	Rule string
	// Slow marks a check that runs a cartridge's test suite; -fast skips it.
	Slow bool
	Run  func(c *Cart, r *Result)
}

// registry lists every check; its order is the report order.
//
// "No allocation after Init" is not checked: syntactic signals cannot tell a
// leak from tolerated idioms such as LCD text formatting, and real violations
// sit several calls below Update. It would need a call graph.
var registry = []Check{
	{
		ID:    "config",
		Title: "the cartridge's optional preflight.json is valid and every exemption is still earning its place",
		Rule:  ruleConfig,
		Run:   checkConfig,
	},
	{
		ID:    "golden-rules",
		Title: "GOLDEN RULES static analysis over the TinyGo-compiled sources",
		Rule:  "docs/GAME_AUTHORING_AI.md GOLDEN RULES 1-8",
		Run:   checkGoldenRules,
	},
	{
		ID:    "wav-audibility",
		Title: "every shipped WAV decodes, and clears the speaker model's floor (spectral findings only warn)",
		Rule:  "docs/GAME_AUTHORING_AI.md 'Peripherals & sound'; tools/sfxlib doc.go",
		Run:   checkWavAudibility,
	},
	{
		ID:    "sprite-alpha",
		Title: "PNG alpha channels read as intended collision masks",
		Rule:  "docs/GAME_AUTHORING_AI.md 'The alpha rule' (A==255 collides, below 255 draws but never collides)",
		Run:   checkSpriteAlpha,
	},
	{
		ID:    "manifest",
		Title: "manifest schema, icon/manual presence, version consistency",
		Rule:  "internal/cartridge/rcart.Manifest.Validate; manifest footer rule; manifest.json is the source of truth",
		Run:   checkManifest,
	},
	{
		ID:    "levels",
		Title: "levels_gen.go is what the cartridge's level packs generate",
		Rule:  ruleLevels,
		Run:   checkGeneratedLevels,
	},
	{
		ID:    "bundle",
		Title: "the cartridge directory packs into a loadable .rcart",
		Rule:  "internal/cartridge/rcart.BuildDirTar + rcart.Load (the mount-time contract)",
		Run:   checkBundle,
	},
	{
		ID:    "lcd",
		Title: "LCD lines fit 16 columns and stay inside the CGRAM budget",
		Rule:  "docs/CARTRIDGE.md 16x2 LCD; internal/hardware/pi/glyphs.go (8 CGRAM slots, unmapped runes render '?'); sdk/cartlib/glyphs.go (cart-defined glyphs)",
		Run:   checkLCD,
	},
	{
		ID:    "tests",
		Title: "the cartridge's own suite (goldens, determinism, wasm-matches-source, hint style)",
		Rule:  "docs/GAME_AUTHORING_AI.md 'Determinism & cross-host goldens'",
		Slow:  true,
		Run:   checkTests,
	},
}

// registryIDs is filled in init because checkConfig reads it and registry
// names checkConfig, which a var initializer would make an init cycle.
var registryIDs = map[string]bool{}

func init() {
	for _, check := range registry {
		registryIDs[check.ID] = true
	}
}
