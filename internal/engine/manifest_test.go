// SPDX-License-Identifier: GPL-2.0-or-later

package engine_test

import (
	"errors"
	"testing"

	"github.com/stealthmonkey99/arkade/internal/engine"
)

func TestRegisterRefusesATakenSlug(t *testing.T) {
	const slug = "test/manifest-duplicate-probe"
	t.Cleanup(func() { engine.Unregister(slug) })

	if err := engine.Register(engine.Manifest{Slug: slug, Name: "First", Hidden: true}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := engine.Register(engine.Manifest{Slug: slug, Name: "Second", Hidden: true})
	if !errors.Is(err, engine.ErrSlugTaken) {
		t.Fatalf("second Register = %v, want ErrSlugTaken", err)
	}
	if m, _ := engine.Lookup(slug); m.Name != "First" {
		t.Errorf("Lookup after a refused Register = %q, want the first registration", m.Name)
	}
	engine.Unregister(slug)
	if _, err := engine.Lookup(slug); err == nil {
		t.Error("one Unregister should leave nothing behind, since the duplicate never went in")
	}
}

// The slug is unique to this test and cleaned up because the registry is process-global.
func TestUnregister(t *testing.T) {
	const slug = "test/manifest-unregister-probe"
	t.Cleanup(func() { engine.Unregister(slug) })

	if got := engine.Unregister(slug); got {
		t.Fatalf("Unregister(%q) before Register = true, want false", slug)
	}

	engine.Register(engine.Manifest{
		Slug:   slug,
		Name:   "Unregister Probe",
		Type:   engine.TypeGame,
		Hidden: true,
		New:    func(ctx *engine.AppContext) engine.App { return nil },
	})

	if _, err := engine.Lookup(slug); err != nil {
		t.Fatalf("Lookup(%q) after Register: %v", slug, err)
	}

	if got := engine.Unregister(slug); !got {
		t.Fatalf("Unregister(%q) = false, want true", slug)
	}

	if _, err := engine.Lookup(slug); err == nil {
		t.Fatalf("Lookup(%q) after Unregister: expected an error", slug)
	}

	// A second Unregister of the same slug is a no-op, not a panic or a spurious true.
	if got := engine.Unregister(slug); got {
		t.Fatalf("second Unregister(%q) = true, want false", slug)
	}
}

func TestUnregisterOnlyRemovesMatchingSlug(t *testing.T) {
	const (
		keepSlug   = "test/manifest-unregister-keep"
		removeSlug = "test/manifest-unregister-remove"
	)
	t.Cleanup(func() {
		engine.Unregister(keepSlug)
		engine.Unregister(removeSlug)
	})

	newApp := func(ctx *engine.AppContext) engine.App { return nil }
	engine.Register(engine.Manifest{Slug: keepSlug, Name: "Keep", Type: engine.TypeGame, Hidden: true, New: newApp})
	engine.Register(engine.Manifest{Slug: removeSlug, Name: "Remove", Type: engine.TypeGame, Hidden: true, New: newApp})

	if !engine.Unregister(removeSlug) {
		t.Fatalf("Unregister(%q) = false, want true", removeSlug)
	}
	if _, err := engine.Lookup(keepSlug); err != nil {
		t.Fatalf("Lookup(%q) after removing a neighbor: %v", keepSlug, err)
	}
	if _, err := engine.Lookup(removeSlug); err == nil {
		t.Fatalf("Lookup(%q) after Unregister: expected an error", removeSlug)
	}
}
