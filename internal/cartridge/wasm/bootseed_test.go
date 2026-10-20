// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import "testing"

// TestBootRandomGetIsFixedSeed pins the seed TinyGo's map hash reads during
// _start. Golden stability and map-iteration determinism both rest on it.
// The browser host mirrors these bytes. Literals, so a re-derived
// formula cannot make the test tautological.
func TestBootRandomGetIsFixedSeed(t *testing.T) {
	want := []byte{
		0xa5, 0xa4, 0xa7, 0xa6, 0xa1, 0xa0, 0xa3, 0xa2, // the u64 TinyGo reads
		0xad, 0xac, 0xaf, 0xae, 0xa9, 0xa8, 0xab, 0xaa,
	}
	for i, w := range want {
		if got := bootSeedByte(uint32(i)); got != w {
			t.Fatalf("boot random_get byte %d: got %#02x, want %#02x — this seed feeds TinyGo's map hash and golden stability; see this test's doc comment before changing it", i, got, w)
		}
	}
}
