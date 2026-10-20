// SPDX-License-Identifier: GPL-2.0-or-later

package gen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// APIDir is the generated reference's directory, relative to sdk/docs.
const APIDir = "api"

// Generate renders every generated file from the SDK at sdkRoot (the sdk/
// directory). It returns their contents keyed by slash-separated path
// relative to sdk/docs: each [Packages] page under [APIDir], and [ABIPage]
// with its generated sections replaced. Output depends on the sources
// only, so two runs over one tree are byte-identical.
func Generate(sdkRoot string) (map[string][]byte, error) {
	hostViews := make([]*loaded, len(Packages))
	ix := symbolIndex{}
	for i, p := range Packages {
		l, err := load(sdkRoot, p, hostContext)
		if err != nil {
			return nil, err
		}
		if err := ix.add(l); err != nil {
			return nil, err
		}
		hostViews[i] = l
	}

	out := map[string][]byte{}
	for _, l := range hostViews {
		t, err := load(sdkRoot, l.Package, tinygoContext)
		if err != nil {
			return nil, err
		}
		host, tiny := exportedSurface(l.doc), exportedSurface(t.doc)
		w := &pageWriter{l: l, ix: ix, parser: l.parser(ix)}
		page := w.page(onlyIn(host, tiny), onlyIn(tiny, host))
		if bad := tagLikeText(page); len(bad) > 0 {
			return nil, fmt.Errorf("%s: the docs site drops text shaped like an HTML tag; put it in backticks in the doc comment: %s", l.importPath, strings.Join(bad, "; "))
		}
		out[APIDir+"/"+l.Page] = append(page, '\n')
	}

	exports, imports, err := abiTables(sdkRoot, hostViews[0], ix)
	if err != nil {
		return nil, err
	}
	abiPath := filepath.Join(sdkRoot, "docs", ABIPage)
	raw, err := os.ReadFile(abiPath)
	if err != nil {
		return nil, err
	}
	abi, err := spliceABI(raw, map[string]string{"exports": exports, "imports": imports})
	if err != nil {
		return nil, err
	}
	out[ABIPage] = abi
	return out, nil
}

// Write regenerates the reference in sdkRoot/docs, deleting any page under
// [APIDir] that [Generate] no longer produces.
func Write(sdkRoot string) error {
	files, err := Generate(sdkRoot)
	if err != nil {
		return err
	}
	docs := filepath.Join(sdkRoot, "docs")
	if err := os.MkdirAll(filepath.Join(docs, APIDir), 0o755); err != nil {
		return err
	}
	for _, stale := range strays(docs, files) {
		if err := os.Remove(filepath.Join(docs, stale)); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(files) {
		if err := os.WriteFile(filepath.Join(docs, filepath.FromSlash(name)), files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Check reports, without writing, every generated file under sdkRoot/docs
// that is missing, stale or no longer generated. A nil error means the
// committed reference matches the sources.
func Check(sdkRoot string) error {
	files, err := Generate(sdkRoot)
	if err != nil {
		return err
	}
	docs := filepath.Join(sdkRoot, "docs")
	var problems []string
	for _, name := range sortedKeys(files) {
		have, err := os.ReadFile(filepath.Join(docs, filepath.FromSlash(name)))
		switch {
		case os.IsNotExist(err):
			problems = append(problems, name+" is missing")
		case err != nil:
			return err
		case !bytes.Equal(have, files[name]):
			problems = append(problems, name+" is stale")
		}
	}
	for _, stale := range strays(docs, files) {
		problems = append(problems, stale+" is no longer generated")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s; run `make sdk-docs`", strings.Join(problems, ", "))
	}
	return nil
}

// strays lists the .md files under docs/api that files does not contain.
func strays(docs string, files map[string][]byte) []string {
	entries, _ := os.ReadDir(filepath.Join(docs, APIDir))
	var out []string
	for _, e := range entries {
		name := APIDir + "/" + e.Name()
		if strings.HasSuffix(name, ".md") && files[name] == nil {
			out = append(out, name)
		}
	}
	return out
}

var (
	codeSpan = regexp.MustCompile("`[^`]*`")
	tagLike  = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
)

// tagLikeText lists the prose outside code that the site's renderer would
// strip as HTML, such as a bare <name> placeholder.
func tagLikeText(page []byte) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(string(page), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		out = append(out, tagLike.FindAllString(codeSpan.ReplaceAllString(line, ""), -1)...)
	}
	return out
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
