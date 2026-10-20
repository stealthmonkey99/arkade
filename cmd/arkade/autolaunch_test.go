// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func TestRequestLaunchHonoredOnlyFromShellMenu(t *testing.T) {
	rig := testkit.NewRig(t, testkit.Options{})
	rig.Tick(80) // boot to shell
	if got := rig.RT.CurrentSlug(); got != "shell" {
		t.Fatalf("expected shell after boot, got %q", got)
	}

	rig.RT.RequestLaunch("apps/clock")
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("expected auto-launch from the shell menu, got %q", got)
	}

	// Dropped, not deferred; the foreground check fails before the slug is resolved.
	rig.RT.RequestLaunch("apps/config")
	rig.Tick(3)
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("a mid-app request must be ignored, got %q", got)
	}
	rig.Tick(3) // and it must not fire later either
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("dropped request resurfaced later: %q", got)
	}

	rig.RT.RequestLaunch("cart/does-not-exist")
	rig.Tick(2)
	if got := rig.RT.CurrentSlug(); got != "apps/clock" {
		t.Fatalf("unknown slug must be a no-op, got %q", got)
	}
}
