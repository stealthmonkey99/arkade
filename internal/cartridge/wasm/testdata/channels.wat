;; channels.wat — a fixture that exercises input channels
;; (docs/ASSETS.md, "Input channels") and nothing else.
;;
;; cart_init binds six things and echoes the six handles under the storage
;; key "binds", as six little-endian i32s:
;;
;;   0  p1.a        as a button   — the panel, always offered
;;   1  p1.joy.x    as an axis    — the panel, always offered
;;   2  p1.spinner  as a delta    — only when a profile offers one
;;   3  p1.mat      as a blob     — only when a profile offers one
;;   4  p1.joy.x    as a BUTTON   — a kind disagreement: must be -1
;;   5  rumble      as an output  — reserved: must be -1
;;
;; cart_update reads all four real channels and echoes them under "read",
;; as five little-endian i32s (button, axis, delta, blob byte count) plus
;; the blob's first four bytes.
;;
;; Handles are echoed rather than assumed because the ABI's promise is that
;; a FAILED bind is -1 and a successful one is a handle the reads then
;; answer — a test that only checked the values could not tell "the host
;; offered nothing" from "the host offered zero".
;;
;; See generate.go for the wat2wasm invocation; channels.wasm is checked in
;; alongside so `go test` never needs wabt.
(module
  (import "arkade" "host_input_bind"
    (func $bind (param i32 i32 i32) (result i32)))
  (import "arkade" "host_input_button"
    (func $button (param i32) (result i32)))
  (import "arkade" "host_input_axis"
    (func $axis (param i32) (result i32)))
  (import "arkade" "host_input_delta"
    (func $delta (param i32) (result i32)))
  (import "arkade" "host_input_blob"
    (func $blob (param i32 i32 i32) (result i32)))
  (import "arkade" "host_output_bind"
    (func $obind (param i32 i32) (result i32)))
  (import "arkade" "host_storage_write"
    (func $storage_write (param i32 i32 i32 i32)))

  (memory (export "memory") 2)

  ;; Channel names.
  (data (i32.const 0)  "p1.a")
  (data (i32.const 16) "p1.joy.x")
  (data (i32.const 32) "p1.spinner")
  (data (i32.const 48) "p1.mat")
  (data (i32.const 64) "rumble")
  ;; Storage keys.
  (data (i32.const 80) "binds")
  (data (i32.const 96) "read")
  ;; cart_manifest's JSON.
  (data (i32.const 128) "{\"abi\":1,\"slug\":\"channelfixture\",\"name\":\"Channel Fixture\",\"version\":\"1.0.0\",\"players\":[1],\"icon\":\"assets/icon.png\"}")

  ;; 512: cart_alloc's reused static buffer.
  ;; 1024: the six echoed handles (6 x i32).
  ;; 1088: the echoed reads (4 x i32, then 4 blob bytes).
  ;; 2048: host_input_blob's destination buffer (256 bytes).

  (global $h_button (mut i32) (i32.const -1))
  (global $h_axis   (mut i32) (i32.const -1))
  (global $h_delta  (mut i32) (i32.const -1))
  (global $h_blob   (mut i32) (i32.const -1))

  (func (export "cart_abi_version") (result i32)
    (i32.const 1))

  (func (export "cart_manifest") (result i32 i32)
    (i32.const 128)
    (i32.const 115))

  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 512))

  (func (export "cart_init") (param $cols i32) (param $rows i32)
    ;; button: p1.a, kind 0
    (global.set $h_button (call $bind (i32.const 0) (i32.const 4) (i32.const 0)))
    ;; axis: p1.joy.x, kind 1
    (global.set $h_axis (call $bind (i32.const 16) (i32.const 8) (i32.const 1)))
    ;; delta: p1.spinner, kind 2
    (global.set $h_delta (call $bind (i32.const 32) (i32.const 10) (i32.const 2)))
    ;; blob: p1.mat, kind 3
    (global.set $h_blob (call $bind (i32.const 48) (i32.const 6) (i32.const 3)))

    (i32.store (i32.const 1024) (global.get $h_button))
    (i32.store (i32.const 1028) (global.get $h_axis))
    (i32.store (i32.const 1032) (global.get $h_delta))
    (i32.store (i32.const 1036) (global.get $h_blob))
    ;; p1.joy.x asked for as a BUTTON: the kind disagrees, so -1.
    (i32.store (i32.const 1040)
      (call $bind (i32.const 16) (i32.const 8) (i32.const 0)))
    ;; an output bind: reserved, so -1.
    (i32.store (i32.const 1044)
      (call $obind (i32.const 64) (i32.const 6)))

    (call $storage_write
      (i32.const 80) (i32.const 5)
      (i32.const 1024) (i32.const 24)))

  (func (export "cart_handle_input") (param $ptr i32) (param $len i32))

  (func (export "cart_update") (param $dt i64)
    (i32.store (i32.const 1088) (call $button (global.get $h_button)))
    (i32.store (i32.const 1092) (call $axis   (global.get $h_axis)))
    (i32.store (i32.const 1096) (call $delta  (global.get $h_delta)))
    (i32.store (i32.const 1100)
      (call $blob (global.get $h_blob) (i32.const 2048) (i32.const 256)))
    ;; The blob's first four bytes, so a test can see the payload and not
    ;; just its length.
    (i32.store (i32.const 1104) (i32.load (i32.const 2048)))
    (call $storage_write
      (i32.const 96) (i32.const 4)
      (i32.const 1088) (i32.const 20)))

  (func (export "cart_render"))

  (func (export "cart_teardown"))
)
