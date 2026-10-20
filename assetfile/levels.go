// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// readLevelsEnvelope is ReadBytes' KindLevels case. A pack with no front
// matter reads as an in-memory envelope whose Generator is "legacy-levels",
// so it still passes validation; nothing is rewritten on disk.
func readLevelsEnvelope(name string, data []byte) (*Envelope, error) {
	envJSON, body, ok := splitLevelsEnvelope(data)
	if !ok {
		env := &Envelope{
			Format:        FormatVersion,
			Kind:          KindLevels,
			Name:          stemName(name),
			Generator:     Generator{Tool: "assetfile", Version: "legacy-levels"},
			LevelsPayload: LevelsPayload{Body: data},
		}
		if err := validated(env, KindLevels, 0, 0); err != nil {
			return nil, err
		}
		return env, nil
	}
	env, err := Unmarshal(envJSON)
	if err != nil {
		return nil, fmt.Errorf("assetfile: %s: invalid envelope front matter: %w", name, err)
	}
	env.Body = body
	if err := validated(env, KindLevels, 0, 0); err != nil {
		return nil, err
	}
	return env, nil
}

// writeLevelsEnvelope is WriteBytes' KindLevels case: env.Body with the
// indented envelope prepended as front matter.
func writeLevelsEnvelope(env *Envelope) ([]byte, error) {
	if len(env.Body) == 0 {
		return nil, errors.New("assetfile: a levels envelope needs Body (the pack file's own text)")
	}
	if err := Validate(env, KindLevels, 0, 0); err != nil {
		return nil, err
	}
	compact, err := Marshal(env)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return nil, err
	}
	block := formatLevelsEnvelope(buf.Bytes())
	out := make([]byte, 0, len(block)+len(env.Body))
	out = append(out, block...)
	out = append(out, env.Body...)
	return out, nil
}

// stemName strips every extension: "walls.arkadelevels" becomes "walls".
func stemName(name string) string {
	base := filepath.Base(name)
	for {
		ext := filepath.Ext(base)
		if ext == "" {
			return base
		}
		base = strings.TrimSuffix(base, ext)
	}
}
