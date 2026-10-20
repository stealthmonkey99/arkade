// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

// Compared through the parser, not byte for byte, because Format normalises
// whitespace.
func TestFormatPreservesHandWrittenPacks(t *testing.T) {
	for _, path := range []string{
		"testdata/pushing_solo.levels",    // canonical already
		"testdata/packing_puzzle.levels",  // hand-aligned metadata columns
		"../example/example.arkadelevels", // hand-aligned metadata columns
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		want, err := packfile.Parse(path, src)
		if err != nil {
			t.Fatalf("%s does not parse in this checkout: %v", path, err)
		}
		out, err := packfile.Format(path, src)
		if err != nil {
			t.Fatalf("%s: Format: %v", path, err)
		}
		got, err := packfile.Parse(path, out)
		if err != nil {
			t.Fatalf("%s: Format's output does not parse: %v\n%s", path, err, out)
		}
		if len(got.Levels) != len(want.Levels) {
			t.Fatalf("%s: Format changed the level count: %d, want %d", path, len(got.Levels), len(want.Levels))
		}
		for i := range want.Levels {
			wl, gl := &want.Levels[i], &got.Levels[i]
			if wl.Name != gl.Name || wl.W != gl.W || wl.H != gl.H || wl.Cells != gl.Cells {
				t.Errorf("%s: level %d changed shape:\n got %+v\nwant %+v", path, i, gl, wl)
			}
		}
	}
}

// packing_puzzle.levels is excluded because it hand-aligns its metadata.
func TestFormatRoundTripsCanonicalPacks(t *testing.T) {
	for _, path := range []string{
		"testdata/pushing_solo.levels",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out, err := packfile.Format(path, src)
		if err != nil {
			t.Fatalf("%s: Format: %v", path, err)
		}
		if string(out) != string(src) {
			t.Errorf("%s: round trip is not byte-identical\n--- got\n%s\n--- want\n%s", path, out, src)
		}
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	for _, path := range []string{
		"testdata/pushing_solo.levels",
		"testdata/packing_puzzle.levels",
		"../example/example.arkadelevels",
	} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		once, err := packfile.Format(path, src)
		if err != nil {
			t.Fatalf("%s: Format: %v", path, err)
		}
		twice, err := packfile.Format(path, once)
		if err != nil {
			t.Fatalf("%s: Format of Format's own output: %v", path, err)
		}
		if string(twice) != string(once) {
			t.Errorf("%s: Format is not idempotent\n--- first\n%s\n--- second\n%s", path, once, twice)
		}
	}
}

// The example pack is hand-aligned (`par  6`, two spaces).
func TestFormatCanonicalisesHandAlignedMetadata(t *testing.T) {
	src, err := os.ReadFile("../example/example.arkadelevels")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "par  6") {
		t.Fatalf("fixture assumption broken: sdk/levels/example/example.arkadelevels no longer hand-aligns `par  6`")
	}
	out, err := packfile.Format("example.levels", src)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if strings.Contains(string(out), "par  6") {
		t.Errorf("Format left the hand-aligned spacing in place:\n%s", out)
	}
	if !strings.Contains(string(out), "par 6") {
		t.Errorf("Format did not canonicalise to a single space:\n%s", out)
	}
}

func TestFormatPreservesCommentsInEveryPosition(t *testing.T) {
	src := `// header, two lines
// of it
pack demo

// about the legend
legend
  // about this one cell
  #  wall
  \s floor
  .  none
  ~  hazard

// about the level
level L01
  par 3
  |###|
  |# #|
  |###|
  // about the layer
  layer hazard
  |...|
  |.~.|
  |...|

// nothing follows me
`
	out, err := packfile.Format("demo.levels", []byte(src))
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	for _, want := range []string{
		"// header, two lines", "// of it", "// about the legend",
		"// about this one cell", "// about the level", "// about the layer",
		"// nothing follows me",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("Format dropped %q\n%s", want, out)
		}
	}
	if _, err := packfile.Parse("demo.levels", out); err != nil {
		t.Errorf("the formatted pack does not parse: %v", err)
	}
	// a comment must not drift down the file on every pass
	again, err := packfile.Format("demo.levels", out)
	if err != nil {
		t.Fatalf("Format of Format's own output: %v", err)
	}
	if string(again) != string(out) {
		t.Errorf("Format is not idempotent on a commented pack\n--- first\n%s\n--- second\n%s", out, again)
	}
}

func TestFormatRefusesAnUnparseablePack(t *testing.T) {
	src := `pack demo
legend
  #  wall
  \s floor
level L01
  |###|
  |# #|
  |####|
`
	_, err := packfile.Format("demo.levels", []byte(src))
	if err == nil {
		t.Fatal("Format accepted a ragged grid")
	}
	if !strings.Contains(err.Error(), "wide") {
		t.Errorf("error = %v, want the parser's own width complaint", err)
	}
}
