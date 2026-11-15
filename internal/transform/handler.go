// Package transform implements the image transformation HTTP endpoint.
package transform

import (
	"errors"
	"image"
	"io"
	"net/http"
	"runtime"

	"image-filter-playground/internal/imageproc"
)

// Admission grants exclusive access. Clear uploaded bytes before releasing it.
var inputBuffer = make([]byte, imageproc.MaxBytes+1)

// Handle serves the Playground UI at / and transforms uploaded images at /api/transform.
func Handle(w http.ResponseWriter, r *http.Request) {
	backpressureInc()
	defer backpressureDec()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/" {
		serveUI(w, r)
		return
	}
	if r.URL.Path != "/api/transform" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method must be POST", http.StatusMethodNotAllowed)
		return
	}
	if r.ContentLength > imageproc.MaxBytes {
		http.Error(w, imageproc.ErrTooLarge.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	opts, err := parseOptions(r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := admission.acquire(r.Context()); err != nil {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "image processor busy", http.StatusServiceUnavailable)
		return
	}
	defer admission.release()
	// Collect after process returns, when its image buffers are no longer live,
	// and before another admitted request starts allocating.
	defer runtime.GC()
	process(w, r, opts)
}

func process(w http.ResponseWriter, r *http.Request, opts options) {
	body := http.MaxBytesReader(w, r.Body, imageproc.MaxBytes)
	defer body.Close()
	// One bounded allocation avoids geometric reader growth and dead copies
	// fragmenting a reused Wasm heap. The extra byte probes unknown-length input.
	data, err := readInput(body, inputBuffer)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, imageproc.ErrTooLarge.Error(), http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "cannot read image", http.StatusBadRequest)
		}
		return
	}
	filtered, err := applyFilter(data, opts)
	// Clear only the written slice immediately after decoding and filtering,
	// before JPEG encoding or response writing.
	clear(data)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, imageproc.ErrSourceDimensions) || errors.Is(err, imageproc.ErrDecodeBudget) || errors.Is(err, imageproc.ErrTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}
	output, err := imageproc.JPEG(filtered)
	if err != nil {
		http.Error(w, "cannot encode image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", `attachment; filename="`+opts.filter+`.jpg"`)
	w.Write(output)
}

func applyFilter(data []byte, opts options) (image.Image, error) {
	switch opts.filter {
	case filterPixelate:
		return imageproc.PreparePixelate(data, opts.blockSize)
	case filterEdge:
		return imageproc.PrepareEdgeWithRoughness(data, opts.roughness)
	default:
		return imageproc.PrepareGrayscale(data)
	}
}

func readInput(r io.Reader, data []byte) ([]byte, error) {
	used, empty := 0, 0
	for used < len(data) {
		n, err := r.Read(data[used:])
		used += n
		if used > imageproc.MaxBytes {
			clear(data[:used])
			return nil, &http.MaxBytesError{Limit: imageproc.MaxBytes}
		}
		if err == io.EOF {
			return data[:used], nil
		}
		if err != nil {
			clear(data[:used])
			return nil, err
		}
		if n == 0 {
			empty++
			if empty == 100 {
				clear(data[:used])
				return nil, io.ErrNoProgress
			}
		} else {
			empty = 0
		}
	}
	clear(data[:used])
	return nil, &http.MaxBytesError{Limit: imageproc.MaxBytes}
}
