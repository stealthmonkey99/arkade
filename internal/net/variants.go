// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"fmt"
	"net/http"

	"github.com/stealthmonkey99/arkade/internal/engine"
)

// VariantControl is the optional half of the CartridgeControl seam. A
// CartridgeControl that also implements it enables the build-variant toggle.
// Variants returns parallel maps because no shared struct type could be named
// without one package importing the other.
type VariantControl interface {
	// Variants reports every slug with more than one stored build. available
	// maps a slug to its variant names in display order; active maps it to the
	// installed variant, absent when none match. Both are nil without a store.
	Variants() (available map[string][]string, active map[string]string)
	// SetVariant installs the named build of slug and re-registers it,
	// returning the registered display name. A running session is undisturbed.
	SetVariant(slug, variant string) (name string, err error)
}

func (s *Server) variants() (VariantControl, bool) {
	vc, ok := s.cart.(VariantControl)
	return vc, ok
}

func (s *Server) variantState() (available map[string][]string, active map[string]string) {
	vc, ok := s.variants()
	if !ok {
		return nil, nil
	}
	return vc.Variants()
}

// handleSetVariant serves POST /api/variants/{variant}/{slug}: install the
// named stored build of a registered cartridge. It does not launch; the next
// launch runs the new build, and the response says so.
func (s *Server) handleSetVariant(w http.ResponseWriter, r *http.Request) *rejection {
	variant := r.PathValue("variant")
	slug := r.PathValue("slug")
	what := fmt.Sprintf("variant %q for %q", variant, slug)

	vc, ok := s.variants()
	if !ok {
		return reject(http.StatusServiceUnavailable, "this cabinet has no build-variant store").
			because("%s rejected: no build-variant store on this cabinet", what)
	}
	if _, err := engine.Lookup(slug); err != nil {
		return reject(http.StatusNotFound, "unknown slug").because("%s rejected: not registered", what)
	}
	available, _ := vc.Variants()
	if !containsString(available[slug], variant) {
		return reject(http.StatusNotFound, fmt.Sprintf("no %q build is stored for %s", variant, slug)).
			because("%s rejected: no such stored build", what)
	}
	name, err := vc.SetVariant(slug, variant)
	if err != nil {
		msg, status := installErrorResponse(err, fmt.Sprintf("installing the %s build failed", variant), http.StatusInternalServerError)
		return reject(status, msg).failed("%s failed: %v", what, err)
	}
	s.ring.Append("info", fmt.Sprintf("http: variant installed: %s -> %s (%q); takes effect on the next launch", slug, variant, name))
	writeJSON(w, http.StatusOK, map[string]string{
		"slug":    slug,
		"variant": variant,
		"name":    name,
		"note":    "installed; a session already running keeps the old build — relaunch to see it",
	})
	return nil
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
