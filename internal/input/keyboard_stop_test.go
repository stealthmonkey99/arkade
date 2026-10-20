// SPDX-License-Identifier: GPL-2.0-or-later

package input

import "testing"

func TestKeyboardStopIdempotent(t *testing.T) {
	k := NewKeyboard(NewHandler())
	k.Stop()
	k.Stop() // second call must not panic
}

func TestKeyboardEventsDrainInTypedOrder(t *testing.T) {
	h := NewHandler()
	k := NewKeyboard(h)
	k.parse([]byte("ai\x1b[A \r"))
	want := []KeyName{"a", KeyJoyA, KeyUp, KeySpace, KeyEnter}
	got := h.Drain()
	if len(got) != len(want) {
		t.Fatalf("drained %d events, want %d: %v", len(got), len(want), got)
	}
	for i, ev := range got {
		if ev.Name != want[i] {
			t.Errorf("event %d = %q, want %q", i, ev.Name, want[i])
		}
	}
}
