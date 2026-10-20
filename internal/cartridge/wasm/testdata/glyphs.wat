;; glyphs.wat — a fixture that exercises host_define_lcd_glyph's boundary
;; (sdk/docs/abi.md, "cartridge-defined LCD glyphs") and nothing else.
;;
;; It is its own fixture rather than more calls bolted onto good.wat because
;; the interesting cases are the REJECTIONS, and proving those needs a cart
;; that deliberately does the wrong thing 14 times — which is not the
;; "well-behaved cartridge" good.wat exists to be.
;;
;; What cart_init does, in order:
;;   1. defines U+058D (֍, the tutorial's basket) with a known 8-row bitmap
;;   2. tries 'A' (U+0041) — must be REJECTED: the panel's ROM draws ASCII
;;   3. tries U+D800 — must be REJECTED: a surrogate is not a code point
;;   4. tries U+1F9FA (🧺) — must be REJECTED: an emoji (internal/lcdrune)
;;   5. tries U+4E00 (一) — must be REJECTED: East Asian wide
;;   6. defines U+E000..U+E028 (41 private-use runes) in a loop — the first
;;      31 fit (basket + 31 = the 32-glyph table cap), the last 10 bounce
;;   7. sums every return value and stores it under "accepted", so the test
;;      can pin the RETURN semantics and not just the resulting table
;;
;; Expected total: 1 + 0 + 0 + 0 + 0 + 31 = 32.
;;
;; See generate.go for the wat2wasm invocation; glyphs.wasm is checked in
;; alongside so `go test` never needs wabt.
(module
  (import "arkade" "host_define_lcd_glyph"
    (func $define (param i32 i64) (result i32)))
  (import "arkade" "host_storage_write"
    (func $storage_write (param i32 i32 i32 i32)))

  (memory (export "memory") 1)

  ;; 0: the storage key the accepted-count is echoed under.
  (data (i32.const 0) "accepted")
  ;; 16: cart_manifest's JSON.
  (data (i32.const 16) "{\"abi\":1,\"slug\":\"glyphfixture\",\"name\":\"Glyph Fixture\",\"version\":\"1.0.0\",\"players\":[1],\"icon\":\"assets/icon.png\"}")
  ;; 256: cart_alloc's reused static buffer.
  ;; 512: 4-byte scratch the accepted count is written into.

  (func (export "cart_abi_version") (result i32)
    (i32.const 1))

  (func (export "cart_manifest") (result i32 i32)
    (i32.const 16)
    (i32.const 110))

  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 256))

  (func (export "cart_init") (param $cols i32) (param $rows i32)
    (local $accepted i32)
    (local $i i32)

    ;; The basket, ֍ U+058D. Rows packed low byte = row 0 (top):
    ;;   row0 0x00  row1 0x0e  row2 0x11  row3 0x1f
    ;;   row4 0x15  row5 0x1f  row6 0x0e  row7 0x00
    ;; => 0x000e1f151f110e00
    (local.set $accepted
      (call $define (i32.const 0x058D) (i64.const 0x000e1f151f110e00)))

    ;; 'A' — ASCII, rejected: the HD44780's own mask ROM draws it, and
    ;; redefining it would burn a CGRAM slot on every line containing it.
    (local.set $accepted (i32.add (local.get $accepted)
      (call $define (i32.const 0x41) (i64.const 0x1f1f1f1f1f1f1f1f))))

    ;; U+D800 — a surrogate half, not a code point at all.
    (local.set $accepted (i32.add (local.get $accepted)
      (call $define (i32.const 0xD800) (i64.const 0x1f1f1f1f1f1f1f1f))))

    ;; 🧺 U+1F9FA — an emoji: a terminal draws it in colour and two cells
    ;; wide, so no surface that shows LCD text could draw it evenly.
    (local.set $accepted (i32.add (local.get $accepted)
      (call $define (i32.const 0x1F9FA) (i64.const 0x000e1f151f110e00))))

    ;; 一 U+4E00 — East Asian wide, two cells by definition.
    (local.set $accepted (i32.add (local.get $accepted)
      (call $define (i32.const 0x4E00) (i64.const 0x1f1f1f1f1f1f1f1f))))

    ;; 41 private-use runes: 31 fit, then the table is full.
    (local.set $i (i32.const 0))
    (block $done
      (loop $next
        (br_if $done (i32.ge_u (local.get $i) (i32.const 41)))
        (local.set $accepted (i32.add (local.get $accepted)
          (call $define
            (i32.add (i32.const 0xE000) (local.get $i))
            (i64.const 0x0000000000000001))))
        (local.set $i (i32.add (local.get $i) (i32.const 1)))
        (br $next)))

    (i32.store (i32.const 512) (local.get $accepted))
    (call $storage_write (i32.const 0) (i32.const 8) (i32.const 512) (i32.const 4)))

  (func (export "cart_handle_input") (param $ptr i32) (param $len i32))
  (func (export "cart_update") (param $dt i64))
  (func (export "cart_render"))
  (func (export "cart_teardown"))
)
