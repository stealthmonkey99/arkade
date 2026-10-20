// SPDX-License-Identifier: GPL-2.0-or-later

package wasm

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// bootPhase marks NewCartridge's _start call, during which random_get serves
// a fixed pattern: TinyGo seeds its map hash there, and drawing from the RNG
// stream would shift every game's script. The web host mirrors it.
type bootPhase struct{ booting bool }

const maxWASIIovecs = 64 // fd_write iovec count

// wasiImports is the minimal wasi_snapshot_preview1 TinyGo needs, routed to
// the Host. Any other WASI import is deliberately unregistered, so
// instantiation fails loudly.
func wasiImports(host Session, boot *bootPhase) []hostImport {
	return []hostImport{
		// stdout logs at info, stderr at warn; any other fd is EBADF.
		{"fd_write", func(_ context.Context, mod api.Module, fd, iovsPtr, iovsLen, nwrittenPtr uint32) uint32 {
			const (
				errnoSuccess = 0
				errnoBadF    = 8
			)
			if fd != 1 && fd != 2 {
				return errnoBadF
			}
			if iovsLen > maxWASIIovecs {
				hostWarnf(host, "wasi fd_write: iovs_len %d exceeds %d", iovsLen, maxWASIIovecs)
				return errnoBadF
			}
			var msg []byte
			var total uint32
			for i := uint32(0); i < iovsLen; i++ {
				iov, ok := mod.Memory().Read(iovsPtr+i*8, 8)
				if !ok {
					hostWarnf(host, "wasi fd_write: iovec %d out of bounds", i)
					return errnoBadF
				}
				ptr := le32(iov[0:4])
				ln := le32(iov[4:8])
				if uint32(len(msg))+ln > maxLogBytes {
					break
				}
				chunk, ok := readBytes(mod, ptr, ln, maxLogBytes)
				if !ok {
					hostWarnf(host, "wasi fd_write: iovec %d payload out of bounds", i)
					return errnoBadF
				}
				msg = append(msg, chunk...)
				total += ln
			}
			level := LogInfo
			if fd == 2 {
				level = LogWarn
			}
			if len(msg) > 0 {
				host.Log(level, string(msg))
			}
			if !mod.Memory().Write(nwrittenPtr, []byte{byte(total), byte(total >> 8), byte(total >> 16), byte(total >> 24)}) {
				hostWarnf(host, "wasi fd_write: nwritten ptr %#x out of bounds", nwrittenPtr)
				return errnoBadF
			}
			return errnoSuccess
		}},
		// An inert no-op, since terminating mid host-call is unsafe. exit(0)
		// ending _start is expected; any other call is logged.
		{"proc_exit", func(_ context.Context, _ api.Module, code uint32) {
			if boot != nil && boot.booting && code == 0 {
				return
			}
			hostWarnf(host, "wasi proc_exit(%d) called; ignoring (cartridges are reactors, not commands)", code)
		}},
		{"random_get", func(_ context.Context, mod api.Module, bufPtr, bufLen uint32) uint32 {
			const errnoInval = 28
			if bufLen > maxWASIRandBytes {
				hostWarnf(host, "wasi random_get: buf_len %d exceeds %d", bufLen, maxWASIRandBytes)
				return errnoInval
			}
			buf := make([]byte, bufLen)
			if boot != nil && boot.booting {
				for i := uint32(0); i < bufLen; i++ {
					buf[i] = bootSeedByte(i)
				}
			} else {
				for i := uint32(0); i < bufLen; i += 8 {
					v := host.RandomU64()
					for j := uint32(0); j < 8 && i+j < bufLen; j++ {
						buf[i+j] = byte(v >> (8 * j))
					}
				}
			}
			if !mod.Memory().Write(bufPtr, buf) {
				hostWarnf(host, "wasi random_get: buf ptr %#x/%d out of bounds", bufPtr, bufLen)
				return errnoInval
			}
			return 0
		}},
		// Every clock_id aliases the engine clock, in nanoseconds.
		{"clock_time_get", func(_ context.Context, mod api.Module, _clockID uint32, _precision uint64, timePtr uint32) uint32 {
			const errnoInval = 28
			ns := host.NowMicros() * 1000
			buf := []byte{
				byte(ns), byte(ns >> 8), byte(ns >> 16), byte(ns >> 24),
				byte(ns >> 32), byte(ns >> 40), byte(ns >> 48), byte(ns >> 56),
			}
			if !mod.Memory().Write(timePtr, buf) {
				hostWarnf(host, "wasi clock_time_get: time ptr %#x out of bounds", timePtr)
				return errnoInval
			}
			return 0
		}},
	}
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// bootSeedByte is pinned by TestBootRandomGetIsFixedSeed; goldens and map
// iteration order rest on it.
func bootSeedByte(i uint32) byte { return byte(0xA5 ^ i) }
