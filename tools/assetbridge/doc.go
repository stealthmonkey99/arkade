// SPDX-License-Identifier: GPL-2.0-or-later

// Command assetbridge lets the hosted asset lab at https://arkade.build read
// and write the asset files of a cartridge on your own disk. It is a small
// loopback file server; the lab does all the editing in the browser and the
// bridge only moves bytes.
//
//	go run ./tools/assetbridge [-addr 127.0.0.1:8140] [-origin URL]... <cartridge-or-library-dir>
//
// The directory is one cartridge when it holds a manifest.json, and
// otherwise a library whose immediate subdirectories holding a
// manifest.json are the cartridges. Cartridges are found once, at start.
//
// On start the bridge prints a fresh token and a link such as
//
//	https://arkade.build/dev/tools/spritelab#bridge=127.0.0.1:8140&token=<t>
//
// Opening the link connects the lab; the fragment never reaches a server.
// Stop the bridge with Ctrl-C; the token dies with the process.
//
// # Security model
//
// Any web page open in your browser can send requests to 127.0.0.1, so the
// bridge assumes every request may come from a hostile page:
//
//   - It binds 127.0.0.1 unless -addr says otherwise, and refuses any
//     request whose Host is not 127.0.0.1:<port> or localhost:<port>, which
//     defeats DNS rebinding.
//   - Every route but health requires the token, a 256-bit random value
//     shown only in your terminal, sent as "Authorization: Bearer <t>" and
//     compared in constant time. The events route takes it as ?token=
//     because EventSource cannot set headers.
//   - CORS is granted only to https://arkade.build, https://www.arkade.build
//     and each -origin. A request carrying any other Origin is refused, and
//     Access-Control-Allow-Origin is never "*". Preflights answer Chrome's
//     Private Network Access check for allowed origins only.
//   - Paths are cartridge-relative: no "..", no absolute paths, no segment
//     starting with ".", and after resolving symlinks the file must still
//     lie inside the cartridge.
//   - Any regular file in a cartridge can be read. Writes and deletes are
//     limited to files under assets/ with an asset suffix (.png, .wav,
//     .arkadeglyph.json, .arkadelights.json, .levels, .arkadelevels) and
//     16 MiB per write. Source code and the manifest cannot be changed.
//
// # Routes
//
//	GET    /api/bridge/health                   {"bridge":"assetbridge","ok":true,"version":"…"}, no token
//	GET    /api/bridge/carts                    [{"slug","title"}]
//	GET    /api/bridge/files?cart=&dir=         [{"name","size","mtime","dir"}], mtime in Unix milliseconds
//	GET    /api/bridge/file?cart=&path=         the file's bytes
//	PUT    /api/bridge/file?cart=&path=         body is the new content; 204, written atomically, parents created
//	DELETE /api/bridge/file?cart=&path=         204
//	GET    /api/bridge/events?token=            text/event-stream of "change" events, data {"cart","path"}
//
// An empty dir lists the cartridge directory itself. Errors carry a JSON
// body {"error":"…"}: 400 a malformed path, 401 a missing or wrong token,
// 403 a refused host, origin or path, 404 an unknown cartridge or file,
// 413 a write over the cap.
//
// Change events report files under assets/ that were added, modified or
// removed, whoever changed them, including the bridge's own writes. They
// come from polling modification times while at least one client listens.
package main
