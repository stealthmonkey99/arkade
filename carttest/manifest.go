// SPDX-License-Identifier: GPL-2.0-or-later

package carttest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/tools/manifestgen/gen"
)

// maxShareCardDescription is three share-card lines of about 33 characters;
// longer descriptions are ellipsis-truncated.
const maxShareCardDescription = 99

// ManifestCopiesAgree checks that the manifest the cartridge returns over
// cart_manifest is byte for byte what the manifest.json at manifestPath
// generates, so the wasm was built from a current manifest_gen.go, and that
// the description is neither the scaffold's "Cartridge <Name>" stub nor too
// long for the share card. rebuildCmd is quoted on a stale wasm.
func ManifestCopiesAgree(t *testing.T, cart *Cartridge, manifestPath, rebuildCmd string) {
	t.Helper()
	got, err := cart.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read %s: %v", manifestPath, err)
	}
	want, err := gen.Runtime(data)
	if err != nil {
		t.Fatalf("%s: %v", manifestPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("cart_manifest returns %s\nbut %s generates %s\n— the wasm is stale: regenerate and rebuild (`%s`)",
			got, manifestPath, want, rebuildCmd)
	}

	var bundle struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("parse %s: %v", manifestPath, err)
	}
	if bundle.Description == "" || bundle.Description == "Cartridge "+bundle.Title {
		t.Errorf("description is %q — that is the developer stub, and it is the public landing page's copy", bundle.Description)
	}
	if n := len(bundle.Description); n > maxShareCardDescription {
		t.Errorf("description is %d characters; the share card wraps to three lines of ~33 and truncates past that", n)
	}
}
