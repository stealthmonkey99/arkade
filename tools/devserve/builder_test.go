// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readTarFile(t *testing.T, data []byte, name string) string {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("tar has no entry %q", name)
		}
		if err != nil {
			t.Fatalf("reading tar: %v", err)
		}
		if hdr.Name != name {
			continue
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, tr); err != nil {
			t.Fatalf("reading %q: %v", name, err)
		}
		return buf.String()
	}
}

type fakeBuilder struct {
	results []buildResult // Build() returns results[0], then [1], ...; the last one repeats once exhausted
	calls   int
}

func (f *fakeBuilder) Build(ctx context.Context) buildResult {
	i := f.calls
	if i >= len(f.results) {
		i = len(f.results) - 1
	}
	f.calls++
	return f.results[i]
}

// gatedBuilder signals started as each build begins and finishes it only on release.
type gatedBuilder struct {
	results []buildResult
	calls   int
	started chan struct{}
	release chan struct{}
}

func newGatedBuilder(results ...buildResult) *gatedBuilder {
	return &gatedBuilder{results: results, started: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedBuilder) Build(ctx context.Context) buildResult {
	res := g.results[min(g.calls, len(g.results)-1)]
	g.calls++
	select {
	case g.started <- struct{}{}:
	case <-ctx.Done():
		return buildResult{Output: "cancelled"}
	}
	select {
	case <-g.release:
	case <-ctx.Done():
		return buildResult{Output: "cancelled"}
	}
	return res
}

func writeStubMake(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub make script is a POSIX shell script")
	}
	bin := t.TempDir()
	path := filepath.Join(bin, "make")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const stubMakePrologue = `[ "$1" = -C ] || { echo "stub make: want -C first, got $1" >&2; exit 99; }
cd "$2" || exit 99
case "$3" in ARKADE=/*) ;; *) echo "stub make: want an absolute ARKADE=, got $3" >&2; exit 99;; esac
[ "$4" = build ] && [ "$5" = rcart ] || { echo "stub make: want build rcart, got $4 $5" >&2; exit 99; }
[ "$#" = 5 ] || { echo "stub make: want exactly 5 args, got $#" >&2; exit 99; }
`

func TestMakeBuilderSuccessReadsTheRcart(t *testing.T) {
	engineRoot := t.TempDir()
	cartDir := filepath.Join(t.TempDir(), "my-cartridges", "widget-game")
	if err := os.MkdirAll(cartDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStubMake(t, stubMakePrologue+`echo "building widget" >&2
mkdir -p stage
printf 'fake rcart bytes' > stage/manifest.json
tar -cf widget.rcart -C stage manifest.json
`)

	b := &makeBuilder{cartDir: cartDir, slug: "widget", engineRoot: engineRoot}
	res := b.Build(context.Background())
	if !res.OK {
		t.Fatalf("Build() failed: %s", res.Output)
	}
	if got := readTarFile(t, res.RCart, "manifest.json"); got != "fake rcart bytes" {
		t.Fatalf("manifest.json = %q, want %q", got, "fake rcart bytes")
	}
	if res.Output == "" {
		t.Fatal("Output is empty; want the build's own stderr/stdout captured")
	}
}

func TestMakeBuilderFailureReportsOutput(t *testing.T) {
	cartDir := t.TempDir()
	writeStubMake(t, stubMakePrologue+`echo "widget/game.go:9:2: undefined: oops" >&2
exit 1
`)

	b := &makeBuilder{cartDir: cartDir, slug: "widget", engineRoot: t.TempDir()}
	res := b.Build(context.Background())
	if res.OK {
		t.Fatal("Build() succeeded, want failure")
	}
	if res.RCart != nil {
		t.Fatalf("RCart = %v on failure, want nil", res.RCart)
	}
	if !strings.Contains(res.Output, "undefined: oops") {
		t.Fatalf("Output = %q, want it to contain the compiler error", res.Output)
	}
}

func TestMakeBuilderRunsTheBuildCommandInsteadOfMake(t *testing.T) {
	cartDir := t.TempDir()
	writeStubMake(t, `echo "stub make should not run" >&2; exit 99`)

	b := &makeBuilder{cartDir: cartDir, slug: "widget", engineRoot: t.TempDir(),
		command: `mkdir -p stage && printf 'collection build' > stage/manifest.json && tar -cf widget.rcart -C stage manifest.json`}
	res := b.Build(context.Background())
	if !res.OK {
		t.Fatalf("Build() failed: %s", res.Output)
	}
	if got := readTarFile(t, res.RCart, "manifest.json"); got != "collection build" {
		t.Fatalf("manifest.json = %q, want %q", got, "collection build")
	}
}

func TestMakeBuilderSuccessButMissingRcartIsAFailure(t *testing.T) {
	cartDir := t.TempDir()
	writeStubMake(t, stubMakePrologue+`exit 0`)

	b := &makeBuilder{cartDir: cartDir, slug: "widget", engineRoot: t.TempDir()}
	res := b.Build(context.Background())
	if res.OK {
		t.Fatal("Build() reported OK with no .rcart on disk")
	}
	if !strings.Contains(res.Output, "widget.rcart") {
		t.Fatalf("Output = %q, want it to mention the missing rcart path", res.Output)
	}
}
