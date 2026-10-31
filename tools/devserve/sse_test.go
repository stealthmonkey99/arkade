// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"strings"
	"testing"
	"time"
)

func TestFormatSSESingleLine(t *testing.T) {
	got := formatSSE("reload", "")
	want := "event: reload\ndata: \n\n"
	if got != want {
		t.Fatalf("formatSSE(%q, %q) = %q, want %q", "reload", "", got, want)
	}
}

func TestFormatSSEMultiLine(t *testing.T) {
	output := "cartridges/orbit/game.go:42:2: undefined: oops\nBuild error: exit status 1"
	got := formatSSE("error", output)
	wantLines := []string{
		"event: error",
		"data: cartridges/orbit/game.go:42:2: undefined: oops",
		"data: Build error: exit status 1",
		"",
		"",
	}
	if got != strings.Join(wantLines, "\n") {
		t.Fatalf("formatSSE multi-line = %q, want %q", got, strings.Join(wantLines, "\n"))
	}
}

func TestBrokerBroadcastReachesSubscribers(t *testing.T) {
	b := newSSEBroker()
	ch := b.subscribe()
	defer b.unsubscribe(ch)

	b.broadcast("hello")
	select {
	case got := <-ch:
		if got != "hello" {
			t.Fatalf("got %q, want %q", got, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestBrokerUnsubscribeStopsDelivery(t *testing.T) {
	b := newSSEBroker()
	ch := b.subscribe()
	b.unsubscribe(ch)

	b.broadcast("should not arrive anywhere that panics")
	select {
	case v, ok := <-ch:
		if ok {
			t.Fatalf("unsubscribed channel received %q", v)
		}
	default:
	}
}

func TestBrokerBroadcastNeverBlocksOnAFullSlowClient(t *testing.T) {
	b := newSSEBroker()
	slow := b.subscribe() // never drained
	fast := b.subscribe()
	defer b.unsubscribe(slow)
	defer b.unsubscribe(fast)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.broadcast("msg")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast blocked on a full, undrained subscriber channel")
	}
	select {
	case <-fast:
	default:
		t.Fatal("the fast subscriber received nothing at all")
	}
}
