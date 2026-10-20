// SPDX-License-Identifier: GPL-2.0-or-later

package engine

import (
	"sync"
	"testing"
	"time"
)

func TestLatestKeepsOnlyTheNewestUntilTaken(t *testing.T) {
	var l latest[string]
	if _, ok := l.Take(); ok {
		t.Fatal("an empty slot reported a request")
	}
	l.Set("first")
	l.Set("second")
	if v, ok := l.Take(); !ok || v != "second" {
		t.Fatalf("Take = %q, %v; want the newest request", v, ok)
	}
	if _, ok := l.Take(); ok {
		t.Error("a taken request was delivered twice")
	}
}

func TestLatestIsSafeFromManyGoroutines(t *testing.T) {
	var l latest[int]
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Set(i)
		}()
	}
	wg.Wait()
	if _, ok := l.Take(); !ok {
		t.Error("no request survived concurrent Sets")
	}
}

// A request that is not anyState is consumed and dropped away from the shell
// menu, not held until the menu comes back.
func TestShellOnlyRequestIsDroppedElsewhere(t *testing.T) {
	rt := &Runtime{standby: newStandby(0, 0, 0, time.Time{})}
	applied := 0
	r := request[string]{apply: func(*Runtime, time.Time, string) { applied++ }}
	r.Set("games/x")
	r.drain(rt, time.Time{})
	if applied != 0 {
		t.Fatal("a shell-only request applied with no shell loaded")
	}
	rt.curCtx = &AppContext{Manifest: Manifest{Type: TypeShell}}
	r.drain(rt, time.Time{})
	if applied != 0 {
		t.Error("a dropped request came back when the shell did")
	}
}
