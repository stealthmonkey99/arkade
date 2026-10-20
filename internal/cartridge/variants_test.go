// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

// variantRig passes a nil runtime: SetVariant never launches anything.
func variantRig(t *testing.T) (liveDir, variantsDir string, rc *cartridge.RemoteControl, reg *cartridge.Registry) {
	t.Helper()
	root := t.TempDir()
	liveDir = filepath.Join(root, "cartridges")
	variantsDir = filepath.Join(root, cartridge.VariantsDirName)
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live: %v", err)
	}
	reg = cartridge.NewRegistry(cartridge.Options{Dirs: []string{liveDir}})
	rc, err := cartridge.NewRemoteControl(reg, nil, liveDir, variantsDir)
	if err != nil {
		t.Fatalf("NewRemoteControl: %v", err)
	}
	return liveDir, variantsDir, rc, reg
}

func TestNewRemoteControlChecksItsDirectories(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "cartridges")
	variants := filepath.Join(root, cartridge.VariantsDirName)
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{filepath.Join(root, "*")}})
	if _, err := cartridge.NewRemoteControl(reg, nil, live, ""); err != nil {
		t.Errorf("install dir matched by a scan glob: %v", err)
	}
	if _, err := cartridge.NewRemoteControl(reg, nil, live, variants); err == nil {
		t.Error("a variants dir the registry scans was accepted")
	}
	unscanned := cartridge.NewRegistry(cartridge.Options{Dirs: []string{variants}})
	if _, err := cartridge.NewRemoteControl(unscanned, nil, live, ""); err == nil {
		t.Error("an install dir the registry does not scan was accepted")
	}
}

func writeVariant(t *testing.T, variantsDir, variant, slug, name string) {
	t.Helper()
	dir := filepath.Join(variantsDir, variant)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	writeRcart(t, dir, slug, name, "the "+variant+" build")
}

func TestVariantsAbsentStoreIsSilent(t *testing.T) {
	liveDir, _, rc, _ := variantRig(t)
	writeRcart(t, liveDir, "demo", "Demo", "")

	available, active := rc.Variants()
	if available != nil || active != nil {
		t.Fatalf("Variants() = %v, %v; want nil, nil with no variants directory", available, active)
	}
	if _, err := rc.SetVariant("cart/demo", "public"); err == nil {
		t.Error("SetVariant succeeded with no variants directory; want an error")
	}
}

func TestVariantsNeedsTwoBuildsToCount(t *testing.T) {
	_, variantsDir, rc, _ := variantRig(t)
	writeVariant(t, variantsDir, "public", "lonely", "Lonely")

	available, _ := rc.Variants()
	if len(available) != 0 {
		t.Fatalf("Variants() = %v, want no entries for a slug with one build", available)
	}
}

func TestVariantsReportsAvailableAndActive(t *testing.T) {
	liveDir, variantsDir, rc, _ := variantRig(t)
	writeVariant(t, variantsDir, "public", "life", "Life")
	writeVariant(t, variantsDir, "private", "life", "Life Deluxe")

	available, active := rc.Variants()
	if got := available["cart/life"]; len(got) != 2 || got[0] != "public" || got[1] != "private" {
		t.Fatalf("available[cart/life] = %v, want [public private] (public leads: it is what ships)", got)
	}
	if a, ok := active["cart/life"]; ok {
		t.Errorf("active[cart/life] = %q, want absent when nothing is installed", a)
	}

	// A hand-copied build is recognized by content.
	raw, err := os.ReadFile(filepath.Join(variantsDir, "private", "life.rcart"))
	if err != nil {
		t.Fatalf("read stored build: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "life.rcart"), raw, 0o644); err != nil {
		t.Fatalf("install by hand: %v", err)
	}
	_, active = rc.Variants()
	if active["cart/life"] != "private" {
		t.Errorf("active[cart/life] = %q, want private", active["cart/life"])
	}

	writeRcart(t, liveDir, "life", "Something Else", "")
	_, active = rc.Variants()
	if a, ok := active["cart/life"]; ok {
		t.Errorf("active[cart/life] = %q, want absent when the installed file is neither build", a)
	}
}

func TestSetVariantInstallsAndRegisters(t *testing.T) {
	liveDir, variantsDir, rc, reg := variantRig(t)
	cleanupRegistry(t, "cart/life")
	writeVariant(t, variantsDir, "public", "life", "Life")
	writeVariant(t, variantsDir, "private", "life", "Life Deluxe")

	name, err := rc.SetVariant("cart/life", "private")
	if err != nil {
		t.Fatalf("SetVariant: %v", err)
	}
	if name != "Life Deluxe" {
		t.Errorf("name = %q, want Life Deluxe", name)
	}
	m, err := engine.Lookup("cart/life")
	if err != nil {
		t.Fatalf("Lookup(cart/life) after SetVariant: %v", err)
	}
	if m.Name != "Life Deluxe" {
		t.Errorf("registered name = %q, want Life Deluxe — SetVariant must re-register, not just copy", m.Name)
	}
	if _, active := rc.Variants(); active["cart/life"] != "private" {
		t.Errorf("active = %q, want private", active["cart/life"])
	}

	// Swapping back re-registers under the same slug, keeping the scores attached.
	if _, err := rc.SetVariant("cart/life", "public"); err != nil {
		t.Fatalf("SetVariant back: %v", err)
	}
	m, err = engine.Lookup("cart/life")
	if err != nil {
		t.Fatalf("Lookup(cart/life) after swapping back: %v", err)
	}
	if m.Name != "Life" {
		t.Errorf("registered name = %q, want Life", m.Name)
	}

	// No stray .tmp left in the library for the watcher to trip on.
	entries, err := os.ReadDir(liveDir)
	if err != nil {
		t.Fatalf("ReadDir(liveDir): %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "life.rcart" {
		t.Errorf("library dir = %v, want just life.rcart", entries)
	}
	_ = reg
}

func TestSetVariantRefusesSlugMismatch(t *testing.T) {
	liveDir, variantsDir, rc, _ := variantRig(t)
	dir := filepath.Join(variantsDir, "private")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeRcart(t, dir, "snake", "Life Deluxe", "")
	if err := os.Rename(filepath.Join(dir, "snake.rcart"), filepath.Join(dir, "life.rcart")); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if _, err := rc.SetVariant("life", "private"); err == nil {
		t.Fatal("SetVariant accepted a bundle whose manifest slug disagrees with its filename")
	}
	if _, err := os.Stat(filepath.Join(liveDir, "life.rcart")); !os.IsNotExist(err) {
		t.Error("a refused bundle was still written into the live library")
	}
}

func TestSetVariantRejectsUnsafeNames(t *testing.T) {
	_, variantsDir, rc, _ := variantRig(t)
	writeVariant(t, variantsDir, "public", "demo", "Demo")
	writeVariant(t, variantsDir, "private", "demo", "Demo Deluxe")

	for _, tc := range []struct{ slug, variant string }{
		{"demo", "../public"},
		{"demo", ".."},
		{"demo", "Public"},
		{"../demo", "public"},
		{"cart/../demo", "public"},
	} {
		if _, err := rc.SetVariant(tc.slug, tc.variant); err == nil {
			t.Errorf("SetVariant(%q, %q) succeeded; want rejection", tc.slug, tc.variant)
		}
	}
}

func TestSetVariantUnknownBuild(t *testing.T) {
	_, variantsDir, rc, _ := variantRig(t)
	writeVariant(t, variantsDir, "public", "demo", "Demo")
	writeVariant(t, variantsDir, "private", "demo", "Demo Deluxe")

	if _, err := rc.SetVariant("demo", "staging"); err == nil {
		t.Error("SetVariant accepted a variant with no stored build")
	}
}
