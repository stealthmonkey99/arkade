// SPDX-License-Identifier: GPL-2.0-or-later

package main

// testdata/golden/catcher-cart-glyphs.json is written here and read by the
// browser host's parity test, which replays the same catcher.wasm and checks
// it hands over the same table.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stealthmonkey99/arkade/carttest"
)

const cartGlyphsGolden = "testdata/golden/catcher-cart-glyphs.json"

// cartGlyphsOracle maps a rune (as a string) to eight row bytes, top to
// bottom: the host is handed the cartridge's whole table each time.
type cartGlyphsOracle map[string][]int

func glyphsFromHost(host *carttest.FakeHost) cartGlyphsOracle {
	out := cartGlyphsOracle{}
	for r, rows := range host.CartGlyphs {
		v := make([]int, len(rows))
		for i, b := range rows {
			v[i] = int(b)
		}
		out[string(r)] = v
	}
	return out
}

func TestCatcherDefinesItsOwnLCDGlyph(t *testing.T) {
	host := newCatcherHost(t)
	newCatcherCartridge(t, host) // cart_init is where the definition happens

	got := glyphsFromHost(host)
	if len(got) == 0 {
		t.Fatal("Catcher defined no glyphs — peripherals.go's defineGlyphs never ran")
	}

	if os.Getenv("UPDATE_GOLDENS") != "" {
		writeCartGlyphsGolden(t, got)
		return
	}

	raw, err := os.ReadFile(cartGlyphsGolden)
	if err != nil {
		t.Fatalf("read %s (UPDATE_GOLDENS=1 regenerates it): %v", cartGlyphsGolden, err)
	}
	var want cartGlyphsOracle
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse %s: %v", cartGlyphsGolden, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("glyph table = %v, want %v\n(the browser host checks itself against this same file — "+
			"if the change is intentional, UPDATE_GOLDENS=1 and re-run the web suite)", got, want)
	}
}

// TestCatcherGlyphFitsTheCGRAMBudget: the panel shows at most eight distinct
// custom glyphs at once, shared with the engine's built-ins. Catcher's
// busiest screen is the playing one, with its own basket on line 1 and the
// engine's hearts on line 2, so two.
func TestCatcherGlyphFitsTheCGRAMBudget(t *testing.T) {
	const cgramSlots = 8 // the HD44780's CGRAM holds eight custom glyphs

	_, host := catcherJourney(t, false)

	host.Mu.Lock()
	texts := append([]carttest.TextSetCall(nil), host.LCDTexts...)
	host.Mu.Unlock()

	worst, where := 0, ""
	for _, c := range texts {
		distinct := map[rune]bool{}
		for _, line := range []string{c.Line1, c.Line2} {
			for _, r := range line {
				if r >= 0x80 {
					distinct[r] = true
				}
			}
		}
		if len(distinct) > worst {
			worst, where = len(distinct), c.Line1+" / "+c.Line2
		}
	}
	if worst > cgramSlots {
		t.Errorf("a screen wants %d distinct custom glyphs (%q) — the chip has %d, so the extras draw '?'",
			worst, where, cgramSlots)
	}
	t.Logf("busiest screen uses %d of %d CGRAM slots (%q)", worst, cgramSlots, where)
}

func writeCartGlyphsGolden(t *testing.T, g cartGlyphsOracle) {
	t.Helper()
	// encoding/json sorts map keys, so the file is stable across runs
	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatalf("marshal glyph oracle: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cartGlyphsGolden), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cartGlyphsGolden, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", cartGlyphsGolden, err)
	}
	t.Logf("wrote %s", cartGlyphsGolden)
}
