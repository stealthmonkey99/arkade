// SPDX-License-Identifier: GPL-2.0-or-later

package packfile

import (
	"fmt"
	"strconv"
	"strings"
)

// Format parses src, so an unparseable pack is refused with [Parse]'s own
// errors, and returns it re-rendered in the canonical shape: one blank line
// before `legend` and before each level, two-space indentation inside a level,
// legend characters column-aligned, and every comment kept where it was.
// Formatting an already-canonical pack is a no-op.
func Format(name string, src []byte) ([]byte, error) {
	if _, err := Parse(name, src); err != nil {
		return nil, err
	}
	return []byte(formatDoc(readDoc(src))), nil
}

// fmtDoc holds a pack file as text with its attached comment lines. Fields keep
// the file's spelling because a value's type comes from its spelling, which a
// decoded value would lose.
type fmtDoc struct {
	pack    string
	header  []string
	legend  fmtLegend
	legends []fmtLegend // the extra named ones, in declaration order
	levels  []fmtLevel
	trailer []string
}

type fmtLegend struct {
	name    string // "" for the pack's main legend
	note    []string
	entries []fmtLegendEntry
}

// fmtLegendEntry's char is the token as written ("#", `\s`, `\\`), and value is
// the optional parameter, "" when absent.
type fmtLegendEntry struct {
	char, name, value string
	note              []string
}

type fmtLevel struct {
	name   string
	note   []string
	meta   []fmtMeta
	rows   []string
	exits  []fmtDirective
	ents   []fmtDirective
	layers []fmtLayer
}

type fmtMeta struct {
	key, value string
}

// fmtDirective is one `ent` or `exit` line, held as argument tokens.
type fmtDirective struct {
	args []string
	note []string
}

type fmtLayer struct {
	name   string
	legend string
	note   []string
	rows   []string
}

// readDoc builds the model from a pack file's bytes, keeping the comments and
// value spellings [Parse] discards. Callers run [Parse] first, since a model of
// a broken file would be a guess.
func readDoc(src []byte) *fmtDoc {
	d := &fmtDoc{}
	var pending []string
	take := func() []string {
		out := pending
		pending = nil
		return out
	}

	var lv *fmtLevel
	var layer *fmtLayer
	var legend *fmtLegend

	for _, raw := range strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			pending = append(pending, line)
			continue
		}
		keyword, rest := split2(line)
		switch {
		case keyword == "pack":
			d.pack, d.header = rest, take()
			legend = nil
		case keyword == "legend" && rest == "":
			d.legend.note = take()
			legend = &d.legend
		case keyword == "legend":
			d.legends = append(d.legends, fmtLegend{name: rest, note: take()})
			legend = &d.legends[len(d.legends)-1]
		case keyword == "level":
			legend = nil
			name := rest
			if unquoted, err := strconv.Unquote(rest); err == nil && strings.HasPrefix(rest, `"`) {
				name = unquoted
			}
			d.levels = append(d.levels, fmtLevel{name: name, note: take()})
			lv, layer = &d.levels[len(d.levels)-1], nil
		case keyword == "layer" && lv != nil:
			name, lgName := split2(rest)
			lv.layers = append(lv.layers, fmtLayer{name: name, legend: lgName, note: take()})
			layer = &lv.layers[len(lv.layers)-1]
		case keyword == "ent" && lv != nil:
			lv.ents = append(lv.ents, fmtDirective{args: fields(rest), note: take()})
		case keyword == "exit" && lv != nil:
			lv.exits = append(lv.exits, fmtDirective{args: fields(rest), note: take()})
		case legend != nil:
			char, rest := split2(line)
			name, value := split2(rest)
			legend.entries = append(legend.entries,
				fmtLegendEntry{char: char, name: name, value: value, note: take()})
		case strings.HasPrefix(line, "|") && lv != nil:
			row := strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
			if layer != nil {
				layer.rows = append(layer.rows, row)
			} else {
				lv.rows = append(lv.rows, row)
			}
		case lv != nil:
			lv.meta = append(lv.meta, fmtMeta{key: keyword, value: rest})
		}
	}
	d.trailer = pending
	return d
}

// fields splits like the parser, falling back to whitespace splitting on an
// unterminated quote.
func fields(rest string) []string {
	if toks, err := splitTokens(rest); err == nil {
		return toks
	}
	return strings.Fields(rest)
}

// formatDoc renders d in the canonical form documented on [Format].
func formatDoc(d *fmtDoc) string {
	var b strings.Builder
	writeNote(&b, d.header, "")
	fmt.Fprintf(&b, "pack %s\n", d.pack)

	writeLegend(&b, &d.legend)
	for i := range d.legends {
		writeLegend(&b, &d.legends[i])
	}

	for i := range d.levels {
		lv := &d.levels[i]
		b.WriteString("\n")
		writeNote(&b, lv.note, "")
		fmt.Fprintf(&b, "level %s\n", levelNameToken(lv.name))
		for _, m := range lv.meta {
			fmt.Fprintf(&b, "  %s %s\n", m.key, m.value)
		}
		for _, row := range lv.rows {
			fmt.Fprintf(&b, "  |%s|\n", row)
		}
		writeDirectives(&b, "exit", lv.exits)
		writeDirectives(&b, "ent", lv.ents)
		for j := range lv.layers {
			ly := &lv.layers[j]
			writeNote(&b, ly.note, "  ")
			fmt.Fprintf(&b, "  layer %s", ly.name)
			if ly.legend != "" {
				fmt.Fprintf(&b, " %s", ly.legend)
			}
			b.WriteString("\n")
			for _, row := range ly.rows {
				fmt.Fprintf(&b, "  |%s|\n", row)
			}
		}
	}

	if len(d.trailer) > 0 {
		b.WriteString("\n")
		writeNote(&b, d.trailer, "")
	}
	return b.String()
}

// writeLegend renders one legend block after a blank line, padding the
// character column and, when any entry has a parameter, the name column.
func writeLegend(b *strings.Builder, lg *fmtLegend) {
	b.WriteString("\n")
	writeNote(b, lg.note, "")
	b.WriteString("legend")
	if lg.name != "" {
		b.WriteString(" " + lg.name)
	}
	b.WriteString("\n")

	charW, nameW := 0, 0
	for _, e := range lg.entries {
		if len(e.char) > charW {
			charW = len(e.char)
		}
		if len(e.name) > nameW {
			nameW = len(e.name)
		}
	}
	for _, e := range lg.entries {
		writeNote(b, e.note, "  ")
		line := fmt.Sprintf("  %-*s %-*s %s", charW, e.char, nameW, e.name, e.value)
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
}

func writeDirectives(b *strings.Builder, keyword string, ds []fmtDirective) {
	for _, d := range ds {
		writeNote(b, d.note, "  ")
		fmt.Fprintf(b, "  %s %s\n", keyword, strings.Join(d.args, " "))
	}
}

// writeNote emits a comment block, followed by a blank line unless indented.
func writeNote(b *strings.Builder, note []string, indent string) {
	for _, line := range note {
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(note) > 0 && indent == "" {
		b.WriteString("\n")
	}
}

// levelNameToken quotes a name that holds whitespace, a quote or a backslash,
// or is empty, so an empty name still reaches the parser as an error.
func levelNameToken(name string) string {
	if name == "" || strings.ContainsAny(name, " \t\"\\") {
		return strconv.Quote(name)
	}
	return name
}
