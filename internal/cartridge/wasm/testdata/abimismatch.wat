;; abimismatch.wat — a cartridge built against a foreign ABI. It reports
;; version 99, outside the range this host accepts
;; (runtime.MinSupportedABIVersion..SupportedABIVersion), which is what a
;; cartridge built for a later, breaking ABI looks like to this host.
;; Rejecting it here — cleanly, before cart_init, with one "unsupported ABI
;; version" line — is the entire job the version number exists to do.
;;
;; App.Init must reject it before ever calling cart_init, and must not
;; touch host_* imports at all: none are even imported here, so that
;; guarantee is structural rather than merely behavioral.
;;
;; go:generate wat2wasm abimismatch.wat -o abimismatch.wasm
(module
  (memory (export "memory") 1)

  (func (export "cart_abi_version") (result i32)
    (i32.const 99))

  ;; Everything below is a trivial stub: version-checking happens before
  ;; any of these would be called, but the exports must still exist for
  ;; the module to link and for NewCartridge's export lookup to succeed
  ;; (a real mismatched cartridge is a valid, just incompatible, module).
  (func (export "cart_manifest") (result i32 i32)
    (i32.const 0)
    (i32.const 0))
  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 0))
  (func (export "cart_init") (param $cols i32) (param $rows i32))
  (func (export "cart_handle_input") (param $ptr i32) (param $len i32))
  (func (export "cart_update") (param $dt i64))
  (func (export "cart_render"))
  (func (export "cart_teardown")))
