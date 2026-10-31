// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type fixture struct {
	t    *testing.T
	srv  *httptest.Server
	cart string // the first cartridge's directory
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeCart(t *testing.T, dir, slug, title string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "manifest.json"), `{"slug":"`+slug+`","title":"`+title+`"}`)
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n")
	writeFile(t, filepath.Join(dir, "assets", "sprites", "hero.png"), "png-bytes")
	writeFile(t, filepath.Join(dir, ".git", "config"), "secret")
}

func serve(t *testing.T, root string) *fixture {
	t.Helper()
	carts, err := findCarts(root)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = newBridge(config{
		carts:   carts,
		token:   testToken,
		origins: append(append([]string{}, defaultOrigins...), "http://localhost:5173"),
		port:    srv.Listener.Addr().(*net.TCPAddr).Port,
	})
	srv.Start()
	t.Cleanup(srv.Close)
	return &fixture{t: t, srv: srv, cart: carts[0].dir}
}

func singleCart(t *testing.T) *fixture {
	dir := t.TempDir()
	makeCart(t, dir, "demo", "Demo")
	return serve(t, dir)
}

func (f *fixture) do(method, target string, body io.Reader, mutate ...func(*http.Request)) *http.Response {
	f.t.Helper()
	req, err := http.NewRequest(method, f.srv.URL+target, body)
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	for _, m := range mutate {
		m(req)
	}
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func expect(t *testing.T, resp *http.Response, status int) []byte {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		t.Fatalf("%s %s: status %d, want %d (%s)", resp.Request.Method, resp.Request.URL, resp.StatusCode, status, body)
	}
	return body
}

func TestTokenRequired(t *testing.T) {
	f := singleCart(t)
	expect(t, f.do("GET", "/api/bridge/carts", nil, func(r *http.Request) { r.Header.Del("Authorization") }), 401)
	expect(t, f.do("GET", "/api/bridge/carts", nil, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+strings.Repeat("0", 64)) }), 401)
	expect(t, f.do("GET", "/api/bridge/carts", nil, func(r *http.Request) { r.Header.Set("Authorization", testToken) }), 401)
	expect(t, f.do("GET", "/api/bridge/carts?token="+testToken, nil, func(r *http.Request) { r.Header.Del("Authorization") }), 401)
	expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path=assets/x.png", strings.NewReader("x"), func(r *http.Request) { r.Header.Del("Authorization") }), 401)
	expect(t, f.do("GET", "/api/bridge/events?token=wrong", nil, func(r *http.Request) { r.Header.Del("Authorization") }), 401)

	body := expect(t, f.do("GET", "/api/bridge/health", nil, func(r *http.Request) { r.Header.Del("Authorization") }), 200)
	var h map[string]any
	if err := json.Unmarshal(body, &h); err != nil || h["bridge"] != "assetbridge" || h["ok"] != true || h["version"] == "" {
		t.Fatalf("health = %s", body)
	}
}

func TestHostChecked(t *testing.T) {
	f := singleCart(t)
	port := f.srv.Listener.Addr().(*net.TCPAddr).Port
	for _, host := range []string{"evil.example", "evil.example:" + strconv.Itoa(port), "127.0.0.1:1", "0.0.0.0:" + strconv.Itoa(port)} {
		expect(t, f.do("GET", "/api/bridge/health", nil, func(r *http.Request) { r.Host = host }), 403)
	}
	expect(t, f.do("GET", "/api/bridge/health", nil, func(r *http.Request) { r.Host = "localhost:" + strconv.Itoa(port) }), 200)
}

func TestCORS(t *testing.T) {
	f := singleCart(t)
	for _, origin := range []string{"https://evil.example", "https://arkade.build.evil.example", "null", "http://arkade.build"} {
		resp := f.do("GET", "/api/bridge/health", nil, func(r *http.Request) { r.Header.Set("Origin", origin) })
		expect(t, resp, 403)
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %s got ACAO %q", origin, got)
		}
	}
	for _, origin := range []string{"https://arkade.build", "https://www.arkade.build", "http://localhost:5173"} {
		resp := f.do("GET", "/api/bridge/carts", nil, func(r *http.Request) { r.Header.Set("Origin", origin) })
		expect(t, resp, 200)
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %s got ACAO %q", origin, got)
		}
		if got := resp.Header.Get("Vary"); got != "Origin" {
			t.Errorf("Vary = %q", got)
		}
	}
	resp := f.do("GET", "/api/bridge/health", nil)
	expect(t, resp, 200)
	if resp.Header.Get("Access-Control-Allow-Origin") != "" || resp.Header.Get("Vary") != "Origin" {
		t.Errorf("no-origin request: ACAO %q, Vary %q", resp.Header.Get("Access-Control-Allow-Origin"), resp.Header.Get("Vary"))
	}
}

func TestPreflight(t *testing.T) {
	f := singleCart(t)
	pre := func(origin string, pna bool) *http.Response {
		return f.do("OPTIONS", "/api/bridge/file?cart=demo&path=assets/x.png", nil, func(r *http.Request) {
			r.Header.Del("Authorization")
			r.Header.Set("Origin", origin)
			r.Header.Set("Access-Control-Request-Method", "PUT")
			r.Header.Set("Access-Control-Request-Headers", "authorization")
			if pna {
				r.Header.Set("Access-Control-Request-Private-Network", "true")
			}
		})
	}
	resp := pre("https://arkade.build", true)
	expect(t, resp, 204)
	h := resp.Header
	if h.Get("Access-Control-Allow-Origin") != "https://arkade.build" ||
		h.Get("Access-Control-Allow-Private-Network") != "true" ||
		!strings.Contains(h.Get("Access-Control-Allow-Methods"), "PUT") ||
		!strings.Contains(h.Get("Access-Control-Allow-Methods"), "DELETE") ||
		!strings.Contains(strings.ToLower(h.Get("Access-Control-Allow-Headers")), "authorization") ||
		h.Get("Vary") != "Origin" {
		t.Fatalf("preflight headers: %v", h)
	}

	resp = pre("https://arkade.build", false)
	expect(t, resp, 204)
	if resp.Header.Get("Access-Control-Allow-Private-Network") != "" {
		t.Error("PNA header sent without being asked")
	}

	resp = pre("https://evil.example", true)
	expect(t, resp, 403)
	if resp.Header.Get("Access-Control-Allow-Origin") != "" || resp.Header.Get("Access-Control-Allow-Private-Network") != "" {
		t.Errorf("disallowed preflight leaked CORS headers: %v", resp.Header)
	}
}

func TestPathTraversalRefused(t *testing.T) {
	f := singleCart(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.png"), "outside")
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(f.cart, "assets", "link.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.cart, "assets", "out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "missing"), filepath.Join(f.cart, "assets", "dangling")); err != nil {
		t.Fatal(err)
	}

	bad := []string{
		"../secret.png",
		"assets/../../secret.png",
		"%2e%2e/secret.png",
		"assets/%2e%2e/%2e%2e/secret.png",
		"assets%2f..%2f..%2fsecret.png",
		"/etc/passwd",
		"%2fetc%2fpasswd",
		".git/config",
		"assets/.hidden.png",
		"assets/./sprites/hero.png",
		"assets//sprites/hero.png",
		"assets\\..\\main.go",
		"assets/link.png",
		"assets/out/secret.png",
		"assets/out/new.png",
		"assets/dangling/new.png",
	}
	for _, p := range bad {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			resp := f.do(method, "/api/bridge/file?cart=demo&path="+p, strings.NewReader("pwned"))
			if resp.StatusCode != 400 && resp.StatusCode != 403 {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("%s %s: status %d (%s)", method, p, resp.StatusCode, body)
			}
		}
		resp := f.do("GET", "/api/bridge/files?cart=demo&dir="+p, nil)
		if resp.StatusCode != 400 && resp.StatusCode != 403 && resp.StatusCode != 404 {
			t.Errorf("list %s: status %d", p, resp.StatusCode)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "secret.png")); string(b) != "outside" {
		t.Errorf("outside file changed: %q", b)
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 1 {
		t.Errorf("outside dir gained files: %v", entries)
	}

	body := expect(t, f.do("GET", "/api/bridge/files?cart=demo&dir=assets", nil), 200)
	if strings.Contains(string(body), "link.png") || strings.Contains(string(body), `"out"`) || strings.Contains(string(body), "dangling") {
		t.Errorf("listing shows escaping symlinks: %s", body)
	}
}

func TestSymlinkInsideCartridgeIsFollowed(t *testing.T) {
	f := singleCart(t)
	if err := os.Symlink("sprites/hero.png", filepath.Join(f.cart, "assets", "alias.png")); err != nil {
		t.Fatal(err)
	}
	if got := expect(t, f.do("GET", "/api/bridge/file?cart=demo&path=assets/alias.png", nil), 200); string(got) != "png-bytes" {
		t.Errorf("read through alias = %q", got)
	}
	if err := os.Symlink("../main.go", filepath.Join(f.cart, "assets", "code.png")); err != nil {
		t.Fatal(err)
	}
	expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path=assets/code.png", strings.NewReader("x")), 403)
}

func TestWriteAllowlist(t *testing.T) {
	f := singleCart(t)
	for _, p := range []string{"main.go", "manifest.json", "new.png", "assets/x.go", "assets/x.json", "assets/x.png.txt", "assets/sprites"} {
		expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path="+p, strings.NewReader("x")), 403)
		expect(t, f.do("DELETE", "/api/bridge/file?cart=demo&path="+p, nil), 403)
	}
	if b, _ := os.ReadFile(filepath.Join(f.cart, "main.go")); string(b) != "package main\n" {
		t.Errorf("main.go changed: %q", b)
	}

	big := bytes.Repeat([]byte{1}, maxWriteBytes+1)
	expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path=assets/big.png", bytes.NewReader(big)), 413)
	expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path=assets/big.png", io.NopCloser(bytes.NewReader(big))), 413) // chunked, no Content-Length
	if _, err := os.Stat(filepath.Join(f.cart, "assets", "big.png")); err == nil {
		t.Error("oversize write landed")
	}
	assertNoTemp(t, f.cart)

	for _, p := range []string{"assets/a.wav", "assets/g/x.arkadeglyph.json", "assets/l/x.arkadelights.json", "assets/x.levels", "assets/x.arkadelevels", "assets/UP.PNG"} {
		expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path="+p, strings.NewReader("ok")), 204)
	}
	expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path=assets/max.png", bytes.NewReader(big[:maxWriteBytes])), 204)
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), ".assetbridge") {
			t.Errorf("temp file left behind: %s", p)
		}
		return nil
	})
}

func TestAtomicWriteAndRead(t *testing.T) {
	f := singleCart(t)
	expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path=assets/new/deep/tile.png", strings.NewReader("first")), 204)
	expect(t, f.do("PUT", "/api/bridge/file?cart=demo&path=assets/new/deep/tile.png", strings.NewReader("second")), 204)
	if got := expect(t, f.do("GET", "/api/bridge/file?cart=demo&path=assets/new/deep/tile.png", nil), 200); string(got) != "second" {
		t.Errorf("read back %q", got)
	}
	if got := expect(t, f.do("GET", "/api/bridge/file?cart=demo&path=main.go", nil), 200); string(got) != "package main\n" {
		t.Errorf("main.go read %q", got)
	}
	expect(t, f.do("GET", "/api/bridge/file?cart=demo&path=assets/sprites", nil), 404)
	expect(t, f.do("GET", "/api/bridge/file?cart=demo&path=assets/nope.png", nil), 404)
	expect(t, f.do("GET", "/api/bridge/file?cart=nope&path=main.go", nil), 404)
	assertNoTemp(t, f.cart)
}

func TestDelete(t *testing.T) {
	f := singleCart(t)
	expect(t, f.do("DELETE", "/api/bridge/file?cart=demo&path=assets/sprites/hero.png", nil), 204)
	if _, err := os.Stat(filepath.Join(f.cart, "assets", "sprites", "hero.png")); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	expect(t, f.do("DELETE", "/api/bridge/file?cart=demo&path=assets/sprites/hero.png", nil), 404)
}

func TestList(t *testing.T) {
	f := singleCart(t)
	writeFile(t, filepath.Join(f.cart, "assets", ".DS_Store"), "x")
	var root []fileEntry
	json.Unmarshal(expect(t, f.do("GET", "/api/bridge/files?cart=demo&dir=", nil), 200), &root)
	names := []string{}
	for _, e := range root {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "assets,main.go,manifest.json" {
		t.Errorf("root listing = %v", names)
	}

	var assets []fileEntry
	json.Unmarshal(expect(t, f.do("GET", "/api/bridge/files?cart=demo&dir=assets", nil), 200), &assets)
	if len(assets) != 1 || assets[0].Name != "sprites" || !assets[0].Dir {
		t.Errorf("assets listing = %+v", assets)
	}

	var sprites []fileEntry
	json.Unmarshal(expect(t, f.do("GET", "/api/bridge/files?cart=demo&dir=assets/sprites", nil), 200), &sprites)
	info, _ := os.Stat(filepath.Join(f.cart, "assets", "sprites", "hero.png"))
	if len(sprites) != 1 || sprites[0] != (fileEntry{Name: "hero.png", Size: 9, Mtime: info.ModTime().UnixMilli()}) {
		t.Errorf("sprites listing = %+v", sprites)
	}

	writeFile(t, filepath.Join(f.cart, "assets", "empty", ".keep"), "")
	if got := expect(t, f.do("GET", "/api/bridge/files?cart=demo&dir=assets/empty", nil), 200); strings.TrimSpace(string(got)) != "[]" {
		t.Errorf("empty listing = %s", got)
	}
	expect(t, f.do("GET", "/api/bridge/files?cart=demo&dir=nope", nil), 404)
}

func TestCartsSingle(t *testing.T) {
	f := singleCart(t)
	got := strings.TrimSpace(string(expect(t, f.do("GET", "/api/bridge/carts", nil), 200)))
	if got != `[{"slug":"demo","title":"Demo"}]` {
		t.Errorf("carts = %s", got)
	}
}

func TestCartsLibrary(t *testing.T) {
	lib := t.TempDir()
	makeCart(t, filepath.Join(lib, "b-dir"), "beta", "Beta")
	makeCart(t, filepath.Join(lib, "a-dir"), "alpha", "")
	writeFile(t, filepath.Join(lib, "notes", "readme.txt"), "not a cartridge")
	makeCart(t, filepath.Join(lib, ".hidden"), "hidden", "Hidden")
	f := serve(t, lib)

	got := strings.TrimSpace(string(expect(t, f.do("GET", "/api/bridge/carts", nil), 200)))
	if got != `[{"slug":"alpha","title":"alpha"},{"slug":"beta","title":"Beta"}]` {
		t.Errorf("carts = %s", got)
	}
	expect(t, f.do("PUT", "/api/bridge/file?cart=beta&path=assets/b.png", strings.NewReader("b")), 204)
	if b, _ := os.ReadFile(filepath.Join(lib, "b-dir", "assets", "b.png")); string(b) != "b" {
		t.Errorf("library write landed wrong: %q", b)
	}
	expect(t, f.do("GET", "/api/bridge/file?cart=hidden&path=main.go", nil), 404)
}

func TestCartsDuplicateSlug(t *testing.T) {
	lib := t.TempDir()
	makeCart(t, filepath.Join(lib, "one"), "same", "One")
	makeCart(t, filepath.Join(lib, "two"), "same", "Two")
	if _, err := findCarts(lib); err == nil {
		t.Error("duplicate slugs accepted")
	}
	if _, err := findCarts(t.TempDir()); err == nil {
		t.Error("empty directory accepted")
	}
}

func TestChangeEventAfterDiskWrite(t *testing.T) {
	f := singleCart(t)
	req, _ := http.NewRequest("GET", f.srv.URL+"/api/bridge/events?token="+testToken, nil)
	req.Header.Set("Origin", "https://arkade.build")
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") ||
		resp.Header.Get("Access-Control-Allow-Origin") != "https://arkade.build" {
		t.Fatalf("events: %d %v", resp.StatusCode, resp.Header)
	}

	events := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		var name string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				events <- name + " " + strings.TrimPrefix(line, "data: ")
			}
		}
		close(events)
	}()

	writeFile(t, filepath.Join(f.cart, "main.go"), "package main // not an asset\n")
	writeFile(t, filepath.Join(f.cart, "assets", "sprites", "enemy.png"), "new")
	want := `change {"cart":"demo","path":"assets/sprites/enemy.png"}`
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("event = %s, want %s", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no change event")
	}

	os.Remove(filepath.Join(f.cart, "assets", "sprites", "hero.png"))
	select {
	case got := <-events:
		if got != `change {"cart":"demo","path":"assets/sprites/hero.png"}` {
			t.Fatalf("event = %s", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no change event for a removal")
	}
}

func TestNormalizeOrigin(t *testing.T) {
	if got, err := normalizeOrigin("http://LocalHost:5173/"); err != nil || got != "http://localhost:5173" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"*", "arkade.build", "https://arkade.build/dev", "file:///x", "https://u@arkade.build"} {
		if _, err := normalizeOrigin(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
