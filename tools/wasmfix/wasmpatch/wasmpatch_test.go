// SPDX-License-Identifier: GPL-2.0-or-later

package wasmpatch

import (
	"bytes"
	"testing"
)

func TestParsedSlicesCannotGrowIntoTheModule(t *testing.T) {
	// One () -> (i32, i32) type entry, then a byte belonging to the next section.
	module := []byte{0x01, 0x60, 0x00, 0x02, 0x7F, 0x7F, 0xEE}
	want := bytes.Clone(module)

	_, raw, err := parseVectorRaw(module[:6], parseFuncType)
	if err != nil {
		t.Fatal(err)
	}
	_ = append(raw, 0x00)

	r := &reader{buf: module}
	b, err := r.bytes(3)
	if err != nil {
		t.Fatal(err)
	}
	_ = append(b, 0x00)

	if !bytes.Equal(module, want) {
		t.Fatalf("appending to a parsed slice modified the module: % x", module)
	}
}
