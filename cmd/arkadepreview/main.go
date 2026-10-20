// SPDX-License-Identifier: GPL-2.0-or-later

// Command arkadepreview pushes a work-in-progress asset at a running Årkade
// cabinet's real peripherals: a WAV to the speakers, a PNG (or a sequence of
// them) to the 64×64 LED matrix, two lines and some custom 5×8 glyphs to the
// character LCDs. It is the reference client for the /api/preview routes.
//
// Usage:
//
//	arkadepreview [flags] <command> [args]
//
//	status                       what the cabinet is doing, and its panel size
//	frame <file.png>             one still on the LED matrix
//	frame -pattern               a built-in test pattern (no file needed)
//	anim <a.png> <b.png> ...     a short sequence
//	sound <file.wav>             play through the speakers
//	lcd "<line 1>" "<line 2>"    both character LCDs
//	cancel                       end whatever preview is up
//
// The cabinet's HTTP surface is loopback-only, so this talks to the near end
// of an SSH tunnel on localhost:8080. The token comes from -token or
// $ARKADE_HTTP_TOKEN.
//
//	ssh -L 8080:127.0.0.1:8080 <cabinet> &
//	export ARKADE_HTTP_TOKEN=<the cabinet's token>
//	arkadepreview frame -pattern
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "arkadepreview: "+err.Error())
		os.Exit(1)
	}
}

type options struct {
	host    string
	token   string
	ms      int
	gain    float64
	loop    bool
	delay   int
	pattern bool
	glyphs  glyphFlag
}

func run(args []string) error {
	var o options
	fs := flag.NewFlagSet("arkadepreview", flag.ContinueOnError)
	fs.StringVar(&o.host, "host", "localhost:8080", "cabinet HTTP address (the near end of an SSH tunnel)")
	fs.StringVar(&o.token, "token", os.Getenv("ARKADE_HTTP_TOKEN"), "control token (default $ARKADE_HTTP_TOKEN)")
	fs.IntVar(&o.ms, "ms", 0, "how long the preview holds, in milliseconds (0 = the cabinet's default)")
	fs.Float64Var(&o.gain, "gain", 0, "sound attenuation, 0..1 (0 = the cabinet's default)")
	fs.BoolVar(&o.loop, "loop", false, "repeat an animation until it times out")
	fs.IntVar(&o.delay, "delay", 0, "per-frame hold for an animation, in milliseconds (0 = the cabinet's default)")
	fs.BoolVar(&o.pattern, "pattern", false, "frame: send a built-in test pattern instead of a file")
	fs.Var(&o.glyphs, "glyph", "lcd: a custom glyph as <rune>=r0,r1,...,r7 (repeatable, max 8)")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("no command given")
	}

	c := &client{host: o.host, token: o.token, http: &http.Client{Timeout: 30 * time.Second}}
	switch cmd := rest[0]; cmd {
	case "status":
		return c.status()
	case "frame":
		return c.frame(o, rest[1:])
	case "anim":
		return c.anim(o, rest[1:])
	case "sound":
		return c.sound(o, rest[1:])
	case "lcd":
		return c.lcd(o, rest[1:])
	case "cancel":
		return c.cancel()
	default:
		fs.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

const usage = `arkadepreview pushes a work-in-progress asset at a running Årkade cabinet.

  arkadepreview [flags] status
  arkadepreview [flags] frame <file.png> | -pattern
  arkadepreview [flags] anim  <a.png> <b.png> ...
  arkadepreview [flags] sound <file.wav>
  arkadepreview [flags] lcd   "<line 1>" ["<line 2>"]
  arkadepreview [flags] cancel

The cabinet listens on loopback only, so run this against the near end of the
SSH tunnel (ssh -L 8080:127.0.0.1:8080 <cabinet>).

Flags:
`

type client struct {
	host  string
	token string
	http  *http.Client
}

func (c *client) url(path string) string {
	host := c.host
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	return strings.TrimSuffix(host, "/") + path
}

// do sends one request and returns the response body; a non-2xx becomes an
// error carrying the server's own message, which names the limit hit.
func (c *client) do(method, path, contentType string, body []byte) ([]byte, error) {
	if c.token == "" {
		return nil, fmt.Errorf("no control token: pass -token or set ARKADE_HTTP_TOKEN " +
			"(the cabinet's HTTP token)")
	}
	req, err := http.NewRequest(method, c.url(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w (is the SSH tunnel up?)", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (o options) query(extra ...string) string {
	var parts []string
	if o.ms > 0 {
		parts = append(parts, "ms="+strconv.Itoa(o.ms))
	}
	parts = append(parts, extra...)
	if len(parts) == 0 {
		return ""
	}
	return "?" + strings.Join(parts, "&")
}

// cabinetStatus is the subset of GET /api/status this tool reads.
type cabinetStatus struct {
	Slug             string `json:"slug"`
	StandbyState     string `json:"standbyState"`
	Version          string `json:"version"`
	DisplayCols      int    `json:"displayCols"`
	DisplayRows      int    `json:"displayRows"`
	LabPreviewActive bool   `json:"labPreviewActive"`
}

// fetchStatus reads the cabinet's status snapshot. It needs no token, so it
// doubles as a reachability probe.
func (c *client) fetchStatus() (cabinetStatus, error) {
	var st cabinetStatus
	resp, err := c.http.Get(c.url("/api/status"))
	if err != nil {
		return st, fmt.Errorf("GET /api/status: %w (is the SSH tunnel up?)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return st, fmt.Errorf("GET /api/status: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return st, fmt.Errorf("GET /api/status: %w", err)
	}
	if st.DisplayCols <= 0 || st.DisplayRows <= 0 {
		return st, fmt.Errorf("cabinet reported a %dx%d panel; is it running a build with the preview routes?",
			st.DisplayCols, st.DisplayRows)
	}
	return st, nil
}

func (c *client) status() error {
	st, err := c.fetchStatus()
	if err != nil {
		return err
	}
	fmt.Printf("cabinet %s  app=%s  standby=%s  panel=%dx%d  preview=%v\n",
		st.Version, orDash(st.Slug), st.StandbyState, st.DisplayCols, st.DisplayRows, st.LabPreviewActive)
	return nil
}

func (c *client) frame(o options, args []string) error {
	st, err := c.fetchStatus()
	if err != nil {
		return err
	}
	var rgb []byte
	switch {
	case o.pattern:
		if len(args) > 0 {
			return fmt.Errorf("frame -pattern takes no file argument")
		}
		rgb = testPattern(st.DisplayCols, st.DisplayRows)
	case len(args) == 1:
		rgb, err = pngToRGB(args[0], st.DisplayCols, st.DisplayRows)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("frame takes exactly one PNG file, or -pattern")
	}
	out, err := c.do(http.MethodPost, "/api/preview/frame"+o.query(), "application/octet-stream", rgb)
	if err != nil {
		return err
	}
	return report("frame", out)
}

func (c *client) anim(o options, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("anim takes one or more PNG files")
	}
	st, err := c.fetchStatus()
	if err != nil {
		return err
	}
	body, err := buildSequence(args, st.DisplayCols, st.DisplayRows, o.delay)
	if err != nil {
		return err
	}
	extra := []string{}
	if o.loop {
		extra = append(extra, "loop=1")
	}
	out, err := c.do(http.MethodPost, "/api/preview/frame"+o.query(extra...), "application/json", body)
	if err != nil {
		return err
	}
	return report("anim", out)
}

func (c *client) sound(o options, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("sound takes exactly one WAV file")
	}
	wav, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	extra := []string{}
	if o.gain > 0 {
		extra = append(extra, "gain="+strconv.FormatFloat(o.gain, 'g', -1, 64))
	}
	out, err := c.do(http.MethodPost, "/api/preview/sound"+o.query(extra...), "application/octet-stream", wav)
	if err != nil {
		return err
	}
	return report("sound", out)
}

func (c *client) lcd(o options, args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf(`lcd takes one or two lines, e.g. lcd "HELLO" "WORLD"`)
	}
	body, err := buildLCD(args, o.glyphs)
	if err != nil {
		return err
	}
	out, err := c.do(http.MethodPost, "/api/preview/lcd"+o.query(), "application/json", body)
	if err != nil {
		return err
	}
	return report("lcd", out)
}

func (c *client) cancel() error {
	out, err := c.do(http.MethodDelete, "/api/preview", "", nil)
	if err != nil {
		return err
	}
	return report("cancel", out)
}

// report prints the cabinet's answer, including the clamped timeout and accepted sizes.
func report(kind string, body []byte) error {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		fmt.Printf("%s: accepted\n", kind)
		return nil
	}
	parts := make([]string, 0, len(resp))
	for _, k := range []string{"timeoutMs", "frames", "bytes", "gain", "glyphs"} {
		if v, ok := resp[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	fmt.Printf("%s: accepted %s\n", kind, strings.Join(parts, " "))
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// sequenceBody is the JSON envelope POST /api/preview/frame takes for an animation.
type sequenceBody struct {
	Frames []sequenceFrame `json:"frames"`
}

type sequenceFrame struct {
	RGB     string `json:"rgb"`
	DelayMs int    `json:"delayMs,omitempty"`
}

// buildSequence turns a list of PNG paths into the animation envelope.
func buildSequence(paths []string, cols, rows, delayMs int) ([]byte, error) {
	if delayMs < 0 {
		return nil, fmt.Errorf("-delay must not be negative")
	}
	body := sequenceBody{Frames: make([]sequenceFrame, 0, len(paths))}
	for _, path := range paths {
		rgb, err := pngToRGB(path, cols, rows)
		if err != nil {
			return nil, err
		}
		body.Frames = append(body.Frames, sequenceFrame{
			RGB:     base64.StdEncoding.EncodeToString(rgb),
			DelayMs: delayMs,
		})
	}
	return json.Marshal(body)
}

// lcdBody is the JSON body POST /api/preview/lcd takes.
type lcdBody struct {
	Line1  string           `json:"line1"`
	Line2  string           `json:"line2"`
	Glyphs map[string][]int `json:"glyphs,omitempty"`
}

// buildLCD assembles an LCD request from one or two lines plus any -glyph
// flags.
func buildLCD(lines []string, glyphs glyphFlag) ([]byte, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("lcd needs at least one line")
	}
	body := lcdBody{Line1: lines[0]}
	if len(lines) > 1 {
		body.Line2 = lines[1]
	}
	if len(glyphs.entries) > 0 {
		body.Glyphs = map[string][]int{}
		for _, e := range glyphs.entries {
			body.Glyphs[e.key] = e.rows
		}
	}
	return json.Marshal(body)
}

type glyphEntry struct {
	key  string
	rows []int
}

type glyphFlag struct{ entries []glyphEntry }

func (g *glyphFlag) String() string { return fmt.Sprintf("%d glyph(s)", len(g.entries)) }

// Set parses "<rune>=r0,r1,...,r7", the eight 5-bit rows of an entry in
// internal/hardware/pi/glyphs.go. Rows accept decimal or 0x-prefixed hex.
func (g *glyphFlag) Set(v string) error {
	key, rows, ok := strings.Cut(v, "=")
	if !ok || key == "" {
		return fmt.Errorf("want <rune>=r0,r1,...,r7, got %q", v)
	}
	if len([]rune(key)) != 1 {
		return fmt.Errorf("glyph key %q must be exactly one character", key)
	}
	fields := strings.Split(rows, ",")
	if len(fields) != 8 {
		return fmt.Errorf("glyph %q has %d rows, want exactly 8", key, len(fields))
	}
	out := make([]int, 0, 8)
	for i, f := range fields {
		n, err := strconv.ParseInt(strings.TrimSpace(f), 0, 32)
		if err != nil {
			return fmt.Errorf("glyph %q row %d: %v", key, i, err)
		}
		if n < 0 || n > 0x1f {
			return fmt.Errorf("glyph %q row %d is %d, want 0..31 (5 pixels wide)", key, i, n)
		}
		out = append(out, int(n))
	}
	g.entries = append(g.entries, glyphEntry{key: key, rows: out})
	return nil
}

// pngToRGB reads a PNG as the cols×rows×3 buffer the frame route wants. A
// smaller image is centred on black, never scaled; a larger one is an error.
func pngToRGB(path string, cols, rows int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return imageToRGB(img, cols, rows, path)
}

// imageToRGB is pngToRGB without the file.
func imageToRGB(img image.Image, cols, rows int, name string) ([]byte, error) {
	b := img.Bounds()
	if b.Dx() > cols || b.Dy() > rows {
		return nil, fmt.Errorf("%s is %dx%d, larger than the cabinet's %dx%d panel",
			name, b.Dx(), b.Dy(), cols, rows)
	}
	offX := (cols - b.Dx()) / 2
	offY := (rows - b.Dy()) / 2

	out := make([]byte, cols*rows*3)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				continue // transparent stays black: the panel has no alpha
			}
			// RGBA() is premultiplied 16-bit; the panel takes straight 8-bit.
			i := ((y+offY)*cols + x + offX) * 3
			out[i] = byte((r * 0xffff / a) >> 8)
			out[i+1] = byte((g * 0xffff / a) >> 8)
			out[i+2] = byte((bl * 0xffff / a) >> 8)
		}
	}
	return out, nil
}

// testPattern builds a wiring check frame: a red/green ramp over the whole
// panel, 4×4 corner markers and a one-pixel white border.
func testPattern(cols, rows int) []byte {
	out := make([]byte, cols*rows*3)
	set := func(x, y int, r, g, b byte) {
		if x < 0 || y < 0 || x >= cols || y >= rows {
			return
		}
		i := (y*cols + x) * 3
		out[i], out[i+1], out[i+2] = r, g, b
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			// A stuck or swapped channel shows against the ramps.
			set(x, y, byte(x*255/max(1, cols-1)), byte(y*255/max(1, rows-1)), 40)
		}
	}
	// TL red, TR green, BL blue, BR white.
	for dy := 0; dy < 4; dy++ {
		for dx := 0; dx < 4; dx++ {
			set(dx, dy, 255, 0, 0)
			set(cols-1-dx, dy, 0, 255, 0)
			set(dx, rows-1-dy, 0, 0, 255)
			set(cols-1-dx, rows-1-dy, 255, 255, 255)
		}
	}
	for x := 0; x < cols; x++ {
		set(x, 0, 255, 255, 255)
		set(x, rows-1, 255, 255, 255)
	}
	for y := 0; y < rows; y++ {
		set(0, y, 255, 255, 255)
		set(cols-1, y, 255, 255, 255)
	}
	return out
}
