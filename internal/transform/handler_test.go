package transform

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http/httptest"
	"testing"

	"image-filter-playground/internal/imageproc"
)

func TestHandle(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 1024, 768))
			img.Set(0, 0, color.RGBA{R: 255, A: 255})
			var input bytes.Buffer
			if format == "png" {
				png.Encode(&input, img)
			} else {
				jpeg.Encode(&input, img, nil)
			}
			r := httptest.NewRequest("POST", "/api/transform", &input)
			r.Header.Set("Content-Type", "text/plain") // Actual bytes determine the format.
			w := httptest.NewRecorder()
			Handle(w, r)
			if bytes.Count(inputBuffer, []byte{0}) != len(inputBuffer) {
				t.Fatal("input scratch buffer retained uploaded bytes")
			}
			if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			output, err := jpeg.Decode(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if output.Bounds() != img.Bounds() {
				t.Fatal(output.Bounds())
			}
			for y := 0; y < img.Bounds().Dy(); y++ {
				for x := 0; x < img.Bounds().Dx(); x++ {
					r, g, b, _ := output.At(x, y).RGBA()
					if r != g || g != b {
						t.Fatal("output is not grayscale")
					}
				}
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing no-store")
			}
		})
	}
}

type unreadBody struct{ t *testing.T }

func (b unreadBody) Read([]byte) (int, error) {
	b.t.Error("rejected request body was read")
	return 0, io.EOF
}

func TestAdmissionRejections(t *testing.T) {
	for range cap(admission.slots) {
		admission.slots <- struct{}{}
	}
	defer func() {
		for len(admission.slots) > 0 {
			<-admission.slots
		}
	}()
	for _, canceled := range []bool{false, true} {
		r := httptest.NewRequest("POST", "/api/transform", unreadBody{t})
		if canceled {
			ctx, cancel := context.WithCancel(r.Context())
			cancel()
			r = r.WithContext(ctx)
		}
		w := httptest.NewRecorder()
		Handle(w, r)
		if w.Code != 503 || w.Header().Get("Retry-After") != "1" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("admission rejection: %d %v", w.Code, w.Header())
		}
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestRejections(t *testing.T) {
	var oversized bytes.Buffer
	png.Encode(&oversized, image.NewGray(image.Rect(0, 0, imageproc.MaxInputDimension+1, 1)))
	for _, tc := range []struct {
		name, method, path string
		body               io.Reader
		length             int64
		want               int
	}{
		{"path", "POST", "/other", nil, 0, 404},
		{"method", "GET", "/api/transform", nil, 0, 405},
		{"empty", "POST", "/api/transform", nil, 0, 400},
		{"spoofed", "POST", "/api/transform", bytes.NewBufferString("not a PNG"), -1, 400},
		{"dimensions", "POST", "/api/transform", bytes.NewReader(oversized.Bytes()), -1, 413},
		{"declared size", "POST", "/api/transform", nil, imageproc.MaxBytes + 1, 413},
		{"streamed size", "POST", "/api/transform", bytes.NewReader(make([]byte, imageproc.MaxBytes+1)), -1, 413},
		{"exact byte limit", "POST", "/api/transform", bytes.NewReader(make([]byte, imageproc.MaxBytes)), -1, 400},
		{"read error", "POST", "/api/transform", brokenReader{}, -1, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, tc.body)
			r.ContentLength = tc.length
			w := httptest.NewRecorder()
			Handle(w, r)
			if bytes.Count(inputBuffer, []byte{0}) != len(inputBuffer) {
				t.Fatal("input scratch buffer retained rejected bytes")
			}
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if tc.want == 405 && w.Header().Get("Allow") != "POST" {
				t.Fatal("missing Allow")
			}
		})
	}
}

func TestServeUI(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		r := httptest.NewRequest(method, "/", nil)
		w := httptest.NewRecorder()
		Handle(w, r)
		if w.Code != 200 {
			t.Fatalf("%s /: status %d", method, w.Code)
		}
		if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Fatalf("%s /: Content-Type %q", method, got)
		}
		if got := w.Header().Get("Content-Security-Policy"); got == "" {
			t.Fatalf("%s /: missing Content-Security-Policy", method)
		}
		if method == "GET" && !bytes.Contains(w.Body.Bytes(), []byte("Image Filter Playground")) {
			t.Fatal("GET /: missing UI body")
		}
		if method == "HEAD" && w.Body.Len() != 0 {
			t.Fatalf("HEAD /: unexpected body length %d", w.Body.Len())
		}
	}
	r := httptest.NewRequest("POST", "/", unreadBody{t})
	w := httptest.NewRecorder()
	Handle(w, r)
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST /: status %d Allow %q", w.Code, w.Header().Get("Allow"))
	}
}
