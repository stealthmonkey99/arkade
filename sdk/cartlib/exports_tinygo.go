// SPDX-License-Identifier: GPL-2.0-or-later

//go:build tinygo

package cartlib

//export cart_abi_version
func cartABIVersion() uint32 { return AbiVersion }

// TinyGo lowers the two results to an output pointer; tools/wasmfix restores them.
//
//export cart_manifest
func cartManifest() (uint32, uint32) {
	b := currentGame.Manifest()
	return ptrOf(b), uint32(len(b))
}

// cartAlloc returns cartlib's static input buffer whatever size is asked
// for; the host writes each batch of input records there before
// cart_handle_input.
//
//export cart_alloc
func cartAlloc(size uint32) uint32 { return ptrOf(allocBuf[:]) }

// cols and rows are always FBWidth and FBHeight.
//
//export cart_init
func cartInit(cols, rows uint32) { dispatchInit() }

//export cart_handle_input
func cartHandleInput(ptr, length uint32) {
	if length > uint32(len(allocBuf)) {
		length = uint32(len(allocBuf))
	}
	dispatchHandleInput(allocBuf[:length])
}

//export cart_update
func cartUpdate(dtMicros uint64) { dispatchUpdate(dtMicros) }

//export cart_render
func cartRender() { dispatchRender() }

//export cart_teardown
func cartTeardown() { dispatchTeardown() }
