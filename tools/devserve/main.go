// SPDX-License-Identifier: GPL-2.0-or-later

// Command devserve builds a cartridge directory on every save and serves the
// result on loopback to the hosted player at arkade.build, which loads it with
// /play?dev=<devserve address> and reloads it in place after each good build.
// The directory needs manifest.json, go.mod and a Makefile with build and
// rcart targets (the sdk/template shape) and may live anywhere.
//
//	go run ./tools/devserve ./sdk/tutorial
//	# then open the printed https://arkade.build/play?dev=http://127.0.0.1:8134
//
// -player DIR additionally serves a locally built web player, for working on
// the player itself.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/stealthmonkey99/arkade/tools/fleet"
	"github.com/stealthmonkey99/arkade/tools/loopcert"
)

const hostedPlayURL = "https://arkade.build/play"

// slugPattern is looser than rcart's manifest rule, which is internal and not importable here.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `usage: devserve [-addr host:port] [-origin URL]... [-player DIR] [-build CMD] [-tls] <cartridge-dir>

<cartridge-dir> is a folder with manifest.json, go.mod and a Makefile with
"build" and "rcart" targets (the sdk/template shape). It may live anywhere.

devserve builds it with its own Makefile (make -C <dir> ARKADE=<engine> build
rcart), or with -build's shell command for a cartridge a collection builds,
rebuilds on every save, and serves the last good <dir>/<slug>.rcart on loopback.
Open the printed https://arkade.build/play?dev=... address; the page reloads
the cartridge after every good build.

`)
	flag.PrintDefaults()
}

// originList is a repeatable -origin flag.
type originList []string

func (o *originList) String() string { return strings.Join(*o, ",") }

func (o *originList) Set(v string) error {
	origin, err := normalizeOrigin(v)
	if err != nil {
		return err
	}
	*o = append(*o, origin)
	return nil
}

// normalizeOrigin reduces a URL to the scheme://host[:port] form browsers send in Origin.
func normalizeOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("origin %q: want http(s)://host[:port]", raw)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("origin %q: want only scheme, host and port", raw)
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

func main() {
	flag.Usage = usage
	cartFlag := flag.String("cart", "", "the cartridge directory (alias for the positional argument)")
	addr := flag.String("addr", "127.0.0.1:8134", "listen address; keep it on loopback")
	var origins originList
	flag.Var(&origins, "origin", "an extra origin allowed to read from devserve (repeatable); arkade.build is always allowed")
	playerFlag := flag.String("player", "", "a built web player directory to serve as well (optional)")
	useTLS := flag.Bool("tls", false, "serve HTTPS with a loopback-only certificate, for Safari")
	buildFlag := flag.String("build", "", "a shell command that builds <dir>/<slug>.rcart, run in the cartridge directory instead of its Makefile")
	positional := parseInterspersed(flag.CommandLine, os.Args[1:])

	cartArg, err := cartridgeArg(*cartFlag, positional)
	if err != nil {
		fmt.Fprintln(os.Stderr, "devserve:", err)
		flag.Usage()
		os.Exit(2)
	}

	cartDir, err := filepath.Abs(cartArg)
	if err != nil {
		fatal(err)
	}
	if err := checkCartridgeDir(cartDir, *buildFlag == ""); err != nil {
		fatal(fmt.Errorf("%s: %w", cartArg, err))
	}
	m, err := readManifest(filepath.Join(cartDir, "manifest.json"))
	if err != nil {
		fatal(fmt.Errorf("%s: %w", cartArg, err))
	}

	engineRoot, err := findEngineRoot()
	if err != nil {
		fatal(err)
	}

	playerDir := ""
	if *playerFlag != "" {
		if playerDir, err = filepath.Abs(*playerFlag); err != nil {
			fatal(err)
		}
		requirePlayer(playerDir)
	}
	requireTinyGo()

	srv := newDevServer(m, &makeBuilder{cartDir: cartDir, slug: m.Slug, engineRoot: engineRoot, command: *buildFlag})
	srv.playerDir = playerDir
	srv.allowOrigins(origins...)
	srv.logf = func(format string, args ...any) { fmt.Printf(format, args...) }

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fatal(fmt.Errorf("%w (another devserve running? pass -addr 127.0.0.1:0 for any free port)", err))
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	devOrigin := "http://127.0.0.1:" + port
	var tlsConfig *tls.Config
	if *useTLS {
		cert, err := loopcert.LoadDefault()
		if err != nil {
			fatal(err)
		}
		if cert.Created {
			fmt.Printf("devserve: made a loopback certificate; %s\n", cert.TrustHint())
		}
		tlsConfig = cert.Config()
		ln = tls.NewListener(ln, tlsConfig)
		devOrigin = "https://127.0.0.1:" + port
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("devserve: engine %s\n", engineRoot)
	fmt.Printf("devserve: building %s (%s) …\n", m.Slug, cartDir)

	firstBuild := make(chan bool, 1)
	var once sync.Once
	go srv.runBuildLoop(ctx, func(ok bool) { once.Do(func() { firstBuild <- ok }) })
	go newWatcher(cartDir, m.Slug).run(ctx, srv.triggerBuild)

	if <-firstBuild {
		fmt.Printf("devserve: build ok\n")
	} else {
		fmt.Printf("devserve: initial build FAILED — serving anyway; fix %s and save to retry\n", cartDir)
	}

	httpSrv := &http.Server{
		Handler:           srv.handler(port),
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         tlsConfig,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("devserve: watching %s for changes\n", cartDir)
	if playerDir != "" {
		fmt.Printf("devserve: local player %s at %s/play?dev=%s\n", playerDir, devOrigin, devOrigin)
	}
	fmt.Printf("devserve: open %s?dev=%s\n", hostedPlayURL, devOrigin)
	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}

// parseInterspersed lets flags follow the cartridge directory, which the flag package alone stops at.
func parseInterspersed(fs *flag.FlagSet, args []string) []string {
	var positional []string
	for {
		fs.Parse(args)
		if fs.NArg() == 0 {
			return positional
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// cartridgeArg reconciles the positional directory with the -cart alias.
func cartridgeArg(cartFlag string, positional []string) (string, error) {
	switch {
	case len(positional) > 1:
		return "", fmt.Errorf("want one cartridge directory, got %d", len(positional))
	case len(positional) == 1 && cartFlag != "" && positional[0] != cartFlag:
		return "", fmt.Errorf("-cart %s and %s disagree; pass one", cartFlag, positional[0])
	case len(positional) == 1:
		return positional[0], nil
	case cartFlag != "":
		return cartFlag, nil
	}
	return "", errors.New("a cartridge directory is required")
}

// checkCartridgeDir names each of manifest.json, go.mod and (when wanted) Makefile that dir lacks.
func checkCartridgeDir(dir string, wantMakefile bool) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("not a directory")
	}
	var missing []string
	want := []string{"manifest.json", "go.mod"}
	if wantMakefile {
		want = append(want, "Makefile")
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("no %s found — is this a cartridge directory? (want manifest.json, go.mod and a Makefile with build and rcart targets, the sdk/template shape, or pass -build)", strings.Join(missing, ", "))
	}
	return nil
}

type cartManifest struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// readManifest returns the manifest's slug, which names the build outputs, and its title.
func readManifest(manifestPath string) (cartManifest, error) {
	var m cartManifest
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("manifest.json: %w", err)
	}
	if m.Slug == "" {
		return m, errors.New("manifest.json has no \"slug\"")
	}
	if !slugPattern.MatchString(m.Slug) {
		return m, fmt.Errorf("manifest.json slug %q is not valid (want lowercase letters, digits, hyphens, starting with a letter or digit)", m.Slug)
	}
	return m, nil
}

// findEngineRoot looks above the working directory, then above the directory
// this source was compiled from.
func findEngineRoot() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok && filepath.IsAbs(file) {
		starts = append(starts, filepath.Dir(file))
	}
	for _, start := range starts {
		if root, err := fleet.EngineRoot(start); err == nil {
			return root, nil
		}
	}
	return "", fmt.Errorf("couldn't find the engine checkout: no go.mod declaring %q above %s — run devserve from inside the engine checkout", fleet.EngineModule, strings.Join(starts, " or "))
}

// requireTinyGo fails fast rather than letting the first rebuild die inside make.
func requireTinyGo() {
	if _, err := exec.LookPath("tinygo"); err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, `devserve: tinygo not found on PATH

devserve rebuilds a cartridge with TinyGo on every save, so it can't run
without it. Install it and try again:

  brew install tinygo-org/tools/tinygo   # macOS (plain "brew install tinygo" does not exist)
  # or see https://tinygo.org/getting-started/install/ for other platforms`)
	os.Exit(2)
}

// requirePlayer fails fast when -player names no built player.
func requirePlayer(playerDir string) {
	for _, name := range []string{"index.html", filepath.Join("dist", "bundle.js")} {
		if _, err := os.Stat(filepath.Join(playerDir, name)); err != nil {
			fmt.Fprintf(os.Stderr, "devserve: -player %s has no %s; build the player first (npm ci && npm run build)\n", playerDir, name)
			os.Exit(2)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "devserve:", err)
	os.Exit(1)
}
