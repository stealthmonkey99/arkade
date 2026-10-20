// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

import (
	"fmt"
	"syscall"
)

const (
	// InstallHeadroomBytes is the free space an install must leave behind on
	// the library's filesystem, so a cabinet full of cartridges can still
	// persist scores, settings and the journal.
	InstallHeadroomBytes = 32 << 20
	// LowSpaceWarnBytes is the free-space level under which a successful
	// install logs a warning.
	LowSpaceWarnBytes = 256 << 20
)

// InsufficientSpaceError is returned by Install and SetVariant when the
// bundle plus InstallHeadroomBytes would not fit. Its message carries sizes
// only, never paths, so it is safe to show the caller.
type InsufficientSpaceError struct {
	Need, Free uint64
}

func (e *InsufficientSpaceError) Error() string {
	return fmt.Sprintf("not enough free space on the cabinet: this cartridge needs %s plus %s headroom, %s free",
		mib(e.Need), mib(InstallHeadroomBytes), mib(e.Free))
}

// InsufficientStorage marks the error for callers that cannot import this
// package (internal/net maps it to 507).
func (e *InsufficientSpaceError) InsufficientStorage() bool { return true }

func mib(n uint64) string { return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20)) }

// statfsFree deliberately excludes root's reserved blocks.
func statfsFree(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// checkRoom allows a write when the filesystem cannot be measured: the write
// itself still fails if the disk is really full.
func (c *RemoteControl) checkRoom(need int) error {
	free, err := c.freeSpace(c.dir)
	if err != nil {
		c.logf("warn", "cartridge: cannot measure free space for the library: %v", err)
		return nil
	}
	if free < uint64(need)+InstallHeadroomBytes {
		return &InsufficientSpaceError{Need: uint64(need), Free: free}
	}
	return nil
}

func (c *RemoteControl) warnIfLowSpace() {
	free, err := c.freeSpace(c.dir)
	if err == nil && free < LowSpaceWarnBytes {
		c.logf("warn", "cartridge: library filesystem is low on space: %s free (warning threshold %s)", mib(free), mib(LowSpaceWarnBytes))
	}
}

func (c *RemoteControl) logf(level, format string, args ...any) {
	if c.log != nil {
		c.log(level, fmt.Sprintf(format, args...))
	}
}
