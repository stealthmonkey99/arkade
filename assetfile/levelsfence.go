// SPDX-License-Identifier: GPL-2.0-or-later

package assetfile

import "strings"

// LevelsFence is the sentinel line that opens and closes a pack file's
// optional envelope block (sdk/docs/reference-levels.md). sdk/levels/packfile
// keeps its own copy so cartridges need not import the levels module.
const LevelsFence = "// --- arkade ---"

var levelsExts = []string{".levels", ".arkadelevels"}

func hasLevelsExt(name string) bool {
	for _, ext := range levelsExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// splitLevelsEnvelope returns the fenced block's JSON and the body after it.
// ok is false for a file with no block or a malformed one, and body is then
// src unchanged.
func splitLevelsEnvelope(src []byte) (json, body []byte, ok bool) {
	norm := strings.ReplaceAll(string(src), "\r\n", "\n")
	lines := strings.Split(norm, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != LevelsFence {
		return nil, src, false
	}
	var content []string
	i := 1
	for ; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == LevelsFence {
			break
		}
		if !strings.HasPrefix(trimmed, "//") {
			return nil, src, false
		}
		content = append(content, strings.TrimPrefix(strings.TrimPrefix(trimmed, "//"), " "))
	}
	if i == len(lines) {
		return nil, src, false
	}
	rest := lines[i+1:]
	if len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}
	return []byte(strings.Join(content, "\n")), []byte(strings.Join(rest, "\n")), true
}

func formatLevelsEnvelope(envelopeJSON []byte) string {
	lines := strings.Split(strings.TrimSuffix(string(envelopeJSON), "\n"), "\n")
	var b strings.Builder
	b.WriteString(LevelsFence + "\n")
	for _, line := range lines {
		if line == "" {
			b.WriteString("//\n")
		} else {
			b.WriteString("// " + line + "\n")
		}
	}
	b.WriteString(LevelsFence + "\n\n")
	return b.String()
}
