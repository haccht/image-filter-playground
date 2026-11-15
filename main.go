package main

import (
	"image-filter-playground/internal/transform"
	"runtime/debug"

	spinhttp "github.com/spinframework/spin-go-sdk/v4/http"
)

func init() {
	// Leave headroom for component code, stacks and adapter buffers under 20 MiB.
	// This is a GC target, not a hard WebAssembly memory limit.
	debug.SetMemoryLimit(8 << 20)
	spinhttp.Handle(transform.Handle)
}

// main is required by the compiler; Spin invokes the registered handler.
func main() {}
