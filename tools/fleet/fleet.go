// SPDX-License-Identifier: GPL-2.0-or-later

// Package fleet locates cartridge directories: <repoRoot>/cartridges by default, or
// the comma-separated roots in ARKADE_CARTRIDGE_ROOTS.
package fleet

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RootsEnv names the cartridge roots, comma-separated. Relative entries
// resolve against the repo root a tool was given.
const RootsEnv = "ARKADE_CARTRIDGE_ROOTS"

// SDKEnv points at the engine's sdk/ directory when it is not
// <repoRoot>/sdk.
const SDKEnv = "ARKADE_SDK_DIR"

// ManifestName is the file that makes a directory a cartridge.
const ManifestName = "manifest.json"

// Roots returns the directories whose subdirectories are cartridges.
func Roots(repoRoot string) []string {
	if env := strings.TrimSpace(os.Getenv(RootsEnv)); env != "" {
		var out []string
		for _, p := range strings.Split(env, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(repoRoot, p)
			}
			out = append(out, p)
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{filepath.Join(repoRoot, "cartridges")}
}

// SDKDir returns the engine's sdk/ directory.
func SDKDir(repoRoot string) string {
	if env := strings.TrimSpace(os.Getenv(SDKEnv)); env != "" {
		if filepath.IsAbs(env) {
			return env
		}
		return filepath.Join(repoRoot, env)
	}
	return filepath.Join(repoRoot, "sdk")
}

// EngineDir returns the engine checkout: the parent of SDKDir when that
// is set, else repoRoot itself.
func EngineDir(repoRoot string) string {
	if strings.TrimSpace(os.Getenv(SDKEnv)) != "" {
		return filepath.Dir(SDKDir(repoRoot))
	}
	return repoRoot
}

// Dirs maps every cartridge slug to its directory across Roots and returns
// the slugs sorted. A root that does not exist contributes nothing; a
// directory without a manifest is not a cartridge.
func Dirs(repoRoot string) (map[string]string, []string, error) {
	dirs := map[string]string{}
	for _, root := range Roots(repoRoot) {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if _, err := os.Stat(filepath.Join(dir, ManifestName)); err != nil {
				continue
			}
			if _, dup := dirs[e.Name()]; !dup {
				dirs[e.Name()] = dir
			}
		}
	}
	slugs := make([]string, 0, len(dirs))
	for s := range dirs {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	return dirs, slugs, nil
}

// Dir returns one cartridge's directory, or "" when no root holds it.
func Dir(repoRoot, slug string) string {
	for _, root := range Roots(repoRoot) {
		dir := filepath.Join(root, slug)
		if _, err := os.Stat(filepath.Join(dir, ManifestName)); err == nil {
			return dir
		}
	}
	return ""
}
