// SPDX-License-Identifier: GPL-2.0-or-later

package cartridge

import (
	"os"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/cartridge/rcart"
	"github.com/stealthmonkey99/arkade/internal/cartridge/wasm"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/testkit"
)

func mustAttract(t *testing.T, parent *engine.AppContext, slug string, wasmBytes []byte) *attractRenderer {
	t.Helper()
	r, ok := newAttractRenderer(parent, slug, wasmBytes, 64, 64, nil, rcart.AssetRefs{})
	if !ok {
		t.Fatalf("newAttractRenderer(%s) failed", slug)
	}
	t.Cleanup(r.Close)
	return r.(*attractRenderer)
}

func TestAttractScoreboardIsReadOnly(t *testing.T) {
	wasmBytes, err := os.ReadFile("wasm/testdata/e2e.wasm")
	if err != nil {
		t.Fatalf("read e2e fixture cartridge: %v", err)
	}
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	parent := &engine.AppContext{Manifest: engine.Manifest{Slug: "attract", Name: "Attract"}, RT: rig.RT}
	attract := mustAttract(t, parent, "cart/alpha", wasmBytes)

	if rank := attract.host.ScoreSubmit(500, "ATT"); rank != 0 {
		t.Fatalf("attract ScoreSubmit ranked %d, want 0", rank)
	}
	attract.host.NoteRecentName(0, "ATT")
	if _, ok := rig.RT.RecentName(0); ok {
		t.Fatal("an attract instance noted a recent name")
	}

	playCtx := &engine.AppContext{Manifest: engine.Manifest{Slug: "cart/alpha", Name: "Alpha"}, RT: rig.RT}
	play := wasm.NewEngineHost(playCtx, wasm.PlayIdentity(playCtx), wasm.ModePlay, 64, 64, nil)
	defer play.Close()
	if n := play.ScoreCount(); n != 0 {
		t.Fatalf("the play table holds %d entries after an attract submit, want 0", n)
	}
	if rank := play.ScoreSubmit(500, "PLY"); rank != 1 {
		t.Fatalf("play ScoreSubmit ranked %d, want 1", rank)
	}
	if score, name, ok := attract.host.ScoreRead(1); !ok || score != 500 || name != "PLY" {
		t.Fatalf("attract ScoreRead(1) = (%d, %q, %v), want the play instance's entry", score, name, ok)
	}
}

func TestAttractStorageIsTheCartridgesOwn(t *testing.T) {
	wasmBytes, err := os.ReadFile("wasm/testdata/e2e.wasm")
	if err != nil {
		t.Fatalf("read e2e fixture cartridge: %v", err)
	}
	rig := testkit.NewRig(t, testkit.Options{StartApp: "shell"})
	parent := &engine.AppContext{Manifest: engine.Manifest{Slug: "attract", Name: "Attract"}, RT: rig.RT}

	alpha := mustAttract(t, parent, "cart/alpha", wasmBytes)
	beta := mustAttract(t, parent, "cart/beta", wasmBytes)

	alpha.host.StorageWrite("save", []byte("alpha"))
	if got, ok := beta.host.StorageRead("save"); ok {
		t.Fatalf("cart/beta's attract instance read %q from cart/alpha's storage", got)
	}

	playCtx := &engine.AppContext{Manifest: engine.Manifest{Slug: "cart/beta", Name: "Beta"}, RT: rig.RT}
	play := wasm.NewEngineHost(playCtx, wasm.PlayIdentity(playCtx), wasm.ModePlay, 64, 64, nil)
	defer play.Close()
	play.StorageWrite("best", []byte("42"))
	got, ok := beta.host.StorageRead("best")
	if !ok || string(got) != "42" {
		t.Fatalf("cart/beta's attract instance read (%q, %v), want the play instance's \"42\"", got, ok)
	}
}
