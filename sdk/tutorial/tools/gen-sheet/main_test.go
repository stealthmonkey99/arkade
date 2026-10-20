// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// TestTheCommittedSheetIsWhatGenSheetWrites: rerunning the tool on unchanged
// PNGs must reproduce the committed sheet's pixels and envelope. The encoded
// bytes are not compared because Go's DEFLATE output differs across releases.
func TestTheCommittedSheetIsWhatGenSheetWrites(t *testing.T) {
	root := filepath.Join("..", "..")
	s, err := buildSheet(root)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join(root, outPNG))
	if err != nil {
		t.Fatal(err)
	}
	if err := assetfile.SamePixels(s.file, committed); err != nil {
		t.Errorf("gen-sheet's pixels differ from the committed %s: %v; run `make sheet` and commit it", outPNG, err)
	}
	want, err := assetfile.ReadBytes(filepath.Base(outPNG), committed)
	if err != nil {
		t.Fatal(err)
	}
	got, err := assetfile.ReadBytes(filepath.Base(outPNG), s.file)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gen-sheet's envelope differs from the committed %s; run `make sheet` and commit it", outPNG)
	}
}
