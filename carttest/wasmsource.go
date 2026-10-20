// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stealthmonkey99/arkade/tools/wasmfix/wasmpatch"
)

// buildHostGOOS and buildHostGOARCH name the machine the committed .wasm
// binaries are built on. TinyGo output is reproducible on one machine but
// differs in size across platforms, so byte equality is asserted only here.
const (
	buildHostGOOS   = "darwin"
	buildHostGOARCH = "arm64"
)

// tinyGoCartridgeBuildArgs are the flags every cartridge is compiled with;
// TestBuildArgsMatchTheCartridgeMakefiles holds them to the SDK Makefiles.
var tinyGoCartridgeBuildArgs = []string{
	"-target=wasi",
	"-scheduler=none",
	"-gc=leaking",
	"-opt=z",
	"-no-debug",
	"-panic=trap",
}

// RequireWasmMatchesSource rebuilds srcDir with TinyGo, applies the wasmfix
// post-pass the cartridge build applies, and compares the result with the
// committed wasmName in srcDir. rebuildCmd is quoted on failure.
//
// It skips when TinyGo is not on PATH. On any host the source must compile
// and the module must survive the cart_manifest sret patch. If the bytes
// differ from the committed binary, it fails on the build host and skips
// elsewhere, where differing toolchain output is expected; the cartridges CI
// workflow covers drift there by running the goldens against a fresh build.
func RequireWasmMatchesSource(t *testing.T, srcDir, wasmName, rebuildCmd string) {
	t.Helper()

	tinygoPath, err := exec.LookPath("tinygo")
	if err != nil {
		t.Skip("tinygo not on PATH; skipping source<->binary equivalence check (the tests around this one still cover the checked-in binary)")
	}

	outPath := filepath.Join(t.TempDir(), wasmName)
	args := append([]string{"build"}, tinyGoCartridgeBuildArgs...)
	args = append(args, "-o", outPath, ".")
	cmd := exec.Command(tinygoPath, args...)
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tinygo build in %s: %v\n%s", srcDir, err, out)
	}
	built, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read rebuilt wasm: %v", err)
	}
	patched, _, _, err := wasmpatch.PatchSretExport(built, "cart_manifest")
	if err != nil {
		t.Fatalf("wasmpatch (the same post-pass `%s` runs): %v", rebuildCmd, err)
	}

	committedPath := filepath.Join(srcDir, wasmName)
	committed, err := os.ReadFile(committedPath)
	if err != nil {
		t.Fatalf("read checked-in %s (run `%s` to rebuild it): %v", committedPath, rebuildCmd, err)
	}
	if bytes.Equal(patched, committed) {
		return
	}

	version, _ := exec.Command(tinygoPath, "version").Output()
	if runtime.GOOS != buildHostGOOS || runtime.GOARCH != buildHostGOARCH {
		t.Skipf("byte-exact source<->binary check not run: this is %s/%s, and the checked-in binaries are built on %s/%s.\n"+
			"A fresh build here is %d bytes vs the checked-in %d — expected, since two platform builds of the same TinyGo release do not emit identical wasm.\n"+
			"Toolchain here: %s"+
			"Drift is caught behaviourally on this platform instead: cartridges.yml reruns this cartridge's goldens against a binary rebuilt in the job.",
			runtime.GOOS, runtime.GOARCH, buildHostGOOS, buildHostGOARCH,
			len(patched), len(committed), version)
	}

	t.Fatalf("checked-in %s does not match a fresh build of its source (%d vs %d bytes).\n"+
		"Toolchain here: %s"+
		"If the source changed, run `%s` and re-review the goldens (UPDATE_GOLDENS=1).\n"+
		"If only the TinyGo version changed, rebuilding is still the fix — goldens pin behavior, not bytes.",
		committedPath, len(patched), len(committed), version, rebuildCmd)
}
