// SPDX-License-Identifier: GPL-2.0-or-later

package rcart

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// CurrentSchemaVersion is the newest bundle-manifest schema this loader
// understands, and the one rcartpack requires of new bundles.
const CurrentSchemaVersion = 1

// Player-mode vocabulary ("players" is check-all-that-apply).
const (
	PlayersSolo        = "1p"
	PlayersCoop        = "2p-coop"
	PlayersVersus      = "2p-versus"
	PlayersAlternating = "alternating" // two players, one at the controls at a time
	// PlayersLegacyTwo is what a legacy integer 2 decodes to. It loads, but
	// schema-1 validation rejects it.
	PlayersLegacyTwo = "2p"
)

// Audience vocabulary. Deliberately not ESRB marks.
const (
	AudienceAllAges = "all-ages"
	AudienceTeen    = "teen"
	AudienceMature  = "mature"
)

// Visibility tiers drive registry listing, not the cabinet: an installed
// bundle plays regardless.
const (
	VisibilityPublic   = "public"    // source published
	VisibilityPlayOnly = "play-only" // rcart distributed, source private
	VisibilityPrivate  = "private"   // never distributed in any form
)

// Manifest is a bundle's manifest.json, the one source of a game's metadata
// for the shell, the registry and the web layer. Artwork is referenced by
// bundle-relative path, never embedded.
//
// Schema 1 is required of newly packed bundles and validated strictly. A
// legacy manifest (no schemaVersion, players as integers) still loads and
// needs only slug and name. The wasm's cart_manifest export is a separate,
// runtime contract.
type Manifest struct {
	SchemaVersion int `json:"schemaVersion,omitempty"`
	// ABI is informational; the runtime checks the wasm export's version.
	ABI  int    `json:"abi,omitempty"`
	Slug string `json:"slug"`
	// Title is the schema-1 display name and Name its legacy spelling; read
	// either through DisplayTitle.
	Title string `json:"title,omitempty"`
	Name  string `json:"name,omitempty"`
	// Publisher is the registry namespace (publisher/slug); Author is a person.
	Publisher   string      `json:"publisher,omitempty"`
	Author      string      `json:"author,omitempty"`
	Version     string      `json:"version,omitempty"`
	Description string      `json:"description,omitempty"` // short card/landing text
	Tags        []string    `json:"tags,omitempty"`
	Players     PlayerModes `json:"players,omitempty"`
	Audience    string      `json:"audience,omitempty"`
	// Capabilities are permission-style requests such as CapabilityHID. An
	// unknown one is carried through and ignored.
	Capabilities []string `json:"capabilities,omitempty"`
	// Assets lists the files the host acts on at mount.
	Assets     *AssetRefs `json:"assets,omitempty"`
	Visibility string     `json:"visibility,omitempty"`
	// Icon, Logo and Manual are bundle-relative paths, e.g. "assets/icon.png".
	Icon   string `json:"icon,omitempty"`
	Logo   string `json:"logo,omitempty"`
	Manual string `json:"manual,omitempty"`
	Links  *Links `json:"links,omitempty"`
	// RawDirections opts out of the host's seat and orientation direction
	// remap, for egocentric controls such as tank rotation.
	RawDirections bool `json:"rawDirections,omitempty"`
}

// AssetRefs is the optional "assets" block: bundle-relative paths the host
// loads at mount, before cart_init. Sprites and sounds are fetched by the
// guest at runtime and need no listing. Every list is dropped at teardown.
type AssetRefs struct {
	// Glyphs are .arkadeglyph.json files registered into the cartridge's
	// LCD glyph table, shadowing the engine's for the session.
	Glyphs []string `json:"glyphs,omitempty"`

	// HID are .arkadehid.json session profiles; their channels become
	// bindable through host_input_bind. Requires CapabilityHID, because a
	// profile changes how the cabinet reads physical hardware.
	HID []string `json:"hid,omitempty"`

	// Mic are .arkademic.json session sound models whose matched classes
	// arrive as mic.cueN keypresses. Requires CapabilityMic.
	Mic []string `json:"mic,omitempty"`

	// Lights are .arkadelights.json lamp sequences. The list is also
	// host_lights_play's namespace: an unlisted sequence cannot be played.
	// Ungated, since a cartridge can already drive the same lamps directly.
	Lights []string `json:"lights,omitempty"`
}

// Mount caps: the most files of each listed kind one cartridge may mount.
// The host refuses the rest with a warning; rcartpack refuses the bundle.
const (
	MaxMountedHIDProfiles = 8
	MaxMountedMicModels   = 4
	MaxMountedLights      = 16
)

// listedAssetKind is one assets.<field> list: how its files are named and
// validated, the capability that gates it, and how many a host mounts.
type listedAssetKind struct {
	field      string // assets.<field>
	category   Category
	label      string // names one file in a warning
	noun       string // names the list's files in a count
	kind       assetfile.Kind
	capability string // "" when ungated
	cap        int    // 0 when unbounded
	list       func(*AssetRefs) *[]string
}

// listedAssetKinds drives both the pack-time warnings (Load) and the
// mount-time capability gate (MountAssets), in this order.
var listedAssetKinds = []listedAssetKind{
	{field: "glyphs", category: CategoryGlyph, label: "glyph", noun: "glyph", kind: assetfile.KindGlyph,
		list: func(a *AssetRefs) *[]string { return &a.Glyphs }},
	{field: "hid", category: CategoryHID, label: "hid profile", noun: "profile", kind: assetfile.KindHID,
		capability: CapabilityHID, cap: MaxMountedHIDProfiles,
		list: func(a *AssetRefs) *[]string { return &a.HID }},
	{field: "mic", category: CategoryMic, label: "mic model", noun: "sound model", kind: assetfile.KindMic,
		capability: CapabilityMic, cap: MaxMountedMicModels,
		list: func(a *AssetRefs) *[]string { return &a.Mic }},
	{field: "lights", category: CategoryLights, label: "lights", noun: "lamp sequence", kind: assetfile.KindLights,
		cap:  MaxMountedLights,
		list: func(a *AssetRefs) *[]string { return &a.Lights }},
}

func (k listedAssetKind) paths(m *Manifest) []string {
	if m.Assets == nil {
		return nil
	}
	return *k.list(m.Assets)
}

// MountAssets returns the assets block as a host mounts it: each list whose
// capability the manifest does not declare is dropped with one warning.
func (m *Manifest) MountAssets() (AssetRefs, []string) {
	var out AssetRefs
	var warnings []string
	if m.Assets == nil {
		return out, nil
	}
	for _, k := range listedAssetKinds {
		paths := *k.list(m.Assets)
		if len(paths) == 0 {
			continue
		}
		if k.capability != "" && !m.HasCapability(k.capability) {
			warnings = append(warnings, fmt.Sprintf("assets.%s ignored — the manifest does not declare the %q capability", k.field, k.capability))
			continue
		}
		*k.list(&out) = paths
	}
	return out, warnings
}

func (m *Manifest) GlyphAssets() []string {
	if m.Assets == nil {
		return nil
	}
	return m.Assets.Glyphs
}

func (m *Manifest) HIDAssets() []string {
	if m.Assets == nil {
		return nil
	}
	return m.Assets.HID
}

func (m *Manifest) MicAssets() []string {
	if m.Assets == nil {
		return nil
	}
	return m.Assets.Mic
}

func (m *Manifest) LightsAssets() []string {
	if m.Assets == nil {
		return nil
	}
	return m.Assets.Lights
}

// CapabilityHID is the capability a manifest must declare before its
// assets.hid list is honoured.
const CapabilityHID = "hid"

// CapabilityMic is the capability a manifest must declare before the cabinet
// sends it any microphone event, gestures and assets.mic cues alike.
const CapabilityMic = "mic"

// HasCapability reports whether the manifest declares a capability.
func (m *Manifest) HasCapability(name string) bool {
	for _, c := range m.Capabilities {
		if c == name {
			return true
		}
	}
	return false
}

// Links holds the manifest's outbound URLs. Source is a pointer so
// "source": null, for unpublished source, round-trips.
type Links struct {
	Source *string `json:"source"`
}

// PlayerModes is the manifest's declared player support. Legacy integer
// counts decode to PlayersSolo and PlayersLegacyTwo.
type PlayerModes []string

// UnmarshalJSON accepts mode strings and the legacy integers 1 and 2.
func (p *PlayerModes) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(PlayerModes, 0, len(raw))
	for _, entry := range raw {
		var s string
		if err := json.Unmarshal(entry, &s); err == nil {
			out = append(out, s)
			continue
		}
		var n int
		if err := json.Unmarshal(entry, &n); err != nil {
			return fmt.Errorf("players entry %s is neither a mode string nor an integer", entry)
		}
		switch n {
		case 1:
			out = append(out, PlayersSolo)
		case 2:
			out = append(out, PlayersLegacyTwo)
		default:
			return fmt.Errorf("players entry %d has no legacy meaning (only 1 and 2 ever did)", n)
		}
	}
	*p = out
	return nil
}

// MaxPlayers returns the largest player count any declared mode supports,
// or 0 when none is declared.
func (p PlayerModes) MaxPlayers() int {
	maxN := 0
	for _, m := range p {
		n := 1
		if m != PlayersSolo {
			n = 2
		}
		if n > maxN {
			maxN = n
		}
	}
	return maxN
}

// SupportsCount reports whether any declared mode plays with exactly n
// players. Alternating counts as 2.
func (p PlayerModes) SupportsCount(n int) bool {
	for _, m := range p {
		if (m == PlayersSolo && n == 1) || (m != PlayersSolo && n == 2) {
			return true
		}
	}
	return false
}

func (m *Manifest) DisplayTitle() string {
	if m.Title != "" {
		return m.Title
	}
	return m.Name
}

// slugPattern also covers tags: lowercase words joined by single hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// maxDescriptionLen keeps the description card-sized; the manual is the long form.
const maxDescriptionLen = 160

// Validate checks the manifest against its declared generation: a legacy
// manifest needs only slug and name, schema 1 gets the full contract.
func (m *Manifest) Validate() error {
	if m.Slug == "" {
		return fmt.Errorf(`"slug" is required`)
	}
	switch m.SchemaVersion {
	case 0:
		if m.DisplayTitle() == "" {
			return fmt.Errorf(`"name" is required`)
		}
		return nil
	case CurrentSchemaVersion:
		return m.validateSchema1()
	default:
		return fmt.Errorf("unsupported schemaVersion %d (this loader understands up to %d)", m.SchemaVersion, CurrentSchemaVersion)
	}
}

func (m *Manifest) validateSchema1() error {
	if !slugPattern.MatchString(m.Slug) {
		return fmt.Errorf("slug %q must be lowercase words joined by hyphens", m.Slug)
	}
	for _, req := range [...]struct{ field, val string }{
		{"title", m.Title}, {"publisher", m.Publisher}, {"author", m.Author},
	} {
		if req.val == "" {
			return fmt.Errorf("%q is required", req.field)
		}
	}
	if !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("version %q must be MAJOR.MINOR.PATCH", m.Version)
	}
	if m.Description == "" {
		return fmt.Errorf(`"description" is required`)
	}
	if len(m.Description) > maxDescriptionLen {
		return fmt.Errorf("description is %d bytes; keep the card text under %d (the manual is the long-form home)", len(m.Description), maxDescriptionLen)
	}
	if len(m.Tags) == 0 {
		return fmt.Errorf(`"tags" needs at least one entry`)
	}
	for _, tag := range m.Tags {
		if !slugPattern.MatchString(tag) {
			return fmt.Errorf("tag %q must be lowercase words joined by hyphens", tag)
		}
	}
	if len(m.Players) == 0 {
		return fmt.Errorf(`"players" needs at least one entry`)
	}
	seen := map[string]bool{}
	for _, mode := range m.Players {
		switch mode {
		case PlayersSolo, PlayersCoop, PlayersVersus, PlayersAlternating:
		case PlayersLegacyTwo:
			return fmt.Errorf(`players entry 2 (legacy integer) is ambiguous under schemaVersion 1 — say "2p-coop", "2p-versus" or "alternating"`)
		default:
			return fmt.Errorf("players entry %q is not in the vocabulary (%s, %s, %s, %s)", mode, PlayersSolo, PlayersCoop, PlayersVersus, PlayersAlternating)
		}
		if seen[mode] {
			return fmt.Errorf("players entry %q is duplicated", mode)
		}
		seen[mode] = true
	}
	switch m.Audience {
	case AudienceAllAges, AudienceTeen, AudienceMature:
	default:
		return fmt.Errorf("audience %q is not in the vocabulary (%s, %s, %s)", m.Audience, AudienceAllAges, AudienceTeen, AudienceMature)
	}
	switch m.Visibility {
	case VisibilityPublic, VisibilityPlayOnly, VisibilityPrivate:
	default:
		return fmt.Errorf("visibility %q is not in the vocabulary (%s, %s, %s)", m.Visibility, VisibilityPublic, VisibilityPlayOnly, VisibilityPrivate)
	}
	for _, c := range m.Capabilities {
		if !slugPattern.MatchString(c) {
			return fmt.Errorf("capability %q must be lowercase words joined by hyphens", c)
		}
	}
	if m.Icon == "" {
		return fmt.Errorf(`"icon" is required`)
	}
	for _, ref := range [...]struct{ field, path string }{
		{"icon", m.Icon}, {"logo", m.Logo}, {"manual", m.Manual},
	} {
		if ref.path == "" {
			continue
		}
		if _, ok := cleanEntryName(ref.path); !ok || strings.Contains(ref.path, "\\") {
			return fmt.Errorf("%s path %q must be a clean bundle-relative path", ref.field, ref.path)
		}
	}
	if m.Links == nil {
		return fmt.Errorf(`"links" is required (use {"source": null} while the source is unpublished)`)
	}
	if src := m.Links.Source; src != nil && !strings.HasPrefix(*src, "https://") {
		return fmt.Errorf("links.source %q must be an https:// URL (or null)", *src)
	}
	return nil
}
