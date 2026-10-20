// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

// ErrUnregistered is wrapped and returned by RemoteControl.Launch when slug
// has no registered engine.Manifest.
var ErrUnregistered = errors.New("cartridge: slug not registered")

// safeNameRE bounds any name this package turns into a path component.
// Deliberately stricter than rcart.Load.
var safeNameRE = regexp.MustCompile(`^[a-z0-9_-]+$`)

// RemoteControl implements internal/net.CartridgeControl by driving the same
// Registry and engine.Runtime a physical USB insert does.
type RemoteControl struct {
	reg *Registry
	rt  *engine.Runtime
	dir string // watched directory Install writes into; must be one reg.Scan covers

	variantsDir string // empty or absent disables the build-variant toggle
	mu          sync.Mutex
	hashes      map[string]hashEntry

	freeSpace func(path string) (uint64, error)
	log       func(level, msg string)
}

// NewRemoteControl builds a RemoteControl that installs into dir, which reg
// must scan. variantsDir is the build-variant store, which reg must not scan;
// "" disables the toggle.
func NewRemoteControl(reg *Registry, rt *engine.Runtime, dir, variantsDir string) (*RemoteControl, error) {
	if !reg.scans(dir) {
		return nil, fmt.Errorf("cartridge: install dir %s is not one the registry scans", dir)
	}
	if variantsDir != "" && reg.scans(variantsDir) {
		return nil, fmt.Errorf("cartridge: variants dir %s must not be one the registry scans", variantsDir)
	}
	return &RemoteControl{reg: reg, rt: rt, dir: dir, variantsDir: variantsDir, freeSpace: statfsFree}, nil
}

// SetLogger routes the low-space warning and other diagnostics to log.
// Without one they are dropped.
func (c *RemoteControl) SetLogger(log func(level, msg string)) { c.log = log }

// Install validates bundle with rcart.Load, writes it into the library as
// "<manifest-slug>.rcart" and rescans immediately. A newly discovered file is
// passed to rt.RequestLaunch, so a network install behaves exactly like
// inserting a physical cartridge, launching only from the shell menu.
//
// Install refuses a bundle that would leave less than InstallHeadroomBytes
// free (InsufficientSpaceError) and warns below LowSpaceWarnBytes. filename
// is used only for error context; the written name always comes from the
// manifest slug.
func (c *RemoteControl) Install(filename string, bundle []byte) (slug, name string, err error) {
	b, err := rcart.Load(bytes.NewReader(bundle))
	if err != nil {
		if filename != "" {
			return "", "", fmt.Errorf("%s: %w", filename, err)
		}
		return "", "", err
	}
	if !safeNameRE.MatchString(b.Manifest.Slug) {
		return "", "", fmt.Errorf("manifest slug %q is not safe as a filename (must match %s)", b.Manifest.Slug, safeNameRE.String())
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return "", "", fmt.Errorf("preparing cartridge library dir %s: %w", c.dir, err)
	}
	if err := c.checkRoom(len(bundle)); err != nil {
		return "", "", err
	}
	dest := filepath.Join(c.dir, b.Manifest.Slug+".rcart")
	if err := writeFileAtomic(dest, bundle); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", dest, err)
	}
	c.warnIfLowSpace()

	prefixed := prefixSlug(b.Manifest.Slug)
	res := c.reg.Scan()
	for _, d := range res.Discovered {
		if d == prefixed {
			c.rt.RequestLaunch(d)
			break
		}
	}

	return prefixed, b.Manifest.DisplayTitle(), nil
}

// Launch switches the engine to slug, a registered app, from any state.
// Unlike a USB insert it uses RequestLaunchNow: the caller is the
// authenticated dashboard, so whatever is on screen, even a game in play or
// one parked behind the screensaver, is torn down.
func (c *RemoteControl) Launch(slug string) error {
	if _, err := engine.Lookup(slug); err != nil {
		return fmt.Errorf("%w: %s", ErrUnregistered, slug)
	}
	c.rt.RequestLaunchNow(slug)
	return nil
}

// writeFileAtomic renames a temp file into place so a concurrent Scan never
// sees a torn bundle.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
