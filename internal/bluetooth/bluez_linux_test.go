// SPDX-License-Identifier: GPL-2.0-or-later

//go:build linux

package bluetooth

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestClosedPairerFailsInsteadOfPanicking(t *testing.T) {
	b := &bluez{}
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Close()
			b.StopScan()
			if err := b.StartScan(ctx); !errors.Is(err, errClosed) {
				t.Errorf("StartScan: %v, want errClosed", err)
			}
			if _, err := b.Devices(); !errors.Is(err, errClosed) {
				t.Errorf("Devices: %v, want errClosed", err)
			}
			if err := b.Pair(ctx, "AA:BB:CC:DD:EE:FF"); !errors.Is(err, errClosed) {
				t.Errorf("Pair: %v, want errClosed", err)
			}
			if err := b.Forget(ctx, "AA:BB:CC:DD:EE:FF"); !errors.Is(err, errClosed) {
				t.Errorf("Forget: %v, want errClosed", err)
			}
			if _, err := b.DisconnectAll(ctx); !errors.Is(err, errClosed) {
				t.Errorf("DisconnectAll: %v, want errClosed", err)
			}
		}()
	}
	wg.Wait()
}
