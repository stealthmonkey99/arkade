// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/tools/fleet"
)

type cart struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	dir   string // symlink-free absolute path
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func findCarts(root string) ([]cart, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return nil, err
	}
	if c, ok, err := readCart(root); err != nil || ok {
		if err != nil {
			return nil, err
		}
		return []cart{c}, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var carts []cart
	seen := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		c, ok, err := readCart(filepath.Join(root, e.Name()))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if prev, dup := seen[c.Slug]; dup {
			return nil, fmt.Errorf("slug %q is used by both %s and %s", c.Slug, prev, c.dir)
		}
		seen[c.Slug] = c.dir
		carts = append(carts, c)
	}
	if len(carts) == 0 {
		return nil, fmt.Errorf("%s holds no %s, and neither do its subdirectories", root, fleet.ManifestName)
	}
	sort.Slice(carts, func(i, j int) bool { return carts[i].Slug < carts[j].Slug })
	return carts, nil
}

func readCart(dir string) (cart, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, fleet.ManifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return cart{}, false, nil
	}
	if err != nil {
		return cart{}, false, err
	}
	var m struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return cart{}, false, fmt.Errorf("%s: %w", filepath.Join(dir, fleet.ManifestName), err)
	}
	if !slugPattern.MatchString(m.Slug) {
		return cart{}, false, fmt.Errorf("%s: slug %q is not a valid cartridge slug", filepath.Join(dir, fleet.ManifestName), m.Slug)
	}
	if m.Title == "" {
		m.Title = m.Slug
	}
	return cart{Slug: m.Slug, Title: m.Title, dir: dir}, true, nil
}
