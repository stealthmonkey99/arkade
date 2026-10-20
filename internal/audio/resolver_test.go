// SPDX-License-Identifier: GPL-2.0-or-later

package audio

import (
	"io"
	"path/filepath"
	"testing"
)

func TestAssetOverlayServesRegisteredOwnerFromMemory(t *testing.T) {
	overlay := NewAssetOverlay()
	overlay.Register("cart/snake", map[string][]byte{
		"assets/omnomnom.wav": []byte("fake wav bytes"),
	})

	resolver := overlay.Resolver(nil) // no fallback needed for a registered owner
	rc, err := resolver("cart/snake", "assets/omnomnom.wav")
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "fake wav bytes" {
		t.Errorf("got %q, want %q", got, "fake wav bytes")
	}
}

func TestAssetOverlayMissWithinRegisteredOwnerErrors(t *testing.T) {
	overlay := NewAssetOverlay()
	overlay.Register("cart/snake", map[string][]byte{"assets/omnomnom.wav": []byte("x")})

	fallbackCalled := false
	fallback := func(owner, file string) (io.ReadCloser, error) {
		fallbackCalled = true
		return nil, nil
	}

	if _, err := overlay.Resolver(fallback)("cart/snake", "assets/nope.wav"); err == nil {
		t.Fatal("expected an error for a file not in the registered owner's assets")
	}
	if fallbackCalled {
		t.Error("fallback must not be consulted for a registered owner")
	}
}

func TestAssetOverlayUnregisteredOwnerFallsThrough(t *testing.T) {
	root := t.TempDir()
	writeWav(t, filepath.Join(root, "games/snake/assets/lost.wav"), MixRate, 10, 0.1)

	overlay := NewAssetOverlay()
	overlay.Register("cart/snake", map[string][]byte{"assets/omnomnom.wav": []byte("x")})

	resolver := overlay.Resolver(DirResolver(root))
	rc, err := resolver("games/snake", "./assets/lost.wav")
	if err != nil {
		t.Fatalf("expected fallthrough to DirResolver for an unregistered owner: %v", err)
	}
	rc.Close()
}

func TestAssetOverlayNilSafe(t *testing.T) {
	var overlay *AssetOverlay
	overlay.Register("cart/snake", map[string][]byte{"a": {1}}) // must not panic
	overlay.Unregister("cart/snake")                            // must not panic

	root := t.TempDir()
	writeWav(t, filepath.Join(root, "games/snake/assets/lost.wav"), MixRate, 10, 0.1)
	resolver := overlay.Resolver(DirResolver(root))
	rc, err := resolver("games/snake", "./assets/lost.wav")
	if err != nil {
		t.Fatalf("nil overlay should transparently fall through: %v", err)
	}
	rc.Close()
}

func TestAssetOverlayUnregisterDropsAssets(t *testing.T) {
	overlay := NewAssetOverlay()
	overlay.Register("cart/snake", map[string][]byte{"assets/omnomnom.wav": []byte("x")})
	overlay.Unregister("cart/snake")

	if _, err := overlay.Resolver(nil)("cart/snake", "assets/omnomnom.wav"); err == nil {
		t.Fatal("expected an error: owner was unregistered and no fallback was given")
	}
}
