// SPDX-License-Identifier: GPL-2.0-or-later

package docs

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
)

// The Quick Start Guide shows the template's files and the toolchain the
// engine pins. These tests hold each to its source, so a change to one that
// forgets the other fails here instead of in a newcomer's terminal.

const (
	quickStart  = "getting-started.md"
	templateDir = "../template"
	ciWorkflow  = "../../.github/workflows/ci.yml"
)

type fence struct {
	info string // everything after the backticks, e.g. "go-mod file=go.mod"
	body string
	line int
}

func fences(t *testing.T, path string) []fence {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	var out []fence
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if !strings.HasPrefix(trimmed, "```") {
			continue
		}
		indent := lines[i][:len(lines[i])-len(trimmed)]
		f := fence{info: strings.TrimSpace(strings.TrimPrefix(trimmed, "```")), line: i + 1}
		var body []string
		for i++; i < len(lines) && !strings.HasPrefix(strings.TrimLeft(lines[i], " "), "```"); i++ {
			body = append(body, strings.TrimPrefix(lines[i], indent))
		}
		f.body = strings.Join(body, "\n") + "\n"
		out = append(out, f)
	}
	return out
}

// quickStartFence returns the one block in the Quick Start Guide whose info
// string starts with prefix.
func quickStartFence(t *testing.T, prefix string) fence {
	t.Helper()
	var found []fence
	for _, f := range fences(t, quickStart) {
		if f.info == prefix || strings.HasPrefix(f.info, prefix+" ") {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: want exactly one ```%s block, found %d", quickStart, prefix, len(found))
	}
	return found[0]
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var goDirective = regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)\s*$`)

func goVersionOf(t *testing.T, gomod string) string {
	t.Helper()
	m := goDirective.FindStringSubmatch(readFile(t, gomod))
	if m == nil {
		t.Fatalf("%s has no go directive", gomod)
	}
	return m[1]
}

var requireLine = regexp.MustCompile(`(?m)^\s*(?:require\s+)?(github\.com/stealthmonkey99/arkade(?:/sdk/\w+)?) (v\d+\.\d+\.\d+)\b`)

// requiresOf maps each arkade module a go.mod requires to its version.
func requiresOf(t *testing.T, gomod string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range requireLine.FindAllStringSubmatch(readFile(t, gomod), -1) {
		out[m[1]] = m[2]
	}
	return out
}

func TestQuickStartGoModIsTheTemplates(t *testing.T) {
	template := readFile(t, filepath.Join(templateDir, "go.mod"))
	if strings.Contains(template, "replace ") {
		t.Fatalf("sdk/template/go.mod has a replace directive, so a copy outside the engine checkout will not build.\n" +
			"Require the published tags there; in-repo resolution belongs in sdk/go.work.")
	}

	shown := quickStartFence(t, "go-mod file=go.mod")
	module := regexp.MustCompile(`(?m)^module .*$`)
	docModule := module.FindString(shown.body)
	if docModule == "" {
		t.Fatalf("%s:%d: the go.mod block has no module line", quickStart, shown.line)
	}
	want := module.ReplaceAllLiteralString(template, docModule)
	if shown.body != want {
		t.Errorf("%s:%d: the go.mod block no longer matches sdk/template/go.mod (module line aside).\n"+
			"Paste the template's file into the block, keeping only the module renamed.\n--- shown\n%s--- want\n%s",
			quickStart, shown.line, shown.body, want)
	}
}

func TestTemplateGoDirectiveIsTheSDKFloor(t *testing.T) {
	floor := ""
	for _, mod := range []string{"cartlib", "ui", "fsm", "levels"} {
		v := goVersionOf(t, filepath.Join("..", mod, "go.mod"))
		if floor == "" {
			floor = v
		} else if v != floor {
			t.Fatalf("sdk/%s/go.mod says go %s but the other SDK modules say go %s; the SDK has one floor", mod, v, floor)
		}
	}
	if got := goVersionOf(t, filepath.Join(templateDir, "go.mod")); got != floor {
		t.Errorf("sdk/template/go.mod says go %s, the SDK modules' floor is go %s.\n"+
			"Keep the template at the floor: raising it would demand a newer Go of every author for nothing.", got, floor)
	}
	if !strings.Contains(readFile(t, quickStart), "`go "+floor+"`") {
		t.Errorf("%s's prerequisites should name the go.mod floor as `go %s`", quickStart, floor)
	}
}

// ciPin reads a value from the engine's CI workflow, which is what the
// pinned toolchain means.
func ciPin(t *testing.T, pattern string) string {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(readFile(t, ciWorkflow))
	if m == nil {
		t.Fatalf("%s: no match for %s — if the pin moved, point this test at its new home", ciWorkflow, pattern)
	}
	return m[1]
}

func TestDocsNameThePinnedToolchain(t *testing.T) {
	tinygo := ciPin(t, `TINYGO_VERSION:\s*"([\d.]+)"`)
	goMinor := ciPin(t, `go-version:\s*"(\d+\.\d+)\.x"`)

	mention := regexp.MustCompile(`TinyGo (\d+\.\d+\.\d+)|tinygo/releases/tag/v(\d+\.\d+\.\d+)`)
	for _, doc := range []string{quickStart, "../../README.md", "../README.md", filepath.Join(templateDir, "README.md")} {
		n := 0
		for _, m := range mention.FindAllStringSubmatch(readFile(t, doc), -1) {
			n++
			if v := m[1] + m[2]; v != tinygo {
				t.Errorf("%s names TinyGo %s, but %s pins %s; update the doc (or the pin) so they agree", doc, v, ciWorkflow, tinygo)
			}
		}
		if n == 0 && (doc == quickStart || doc == "../../README.md") {
			t.Errorf("%s no longer names a TinyGo version; it should say %s, the one %s pins", doc, tinygo, ciWorkflow)
		}
	}

	if !strings.Contains(readFile(t, quickStart), "**Go "+goMinor+"**") {
		t.Errorf("%s should tell readers to install **Go %s**, the Go %s builds cartridges with", quickStart, goMinor, ciWorkflow)
	}
}

func TestDocsShowTheTemplatesSDKVersions(t *testing.T) {
	want := requiresOf(t, filepath.Join(templateDir, "go.mod"))
	for mod, v := range requiresOf(t, "../tutorial/go.mod") {
		if w, ok := want[mod]; ok && w != v {
			t.Errorf("sdk/tutorial/go.mod requires %s %s but sdk/template/go.mod requires %s; bump both together", mod, v, w)
		}
		want[mod] = v
	}

	docs, err := filepath.Glob("*.md")
	if err != nil {
		t.Fatal(err)
	}
	more, err := filepath.Glob("../*/README.md")
	if err != nil {
		t.Fatal(err)
	}
	docs = append(docs, more...)
	docs = append(docs, "../README.md", "../../README.md", "../../docs/GAME_AUTHORING_AI.md")

	mention := regexp.MustCompile(`(github\.com/stealthmonkey99/arkade(?:/sdk/\w+)?)(?:@| +)(v\d+\.\d+\.\d+)\b`)
	for _, doc := range docs {
		for _, m := range mention.FindAllStringSubmatch(readFile(t, doc), -1) {
			w, ok := want[m[1]]
			if !ok {
				t.Errorf("%s shows %s %s, which neither sdk/template nor sdk/tutorial requires", doc, m[1], m[2])
				continue
			}
			if m[2] != w {
				t.Errorf("%s shows %s %s, but the template and tutorial require %s; update the doc", doc, m[1], m[2], w)
			}
		}
	}
}

// topLevelKeys returns a JSON object's keys in source order.
func topLevelKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestQuickStartManifestIsTheTemplates(t *testing.T) {
	yours := map[string]bool{"slug": true, "title": true, "author": true, "description": true}

	shown := quickStartFence(t, "json file=manifest.json")
	template := []byte(readFile(t, filepath.Join(templateDir, "manifest.json")))

	if got, want := topLevelKeys(t, []byte(shown.body)), topLevelKeys(t, template); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s:%d: the manifest block's fields %v differ from sdk/template/manifest.json's %v; paste the template's file in", quickStart, shown.line, got, want)
	}
	var doc, tmpl map[string]json.RawMessage
	if err := json.Unmarshal([]byte(shown.body), &doc); err != nil {
		t.Fatalf("%s:%d: %v", quickStart, shown.line, err)
	}
	if err := json.Unmarshal(template, &tmpl); err != nil {
		t.Fatal(err)
	}
	for k, v := range tmpl {
		if !yours[k] && string(doc[k]) != string(v) {
			t.Errorf("%s:%d: manifest field %q is %s in the guide but %s in sdk/template/manifest.json; only %v are the reader's to change",
				quickStart, shown.line, k, doc[k], v, []string{"slug", "title", "author", "description"})
		}
	}

	var m rcart.Manifest
	if err := json.Unmarshal([]byte(shown.body), &m); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("%s:%d: the guide's manifest would be rejected by rcartpack: %v", quickStart, shown.line, err)
	}
	if m.Slug != "my-game" {
		t.Errorf("%s:%d: the manifest's slug is %q, but the guide's directory and commands use my-game", quickStart, shown.line, m.Slug)
	}
}

func TestQuickStartMakefileLineIsTheTemplates(t *testing.T) {
	shown := quickStartFence(t, "make file=Makefile")
	start := 0
	for _, word := range strings.Fields(shown.info) {
		if v, ok := strings.CutPrefix(word, "start="); ok {
			start, _ = strconv.Atoi(v)
		}
	}
	if start < 1 {
		t.Fatalf("%s:%d: the Makefile block needs start=<line> so the guide numbers it as the template does", quickStart, shown.line)
	}
	lines := strings.Split(readFile(t, filepath.Join(templateDir, "Makefile")), "\n")
	if start > len(lines) || !strings.HasPrefix(lines[start-1], "ARKADE ?= ") {
		t.Fatalf("%s:%d: sdk/template/Makefile line %d is no longer the ARKADE line; fix start= in the block", quickStart, shown.line, start)
	}
	if shown.body != "ARKADE ?= ../../arkade\n" {
		t.Errorf("%s:%d: the Makefile block should be the one edited line, ARKADE ?= ../../arkade, matching the guide's layout; got %q", quickStart, shown.line, shown.body)
	}
	shipped := strings.TrimPrefix(lines[start-1], "ARKADE ?= ")
	if !strings.Contains(readFile(t, quickStart), "`"+shipped+"`") {
		t.Errorf("%s should say the template ships with `%s`, its ARKADE value", quickStart, shipped)
	}
}
