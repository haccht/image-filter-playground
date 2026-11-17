# Architecture

## Overview

`image-filter-playground` is a single Spin 4 HTTP component built with standard Go (`>= 1.27.1`), `spin-go-sdk/v4`, and `componentize-go`.

- `main.go`: Registers the HTTP handler with `spinhttp.Handle` and sets Go's soft memory target (`debug.SetMemoryLimit(8 << 20)`).
- `internal/transform`:
  - Serves the embedded single-page Vanilla HTML/CSS/JS Playground UI on `GET /` (and `HEAD /`) with strict `Content-Security-Policy` headers and without acquiring the image-processing admission slot.
  - Handles `POST /api/transform`, query option parsing (`filter`, `block_size`, `brush_size`, `roughness`), Component Model backpressure, nonblocking single-request admission, bounded body reads into a reusable scratch buffer, immediate input-slice clearing after filter preparation, and HTTP response formatting.
- `internal/imageproc`:
  - `InspectSource`: Validates encoded byte length (`<= 2 MiB`), actual JPEG/PNG headers, source dimensions (`<= 4096` per side, `<= 1,048,576` total pixels), and a conservative format-specific decode allocation budget (`<= 4 MiB`).
  - `ResizeToFit` / `GrayscaleToFit`: Box-filter downsampling to a maximum longest side of 1024 pixels without upscaling. `GrayscaleToFit` fuses downsampling and white-composited luminance conversion into a single `*image.Gray` output buffer, avoiding an intermediate RGBA allocation.
  - `Pixelate` / `PixelateToFit`: Computes block-averaged RGB colors (composited onto white) and exposes a compact block-color `image.Image` view (`pixelatedImage`) that `image/jpeg` encodes directly without allocating a full-size RGB buffer.
  - `EdgeDetection` / `EdgeDetectionWithRoughness` / `PrepareEdgeWithRoughness`: Applies optional O(width × height) sliding-window box smoothing (`roughness=0..4`, default `2`) followed by 3×3 Sobel gradient magnitude extraction (`abs(Gx) + abs(Gy)` clipped to 255). `PrepareEdgeWithRoughness` reuses the initial grayscale buffer as the Sobel output destination when smoothing is active, saving up to 1 MiB of allocation.
  - `OilPaint` / `OilPaintToFit` / `PrepareOil`: Applies a luminance-histogram oil painting filter with configurable brush radius (`brush_size=1..6`, default `4`) and tonal coarseness (`roughness=0..4`, default `2`, corresponding to `32, 24, 16, 12, 8` luminance bins). Uses per-column sliding histograms (`O(width × height × levels)` independent of brush radius), caches quantized luminance bins in the alpha byte as rows enter the window, and writes finished rows back in place via a `(brush_size + 1)`-row ring buffer (`<= 28 KiB`). `PrepareOil` also reuses the decoded 4-byte-per-pixel buffer in place (including during box-filter downsampling of oversized `*image.RGBA`/`*image.NRGBA` sources) to avoid a second 4 MiB full-frame allocation.
  - `JPEG`: Encodes filtered images as JPEG at quality 85 with source metadata stripped.
- `scripts/smoke.go`: Standalone HTTP smoke test verifying `GET /`, all filters, parameter combinations, automatic resizing, native output equivalence, and `400`/`404`/`405`/`413` error paths.
- `scripts/resources`: Standalone deterministic corpus and loopback OTLP metrics collector for testing Spin instance reuse and memory caps.

## Security & Input Validation

1. **Untrusted Content-Type**: Actual magic bytes and codec headers are inspected via `image.DecodeConfig` and custom JPEG/PNG header checks before full pixel decoding.
2. **Decompression bomb protection**: `InspectSource` rejects high-bit-depth/interlaced PNGs and progressive/CMYK JPEGs whose estimated decoder working set exceeds the 4 MiB decode budget, even when the compressed upload is well under 2 MiB.
3. **Bounded memory & scratch hygiene**: Request bodies are read into an exclusively owned `2 MiB + 1 B` buffer (`inputBuffer`). The written slice is zero-cleared (`clear(data)`) immediately after filter preparation (and on any read failure) before releasing the admission gate.
4. **No external side effects**: `allowed_outbound_hosts = []` in `spin.toml`; uploaded images are never logged or persisted.
