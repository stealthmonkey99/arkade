// SPDX-License-Identifier: GPL-2.0-or-later

// Package assetfile reads and writes the Årkade asset envelope described in
// docs/ASSETS.md: one JSON document embedded in an ordinary container (PNG
// tEXt/zTXt chunk, WAV "arkd" chunk, or the file itself for JSON kinds) and
// named <name>.arkade<kind>.<container>. Marshal output is canonical, so
// writing the same envelope twice yields identical bytes in every container.
//
// It is the public asset format for cartridge authors: a cartridge's tests
// and tools import it by requiring github.com/stealthmonkey99/arkade.
package assetfile
