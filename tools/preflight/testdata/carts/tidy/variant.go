//go:build !variant

// Fixture: a variant build split (cartridges/*/branding.go). This
// constraint does NOT exclude the file from the TinyGo build — `variant` is
// off by default — so anything wrong in here must be reported. Nothing
// is, deliberately: the file exists to prove the constraint evaluator
// keeps it IN.
package main

const brandTitle = "TIDY"
