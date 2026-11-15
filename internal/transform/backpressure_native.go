//go:build !wasip1

package transform

// Native tests exercise the application admission gate instead of a Wasm host.
func backpressureInc() {}
func backpressureDec() {}
