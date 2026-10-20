// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge_test

import (
	"testing"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/engine"
)

func TestMicCapabilityMatchesEngine(t *testing.T) {
	if rcart.CapabilityMic != engine.CapabilityMic {
		t.Fatalf("rcart.CapabilityMic = %q, engine.CapabilityMic = %q", rcart.CapabilityMic, engine.CapabilityMic)
	}
}
