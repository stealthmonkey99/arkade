# The platform contract

Under [cartlib](cartlib.md) sits the contract every host holds a
cartridge to: what the `.wasm` exports and imports, and how a finished
game is described and packed so a cabinet or a browser can load it.
Most authors never need this layer, because cartlib and `rcartpack`
speak it for you. Read it to debug at the boundary, to write a host, or
to target the ABI from another language.

## The pages

| Page | What it covers |
| --- | --- |
| [Manifests & packaging](packaging.md) | The manifest schema, the `.rcart` bundle format, `rcartpack`, installing on a cabinet |
| [ABI reference](abi.md) | The wasm contract itself — what `cartlib` implements for you: exports, host imports, input records, determinism rules |
