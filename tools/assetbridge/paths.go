// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxWriteBytes = 16 << 20

var writableSuffixes = []string{".png", ".wav", ".arkadeglyph.json", ".arkadelights.json", ".levels", ".arkadelevels"}

var (
	errBadPath  = errors.New("malformed path")
	errOutside  = errors.New("path leaves the cartridge")
	errReadOnly = errors.New("only asset files under assets/ can be written or deleted")
)

// checkRel accepts a slash-separated cartridge-relative path; "" is the
// cartridge itself.
func checkRel(rel string) error {
	if rel == "" {
		return nil
	}
	if strings.ContainsAny(rel, "\\\x00") || strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) {
		return errBadPath
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || strings.HasPrefix(seg, ".") {
			return errBadPath
		}
	}
	return nil
}

// resolve maps rel to a symlink-free path inside the cartridge. A missing
// tail is allowed so writes can create files and parents; a dangling symlink
// on the way is refused, since its target cannot be checked.
func resolve(c cart, rel string) (string, error) {
	if err := checkRel(rel); err != nil {
		return "", err
	}
	p := filepath.Join(c.dir, filepath.FromSlash(rel))
	var missing []string
	for {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			p = filepath.Join(append([]string{real}, missing...)...)
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if _, lerr := os.Lstat(p); lerr == nil {
			return "", errOutside
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", errOutside
		}
		missing = append([]string{filepath.Base(p)}, missing...)
		p = parent
	}
	inner, err := filepath.Rel(c.dir, p)
	if err != nil {
		return "", errOutside
	}
	inner = filepath.ToSlash(inner)
	if inner == "." {
		inner = ""
	}
	if inner == ".." || strings.HasPrefix(inner, "../") || checkRel(inner) != nil {
		return "", errOutside
	}
	return p, nil
}

// resolveWritable is resolve plus the write allowlist, applied to both the
// requested path and where it lands.
func resolveWritable(c cart, rel string) (string, error) {
	p, err := resolve(c, rel)
	if err != nil {
		return "", err
	}
	inner, _ := filepath.Rel(c.dir, p)
	if !writable(rel) || !writable(filepath.ToSlash(inner)) {
		return "", errReadOnly
	}
	return p, nil
}

func writable(rel string) bool {
	if !strings.HasPrefix(rel, "assets/") {
		return false
	}
	lower := strings.ToLower(rel)
	for _, s := range writableSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}
