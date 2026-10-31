// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func newTestWatcher(dir string) *watcher {
	w := newWatcher(dir, "testslug")
	w.pollInterval = 15 * time.Millisecond
	w.quietPeriod = 60 * time.Millisecond
	return w
}

func waitForCount(t *testing.T, got *atomic.Int32, want int32, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got.Load() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("trigger count = %d after %s, want >= %d", got.Load(), timeout, want)
}

func TestWatcherTriggersOnFileWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := newTestWatcher(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var triggers atomic.Int32
	go w.run(ctx, func() { triggers.Add(1) })

	time.Sleep(80 * time.Millisecond)
	if n := triggers.Load(); n != 0 {
		t.Fatalf("triggered %d times with no change", n)
	}

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForCount(t, &triggers, 1, 2*time.Second)
}

func TestWatcherCoalescesABurstOfSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")

	w := newTestWatcher(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var triggers atomic.Int32
	go w.run(ctx, func() { triggers.Add(1) })

	for i := 0; i < 5; i++ {
		if err := os.WriteFile(path, []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond) // well under quietPeriod between writes
	}
	waitForCount(t, &triggers, 1, 2*time.Second)

	time.Sleep(150 * time.Millisecond)
	if n := triggers.Load(); n != 1 {
		t.Fatalf("triggered %d times for one coalesced burst, want 1", n)
	}

	if err := os.WriteFile(path, []byte("more"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForCount(t, &triggers, 2, 2*time.Second)
}

func TestWatcherIgnoresOwnBuildOutputs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := newTestWatcher(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var triggers atomic.Int32
	go w.run(ctx, func() { triggers.Add(1) })
	time.Sleep(80 * time.Millisecond) // let it settle on a baseline

	if err := os.WriteFile(filepath.Join(dir, "testslug.wasm"), []byte("wasm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "testslug.rcart"), []byte("rcart"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if n := triggers.Load(); n != 0 {
		t.Fatalf("triggered %d times from the build's own output files, want 0", n)
	}
}
