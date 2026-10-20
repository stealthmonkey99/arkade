;; trap.wat — a cartridge that traps on its first cart_update, the way a
;; TinyGo -panic=trap build does on any runtime panic.
;;
;; cart_render draws a visible square, so a test can tell a frozen last
;; frame from a dark one. It imports host_exit, which hands it the quit keys,
;; and never calls it: once trapped, only the host can honour a quit press.
;; cart_teardown traps too, so a host that tears a faulted guest down
;; anyway is caught.
;;
;; go:generate wat2wasm trap.wat -o trap.wasm
(module
  (import "arkade" "host_fill_rect" (func $host_fill_rect (param i32 i32 i32 i32 i32)))
  (import "arkade" "host_exit" (func $host_exit))

  (memory (export "memory") 1)

  (data (i32.const 0) "{\"abi\":1,\"slug\":\"trap\",\"name\":\"Trap Fixture\",\"version\":\"1.0.0\",\"players\":[1]}")

  (func (export "cart_abi_version") (result i32) (i32.const 1))

  (func (export "cart_manifest") (result i32 i32)
    (i32.const 0) (i32.const 77))

  (func (export "cart_alloc") (param $size i32) (result i32)
    (i32.const 4096))

  (func (export "cart_init") (param $cols i32) (param $rows i32))

  (func (export "cart_handle_input") (param $ptr i32) (param $len i32))

  (func (export "cart_update") (param $dt i64)
    (unreachable))

  (func (export "cart_render")
    (call $host_fill_rect
      (i32.const 0) (i32.const 0) (i32.const 4) (i32.const 4)
      (i32.const -1)))

  (func (export "cart_teardown")
    (unreachable)))
