// SPDX-License-Identifier: GPL-2.0-or-later

package packfile_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/levels/packfile"
)

func TestSplitEnvelopeNoFrontMatter(t *testing.T) {
	src := []byte("pack walls\n\nlegend\n  # wall\n")
	js, body, ok := packfile.SplitEnvelope(src)
	if ok {
		t.Fatalf("SplitEnvelope: got ok=true for a file with no front matter")
	}
	if js != nil {
		t.Errorf("SplitEnvelope: json = %q, want nil", js)
	}
	if string(body) != string(src) {
		t.Errorf("SplitEnvelope: body = %q, want the whole input unchanged", body)
	}
}

func TestSplitEnvelopeRoundTrip(t *testing.T) {
	envJSON := []byte(`{
  "arkade": 1,
  "kind": "levels",
  "name": "walls"
}`)
	body := "pack walls\n\nlegend\n  # wall\n"
	block := packfile.FormatEnvelope(envJSON)
	src := []byte(block + body)

	gotJSON, gotBody, ok := packfile.SplitEnvelope(src)
	if !ok {
		t.Fatalf("SplitEnvelope: ok = false for a file this package itself wrote")
	}
	if string(gotJSON) != string(envJSON) {
		t.Errorf("SplitEnvelope: json = %q, want %q", gotJSON, envJSON)
	}
	if string(gotBody) != body {
		t.Errorf("SplitEnvelope: body = %q, want %q", gotBody, body)
	}
}

// Parse ignores the front matter as comment lines, so both forms decode alike.
func TestSplitEnvelopeDoesNotChangeWhatParseSees(t *testing.T) {
	body, err := os.ReadFile("testdata/pushing_solo.levels")
	if err != nil {
		t.Fatal(err)
	}
	envJSON := []byte(`{"arkade":1,"kind":"levels","name":"solo","generator":{"tool":"test","version":"1.0.0"}}`)
	withFront := []byte(packfile.FormatEnvelope(envJSON) + string(body))

	want, err := packfile.Parse("solo.levels", body)
	if err != nil {
		t.Fatalf("parsing the plain file: %v", err)
	}
	got, err := packfile.Parse("solo.levels", withFront)
	if err != nil {
		t.Fatalf("parsing the file with front matter: %v", err)
	}
	if len(got.Levels) != len(want.Levels) {
		t.Fatalf("front matter changed the level count: %d, want %d", len(got.Levels), len(want.Levels))
	}
	for i := range want.Levels {
		wl, gl := &want.Levels[i], &got.Levels[i]
		if wl.Name != gl.Name || wl.W != gl.W || wl.H != gl.H || wl.Cells != gl.Cells {
			t.Errorf("level %d changed shape:\n got %+v\nwant %+v", i, gl, wl)
		}
	}
}

func TestSplitEnvelopeUnterminatedFence(t *testing.T) {
	src := []byte(packfile.EnvelopeFence + "\n// {\"arkade\":1\npack walls\n")
	_, body, ok := packfile.SplitEnvelope(src)
	if ok {
		t.Fatalf("SplitEnvelope: got ok=true for an unterminated fence")
	}
	if string(body) != string(src) {
		t.Errorf("SplitEnvelope: body = %q, want the whole input unchanged", body)
	}
}

func TestFormatEnvelopeIndentedDocument(t *testing.T) {
	js := "{\n  \"arkade\": 1,\n  \"kind\": \"levels\"\n}\n"
	block := packfile.FormatEnvelope([]byte(js))
	want := strings.Join([]string{
		packfile.EnvelopeFence,
		"// {",
		"//   \"arkade\": 1,",
		"//   \"kind\": \"levels\"",
		"// }",
		packfile.EnvelopeFence,
		"",
		"",
	}, "\n")
	if block != want {
		t.Errorf("FormatEnvelope:\n got %q\nwant %q", block, want)
	}
}
