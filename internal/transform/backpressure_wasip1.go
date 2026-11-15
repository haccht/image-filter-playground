package transform

// Prevent another async export from entering while image work or I/O is active.
//
//go:wasmimport $root [backpressure-inc]
func backpressureInc()

//go:wasmimport $root [backpressure-dec]
func backpressureDec()
