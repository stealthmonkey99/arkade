// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"reflect"
	"testing"
)

func TestDefaultHTTPAddrIsLoopback(t *testing.T) {
	if !httpAddrIsLoopback(defaultHTTPAddr) {
		t.Fatalf("defaultHTTPAddr %q must be loopback-only", defaultHTTPAddr)
	}
}

func TestHTTPAddrIsLoopback(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.1:0", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{":8080", false},
		{"0.0.0.0:8080", false},
		{"192.168.1.5:8080", false},
		{"cabinet.local:8080", false},
	}
	for _, c := range cases {
		if got := httpAddrIsLoopback(c.addr); got != c.want {
			t.Errorf("httpAddrIsLoopback(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	if got := splitCSV(""); got != nil {
		t.Errorf("splitCSV(\"\") = %#v, want nil", got)
	}
	if got := splitCSV("   "); got != nil {
		t.Errorf("splitCSV(whitespace) = %#v, want nil", got)
	}
	got := splitCSV(" cabinet.local , foo.local ,,bar.local")
	want := []string{"cabinet.local", "foo.local", "bar.local"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitCSV = %#v, want %#v", got, want)
	}
}

// TestCartridgeDirsStillDefaultsWithEmptyFlag: an unset --cartridges falls
// back to "<data>/cartridges", not an empty list.
func TestCartridgeDirsStillDefaultsWithEmptyFlag(t *testing.T) {
	got := cartridgeDirs("", "data")
	want := []string{"data/cartridges"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cartridgeDirs(\"\", \"data\") = %#v, want %#v", got, want)
	}
}
