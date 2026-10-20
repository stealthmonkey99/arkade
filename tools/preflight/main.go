// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func main() {
	var (
		all          = flag.Bool("all", false, "scan every cartridge")
		asJSON       = flag.Bool("json", false, "emit the report as machine-readable JSON")
		verbose      = flag.Bool("v", false, "print the measurements passing checks took, and each finding's rule citation")
		fast         = flag.Bool("fast", false, "skip the per-cartridge test suite")
		strict       = flag.Bool("strict", false, "treat warnings as failures")
		only         = flag.String("only", "", "run only these checks (comma-separated ids; see -list)")
		list         = flag.Bool("list", false, "list the registered checks and exit")
		root         = flag.String("root", "", "repo root (default: found by walking up from the working directory)")
		wavFloorHz   = flag.Float64("wav-floor-hz", audioDefaultFloorHz, "top of the sub-bass band the cabinet's speakers cannot reproduce")
		wavFloorFrac = flag.Float64("wav-floor-frac", audioDefaultFloorFrac, "how much of a sound's energy may sit below -wav-floor-hz")
	)
	flag.Usage = usage
	flag.Parse()

	opts := Options{AudioFloor: audioFloorModel{hz: *wavFloorHz, frac: *wavFloorFrac}}

	if *list {
		listChecks()
		return
	}

	repoRoot, err := resolveRoot(*root)
	if err != nil {
		fatal(err)
	}

	slugs, err := selectCarts(repoRoot, *all, flag.Args())
	if err != nil {
		fatal(err)
	}

	checks, err := selectChecks(*only, *fast)
	if err != nil {
		fatal(err)
	}

	rep := Report{Tool: "preflight", Root: repoRoot, Strict: *strict}
	for _, slug := range slugs {
		rep.Carts = append(rep.Carts, scan(repoRoot, slug, checks, opts))
	}
	rep.finish()

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fatal(err)
		}
	} else {
		fmt.Print(rep.Text(*verbose))
	}

	if rep.Summary.Failed > 0 {
		os.Exit(1)
	}
}

// scan runs the selected checks over one cartridge and builds its card.
func scan(root, slug string, checks []Check, opts Options) CartReport {
	cart := NewCart(root, slug, opts)
	card := CartReport{Slug: slug}
	if m, _, err := cart.Manifest(); err == nil {
		card.Title = m.DisplayTitle()
		card.Version = m.Version
	}
	for _, check := range checks {
		res := Result{ID: check.ID, Title: check.Title, Rule: check.Rule, Status: StatusPass}
		start := time.Now()
		check.Run(cart, &res)
		res.ElapsedMS = time.Since(start).Milliseconds()
		res.summarize("no findings")
		card.Checks = append(card.Checks, res)
	}
	// A config that fails to load suppresses nothing; checkConfig reports why.
	if cfg, err := cart.Config(); err == nil {
		applySuppressions(cart, cfg, &card)
	}
	card.Status = card.verdict()
	return card
}

// resolveRoot uses -root, or searches up from the working directory.
func resolveRoot(flagValue string) (string, error) {
	if flagValue != "" {
		return findRoot(flagValue)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return findRoot(cwd)
}

// selectCarts returns the slugs to scan, rejecting unknown ones.
func selectCarts(root string, all bool, args []string) ([]string, error) {
	fleet, err := discoverCarts(root)
	if err != nil {
		return nil, fmt.Errorf("listing cartridges: %w", err)
	}
	if all {
		if len(args) > 0 {
			return nil, fmt.Errorf("-all takes no cartridge arguments (got %s)", strings.Join(args, " "))
		}
		return fleet, nil
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("name a cartridge slug, or pass -all (%d known: %s)", len(fleet), strings.Join(fleet, " "))
	}
	known := map[string]bool{}
	for _, s := range fleet {
		known[s] = true
	}
	for _, a := range args {
		if !known[a] {
			return nil, fmt.Errorf("no cartridge %q under %s/cartridges", a, root)
		}
	}
	return args, nil
}

// selectChecks applies -only and -fast to the registry, keeping its order.
func selectChecks(only string, fast bool) ([]Check, error) {
	want := map[string]bool{}
	if only != "" {
		for _, id := range strings.Split(only, ",") {
			want[strings.TrimSpace(id)] = true
		}
	}
	var out []Check
	seen := map[string]bool{}
	for _, check := range registry {
		seen[check.ID] = true
		if len(want) > 0 && !want[check.ID] {
			continue
		}
		if fast && check.Slow {
			continue
		}
		out = append(out, check)
	}
	var unknown []string
	for id := range want {
		if !seen[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown check(s) %s — see -list", strings.Join(unknown, ", "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no checks selected")
	}
	return out, nil
}

// listChecks prints the registered checks.
func listChecks() {
	for _, check := range registry {
		slow := ""
		if check.Slow {
			slow = "  [slow: skipped by -fast]"
		}
		fmt.Printf("%-15s %s%s\n", check.ID, check.Title, slow)
		fmt.Printf("%-15s   enforces: %s\n", "", check.Rule)
	}
}

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `preflight — is this cartridge ready?

usage:
  preflight [flags] <slug> [<slug>...]
  preflight [flags] -all

examples:
  go run ./tools/preflight snake
  go run ./tools/preflight -all -fast
  go run ./tools/preflight -all -json > preflight.json
  go run ./tools/preflight -only lcd,wav-audibility -all

exit status: 0 all clear, 1 a check failed, 2 usage or IO error.

flags:
`)
	flag.PrintDefaults()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "preflight:", err)
	os.Exit(2)
}
