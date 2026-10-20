// SPDX-License-Identifier: GPL-2.0-or-later

package bluetooth

import (
	"context"
	"errors"
)

// unsupported is the Pairer for non-Linux builds and for Linux without an
// adapter; every operation fails with reason.
type unsupported struct{ reason string }

func (u unsupported) Available() bool { return false }

func (u unsupported) StartScan(context.Context) error { return u.err() }

func (u unsupported) StopScan() {}

func (u unsupported) Devices() ([]Device, error) { return nil, u.err() }

func (u unsupported) Pair(context.Context, string) error { return u.err() }

func (u unsupported) Forget(context.Context, string) error { return u.err() }

func (u unsupported) DisconnectAll(context.Context) (int, error) { return 0, u.err() }

func (u unsupported) Close() error { return nil }

func (u unsupported) err() error {
	if u.reason == "" {
		return errors.New("bluetooth: not supported on this build")
	}
	return errors.New("bluetooth: " + u.reason)
}

func Unavailable(reason string) Pairer { return unsupported{reason: reason} }
