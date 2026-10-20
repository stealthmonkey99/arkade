// SPDX-License-Identifier: GPL-2.0-or-later

package cartlib

// AbiVersion is the docs/CARTRIDGE.md ABI version cartlib implements and
// cart_abi_version reports. A host refuses a cartridge whose version it does
// not support, before cart_init runs.
const AbiVersion uint32 = 1

// Static so the run loop never allocates. allocBuf is what cart_alloc
// returns for every size, which docs/CARTRIDGE.md permits, and holds the
// largest input batch either host sends.
var (
	allocBuf  [maxDecodedRecords * 8]byte
	recordBuf [maxDecodedRecords]InputRecord
	frame     Framebuffer
)
