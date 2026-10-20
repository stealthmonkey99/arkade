// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"math"
	"net"
	"os"
	"strconv"
	"strings"
)

// Paths are variables so tests can point them at fixtures.
var (
	thermalPath  = "/sys/class/thermal/thermal_zone0/temp"
	wirelessPath = "/proc/net/wireless"
)

// systemInfo is everything P2 shows, sampled together.
type systemInfo struct {
	ip     string
	tempC  int
	tempOK bool
	rssi   int
	rssiOK bool
}

func readSystemInfo() systemInfo {
	s := systemInfo{ip: firstIPv4()}
	s.tempC, s.tempOK = parseThermal(readFile(thermalPath))
	s.rssi, s.rssiOK = parseWireless(readFile(wirelessPath))
	return s
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// firstIPv4 returns the first non-loopback, non-link-local IPv4 on an up interface.
func firstIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if v4 := ipNet.IP.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsLinkLocalUnicast() {
				return v4.String()
			}
		}
	}
	return ""
}

// parseThermal reads the thermal zone's millidegrees C, rounded to whole
// degrees.
func parseThermal(raw string) (int, bool) {
	milli, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return int(math.Round(milli / 1000)), true
}

// parseWireless returns the first nonzero signal level (dBm) from
// /proc/net/wireless, whose level field has a trailing dot. A level of 0
// means no link, not a strong signal.
func parseWireless(raw string) (int, bool) {
	for _, line := range strings.Split(raw, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok || strings.ContainsAny(name, "|") {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 3 {
			continue
		}
		level, err := strconv.ParseFloat(strings.TrimSuffix(fields[2], "."), 64)
		if err != nil || level == 0 {
			continue
		}
		return int(math.Round(level)), true
	}
	return 0, false
}
