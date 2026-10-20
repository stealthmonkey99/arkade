// SPDX-License-Identifier: GPL-2.0-or-later

// Command preflight scans cartridges for the bug classes they have actually
// hit, and reports a per-cartridge readiness card.
//
//	go run ./tools/preflight <slug>     # one cartridge
//	go run ./tools/preflight -all       # every cartridge
//	go run ./tools/preflight -all -json # machine-readable, for CI
//
// Each check either decides its question from the syntax tree or asset bytes,
// or reports WARN and says what to look at. Checks live in check_*.go and are
// listed in registry.go, whose order is the report order.
//
// # Per-cartridge exceptions
//
// An optional cartridges/<slug>/preflight.json records findings that are
// correct about the code but wrong about the intent (see Config, Suppression
// and WavFloor):
//
//	{
//	  "suppress": [
//	    {
//	      "check": "golden-rules",
//	      "where": "cartridges/example/effects.go",
//	      "contains": "warmUpParticles",
//	      "reason": "why this is not a bug, in a sentence a stranger can check"
//	    }
//	  ],
//	  "wavFloor": { "hz": 150, "frac": 0.6, "reason": "auditioned on the cabinet" }
//	}
//
// Suppressed findings are still printed, tagged "[suppressed fail: <reason>]".
// A suppression that matches nothing warns, and a config that fails to load
// suppresses nothing.
//
// # Exit status
//
// 0 when every scanned cartridge passes (warnings allowed), 1 when any check
// fails, 2 on a usage or IO error. -strict promotes warnings to failures.
package main
