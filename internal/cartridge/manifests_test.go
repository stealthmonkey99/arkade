// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

import (
	"encoding/json"
	"github.com/stealthmonkey99/arkade/tools/fleet"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
)

func TestFleetManifestsSchemaValid(t *testing.T) {
	// A tree with no cartridges has nothing to gate.
	dirs, slugs, err := fleet.Dirs("../..")
	if err != nil {
		t.Fatalf("listing cartridges: %v", err)
	}
	if len(slugs) == 0 {
		t.Skipf("no cartridges under %s", strings.Join(fleet.Roots("../.."), ", "))
	}
	checked := 0
	for _, slug := range slugs {
		cartsDir := filepath.Dir(dirs[slug])
		raw, err := os.ReadFile(filepath.Join(cartsDir, slug, "manifest.json"))
		if err != nil {
			continue
		}
		var m rcart.Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Errorf("%s: unparseable manifest.json: %v", slug, err)
			continue
		}
		checked++

		if m.SchemaVersion != rcart.CurrentSchemaVersion {
			t.Errorf("%s: schemaVersion %d, want %d", slug, m.SchemaVersion, rcart.CurrentSchemaVersion)
		}
		if err := m.Validate(); err != nil {
			t.Errorf("%s: %v", slug, err)
		}
		if m.Slug != slug {
			t.Errorf("%s: manifest slug %q does not match its directory", slug, m.Slug)
		}
		for _, ref := range []struct{ field, path string }{
			{"icon", m.Icon}, {"logo", m.Logo}, {"manual", m.Manual},
		} {
			if ref.path == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(cartsDir, slug, filepath.FromSlash(ref.path))); err != nil {
				t.Errorf("%s: %s %q not found in the cartridge directory", slug, ref.field, ref.path)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("found no cartridges under %s", strings.Join(fleet.Roots("../.."), ", "))
	}
}
