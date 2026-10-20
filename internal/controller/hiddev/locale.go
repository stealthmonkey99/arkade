// SPDX-License-Identifier: GPL-2.0-or-later

package hiddev

/*
#include <stdlib.h>
#include <locale.h>

static int mb_cur_max(void) { return (int)MB_CUR_MAX; }
*/
import "C"

import "unsafe"

// init selects a UTF-8 LC_CTYPE before any hidapi call. go-hid converts
// device strings with wcstombs and panics on EILSEQ, and a Go binary starts
// in the ASCII "C" locale, so a non-ASCII descriptor would crash Enumerate.
func init() {
	// "" honours the environment; the cabinet boots with none and macOS
	// rejects "C.UTF-8", so fall through until the codeset is multi-byte.
	for _, name := range []string{"", "C.UTF-8", "en_US.UTF-8"} {
		cname := C.CString(name)
		C.setlocale(C.LC_CTYPE, cname)
		C.free(unsafe.Pointer(cname))
		if C.mb_cur_max() > 1 {
			return
		}
	}
}
