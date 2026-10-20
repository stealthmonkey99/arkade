// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoEnvelope is what Read and ReadBytes return for a plain .png that
// carries no envelope. Callers with their own plain-sprite handling test for
// it with errors.Is.
var ErrNoEnvelope = errors.New("assetfile: no arkade envelope")

// KindFromFilename applies the naming rule <name>.arkade<kind>.<container>
// to name, ignoring any directory component. KindLevels is the exception:
// <name>.arkadelevels and <name>.levels both match. It reports ok=false for
// anything else, including a plain "hero.png".
func KindFromFilename(name string) (Kind, bool) {
	base := filepath.Base(name)
	if hasLevelsExt(base) {
		return KindLevels, true
	}
	ext := filepath.Ext(base)
	if ext == "" {
		return "", false
	}
	stem := strings.TrimSuffix(base, ext)
	dot := strings.LastIndex(stem, ".")
	if dot < 0 {
		return "", false
	}
	mid := stem[dot+1:]
	if !strings.HasPrefix(mid, "arkade") {
		return "", false
	}
	k := Kind(strings.TrimPrefix(mid, "arkade"))
	wantExt, ok := containerExt[k]
	if !ok {
		return "", false
	}
	if !strings.EqualFold(strings.TrimPrefix(ext, "."), wantExt) {
		return "", false
	}
	return k, true
}

// Read loads path and returns its validated envelope; see ReadBytes.
func Read(path string) (*Envelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ReadBytes(filepath.Base(path), data)
}

// ReadBytes is Read without touching disk. name picks the container via
// KindFromFilename; a plain .png that matches no kind is read for an
// envelope all the same. The result is always validated.
func ReadBytes(name string, data []byte) (*Envelope, error) {
	if kind, ok := KindFromFilename(name); ok {
		switch kind {
		case KindSprite:
			env, _, _, err := ReadPNGEnvelope(data)
			if err != nil {
				return nil, err
			}
			if env == nil {
				return nil, fmt.Errorf("assetfile: %s carries no arkade envelope", name)
			}
			return env, nil
		case KindSound:
			env, err := readWAVEnvelope(data)
			if err != nil {
				return nil, err
			}
			if env == nil {
				return nil, fmt.Errorf("assetfile: %s carries no arkade envelope", name)
			}
			if err := validated(env, kind, 0, 0); err != nil {
				return nil, err
			}
			return env, nil
		case KindGlyph, KindHID, KindLights, KindMic:
			env, err := Unmarshal(data)
			if err != nil {
				return nil, err
			}
			if err := validated(env, kind, 0, 0); err != nil {
				return nil, err
			}
			return env, nil
		case KindLevels:
			return readLevelsEnvelope(name, data)
		default:
			return nil, fmt.Errorf("assetfile: unknown kind %q", kind)
		}
	}

	if !strings.EqualFold(filepath.Ext(name), ".png") {
		return nil, fmt.Errorf("assetfile: %q does not match the <name>.arkade<kind>.<container> naming rule", name)
	}

	env, _, _, err := ReadPNGEnvelope(data)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, ErrNoEnvelope
	}
	return env, nil
}

// Write renders env in the container path's name implies and writes it to
// path. See WriteBytes for base.
func Write(path string, env *Envelope, base []byte) error {
	out, err := WriteBytes(filepath.Base(path), env, base)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// WriteBytes renders env in the container name implies. name must match
// env.Kind via KindFromFilename or WriteBytes returns an error. base is:
//
//   - sprite: the sheet's existing PNG bytes; the arkade chunk is set or
//     replaced.
//   - sound: the existing WAV bytes; the arkd chunk is set or replaced and
//     every other chunk is kept.
//   - glyph, hid, lights, mic, levels: ignored. A levels envelope needs
//     env.Body, which follows the envelope as front matter.
func WriteBytes(name string, env *Envelope, base []byte) ([]byte, error) {
	kind, ok := KindFromFilename(name)
	if !ok {
		return nil, fmt.Errorf("assetfile: %q does not match the <name>.arkade<kind>.<container> naming rule", name)
	}
	if env.Kind != kind {
		return nil, fmt.Errorf("assetfile: envelope kind %q does not match filename kind %q", env.Kind, kind)
	}
	switch kind {
	case KindSprite:
		return writePNG(base, env)
	case KindSound:
		return writeWAV(base, env)
	case KindGlyph, KindHID, KindLights, KindMic:
		return WriteJSON(env)
	case KindLevels:
		return writeLevelsEnvelope(env)
	default:
		return nil, fmt.Errorf("assetfile: unknown kind %q", kind)
	}
}
