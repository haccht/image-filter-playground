# Image Filter Playground

A lightweight Go WebAssembly HTTP application for **Akamai Functions / Spin 4**.

It provides an embedded single-page Vanilla HTML/CSS/JS Playground UI (`GET /`) and a stateless image transformation API (`POST /api/transform`) supporting **Grayscale**, **Pixelate**, **Edge Detection (Sobel)**, and **Oil Paint** filters with strict memory and input bounds. No persistent storage, external AI services, or outbound network access are used.

See [Architecture](docs/architecture.md) for component boundaries and image processing design, [Component Memory Policy](docs/memory-policy.md) for WASIp3 reuse and memory controls, and [Toolchain Notes](docs/toolchain.md) for Spin 4 / Go SDK v4 build requirements.

## Web UI & HTTP API

- **`GET /`**: Serves the embedded Vanilla HTML/CSS/JS Playground UI for selecting or dragging-and-dropping JPEG/PNG images, configuring filter parameters (`block_size`, `brush_size`, `roughness`), optionally pre-scaling oversized images in the browser, previewing side-by-side results, and downloading the processed JPEG.
- **`POST /api/transform`**: Accepts the raw bytes of a JPEG or PNG image (`--data-binary`, not multipart form data). The actual encoded format is validated independently of the request `Content-Type`.

### Input & Processing Limits

- **Maximum request body**: 2 MiB (2,097,152 bytes), enforced even for unknown-length streams.
- **Source dimensions**: At most 4096 pixels per side and 1,048,576 total pixels.
- **Decode allocation budget**: Format-specific estimate must fit within 4 MiB before full decoding (16-bit/Adam7 PNG and progressive/CMYK JPEG use stricter multipliers).
- **Output dimension cap**: Longest side at most 1024 pixels across all filters. Aspect ratio is preserved with integer flooring; smaller images are never enlarged.
- **Output format**: Newly encoded JPEG (quality 85) with source metadata stripped and transparency composited onto white.
- **Caching**: Responses include `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

| Status | Meaning |
| --- | --- |
| 200 | Filtered JPEG (`Content-Type: image/jpeg`) or Playground HTML (`GET /`) |
| 400 | Invalid query parameters, or empty, unsupported, malformed, truncated, or unreadable image |
| 404 | Unknown path |
| 405 | Unsupported HTTP method (`Allow: POST` for `/api/transform`, `Allow: GET, HEAD` for `/`) |
| 413 | Request body, source dimensions, or estimated decode work exceed limits |
| 500 | Output encoding failure |
| 503 | Processor busy or request canceled before admission (`Retry-After: 1`) |

## Filter Options

| Query parameter | Allowed values | Default | Description |
| --- | --- | --- | --- |
| `filter` | `grayscale`, `pixelate`, `edge`, `oil` | `grayscale` | Selects the visual filter |
| `block_size` | Integer `2`–`128` (`filter=pixelate` only) | `16` | Mosaic block size in output pixels |
| `brush_size` | Integer `1`–`6` (`filter=oil` only) | `4` | Oil brush neighborhood radius in output pixels (`(2 * brush_size + 1)` square window) |
| `roughness` | Integer `0`–`4` (`filter=edge` or `filter=oil`) | `2` | Pre-Sobel box smoothing radius (`2 * roughness`) for `edge`, or luminance quantization coarseness (`32, 24, 16, 12, 8` histogram bins) for `oil` |

Unknown parameters, duplicate parameters, malformed queries, and out-of-range values return `400 Bad Request` before reading the request body. Download filenames are set to `grayscale.jpg`, `pixelate.jpg`, `edge.jpg`, or `oil.jpg`.

## Local Build & Execution

Verified toolchain: **Go >= 1.27.1**, **Spin 4.3+**, **spin-go-sdk/v4 v4.0.0**, and **componentize-go v0.5.3**. See [Toolchain Notes](docs/toolchain.md) for details.

```sh
go mod download
gofmt -w main.go internal/imageproc internal/transform scripts
go test -race ./internal/... ./scripts/...
spin build
spin up --listen 127.0.0.1:3000 --disable-cache --max-instance-memory 20971520
```

Open `http://127.0.0.1:3000/` in your browser to use the Playground UI, or send requests from another terminal:

```sh
# Grayscale (default)
curl --fail-with-body --data-binary @input.png \
  -H 'Content-Type: image/png' \
  'http://127.0.0.1:3000/api/transform' \
  --output grayscale.jpg

# Pixelate
curl --fail-with-body --data-binary @input.jpg \
  -H 'Content-Type: image/jpeg' \
  'http://127.0.0.1:3000/api/transform?filter=pixelate&block_size=16' \
  --output pixelate.jpg

# Edge Detection (Sobel with smoothing roughness 2)
curl --fail-with-body --data-binary @input.jpg \
  -H 'Content-Type: image/jpeg' \
  'http://127.0.0.1:3000/api/transform?filter=edge&roughness=2' \
  --output edge.jpg

# Oil Paint (luminance histogram mode with brush radius 4 and roughness 2)
curl --fail-with-body --data-binary @input.jpg \
  -H 'Content-Type: image/jpeg' \
  'http://127.0.0.1:3000/api/transform?filter=oil&brush_size=4&roughness=2' \
  --output oil.jpg

# Automated 54-request HTTP smoke test
go run scripts/smoke.go http://127.0.0.1:3000
```

## Component Resource Harness

A standalone local resource harness (`scripts/resources`) exercises 22 deterministic valid and invalid inputs (including progressive JPEG and 16-bit Adam7 PNG fixtures) and collects Spin's OTLP component telemetry (`spin.component_cpu_time` and `spin.component_memory_used`) over loopback. Stop any manually running Spin process on port 3000 before running:

```sh
spin build
mkdir -p .cache
go run ./scripts/resources -memory-mib 20 -repeats 3 > .cache/isolated-20mib.json
go run ./scripts/resources -edge -roughness 4 -mode reuse -memory-mib 20 -repeats 5 > .cache/reuse-20mib.json
go run ./scripts/resources -oil -brush-size 6 -roughness 0 -mode reuse -memory-mib 20 -repeats 5 > .cache/oil-reuse-20mib.json
go run ./scripts/resources -oil -brush-size 6 -roughness 0 -mode concurrent -concurrency 16 -memory-mib 20 -repeats 3 > .cache/oil-concurrent-20mib.json
```

See [Component Memory Policy](docs/memory-policy.md) for the memory design (Component Model backpressure, nonblocking admission gate, exclusive 2 MiB + 1 B input scratch buffer, and 8 MiB soft GC target).

## Deploying to Akamai Functions

Deploy using the `aka` plugin for Spin 4:

```sh
spin aka login
spin aka deploy
```

Once deployed, run the smoke verifier against your endpoint URL:

```sh
go run scripts/smoke.go https://<your-app-id>.fwf.app
```
