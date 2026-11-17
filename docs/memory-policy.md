# Component Memory Policy

## Design Summary

When a Spin 4 / WASIp3 component instance is reused across sequential or concurrent HTTP requests under a tight linear memory cap (such as 20 MiB), standard Go heap growth and large contiguous allocations (e.g., 16-bit PNG or progressive JPEG decoding) can cause heap fragmentation or out-of-memory traps if multiple requests overlap inside one instance.

To keep linear memory bounded and predictable across reused lifetimes, the component enforces four coordinated mechanisms:

1. **Component Model Backpressure (`[backpressure-inc]` / `[backpressure-dec]`)**:
   - Invoked on handler entry and balanced via `defer` on return (`internal/transform/backpressure_wasip1.go`).
   - Signals the Spin host not to dispatch additional concurrent async exports into the active instance while a request is executing, allowing the host to route overlapping clients to other instances.
2. **Nonblocking Application Admission Gate (`internal/transform/admission.go`)**:
   - A capacity-1 nonblocking gate protects the image processing pipeline (`POST /api/transform`).
   - If an overlapping or pre-canceled request enters `POST /api/transform`, it fails fast with `503 Service Unavailable` (`Retry-After: 1`) before reading the request body. No goroutines queue or buffer request bodies in memory.
   - Lightweight UI requests (`GET /`, `HEAD /`) return before acquiring the admission gate.
3. **Exclusive Preallocated Input Scratch Buffer (`inputBuffer`)**:
   - A single `2 MiB + 1 B` buffer is allocated at package initialization and owned exclusively by the admitted request.
   - Avoids repeated geometric `io.ReadAll` slice growth that would fragment the Wasm linear memory heap prior to image decoding.
   - Only the written slice (`data[:used]`) is zero-cleared immediately after decoding/filtering (and on any read error), ensuring uploaded bytes are never retained between requests.
4. **Unified Single-Buffer Filter Preparation & Post-Request GC**:
   - All filters share `InspectSource` (`<= 4 MiB` decode budget) and avoid holding duplicate full-size intermediate RGBA buffers:
     - `GrayscaleToFit` fuses downsampling and luminance conversion into one `*image.Gray` (`<= 1 MiB`).
     - `PixelateToFit` produces a compact block-color view (`*pixelatedImage`, `<= 1 MiB`).
     - `PrepareEdgeWithRoughness` reuses the initial `*image.Gray` buffer as the Sobel destination after box smoothing (`<= 2 MiB` total grayscale buffers).
     - `PrepareOil` reuses the decoded 4-byte-per-pixel slice in place for `*image.RGBA`/`*image.NRGBA` sources (including in-place box-filter downsampling when source dimensions exceed 1024px) and runs the sliding-window luminance histogram filter in place via a `(brush_size + 1)`-row ring buffer (`<= 28 KiB`) and column histograms (`<= 256 KiB`).
   - `main.go` sets `debug.SetMemoryLimit(8 << 20)` (8 MiB soft Go heap target) to leave headroom for Wasm code, stacks, and runtime structures within a 20 MiB instance budget.
   - `defer runtime.GC()` runs when `process()` returns—after intermediate image buffers become unreachable and before the admission gate is released.

## Local Verification Commands

Ensure no manual `spin up` process is listening on port 3000, then run:

```sh
spin build
mkdir -p .cache
go run ./scripts/resources -memory-mib 20 -repeats 3 > .cache/isolated-20mib.json
go run ./scripts/resources -edge -roughness 4 -mode reuse -memory-mib 20 -repeats 5 > .cache/reuse-20mib.json
go run ./scripts/resources -oil -brush-size 6 -roughness 0 -mode reuse -memory-mib 20 -repeats 5 > .cache/oil-reuse-20mib.json
go run ./scripts/resources -oil -brush-size 6 -roughness 0 -mode concurrent -concurrency 16 -memory-mib 20 -repeats 3 > .cache/oil-concurrent-20mib.json
```

In local validation on Apple Silicon (macOS arm64, Spin 4.3.0, Go 1.27.4), mixed reuse and 16-worker concurrent workloads across all four filters (including `oil` at maximum `brush_size=6`, `roughness=0`) completed with zero unexpected errors and a peak observed component linear memory of **15.0 MiB** in sequential reuse and **18.0 MiB** under 16-client concurrent bursts (within the 20 MiB cap).

## Official References

- [Spin 4 HTTP Trigger — Controlling Instance Reuse](https://spinframework.dev/v4/http-trigger#controlling-instance-reuse)
- [WebAssembly Component Model — Concurrency & Backpressure](https://github.com/WebAssembly/component-model/blob/main/design/mvp/Concurrency.md#backpressure)
- [Go `runtime/debug.SetMemoryLimit`](https://pkg.go.dev/runtime/debug#SetMemoryLimit)
- [Akamai Functions Quotas and Limits](https://techdocs.akamai.com/akamai-functions/docs/quotas-and-limits)
