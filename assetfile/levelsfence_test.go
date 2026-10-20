// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

func TestLevelsFenceAgreesWithPackfile(t *testing.T) {
	if LevelsFence != packfile.EnvelopeFence {
		t.Fatalf("fence %q != packfile's %q", LevelsFence, packfile.EnvelopeFence)
	}
	matches, _ := filepath.Glob("testdata/*levels")
	if len(matches) == 0 {
		t.Fatal("no levels fixtures under testdata")
	}
	for _, path := range matches {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		j1, b1, ok1 := splitLevelsEnvelope(src)
		j2, b2, ok2 := packfile.SplitEnvelope(src)
		if ok1 != ok2 || !bytes.Equal(j1, j2) || !bytes.Equal(b1, b2) {
			t.Errorf("%s: split disagrees with packfile (ok %v/%v)", path, ok1, ok2)
		}
		if ok1 && formatLevelsEnvelope(j1) != packfile.FormatEnvelope(j2) {
			t.Errorf("%s: format disagrees with packfile", path)
		}
		if !hasLevelsExt(filepath.Base(path)) || !packfile.HasExt(filepath.Base(path)) {
			t.Errorf("%s: extension rule disagrees", path)
		}
	}
}
