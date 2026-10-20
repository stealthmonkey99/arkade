// SPDX-License-Identifier: GPL-2.0-or-later

package net

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/stealthmonkey99/arkade/internal/display"
	"github.com/stealthmonkey99/arkade/internal/engine"
	"github.com/stealthmonkey99/arkade/internal/scores"
)

// Attract capability values reported per game by GET /api/games.
const (
	// attractCustom means the cartridge exports cart_attract.
	attractCustom = "custom"
	// attractShowcase means no cart_attract but a recorded score, so the
	// default showcase slot is shown.
	attractShowcase = "showcase"
	// attractNone means there is nothing to preview.
	attractNone = "none"
)

type gameInfo struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Type is engine.AppType: "game" or "app".
	Type    string   `json:"type"`
	Version string   `json:"version,omitempty"`
	Author  string   `json:"author,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Players []string `json:"players,omitempty"`
	// Icon is the manifest's 8x8 sprite as a data: URI.
	Icon    string `json:"icon,omitempty"`
	Attract string `json:"attract"`
	// Scores is not omitempty so a game with scores (list), a game with none
	// ([]) and a non-game app (null) stay distinguishable.
	Scores []scores.Entry `json:"scores"`
	// ScoresEditable is true for games only.
	ScoresEditable bool `json:"scoresEditable"`
	// Variants names the stored builds of this slug in offer order, omitted
	// unless there are at least two.
	Variants []string `json:"variants,omitempty"`
	// ActiveVariant is the installed one of Variants, omitted when the
	// installed bundle matches none.
	ActiveVariant string `json:"activeVariant,omitempty"`
}

type gamesResponse struct {
	Games []gameInfo `json:"games"`
	// ControlEnabled reports whether a bearer token is configured; mutating
	// routes already reveal this by status code.
	ControlEnabled bool `json:"controlEnabled"`
}

// handleGames serves GET /api/games: every non-hidden app in shell menu
// order. It is unauthenticated, like the other read routes.
func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) *rejection {
	apps := engine.Apps()
	// Once per table, since it stats and sometimes hashes files.
	variants, activeVariant := s.variantState()
	out := make([]gameInfo, 0, len(apps))
	for _, m := range apps {
		info := gameInfo{
			Slug:        m.Slug,
			Name:        m.Name,
			Description: m.Description,
			Type:        string(m.Type),
			Version:     m.Version,
			Author:      m.Author,
			Tags:        m.Tags,
			Players:     m.Players,
			Icon:        iconDataURI(m.Icon),
			Attract:     attractNone,
		}
		if m.Type == engine.TypeGame {
			table := scores.Load(s.store, m.Slug)
			if table.Entries == nil {
				table.Entries = []scores.Entry{}
			}
			info.Scores = table.Entries
			info.ScoresEditable = true
			info.Attract = attractModeFor(m, table)
		}
		if v := variants[m.Slug]; len(v) > 1 {
			info.Variants = v
			info.ActiveVariant = activeVariant[m.Slug]
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, gamesResponse{Games: out, ControlEnabled: s.token != ""})
	return nil
}

// attractModeFor mirrors apps/attract's readHighScores. Only games have an
// attract slot.
func attractModeFor(m engine.Manifest, table scores.Table) string {
	switch {
	case m.Type != engine.TypeGame:
		return attractNone
	case m.NewAttract != nil:
		return attractCustom
	case len(table.Entries) > 0:
		return attractShowcase
	default:
		return attractNone
	}
}

// iconDataURI encodes an 8x8 manifest sprite as a base64 PNG data URI, or ""
// for an app with no icon. Icons are inlined because each is about 150 bytes.
func iconDataURI(sprite display.Sprite) string {
	h := len(sprite)
	if h == 0 {
		return ""
	}
	w := 0
	for _, row := range sprite {
		if len(row) > w {
			w = len(row)
		}
	}
	if w == 0 {
		return ""
	}
	// NRGBA because display.Pixel alpha is not premultiplied.
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y, row := range sprite {
		for x, p := range row {
			img.SetNRGBA(x, y, color.NRGBA{R: p.R, G: p.G, B: p.B, A: p.A})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// handleAttract serves POST /api/attract/{slug}: show that app's attract slot
// now. It answers 404 for an unknown slug and 409 when standby is disabled or
// the app has no attract slot.
func (s *Server) handleAttract(w http.ResponseWriter, r *http.Request) *rejection {
	slug := r.PathValue("slug")
	m, err := engine.Lookup(slug)
	if err != nil {
		return reject(http.StatusNotFound, "unknown slug").because("attract %q rejected: not registered", slug)
	}
	if !s.rt.Status().StandbyEnabled {
		return reject(http.StatusConflict, "standby is disabled (--attract-after 0): there is no attract screen to show").
			because("attract %q rejected: standby disabled", slug)
	}
	mode := attractModeFor(m, scores.Load(s.store, m.Slug))
	if mode == attractNone {
		return reject(http.StatusConflict, "this app has no attract slot: it exports no cart_attract and has no recorded score for the default showcase").
			because("attract %q rejected: no attract slot", slug)
	}
	s.rt.RequestAttractPreview(slug)
	s.ring.Append("info", fmt.Sprintf("http: attract preview requested: %s (%s)", slug, mode))
	writeJSON(w, http.StatusOK, map[string]string{"slug": slug, "attract": mode})
	return nil
}

const maxScoreWriteBytes = 16 << 10

// maxEditableScore is int32's ceiling because the Pi build is 32-bit ARM,
// where a larger value would fail to decode.
const maxEditableScore = math.MaxInt32

// maxEditableNameLen exceeds the name-entry widget's 7 characters to allow
// longer names submitted by cartridges.
const maxEditableNameLen = 12

// scoresWriteRequest is the PUT /api/scores/{app} body. Entries is a pointer
// so an omitted key is a 400 rather than clearing the table; clearing needs
// {"entries":[]}.
type scoresWriteRequest struct {
	Entries *[]scores.Entry `json:"entries"`
}

// handleScoresWrite serves PUT /api/scores/{app}: replace a registered game's
// saved table. Names are uppercased and trimmed and entries stable-sorted
// descending; the response echoes the stored table. A running game may
// overwrite the write when it saves on teardown.
func (s *Server) handleScoresWrite(w http.ResponseWriter, r *http.Request) *rejection {
	app := r.PathValue("app")
	what := fmt.Sprintf("score write %q", app)
	if !slugRE.MatchString(app) {
		return reject(http.StatusBadRequest, "invalid app slug").because("%s rejected: invalid slug", what)
	}
	m, err := engine.Lookup(app)
	if err != nil {
		return reject(http.StatusNotFound, "unknown slug").because("%s rejected: not registered", what)
	}
	if m.Type != engine.TypeGame {
		return reject(http.StatusNotFound, "only games have a score table").because("%s rejected: not a game", what)
	}

	var req scoresWriteRequest
	if rej := decodeBody(w, r, what, maxScoreWriteBytes, &req); rej != nil {
		return rej
	}
	if req.Entries == nil {
		return reject(http.StatusBadRequest, `body must set "entries" (use {"entries":[]} to clear the table)`).
			because("%s rejected: no entries key", what)
	}
	table, err := validateScoreTable(*req.Entries)
	if err != nil {
		return invalid(what, err)
	}
	if err := scores.Save(s.store, app, table); err != nil {
		return reject(http.StatusInternalServerError, "writing scores").failed("%s failed: %v", what, err)
	}
	s.ring.Append("info", fmt.Sprintf("http: scores written: %s (%d entries)", app, len(table.Entries)))
	writeJSON(w, http.StatusOK, table)
	return nil
}

func validateScoreTable(entries []scores.Entry) (scores.Table, error) {
	if len(entries) > scores.Max {
		return scores.Table{}, fmt.Errorf("too many entries: %d (max %d)", len(entries), scores.Max)
	}
	out := make([]scores.Entry, 0, len(entries))
	for i, e := range entries {
		if e.Score < 0 || e.Score > maxEditableScore {
			return scores.Table{}, fmt.Errorf("entry %d: score %d out of range (0..%d)", i, e.Score, maxEditableScore)
		}
		name, err := normalizeScoreName(e.Name)
		if err != nil {
			return scores.Table{}, fmt.Errorf("entry %d: %w", i, err)
		}
		out = append(out, scores.Entry{Score: e.Score, Name: name})
	}
	// scores.Table must be sorted descending, ties keeping their order.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return scores.Table{Entries: out}, nil
}

// normalizeScoreName uppercases and trims like ui.NameEntry but rejects
// characters outside A-Z, 0-9 and space instead of dropping them.
func normalizeScoreName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", errors.New("name is not valid UTF-8")
	}
	name = strings.TrimSpace(strings.ToUpper(name))
	if utf8.RuneCountInString(name) > maxEditableNameLen {
		return "", fmt.Errorf("name %q is longer than %d characters", name, maxEditableNameLen)
	}
	for _, c := range name {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != ' ' {
			return "", fmt.Errorf("name %q contains %q: only A-Z, 0-9 and spaces can be shown on the cabinet", name, c)
		}
	}
	return name, nil
}
