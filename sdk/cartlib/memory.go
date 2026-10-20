// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

import "unsafe"

// emptyBytes gives a zero-length argument a valid address.
var emptyBytes = []byte{0}

// ptrOf returns b's address as a wasm linear-memory pointer.
func ptrOf(b []byte) uint32 {
	if len(b) == 0 {
		return uint32(uintptr(unsafe.Pointer(&emptyBytes[0])))
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

// stringBytes views s's own bytes as a slice without copying, so a string
// argument reaches a []byte host method allocation-free. The host never
// writes through it.
func stringBytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// ptrOfString is ptrOf for a string's own bytes, so passing a string to the
// host never copies it. Safe because no host import retains the pointer
// past the call and TinyGo's collectors never move memory.
func ptrOfString(s string) uint32 {
	if len(s) == 0 {
		return uint32(uintptr(unsafe.Pointer(&emptyBytes[0])))
	}
	return uint32(uintptr(unsafe.Pointer(unsafe.StringData(s))))
}
