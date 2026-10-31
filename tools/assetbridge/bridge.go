// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type config struct {
	carts   []cart
	token   string
	origins []string // normalized scheme://host[:port]
	port    int
}

type bridge struct {
	carts   map[string]cart
	order   []cart
	token   []byte
	origins map[string]bool
	hosts   map[string]bool
	hub     *hub
	mux     *http.ServeMux
}

type fileEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
	Dir   bool   `json:"dir"`
}

func newBridge(cfg config) *bridge {
	b := &bridge{
		carts:   map[string]cart{},
		order:   cfg.carts,
		token:   []byte(cfg.token),
		origins: map[string]bool{},
		hosts: map[string]bool{
			"127.0.0.1:" + strconv.Itoa(cfg.port): true,
			"localhost:" + strconv.Itoa(cfg.port): true,
		},
		hub: newHub(cfg.carts),
		mux: http.NewServeMux(),
	}
	for _, c := range cfg.carts {
		b.carts[c.Slug] = c
	}
	for _, o := range cfg.origins {
		b.origins[o] = true
	}
	b.mux.HandleFunc("GET /api/bridge/health", b.health)
	b.mux.HandleFunc("GET /api/bridge/carts", b.authed(b.listCarts))
	b.mux.HandleFunc("GET /api/bridge/files", b.authed(b.listFiles))
	b.mux.HandleFunc("GET /api/bridge/file", b.authed(b.readFile))
	b.mux.HandleFunc("PUT /api/bridge/file", b.authed(b.writeFile))
	b.mux.HandleFunc("DELETE /api/bridge/file", b.authed(b.deleteFile))
	b.mux.HandleFunc("GET /api/bridge/events", b.authed(b.events))
	return b
}

func normalizeOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
		return "", fmt.Errorf("origin %q is not of the form scheme://host[:port]", raw)
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

func (b *bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Vary", "Origin")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	if !b.hosts[strings.ToLower(r.Host)] {
		fail(w, http.StatusForbidden, "host not allowed")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		if !b.origins[origin] {
			fail(w, http.StatusForbidden, "origin not allowed")
			return
		}
		h.Set("Access-Control-Allow-Origin", origin)
	}
	if r.Method == http.MethodOptions {
		b.preflight(w, r)
		return
	}
	b.mux.ServeHTTP(w, r)
}

func (b *bridge) preflight(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") == "" {
		fail(w, http.StatusForbidden, "preflight without an origin")
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	h.Set("Access-Control-Max-Age", "600")
	if strings.EqualFold(r.Header.Get("Access-Control-Request-Private-Network"), "true") {
		h.Set("Access-Control-Allow-Private-Network", "true")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *bridge) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok && r.URL.Path == "/api/bridge/events" {
			got, ok = r.URL.Query().Get("token"), true
		}
		if !ok || subtle.ConstantTimeCompare([]byte(got), b.token) != 1 {
			fail(w, http.StatusUnauthorized, "missing or wrong token")
			return
		}
		next(w, r)
	}
}

func (b *bridge) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"bridge": "assetbridge", "ok": true, "version": version})
}

func (b *bridge) listCarts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, b.order)
}

func (b *bridge) cart(w http.ResponseWriter, r *http.Request) (cart, bool) {
	c, ok := b.carts[r.URL.Query().Get("cart")]
	if !ok {
		fail(w, http.StatusNotFound, "unknown cartridge")
	}
	return c, ok
}

func (b *bridge) listFiles(w http.ResponseWriter, r *http.Request) {
	c, ok := b.cart(w, r)
	if !ok {
		return
	}
	dir, err := resolve(c, r.URL.Query().Get("dir"))
	if err != nil {
		failPath(w, err)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		failPath(w, err)
		return
	}
	out := []fileEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil || !(info.Mode().IsRegular() || info.IsDir()) {
			continue
		}
		if e.Type()&fs.ModeSymlink != 0 {
			if _, err := resolve(c, relTo(c, filepath.Join(dir, e.Name()))); err != nil {
				continue
			}
		}
		out = append(out, fileEntry{Name: e.Name(), Size: info.Size(), Mtime: info.ModTime().UnixMilli(), Dir: info.IsDir()})
	}
	writeJSON(w, out)
}

func (b *bridge) readFile(w http.ResponseWriter, r *http.Request) {
	c, ok := b.cart(w, r)
	if !ok {
		return
	}
	p, err := resolve(c, r.URL.Query().Get("path"))
	if err != nil {
		failPath(w, err)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		failPath(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		fail(w, http.StatusNotFound, "not a regular file")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	io.Copy(w, f)
}

func (b *bridge) writeFile(w http.ResponseWriter, r *http.Request) {
	c, ok := b.cart(w, r)
	if !ok {
		return
	}
	p, err := resolveWritable(c, r.URL.Query().Get("path"))
	if err != nil {
		failPath(w, err)
		return
	}
	if r.ContentLength > maxWriteBytes {
		fail(w, http.StatusRequestEntityTooLarge, "write exceeds 16 MiB")
		return
	}
	if info, err := os.Stat(p); err == nil && !info.Mode().IsRegular() {
		fail(w, http.StatusForbidden, "not a regular file")
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		failPath(w, err)
		return
	}
	if err := atomicWrite(p, http.MaxBytesReader(w, r.Body, maxWriteBytes)); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, "write exceeds 16 MiB")
			return
		}
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func atomicWrite(path string, body io.Reader) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".assetbridge-*")
	if err != nil {
		return err
	}
	_, err = io.Copy(tmp, body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

func (b *bridge) deleteFile(w http.ResponseWriter, r *http.Request) {
	c, ok := b.cart(w, r)
	if !ok {
		return
	}
	p, err := resolveWritable(c, r.URL.Query().Get("path"))
	if err != nil {
		failPath(w, err)
		return
	}
	info, err := os.Lstat(p)
	if err != nil {
		failPath(w, err)
		return
	}
	if !info.Mode().IsRegular() {
		fail(w, http.StatusForbidden, "not a regular file")
		return
	}
	if err := os.Remove(p); err != nil {
		failPath(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *bridge) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ch := b.hub.subscribe()
	defer b.hub.unsubscribe(ch)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": ping\n\n")
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", data)
		}
		flusher.Flush()
	}
}

func relTo(c cart, p string) string {
	rel, _ := filepath.Rel(c.dir, p)
	return filepath.ToSlash(rel)
}

func failPath(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBadPath):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, errOutside), errors.Is(err, errReadOnly), errors.Is(err, fs.ErrPermission):
		fail(w, http.StatusForbidden, err.Error())
	case errors.Is(err, fs.ErrNotExist):
		fail(w, http.StatusNotFound, "not found")
	default:
		fail(w, http.StatusInternalServerError, "file error")
	}
}

func fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
