// SPDX-License-Identifier: GPL-2.0-or-later

package flight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreWritesAndReadsBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "flight") // not pre-created: Write must make it
	s := NewStore(dir, 0)
	rec := sampleRecording(50)
	at := time.Date(2026, 9, 5, 14, 13, 12, 0, time.UTC)

	path, err := s.Write(rec, at)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := filepath.Join(dir, "20260905T141312Z-cart_snake-crash"+Extension)
	if path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening the written dump: %v", err)
	}
	defer func() { _ = f.Close() }()
	got, err := Decode(f)
	if err != nil {
		t.Fatalf("Decode of the written dump: %v", err)
	}
	if got.Header.Slug != rec.Header.Slug || len(got.Events) != len(rec.Events) {
		t.Errorf("read back %s with %d events, want %s with %d",
			got.Header.Slug, len(got.Events), rec.Header.Slug, len(rec.Events))
	}
}

func TestStoreRotatesToKeep(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 3)
	base := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)

	var written []string
	for i := 0; i < 8; i++ {
		p, err := s.Write(sampleRecording(2), base.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
		written = append(written, p)
	}

	files, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("kept %d dumps, want 3: %v", len(files), files)
	}
	for i, want := range written[5:] {
		if files[i] != want {
			t.Errorf("kept[%d] = %s, want %s", i, files[i], want)
		}
	}
	for _, gone := range written[:5] {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s should have been rotated out", gone)
		}
	}
}

// Two dumps in one second must keep the order they were taken in, whatever
// their names would sort as: a manual dump an instant before the crash it
// was chasing, and two crashes with the same name.
func TestStoreKeepsSameSecondDumpsInOrder(t *testing.T) {
	at := time.Date(2026, 9, 5, 14, 13, 12, 0, time.UTC)
	for _, reasons := range [][2]string{{"manual", "crash"}, {"crash", "crash"}} {
		dir := t.TempDir()
		s := NewStore(dir, 3)
		var written []string
		for _, reason := range reasons {
			rec := sampleRecording(2)
			rec.Header.Reason = reason
			p, err := s.Write(rec, at)
			if err != nil {
				t.Fatalf("Write %s: %v", reason, err)
			}
			written = append(written, p)
		}
		if written[0] == written[1] {
			t.Fatalf("%v: both dumps wrote %s", reasons, written[0])
		}
		files, err := s.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(files) != 2 || files[0] != written[0] || files[1] != written[1] {
			t.Errorf("%v: List = %v, want %v", reasons, files, written)
		}

		s.keep = 1
		if err := s.rotate(); err != nil {
			t.Fatalf("rotate: %v", err)
		}
		if _, err := os.Stat(written[1]); err != nil {
			t.Errorf("%v: rotation removed the newer dump: %v", reasons, err)
		}
		if _, err := os.Stat(written[0]); !os.IsNotExist(err) {
			t.Errorf("%v: rotation kept the older dump", reasons)
		}
	}
}

func TestStoreLeavesNoPartialFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 0)
	if _, err := s.Write(sampleRecording(10), time.Now()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), Extension) {
			t.Errorf("leftover non-recording file %q in the dump directory", e.Name())
		}
	}
}

func TestListOnAMissingDirectoryIsEmpty(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "never-created"), 0)
	files, err := s.List()
	if err != nil {
		t.Fatalf("List on a missing directory: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("List = %v, want empty", files)
	}
}

func TestFilenameSanitizesTheSlug(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := map[string]string{
		"cart/snake": "20260102T030405Z-cart_snake-manual" + Extension,
		"":           "20260102T030405Z-unknown-manual" + Extension,
		"../../etc":  "20260102T030405Z-______etc-manual" + Extension,
	}
	for slug, want := range cases {
		if got := Filename(at, slug, ""); got != want {
			t.Errorf("Filename(%q) = %q, want %q", slug, got, want)
		}
	}
}
