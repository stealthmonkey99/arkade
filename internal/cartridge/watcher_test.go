// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge_test

import (
	"testing"
	"time"

	"github.com/stealthmonkey99/arkade/internal/cartridge"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met within %s", timeout)
	}
}

func TestWatcherRegistersOnStartAndPolls(t *testing.T) {
	dir := t.TempDir()
	writeBareWasm(t, dir, "snake")
	cleanupRegistry(t, "cart/snake", "cart/pong")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	w := cartridge.NewWatcher(reg, 20*time.Millisecond)
	w.Start()
	defer w.Stop()

	if _, err := engine.Lookup("cart/snake"); err != nil {
		t.Fatalf("Lookup(cart/snake) right after Start: %v", err)
	}

	writeBareWasm(t, dir, "pong")
	waitFor(t, time.Second, func() bool {
		_, err := engine.Lookup("cart/pong")
		return err == nil
	})
}

func TestWatcherStopEndsPolling(t *testing.T) {
	dir := t.TempDir()
	cleanupRegistry(t, "cart/late")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	w := cartridge.NewWatcher(reg, 10*time.Millisecond)
	w.Start()
	w.Stop()

	writeBareWasm(t, dir, "late")
	time.Sleep(100 * time.Millisecond)
	if _, err := engine.Lookup("cart/late"); err == nil {
		t.Fatal("Lookup(cart/late) succeeded after Stop; the watcher kept polling")
	}
}

func TestWatcherStartStopAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	w := cartridge.NewWatcher(reg, 10*time.Millisecond)

	w.Stop()

	w.Start()
	w.Start()
	w.Stop()
	w.Stop()
}

func TestWatcherSeedsBeforeStartReturns(t *testing.T) {
	dir := t.TempDir()
	writeBareWasm(t, dir, "snake")
	cleanupRegistry(t, "cart/snake")

	reg := cartridge.NewRegistry(cartridge.Options{Dirs: []string{dir}})
	w := cartridge.NewWatcher(reg, time.Hour) // only the synchronous seed scan can satisfy this
	w.Start()
	defer w.Stop()

	if _, err := engine.Lookup("cart/snake"); err != nil {
		t.Fatalf("Lookup(cart/snake) immediately after Start: %v", err)
	}
}
