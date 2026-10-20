// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigName is the optional per-cartridge exceptions file.
const ConfigName = "preflight.json"

// Config is the decoded preflight.json. A cartridge without one is scanned
// with the defaults.
type Config struct {
	// Suppress lists findings the author has answered for.
	Suppress []Suppression `json:"suppress,omitempty"`
	// WavFloor overrides the audibility check's speaker model.
	WavFloor *WavFloor `json:"wavFloor,omitempty"`
}

// Suppression silences matching findings in one cartridge. The finding is
// still printed with its reason.
type Suppression struct {
	// Check is the registry id whose findings this may match. Required.
	Check string `json:"check"`
	// Where narrows to one file, either cartridge-relative ("assets/x.wav")
	// or with a "cartridges/<slug>/" prefix. It matches a finding's location
	// exactly or as its file part, so it survives line moves.
	Where string `json:"where,omitempty"`
	// Contains narrows to findings whose message contains this substring.
	Contains string `json:"contains,omitempty"`
	// Reason is why this is not a bug. Required, and printed on the card.
	Reason string `json:"reason"`
}

// WavFloor overrides the default speaker model for one cartridge.
type WavFloor struct {
	// Hz is the top of the sub-bass band the cabinet cannot reproduce.
	Hz float64 `json:"hz,omitempty"`
	// Frac is how much of a sound's energy may sit below Hz.
	Frac float64 `json:"frac,omitempty"`
	// Reason is required.
	Reason string `json:"reason"`
}

// matches reports whether the suppression covers a finding of the given
// check. validate rejects one with neither Where nor Contains.
func (s Suppression) matches(c *Cart, checkID string, f Finding) bool {
	if s.Check != checkID {
		return false
	}
	if s.Where != "" && !parseWhere(s.Where).covers(f.Loc, c.cartPrefix()) {
		return false
	}
	if s.Contains != "" && !strings.Contains(f.Message, s.Contains) {
		return false
	}
	return true
}

// cartPrefix is the "cartridges/<slug>/" a suppression may put before a
// cartridge-relative path.
func (c *Cart) cartPrefix() string {
	return filepath.Base(filepath.Dir(c.Dir)) + "/" + c.Slug + "/"
}

// LoadConfig reads and validates a preflight.json. A missing file returns a
// nil Config. Unknown fields, unknown check ids, a missing reason and a
// suppression that narrows nothing are errors.
func LoadConfig(path string, knownChecks map[string]bool) (*Config, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("unparseable %s: %w", ConfigName, err)
	}
	if err := cfg.validate(knownChecks); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate(knownChecks map[string]bool) error {
	if len(c.Suppress) == 0 && c.WavFloor == nil {
		return fmt.Errorf("%s declares neither a suppression nor a wavFloor — delete it rather than shipping an empty exemption file", ConfigName)
	}
	for i, s := range c.Suppress {
		switch {
		case s.Check == "":
			return fmt.Errorf("suppress[%d]: no \"check\" — name the check id this answers for (see -list)", i)
		case !knownChecks[s.Check]:
			return fmt.Errorf("suppress[%d]: no check %q in the registry — see -list", i, s.Check)
		case strings.TrimSpace(s.Reason) == "":
			return fmt.Errorf("suppress[%d] (%s): no \"reason\" — an exemption without a written justification is indistinguishable from one nobody understands any more", i, s.Check)
		case s.Where == "" && s.Contains == "":
			return fmt.Errorf("suppress[%d] (%s): needs a \"where\" or a \"contains\" — a suppression that narrows nothing turns the whole check off for this cartridge", i, s.Check)
		}
	}
	if w := c.WavFloor; w != nil {
		if strings.TrimSpace(w.Reason) == "" {
			return fmt.Errorf("wavFloor: no \"reason\" — say which sounds were auditioned on which hardware")
		}
		if w.Hz < 0 || w.Frac < 0 || w.Frac > 1 {
			return fmt.Errorf("wavFloor: hz must be >= 0 and frac in [0,1]; got hz=%g frac=%g", w.Hz, w.Frac)
		}
	}
	return nil
}

// Config loads the cartridge's preflight.json once and caches the result and error.
func (c *Cart) Config() (*Config, error) {
	if !c.configGot {
		c.configGot = true
		c.config, c.configErr = LoadConfig(filepath.Join(c.Dir, ConfigName), registryIDs)
	}
	return c.config, c.configErr
}

const ruleConfig = "tools/preflight/doc.go, 'Per-cartridge exceptions': an exception is a written reason in the tree, scoped to one finding, and printed on the report card rather than hidden"

// checkConfig validates preflight.json. It is registered first because a
// config that fails to load suppresses nothing.
func checkConfig(c *Cart, r *Result) {
	path := filepath.Join(c.Dir, ConfigName)
	where := c.At(path)
	cfg, err := c.Config()
	if err != nil {
		r.fail(ruleConfig, where, "%v", err)
		return
	}
	if cfg == nil {
		r.skip("no %s — scanned with the defaults", ConfigName)
		return
	}
	for _, s := range cfg.Suppress {
		r.info(where, "suppresses %s findings matching %s (%s)", s.Check, s.scope(), s.Reason)
	}
	if w := cfg.WavFloor; w != nil {
		m := c.audioFloor()
		r.info(where, "wav floor overridden to %g Hz / %.0f%% (%s)", m.hz, 100*m.frac, w.Reason)
	}
	r.summarize("%d suppression(s), wav floor %s", len(cfg.Suppress), cfg.wavFloorSummary())
}

func (s Suppression) scope() string {
	switch {
	case s.Where != "" && s.Contains != "":
		return fmt.Sprintf("%s containing %q", s.Where, s.Contains)
	case s.Where != "":
		return s.Where
	default:
		return fmt.Sprintf("%q", s.Contains)
	}
}

func (c *Config) wavFloorSummary() string {
	if c == nil || c.WavFloor == nil {
		return "default"
	}
	return "overridden"
}

// over applies the override to def, keeping def for fields left zero.
func (w *WavFloor) over(def audioFloorModel) audioFloorModel {
	if w == nil {
		return def
	}
	if w.Hz != 0 {
		def.hz = w.Hz
	}
	if w.Frac != 0 {
		def.frac = w.Frac
	}
	return def
}

// applySuppressions marks matching warn and fail findings StatusSuppressed,
// keeping their original severity, and recomputes each check's status. A
// suppression that matched nothing warns on the config check.
func applySuppressions(c *Cart, cfg *Config, card *CartReport) {
	if cfg == nil || len(cfg.Suppress) == 0 {
		return
	}
	used := make([]bool, len(cfg.Suppress))
	for i := range card.Checks {
		res := &card.Checks[i]
		changed := false
		for j := range res.Findings {
			f := &res.Findings[j]
			if f.Status != StatusWarn && f.Status != StatusFail {
				continue
			}
			for k, s := range cfg.Suppress {
				if !s.matches(c, res.ID, *f) {
					continue
				}
				used[k] = true
				f.Severity = f.Status
				f.Status = StatusSuppressed
				f.Reason = s.Reason
				changed = true
				break
			}
		}
		if changed {
			res.restatus()
		}
	}
	cfgRes := card.check("config")
	if cfgRes == nil {
		return
	}
	where := c.At(filepath.Join(c.Dir, ConfigName))
	for k, s := range cfg.Suppress {
		if used[k] {
			continue
		}
		cfgRes.warn(ruleConfig, where, "suppress[%d] (%s, %s) matched nothing — the finding it answers for is gone, so delete it", k, s.Check, s.scope())
	}
}
