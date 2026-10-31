// SPDX-License-Identifier: GPL-2.0-or-later

package loopcert

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMakesALoopbackOnlyLeafAndReusesIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	first, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created {
		t.Fatal("first Load should report a new certificate")
	}
	leaf, err := x509.ParseCertificate(first.TLS.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if leaf.IsCA {
		t.Fatal("the certificate must not be a CA")
	}
	for _, name := range []string{"localhost", "127.0.0.1", "::1"} {
		if err := leaf.VerifyHostname(name); err != nil {
			t.Errorf("not valid for %s: %v", name, err)
		}
	}
	if leaf.VerifyHostname("arkade.build") == nil {
		t.Error("valid for a non-loopback name")
	}
	if got := leaf.NotAfter.Sub(leaf.NotBefore); got > lifetime+2*time.Hour {
		t.Errorf("lifetime %v exceeds Apple's 825-day limit", got)
	}
	if fi, err := os.Stat(filepath.Join(dir, "key.pem")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("key.pem should be 0600: %v %v", fi.Mode(), err)
	}

	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || string(again.TLS.Certificate[0]) != string(first.TLS.Certificate[0]) {
		t.Fatal("a second Load should reuse the trusted certificate, not replace it")
	}
}

func TestServesHTTPSOnEveryLoopbackName(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	srv.TLS = c.Config()
	srv.StartTLS()
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	pool := x509.NewCertPool()
	pool.AddCert(mustLeaf(t, c))
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	for _, host := range []string{"127.0.0.1", "localhost"} {
		res, err := client.Get("https://" + net.JoinHostPort(host, port))
		if err != nil {
			t.Fatalf("%s: %v", host, err)
		}
		res.Body.Close()
	}
}

func mustLeaf(t *testing.T, c Cert) *x509.Certificate {
	t.Helper()
	leaf, err := x509.ParseCertificate(c.TLS.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

func TestUsableRenewsNearExpiry(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !usable(c.TLS, time.Now()) {
		t.Fatal("a fresh certificate should be usable")
	}
	if usable(c.TLS, time.Now().Add(lifetime-renewBefore/2)) {
		t.Fatal("a certificate inside the renewal window should be replaced")
	}
}
