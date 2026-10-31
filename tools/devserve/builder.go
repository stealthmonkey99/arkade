// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// buildResult is one rebuild attempt. A failed build's Output is the
// compiler text, shown to the author verbatim.
type buildResult struct {
	OK       bool
	Output   string // combined stdout and stderr
	RCart    []byte // only when OK
	Duration time.Duration
}

// Builder runs one rebuild-and-repack cycle; tests fake it so they need no TinyGo.
type Builder interface {
	Build(ctx context.Context) buildResult
}

// makeBuilder runs the cartridge's own Makefile, `make -C <cartDir>
// ARKADE=<engineRoot> build rcart`, so devserve cannot drift from how the
// cartridge builds. A Makefile without those targets fails with make's own error.
// A non-empty command replaces make, for cartridges a collection builds.
type makeBuilder struct {
	cartDir    string // absolute; where make runs and <slug>.rcart lands
	slug       string
	engineRoot string // passed as ARKADE=, overriding the template default
	command    string // run with sh -c in cartDir
}

func (b *makeBuilder) Build(ctx context.Context) buildResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "make", "-C", b.cartDir, "ARKADE="+b.engineRoot, "build", "rcart")
	if b.command != "" {
		cmd = exec.CommandContext(ctx, "sh", "-c", b.command)
	}
	cmd.Dir = b.cartDir
	out, err := cmd.CombinedOutput()
	dur := time.Since(start)
	if err != nil {
		return buildResult{OK: false, Output: string(out), Duration: dur}
	}

	rcartPath := filepath.Join(b.cartDir, b.slug+".rcart")
	data, rerr := os.ReadFile(rcartPath)
	if rerr != nil {
		return buildResult{
			OK: false,
			Output: string(out) + fmt.Sprintf(
				"\ndevserve: make reported success but %s could not be read: %v\n", rcartPath, rerr,
			),
			Duration: dur,
		}
	}
	return buildResult{OK: true, Output: string(out), RCart: data, Duration: dur}
}
