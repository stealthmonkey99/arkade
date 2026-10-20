// SPDX-License-Identifier: GPL-2.0-or-later

//go:build !linux

package bluetooth

func New() Pairer {
	return unsupported{reason: "pairing needs the cabinet's Linux/BlueZ stack"}
}
