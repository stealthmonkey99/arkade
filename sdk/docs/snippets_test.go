// SPDX-License-Identifier: GPL-2.0-or-later

// Package docs has no Go code of its own: its test is the compile gate for
// the documentation set. Every ```go block in sdk/docs/*.md is a complete
// file starting with a package clause, extracted into a scratch module with
// replace directives onto the real SDK modules and built. A ```go fragment
// block is not checked.
//
// A tutorial listing fenced ```go file=render.go step=2 means "at step 2,
// this is the whole of render.go". Those blocks are replayed in step order
// into an accumulating directory that is built, with its listed tests run, at
// every step boundary, and the last step must match sdk/tutorial byte for byte.
// checkpointSeeds supplies the generated files the reader does not type.
//
// The test also checks that every relative link names a real file and, with
// an anchor into markdown, an existing heading; api_test.go holds the SDK's
// doc comments to their conventions and the generated reference to them.
package docs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/sdk/tools/manifestgen/gen"
)

// referenceCartDir is the checked-in cartridge the tutorial builds.
const referenceCartDir = "../tutorial"

// checkpointSeeds maps each file the reader generates rather than types to
// the step at which the reader runs the tool, so a listing that uses
// generated code too early fails here.
var checkpointSeeds = map[string]int{
	// make build writes it from manifest.json before the first compile
	"manifest_gen.go": 1,
	// tools/gen-levels produces it, at the step whose catcher.go listing
	// first reads a wave from it
	"levels_gen.go": 6,
}

// snippetManifest stands in for the manifest.json every package-main
// example has beside it; its manifest_gen.go is generated the way a
// cartridge build generates it.
const snippetManifest = `{"abi":1,"slug":"snippet","title":"Snippet","version":"0.1.0","players":["1p"]}`

// snippet is one extracted ```go block.
type snippet struct {
	doc    string // source .md filename
	line   int    // 1-based line of the opening fence
	body   string
	isMain bool // package main (needs its own dir either way)

	// file and step are set for checkpoint listings, which are not compiled
	// standalone
	file string
	step int
}

// extractGoSnippets pulls every go-fenced block out of one markdown file,
// stripping the indent of fences inside list items.
func extractGoSnippets(t *testing.T, path string) []snippet {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	var out []snippet
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		// ```go or ```go with attributes; ```go-mod is a different language
		if trimmed != "```go" && !strings.HasPrefix(trimmed, "```go ") {
			continue
		}
		indent := lines[i][:len(lines[i])-len(trimmed)]
		info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```go"))
		open := i
		var body []string
		for i++; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimLeft(lines[i], " "), "```") {
				break
			}
			body = append(body, strings.TrimPrefix(lines[i], indent))
		}
		text := strings.Join(body, "\n")
		// ```go fragment has no package clause and is never compiled
		if slices.Contains(strings.Fields(info), "fragment") {
			continue
		}
		clause := packageClause(text)
		if clause == "" {
			t.Errorf("%s:%d: go snippet must be a standalone file with a package clause (tag a fragment ```go fragment)", filepath.Base(path), open+1)
			continue
		}
		s := snippet{
			doc:    filepath.Base(path),
			line:   open + 1,
			body:   text,
			isMain: clause == "main",
		}
		if err := parseFenceInfo(&s, info); err != nil {
			t.Errorf("%s:%d: %v", s.doc, s.line, err)
			continue
		}
		out = append(out, s)
	}
	return out
}

// packageClause returns the package a snippet declares, or "" if it declares
// none. A leading doc comment is allowed.
func packageClause(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		name, ok := strings.CutPrefix(line, "package ")
		if !ok {
			return ""
		}
		return strings.TrimSpace(name)
	}
	return ""
}

// parseFenceInfo reads the `file=... step=...` words after a ```go fence.
// Unknown words are rejected, since a typo would silently drop the listing
// from the checkpoint build.
//
// `file=` alone only titles the rendered block; a checkpoint listing needs
// `step=` as well, and `step=` without `file=` is an error.
func parseFenceInfo(s *snippet, info string) error {
	for _, word := range strings.Fields(info) {
		key, value, ok := strings.Cut(word, "=")
		if !ok || value == "" {
			return fmt.Errorf("unrecognized go fence attribute %q (want file=<name> and step=<n>)", word)
		}
		switch key {
		case "file":
			if !strings.HasSuffix(value, ".go") {
				return fmt.Errorf("file=%s: a checkpoint listing must be a .go file", value)
			}
			s.file = value
		case "step":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return fmt.Errorf("step=%s: want a positive integer", value)
			}
			s.step = n
		default:
			return fmt.Errorf("unrecognized go fence attribute %q (want file=<name> and step=<n>)", key)
		}
	}
	if s.step != 0 && s.file == "" {
		return fmt.Errorf("a checkpoint listing needs file= as well as step= (got step=%d)", s.step)
	}
	return nil
}

// isCheckpoint reports whether a snippet is a checkpoint listing rather than
// a standalone example.
func (s snippet) isCheckpoint() bool { return s.step != 0 }

func docFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("no markdown files found in sdk/docs (err=%v)", err)
	}
	return files
}

// allSnippets extracts every go block in the doc set and fails if fewer than
// ten are found.
func allSnippets(t *testing.T) []snippet {
	t.Helper()
	var snippets []snippet
	for _, f := range docFiles(t) {
		snippets = append(snippets, extractGoSnippets(t, f)...)
	}
	if len(snippets) < 10 {
		t.Fatalf("only %d go snippets found — extraction is likely broken", len(snippets))
	}
	return snippets
}

// writeScratchModule lays down the go.mod every extracted snippet is
// compiled against: the real cartlib, ui, fsm and levels, plus this repo for
// host-side listings that import carttest, sfxlib, musiclib or
// levels/packfile.
func writeScratchModule(t *testing.T, root string) {
	t.Helper()
	sdkDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	gomod := fmt.Sprintf(`module docsnippets

go 1.23

require github.com/stealthmonkey99/arkade/sdk/cartlib v0.0.0
require github.com/stealthmonkey99/arkade/sdk/ui v0.0.0
require github.com/stealthmonkey99/arkade/sdk/fsm v0.0.0
require github.com/stealthmonkey99/arkade/sdk/levels v0.0.0
require github.com/stealthmonkey99/arkade v0.0.0

replace github.com/stealthmonkey99/arkade/sdk/cartlib => %s
replace github.com/stealthmonkey99/arkade/sdk/ui => %s
replace github.com/stealthmonkey99/arkade/sdk/fsm => %s
replace github.com/stealthmonkey99/arkade/sdk/levels => %s
replace github.com/stealthmonkey99/arkade => %s
`, filepath.Join(sdkDir, "cartlib"), filepath.Join(sdkDir, "ui"), filepath.Join(sdkDir, "fsm"),
		filepath.Join(sdkDir, "levels"), repoRoot)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	// the repo's go.sum covers the dependencies those listings reach
	if sum, err := os.ReadFile(filepath.Join(repoRoot, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(root, "go.sum"), sum, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// buildScratchModule compiles everything laid down under root.
func buildScratchModule(t *testing.T, root, what string) {
	t.Helper()
	runInScratchModule(t, root, what, "build")
}

// runInScratchModule runs a go subcommand over everything under root.
func runInScratchModule(t *testing.T, root, what, verb string) {
	t.Helper()
	cmd := exec.Command("go", verb, "./...")
	cmd.Dir = root
	// -mod=mod lets the scratch module fill in indirect requirements
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		// each file's header comment names the doc and fence in compiler output
		t.Fatalf("go %s: %s failed:\n%s", verb, what, out)
	}
}

// TestSnippetsCompile builds every standalone documented example against
// the real SDK modules. Checkpoint listings are covered by
// TestTutorialCheckpointsCompile.
func TestSnippetsCompile(t *testing.T) {
	root := t.TempDir()
	writeScratchModule(t, root)

	generated, err := gen.Source([]byte(snippetManifest))
	if err != nil {
		t.Fatal(err)
	}

	// One directory per snippet: package main blocks may not share a
	// directory, and non-main blocks may redeclare names across docs.
	n := 0
	for _, s := range allSnippets(t) {
		if s.isCheckpoint() {
			continue // a checkpoint listing; not standalone by design
		}
		dir := filepath.Join(root, fmt.Sprintf("s%03d", n))
		n++
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		header := fmt.Sprintf("// %s:%d\n", s.doc, s.line)
		if err := os.WriteFile(filepath.Join(dir, "snippet.go"), []byte(header+s.body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if s.isMain {
			if err := os.WriteFile(filepath.Join(dir, gen.FileName), generated, 0o644); err != nil {
				t.Fatal(err)
			}
			// a test-only listing sits beside the cartridge's main.go, which it does not repeat
			if !strings.Contains(s.body, "func main()") {
				if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if n < 10 {
		t.Fatalf("only %d standalone snippets found — extraction is likely broken", n)
	}

	buildScratchModule(t, root, "doc snippets")
}

// checkpoints replays the tutorial's listings in step order, returning the
// reader's project after each step.
func checkpoints(t *testing.T, snippets []snippet) (steps []int, states []map[string]string) {
	t.Helper()

	var listings []snippet
	for _, s := range snippets {
		if s.isCheckpoint() {
			listings = append(listings, s)
		}
	}
	if len(listings) == 0 {
		return nil, nil
	}
	// step order first, then document and line, independent of glob order
	sort.SliceStable(listings, func(i, j int) bool {
		a, b := listings[i], listings[j]
		if a.step != b.step {
			return a.step < b.step
		}
		if a.doc != b.doc {
			return a.doc < b.doc
		}
		return a.line < b.line
	})

	seeds := map[string]string{}
	for name := range checkpointSeeds {
		data, err := os.ReadFile(filepath.Join(referenceCartDir, name))
		if err != nil {
			t.Fatalf("checkpoint seed %s: %v", name, err)
		}
		seeds[name] = string(data)
	}

	state := map[string]string{}
	flush := func(step int) {
		for name, from := range checkpointSeeds {
			if step >= from {
				state[name] = seeds[name]
			}
		}
		snapshot := make(map[string]string, len(state))
		for k, v := range state {
			snapshot[k] = v
		}
		steps = append(steps, step)
		states = append(states, snapshot)
	}
	current := listings[0].step
	for _, s := range listings {
		if s.step != current {
			flush(current)
			current = s.step
		}
		state[s.file] = s.body + "\n"
	}
	flush(current)
	return steps, states
}

// TestTutorialCheckpointsCompile checks that a reader who has typed
// everything up to step N has a project that builds and whose listed tests
// pass.
func TestTutorialCheckpointsCompile(t *testing.T) {
	steps, states := checkpoints(t, allSnippets(t))
	if len(steps) == 0 {
		t.Skip("no tutorial checkpoint listings in the doc set")
	}
	if len(steps) < 3 {
		t.Fatalf("only %d checkpoints (%v) — a step-by-step tutorial needs more than that to be worth checking", len(steps), steps)
	}

	root := t.TempDir()
	writeScratchModule(t, root)
	for i, step := range steps {
		dir := filepath.Join(root, fmt.Sprintf("cp%02d", step))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range states[i] {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("checkpoint %d: %d files", step, len(states[i]))
	}
	runInScratchModule(t, root, fmt.Sprintf("tutorial checkpoints %v", steps), "test")
}

// TestTutorialFinalStateMatchesTheReferenceCartridge keeps the tutorial and
// sdk/tutorial in step: the last checkpoint equals every non-test source file
// there, byte for byte.
func TestTutorialFinalStateMatchesTheReferenceCartridge(t *testing.T) {
	steps, states := checkpoints(t, allSnippets(t))
	if len(steps) == 0 {
		t.Skip("no tutorial checkpoint listings in the doc set")
	}
	final := states[len(states)-1]

	for name, body := range final {
		if _, ok := checkpointSeeds[name]; ok {
			continue
		}
		want, err := os.ReadFile(filepath.Join(referenceCartDir, name))
		if err != nil {
			t.Errorf("the tutorial's final state has %s, but %s does not: %v", name, referenceCartDir, err)
			continue
		}
		if string(want) != body {
			t.Errorf("%s: the tutorial's final listing does not match %s.\n"+
				"They are the same file by contract — update the listing (or the cartridge) so they agree.",
				name, filepath.Join(referenceCartDir, name))
		}
	}

	// the cartridge must have no source file the tutorial never lists
	entries, err := os.ReadDir(referenceCartDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := checkpointSeeds[name]; ok {
			continue
		}
		if _, ok := final[name]; !ok {
			t.Errorf("%s exists in %s but no tutorial step ever lists it — the reader would never write it",
				name, referenceCartDir)
		}
	}
}

// slugify mirrors the docs renderer's GitHub-style heading IDs: lowercase,
// letters, digits and hyphens only, spaces to hyphens.
func slugify(heading string) string {
	heading = strings.NewReplacer("`", "", "*", "", "_", "").Replace(heading)
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteByte('-')
		}
	}
	return b.String()
}

var (
	linkPattern    = regexp.MustCompile(`\]\(([^()\s]*)\)`)
	headingPattern = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	anchorPattern  = regexp.MustCompile(`<a id="([^"]+)"></a>`)
)

// proseLines returns a markdown file's lines outside code fences.
func proseLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	inFence := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			out = append(out, line)
		}
	}
	return out
}

// TestDocLinksResolve checks every relative link in sdk/docs and the
// generated api/ pages: same-page anchors, and file links resolved against
// the linking page's directory, with an anchor into a .md target checked
// against that file's headings and <a id> anchors.
func TestDocLinksResolve(t *testing.T) {
	headings := map[string]map[string]bool{}
	slugsOf := func(path string) map[string]bool {
		if set, ok := headings[path]; ok {
			return set
		}
		set := map[string]bool{}
		for _, line := range proseLines(t, path) {
			if m := headingPattern.FindStringSubmatch(line); m != nil {
				set[slugify(m[2])] = true
			}
			for _, m := range anchorPattern.FindAllStringSubmatch(line, -1) {
				set[m[1]] = true
			}
		}
		headings[path] = set
		return set
	}

	api, err := filepath.Glob("api/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range append(docFiles(t), api...) {
		for _, line := range proseLines(t, f) {
			for _, m := range linkPattern.FindAllStringSubmatch(line, -1) {
				link := m[1]
				if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") {
					continue
				}
				target, anchor, _ := strings.Cut(link, "#")
				path := f
				if target != "" {
					path = filepath.Join(filepath.Dir(f), target)
					if _, err := os.Stat(path); err != nil {
						t.Errorf("%s links to %s, which does not exist", f, target)
						continue
					}
				}
				if anchor != "" && strings.HasSuffix(path, ".md") && !slugsOf(path)[anchor] {
					t.Errorf("%s links to %s#%s, but no heading there slugs to %q", f, target, anchor, anchor)
				}
			}
		}
	}
}
