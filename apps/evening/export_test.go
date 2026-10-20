// SPDX-License-Identifier: GPL-2.0-or-later

package evening

import "time"

// ResetGate clears the package-level gate state between tests.
func ResetGate() {
	room.forget()
	room.setOK = false
	room.lastCall = time.Time{}
}
