// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

func fixedFree(free uint64) func(string) (uint64, error) {
	return func(string) (uint64, error) { return free, nil }
}

func TestInstallRefusesWhenTheBundleWouldNotFit(t *testing.T) {
	liveDir, _, rc, _ := variantRig(t)
	bundle, err := os.ReadFile(writeRcart(t, t.TempDir(), "demo", "Demo", ""))
	if err != nil {
		t.Fatal(err)
	}
	cartridge.SetFreeSpaceFunc(rc, fixedFree(uint64(len(bundle))+cartridge.InstallHeadroomBytes-1))

	_, _, err = rc.Install("demo.rcart", bundle)
	var full *cartridge.InsufficientSpaceError
	if !errors.As(err, &full) {
		t.Fatalf("Install err = %v, want an InsufficientSpaceError", err)
	}
	if strings.Contains(err.Error(), liveDir) {
		t.Errorf("error %q names a server path", err)
	}
	if _, statErr := os.Stat(filepath.Join(liveDir, "demo.rcart")); !os.IsNotExist(statErr) {
		t.Error("a refused install still wrote the bundle")
	}
}

func TestSetVariantChecksSpaceAndWarnsWhenLow(t *testing.T) {
	liveDir, variantsDir, rc, _ := variantRig(t)
	writeVariant(t, variantsDir, "public", "roomy", "Roomy")
	t.Cleanup(func() { engine.Unregister("cart/roomy") })
	var logged []string
	rc.SetLogger(func(level, msg string) { logged = append(logged, level+": "+msg) })

	cartridge.SetFreeSpaceFunc(rc, fixedFree(cartridge.InstallHeadroomBytes))
	var full *cartridge.InsufficientSpaceError
	if _, err := rc.SetVariant("cart/roomy", "public"); !errors.As(err, &full) {
		t.Fatalf("SetVariant err = %v, want an InsufficientSpaceError", err)
	}

	cartridge.SetFreeSpaceFunc(rc, fixedFree(cartridge.LowSpaceWarnBytes-1))
	if _, err := rc.SetVariant("cart/roomy", "public"); err != nil {
		t.Fatalf("SetVariant with room to spare: %v", err)
	}
	if _, err := os.Stat(filepath.Join(liveDir, "roomy.rcart")); err != nil {
		t.Fatalf("bundle not installed: %v", err)
	}
	if len(logged) != 1 || !strings.HasPrefix(logged[0], "warn: ") || !strings.Contains(logged[0], "low on space") {
		t.Errorf("logged = %q, want one low-space warning", logged)
	}

	logged = nil
	cartridge.SetFreeSpaceFunc(rc, fixedFree(cartridge.LowSpaceWarnBytes))
	if _, err := rc.SetVariant("cart/roomy", "public"); err != nil {
		t.Fatalf("SetVariant: %v", err)
	}
	if len(logged) != 0 {
		t.Errorf("logged = %q at the threshold, want nothing", logged)
	}
}

func TestUnmeasurableFilesystemDoesNotBlockInstall(t *testing.T) {
	_, variantsDir, rc, _ := variantRig(t)
	writeVariant(t, variantsDir, "public", "roomy", "Roomy")
	t.Cleanup(func() { engine.Unregister("cart/roomy") })
	cartridge.SetFreeSpaceFunc(rc, func(string) (uint64, error) { return 0, errors.New("statfs: not supported") })
	if _, err := rc.SetVariant("cart/roomy", "public"); err != nil {
		t.Fatalf("SetVariant with an unmeasurable filesystem: %v", err)
	}
}

func TestStatfsFreeReportsSomething(t *testing.T) {
	free, err := cartridge.StatfsFree(t.TempDir())
	if err != nil {
		t.Fatalf("statfs: %v", err)
	}
	if free == 0 {
		t.Error("statfs reports 0 bytes free on the test machine's temp dir")
	}
}
