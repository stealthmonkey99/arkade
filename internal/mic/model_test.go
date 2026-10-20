// SPDX-License-Identifier: GPL-2.0-or-later

package mic

import (
	"testing"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// The asset validator pins the model rate so a file that packs also
// mounts; that pin has to be the rate this package actually runs at.
func TestModelRateMatchesAssetPin(t *testing.T) {
	if Rate != assetfile.MicRateHz {
		t.Fatalf("mic.Rate is %d but assetfile.MicRateHz is %d", Rate, assetfile.MicRateHz)
	}
}
