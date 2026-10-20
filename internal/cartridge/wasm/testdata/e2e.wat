;; e2e.wat — a core-owned fixture cartridge for the tests that used to
;; borrow a real game's checked-in binary (internal/cartridge/e2e_test.go
;; drove snake.wasm; the per-game golden suites now live in each
;; cartridge's own module — public-release plan, migration step 2). It is
;; deliberately size-agnostic, unlike good.wat's fixed 4x4 data segments:
;; cart_init records the host's cols/rows and every blit is sized from
;; them, so the same fixture runs under a 4x4 FakeHost test and the
;; engine's real 64x64 display.
;;
;; What it models, minimally:
;;   - a VISIBLE render: an opaque fill_rect plus a blit whose first four
;;     pixels are opaque white on a transparent background
;;   - cart_attract (optional export): same overlay shape —
;;     transparent background, something opaque on top — and NO audio
;;     calls at all (the fixture doesn't even import the sound functions,
;;     so attract-mode silence holds by construction)
;;   - quit-key ownership: it imports host_exit (the opt-in that makes the
;;     engine forward quit keys — see runtime.go ImportsExit), swallows
;;     the FIRST Escape keypress like a game raising a confirm dialog,
;;     and calls host_exit on the SECOND — the exact two-press contract
;;     TestEndToEndScanRegisterLaunchCartridge scripts.
;;
;; Memory layout:
;;   0     len 75    manifest JSON       cart_manifest() return
;;   1024  len 16384 blit buffer         zero (transparent) + 4 white pixels
;;   20480 len 256   cart_alloc's reused static buffer
;;
;; go:generate wat2wasm e2e.wat -o e2e.wasm
(module
  (import "arkade" "host_fill_rect" (func $host_fill_rect (param i32 i32 i32 i32 i32)))
  (import "arkade" "host_blit" (func $host_blit (param i32 i32)))
  (import "arkade" "host_exit" (func $host_exit))

  (memory (export "memory") 1)

  (data (i32.const 0) "{\"abi\":1,\"slug\":\"e2e\",\"name\":\"E2E Fixture\",\"version\":\"1.0.0\",\"players\":[1]}")

  (global $cols (mut i32) (i32.const 0))
  (global $rows (mut i32) (i32.const 0))
  (global $escapes (mut i32) (i32.const 0))

  (func (export "cart_abi_version") (result i32) (i32.const 1))

  (func (export "cart_manifest") (result i32 i32)
    (i32.const 0) (i32.const 75))

  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 20480))

  (func (export "cart_init") (param $cols i32) (param $rows i32)
    (global.set $cols (local.get $cols))
    (global.set $rows (local.get $rows)))

  ;; cart_handle_input(ptr, len) — scans the packed 8-byte records for an
  ;; Escape keypress (kind 2, key ordinal 3; see keys.go's keyOrdinal).
  ;; First one is swallowed (a real game would open its confirm dialog);
  ;; the second reaches host_exit.
  (func (export "cart_handle_input") (param $ptr i32) (param $len i32)
    (local $i i32)
    (local $n i32)
    (local $rec i32)
    (local.set $n (i32.div_u (local.get $len) (i32.const 8)))
    (block $done
      (loop $next
        (br_if $done (i32.ge_u (local.get $i) (local.get $n)))
        (local.set $rec
          (i32.add (local.get $ptr) (i32.mul (local.get $i) (i32.const 8))))
        (if (i32.and
              (i32.eq (i32.load8_u (local.get $rec)) (i32.const 2))
              (i32.eq (i32.load8_u offset=1 (local.get $rec)) (i32.const 3)))
          (then
            (global.set $escapes (i32.add (global.get $escapes) (i32.const 1)))
            (if (i32.ge_u (global.get $escapes) (i32.const 2))
              (then (call $host_exit)))))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $next))))

  (func (export "cart_update") (param $dt i64))

  ;; $overlay — four opaque white pixels at the top-left of an otherwise
  ;; transparent (zeroed) cols*rows frame, blitted whole.
  (func $overlay
    (i32.store (i32.const 1024) (i32.const -1))
    (i32.store (i32.const 1028) (i32.const -1))
    (i32.store (i32.const 1032) (i32.const -1))
    (i32.store (i32.const 1036) (i32.const -1))
    (call $host_blit
      (i32.const 1024)
      (i32.mul (i32.mul (global.get $cols) (global.get $rows)) (i32.const 4))))

  (func (export "cart_render")
    (call $host_fill_rect
      (i32.const 1) (i32.const 1) (i32.const 2) (i32.const 2)
      (i32.const 0xFF0000FF))
    (call $overlay))

  (func (export "cart_attract") (param $dt i64)
    (call $host_fill_rect
      (i32.const 0) (i32.const 0) (i32.const 2) (i32.const 2)
      (i32.const 0x00FF00FF))
    (call $overlay))

  (func (export "cart_teardown")))
