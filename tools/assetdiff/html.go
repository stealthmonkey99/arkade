// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stealthmonkey99/arkade/assetfile"
)

// runReport writes the -html page. pairs is the flat old/new argument list.
func runReport(path string, pairs []string, leftName, rightName string) error {
	sections := make([]htmlSection, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		s, err := buildSection(sectionLabel(pairs[i+1]), pairs[i], pairs[i+1])
		if err != nil {
			return err
		}
		sections = append(sections, s)
	}
	if err := writeHTML(path, sections, leftName, rightName); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "assetdiff: wrote %s\n", path)
	return nil
}

// sectionLabel names a section after the cartridge, not its assets directory.
func sectionLabel(dir string) string {
	clean := filepath.Clean(dir)
	if filepath.Base(clean) == "assets" {
		if parent := filepath.Base(filepath.Dir(clean)); parent != "." && parent != string(filepath.Separator) {
			return parent
		}
	}
	return filepath.Base(clean)
}

type htmlRow struct {
	Name    string
	Verdict string
	Detail  string
	Old     []byte
	New     []byte
	Recipe  string
}

type htmlSection struct {
	Label string
	Rows  []htmlRow
}

// buildSection pairs two directories; an asset missing on either side is a row, not skipped.
func buildSection(label, left, right string) (htmlSection, error) {
	l, err := assetStems(left)
	if err != nil {
		return htmlSection{}, err
	}
	r, err := assetStems(right)
	if err != nil {
		return htmlSection{}, err
	}
	stems := make([]string, 0, len(l))
	for s := range l {
		stems = append(stems, s)
	}
	for s := range r {
		if _, ok := l[s]; !ok {
			stems = append(stems, s)
		}
	}
	sort.Strings(stems)

	out := htmlSection{Label: label}
	for _, s := range stems {
		ln, lok := l[s]
		rn, rok := r[s]
		row := htmlRow{Name: s}
		if lok {
			row.Old, _ = os.ReadFile(filepath.Join(left, ln))
		}
		if rok {
			row.New, _ = os.ReadFile(filepath.Join(right, rn))
			row.Recipe = recipeOf(rn, row.New)
		}
		switch {
		case !rok:
			row.Verdict, row.Detail = "gone", "no longer present"
		case !lok:
			row.Verdict, row.Detail = "new", "added by this change"
		default:
			if err := assetfile.SamePayload(rn, row.Old, row.New); err != nil {
				row.Verdict, row.Detail = "diff", err.Error()
			} else {
				row.Verdict = "same"
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// recipeOf returns the asset's indented recipe, or "" when it carries none.
func recipeOf(name string, data []byte) string {
	env, err := assetfile.ReadBytes(name, data)
	if err != nil || env == nil || len(env.Recipe) == 0 {
		return ""
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, env.Recipe, "", "  "); err != nil {
		return string(env.Recipe)
	}
	return pretty.String()
}

// dataURI sniffs the media type from the bytes, not the name.
func dataURI(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	mime := "application/octet-stream"
	switch {
	case strings.HasPrefix(string(b[:min(4, len(b))]), "RIFF"):
		mime = "audio/wav"
	case len(b) > 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		mime = "image/png"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
}

func player(b []byte) string {
	uri := dataURI(b)
	if uri == "" {
		return `<span class="none">—</span>`
	}
	if strings.HasPrefix(uri, "data:image/png") {
		return fmt.Sprintf(`<img src="%s" alt="">`, uri)
	}
	return fmt.Sprintf(`<audio controls preload="none" src="%s"></audio>
      <span class="size">%s</span>`, uri, size(len(b)))
}

func size(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.0f KB", float64(n)/1024)
}

func writeHTML(path string, sections []htmlSection, leftName, rightName string) error {
	var b strings.Builder
	b.WriteString(`<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Asset comparison</title>
<style>
  :root { color-scheme: light dark; --bg:#fff; --fg:#181818; --dim:#666; --line:#e2e2e2; --card:#fafafa;
          --same:#137333; --diff:#b3261e; --other:#8a5a00; }
  @media (prefers-color-scheme: dark) { :root {
    --bg:#16181c; --fg:#e8e8e8; --dim:#9aa0a6; --line:#2c2f36; --card:#1d2026;
    --same:#6dd58c; --diff:#f2b8b5; --other:#e6c068; } }
  body { margin:0; padding:24px 16px 64px; background:var(--bg); color:var(--fg);
         font:15px/1.5 ui-sans-serif,-apple-system,Segoe UI,Roboto,sans-serif; }
  .wrap { max-width:1000px; margin:0 auto; }
  h1 { font-size:20px; margin:0 0 4px; }
  .lede { color:var(--dim); margin:0 0 28px; }
  h2 { font-size:16px; margin:32px 0 8px; padding-bottom:6px; border-bottom:1px solid var(--line); }
  .row { display:grid; grid-template-columns:minmax(120px,1fr) 2fr 2fr; gap:12px; align-items:center;
         padding:10px 0; border-bottom:1px solid var(--line); }
  .head { font-size:12px; letter-spacing:.06em; text-transform:uppercase; color:var(--dim); border-bottom:none; }
  .name { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:13px; word-break:break-all; }
  .tag { display:inline-block; margin-top:3px; font-size:11px; letter-spacing:.05em; text-transform:uppercase; }
  .same .tag { color:var(--same); } .diff .tag { color:var(--diff); }
  .new .tag, .gone .tag { color:var(--other); }
  audio { width:100%; max-width:300px; vertical-align:middle; }
  img { image-rendering:pixelated; width:64px; height:64px; border:1px solid var(--line); background:var(--card); }
  .size { display:block; font-size:11px; color:var(--dim); }
  .none { color:var(--dim); }
  .detail { grid-column:1/-1; font-size:12px; color:var(--diff); }
  details { grid-column:1/-1; margin-top:4px; }
  summary { cursor:pointer; font-size:12px; color:var(--dim); }
  pre { background:var(--card); border:1px solid var(--line); border-radius:6px; padding:10px;
        overflow-x:auto; font-size:12px; margin:8px 0 0; }
  @media (max-width:720px) { .row { grid-template-columns:1fr; } .head { display:none; } }
</style>
<div class="wrap">
`)
	fmt.Fprintf(&b, "<h1>Asset comparison</h1>\n<p class=\"lede\">%s versus %s. Each row plays both versions; the payload verdict is assetdiff's, the ears are yours.</p>\n",
		html.EscapeString(leftName), html.EscapeString(rightName))

	for _, s := range sections {
		fmt.Fprintf(&b, "<h2>%s</h2>\n", html.EscapeString(s.Label))
		fmt.Fprintf(&b, `<div class="row head"><div>Asset</div><div>%s</div><div>%s</div></div>`+"\n",
			html.EscapeString(leftName), html.EscapeString(rightName))
		for _, r := range s.Rows {
			fmt.Fprintf(&b, `<div class="row %s"><div class="name">%s<span class="tag">%s</span></div><div>%s</div><div>%s</div>`,
				r.Verdict, html.EscapeString(r.Name), r.Verdict, player(r.Old), player(r.New))
			if r.Detail != "" && r.Verdict == "diff" {
				fmt.Fprintf(&b, `<div class="detail">%s</div>`, html.EscapeString(r.Detail))
			}
			if r.Recipe != "" {
				fmt.Fprintf(&b, "<details><summary>recipe — the parameters this file now carries</summary><pre>%s</pre></details>",
					html.EscapeString(r.Recipe))
			}
			b.WriteString("</div>\n")
		}
	}
	b.WriteString("</div>\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
