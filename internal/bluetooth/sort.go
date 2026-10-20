// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import "sort"

// tier orders the picker: controllers, then named devices, then the rest.
// Within a tier order is first seen, so rows never move. RSSI is excluded
// because it wanders between scans and would reshuffle rows.
type tier int

const (
	tierController tier = iota
	tierNamed
	tierAnonymous
)

func tierOf(d Device) tier {
	switch {
	case d.Input:
		return tierController
	case HasRealName(d):
		return tierNamed
	default:
		return tierAnonymous
	}
}

// HasRealName reports whether the device has a name other than its address.
// BlueZ fills Alias with the dashed address when no name is advertised.
func HasRealName(d Device) bool {
	if d.Name == "" {
		return false
	}
	return !sameAddress(d.Name, d.Address)
}

// sameAddress compares ignoring case and separators; BlueZ dashes the alias
// and colons the address.
func sameAddress(name, address string) bool {
	return macKey(name) == macKey(address)
}

func macKey(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r == ':' || r == '-' || r == '_' || r == ' ':
			continue
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// Ranker remembers the order devices were first seen in. Use one per scan.
type Ranker struct {
	firstSeen map[string]int
	next      int
}

func (r *Ranker) Sort(devs []Device) {
	if r.firstSeen == nil {
		r.firstSeen = make(map[string]int, len(devs))
	}
	for _, d := range devs {
		if _, ok := r.firstSeen[d.Address]; !ok {
			r.firstSeen[d.Address] = r.next
			r.next++
		}
	}
	sort.SliceStable(devs, func(i, j int) bool {
		a, b := devs[i], devs[j]
		if ta, tb := tierOf(a), tierOf(b); ta != tb {
			return ta < tb
		}
		return r.firstSeen[a.Address] < r.firstSeen[b.Address]
	})
}

func DisplayName(d Device) string {
	if d.Name != "" {
		return d.Name
	}
	return d.Address
}
