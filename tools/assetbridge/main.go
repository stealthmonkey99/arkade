// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/stealthmonkey99/arkade/tools/loopcert"
)

const version = "0.1.0"

const labURL = "https://arkade.build/dev/tools/spritelab"

var defaultOrigins = []string{"https://arkade.build", "https://www.arkade.build"}

type originFlag []string

func (o *originFlag) String() string { return strings.Join(*o, ",") }

func (o *originFlag) Set(v string) error {
	origin, err := normalizeOrigin(v)
	if err != nil {
		return err
	}
	*o = append(*o, origin)
	return nil
}

func main() {
	var origins originFlag
	addr := flag.String("addr", "127.0.0.1:8140", "listen address; keep it on loopback")
	flag.Var(&origins, "origin", "also allow this web origin (repeatable)")
	useTLS := flag.Bool("tls", false, "serve HTTPS with a loopback-only certificate, for Safari")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}

	carts, err := findCarts(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	token, err := newToken()
	if err != nil {
		fatal(err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	b := newBridge(config{
		carts:   carts,
		token:   token,
		origins: append(append([]string{}, defaultOrigins...), origins...),
		port:    port,
	})

	for _, c := range carts {
		fmt.Printf("assetbridge: serving %s (%s)\n", c.Slug, c.dir)
	}
	srv := &http.Server{Handler: b, ReadHeaderTimeout: 10 * time.Second}
	bridgeAddr := fmt.Sprintf("127.0.0.1:%d", port)
	if *useTLS {
		cert, err := loopcert.LoadDefault()
		if err != nil {
			fatal(err)
		}
		if cert.Created {
			fmt.Printf("assetbridge: made a loopback certificate; %s\n", cert.TrustHint())
		}
		srv.TLSConfig = cert.Config()
		ln = tls.NewListener(ln, srv.TLSConfig)
		bridgeAddr = "https://" + bridgeAddr
	}

	fmt.Printf("assetbridge: token %s\n", token)
	fmt.Printf("assetbridge: open %s#bridge=%s&token=%s\n", labURL, bridgeAddr, token)
	fatal(srv.Serve(ln))
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `assetbridge — let the hosted asset lab edit a local cartridge folder

usage:
  assetbridge [-addr 127.0.0.1:8140] [-origin URL]... [-tls] <cartridge-or-library-dir>

flags:
`)
	flag.PrintDefaults()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "assetbridge:", err)
	os.Exit(2)
}
