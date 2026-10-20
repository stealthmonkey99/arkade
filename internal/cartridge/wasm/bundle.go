// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import "github.com/stealthmonkey99/arkade/assetfile"

// bundleReader is what every mount-time asset loader needs: the files, and
// somewhere to warn about the ones it skips.
type bundleReader interface {
	Bundle
	Logger
}

// sessionAsset loads before cart_init and drops at teardown, even when
// cart_teardown fails.
type sessionAsset struct {
	load func()
	drop func()
}

// readBundleEnvelope warns and reports ok=false on any failure: at mount a
// malformed asset is skipped so the game still starts.
func readBundleEnvelope(host bundleReader, label, path string, kind assetfile.Kind) (*assetfile.Envelope, bool) {
	data, ok := host.BundleAsset(path)
	if !ok {
		hostWarnf(host, "%s %q: no such asset in this cartridge's bundle", label, path)
		return nil, false
	}
	if k, ok := assetfile.KindFromFilename(path); !ok || k != kind {
		hostWarnf(host, "%s %q: not a <name>.arkade%s.json file (docs/ASSETS.md's naming rule)", label, path, kind)
		return nil, false
	}
	env, err := assetfile.ReadBytes(path, data)
	if err != nil {
		hostWarnf(host, "%s %q: %v", label, path, err)
		return nil, false
	}
	return env, true
}
