// SPDX-License-Identifier: GPL-2.0-or-later

package packfile

import "strings"

// EnvelopeFence is the sentinel line that opens and closes a pack file's
// optional envelope block. A pack whose first line is exactly this text has it
// read as the start of an envelope.
const EnvelopeFence = "// --- arkade ---"

// SplitEnvelope reports whether src begins with an envelope block: its
// first line is exactly [EnvelopeFence], followed by one or more `//` comment
// lines, closed by a second [EnvelopeFence] line. When it does, json is those
// comment lines with the leading "//" and one following space stripped, joined
// with "\n" and left undecoded. body is everything after the closing fence,
// less one immediately following blank line.
//
// ok is false for anything else, including a truncated or malformed fence. That
// is not an error: body is then src, unchanged.
func SplitEnvelope(src []byte) (json, body []byte, ok bool) {
	norm := strings.ReplaceAll(string(src), "\r\n", "\n")
	lines := strings.Split(norm, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != EnvelopeFence {
		return nil, src, false
	}
	var content []string
	i := 1
	for ; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == EnvelopeFence {
			break
		}
		if !strings.HasPrefix(trimmed, "//") {
			// unterminated fence: the block ran into pack syntax
			return nil, src, false
		}
		content = append(content, strings.TrimPrefix(strings.TrimPrefix(trimmed, "//"), " "))
	}
	if i == len(lines) {
		return nil, src, false // the fence never closed
	}
	rest := lines[i+1:]
	if len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}
	return []byte(strings.Join(content, "\n")), []byte(strings.Join(rest, "\n")), true
}

// FormatEnvelope renders envelopeJSON, one JSON document in any layout, as the
// fenced comment block [SplitEnvelope] reads back: one `//` line per line of
// the document between two [EnvelopeFence] lines, then a blank line.
func FormatEnvelope(envelopeJSON []byte) string {
	lines := strings.Split(strings.TrimSuffix(string(envelopeJSON), "\n"), "\n")
	var b strings.Builder
	b.WriteString(EnvelopeFence + "\n")
	for _, line := range lines {
		if line == "" {
			b.WriteString("//\n")
		} else {
			b.WriteString("// " + line + "\n")
		}
	}
	b.WriteString(EnvelopeFence + "\n\n")
	return b.String()
}
