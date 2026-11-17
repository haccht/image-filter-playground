package transform

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"strconv"
	"testing"

	"image-filter-playground/internal/imageproc"
)

func TestPixelateOptions(t *testing.T) {
	for query, want := range map[string]options{
		"":                                    {"grayscale", 16, 0, 4},
		"filter=grayscale":                    {"grayscale", 16, 0, 4},
		"filter=edge&roughness=4":             {"edge", 16, 4, 4},
		"filter=edge&roughness=0":             {"edge", 16, 0, 4},
		"filter=edge":                         {"edge", 16, 2, 4},
		"filter=pixelate":                     {"pixelate", 16, 0, 4},
		"filter=pixelate&block_size=2":        {"pixelate", 2, 0, 4},
		"filter=pixelate&block_size=128":      {"pixelate", 128, 0, 4},
		"filter=oil":                          {"oil", 16, 2, 4},
		"filter=oil&brush_size=1":             {"oil", 16, 2, 1},
		"filter=oil&brush_size=6":             {"oil", 16, 2, 6},
		"filter=oil&roughness=0":              {"oil", 16, 0, 4},
		"filter=oil&roughness=4":              {"oil", 16, 4, 4},
		"filter=oil&brush_size=6&roughness=0": {"oil", 16, 0, 6},
	} {
		got, err := parseOptions(query)
		if err != nil || got != want {
			t.Fatalf("%s: %+v, %v", query, got, err)
		}
	}
	for _, query := range []string{
		"filter=edge&roughness=-1", "filter=edge&roughness=5", "filter=edge&roughness=", "filter=edge&roughness=1.5", "filter=edge&roughness=x", "filter=edge&roughness=999999999999999999999", "filter=edge&roughness=1&roughness=2", "roughness=1", "filter=pixelate&roughness=1", "filter=edge&block_size=16", "filter=sepia", "filter=pixelate&filter=grayscale", "block_size=8", "other=1", "filter=%ZZ",
		"filter=pixelate;block_size=8", "filter=pixelate&block_size=2&block_size=3",
		"filter=pixelate&block_size=", "filter=pixelate&block_size=-1", "filter=pixelate&block_size=1",
		"filter=pixelate&block_size=129", "filter=pixelate&block_size=2.5", "filter=pixelate&block_size=999999999999999999999",
		"brush_size=4", "filter=grayscale&brush_size=4", "filter=pixelate&brush_size=4", "filter=edge&brush_size=4",
		"filter=oil&block_size=16", "filter=oil&brush_size=0", "filter=oil&brush_size=-1", "filter=oil&brush_size=7",
		"filter=oil&brush_size=", "filter=oil&brush_size=2.5", "filter=oil&brush_size=1&brush_size=2",
		"filter=oil&roughness=-1", "filter=oil&roughness=5", "filter=oil&roughness=", "filter=oil&roughness=1&roughness=2",
	} {
		r := httptest.NewRequest("POST", "/api/transform?"+query, unreadBody{t})
		w := httptest.NewRecorder()
		Handle(w, r)
		if w.Code != 400 {
			t.Fatalf("%s: status %d", query, w.Code)
		}
	}
}

func TestHandlePixelate(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 17, 9))
	for y := 0; y < 9; y++ {
		for x := 0; x < 17; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 15), G: uint8(y * 25), A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, src); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{2, 16, 128} {
		query := "/api/transform?filter=pixelate&block_size=" + strconv.Itoa(size)
		w := httptest.NewRecorder()
		Handle(w, httptest.NewRequest("POST", query, bytes.NewReader(input.Bytes())))
		reference, err := imageproc.Pixelate(src, size)
		if err != nil {
			t.Fatal(err)
		}
		want, err := imageproc.JPEG(reference)
		if err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), want) || w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("Content-Disposition") != `attachment; filename="pixelate.jpg"` {
			t.Fatalf("size %d: status=%d headers=%v", size, w.Code, w.Header())
		}
		if bytes.Count(inputBuffer, []byte{0}) != len(inputBuffer) {
			t.Fatal("uploaded bytes were retained")
		}
	}
}

func TestHandleOil(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 17, 9))
	for y := 0; y < 9; y++ {
		for x := 0; x < 17; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 15), G: uint8(y * 25), B: uint8((x ^ y) * 19), A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, src); err != nil {
		t.Fatal(err)
	}
	for _, brush := range []int{1, 4, 6} {
		for _, roughness := range []int{0, 2, 4} {
			query := "/api/transform?filter=oil&brush_size=" + strconv.Itoa(brush) + "&roughness=" + strconv.Itoa(roughness)
			w := httptest.NewRecorder()
			Handle(w, httptest.NewRequest("POST", query, bytes.NewReader(input.Bytes())))
			reference, err := imageproc.OilPaint(src, brush, roughness)
			if err != nil {
				t.Fatal(err)
			}
			want, err := imageproc.JPEG(reference)
			if err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), want) || w.Header().Get("Content-Type") != "image/jpeg" || w.Header().Get("Content-Disposition") != `attachment; filename="oil.jpg"` {
				t.Fatalf("brush %d roughness %d: status=%d headers=%v", brush, roughness, w.Code, w.Header())
			}
			if bytes.Count(inputBuffer, []byte{0}) != len(inputBuffer) {
				t.Fatal("uploaded bytes were retained")
			}
		}
	}
}

func TestPixelateDimensionAdmission(t *testing.T) {
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewGray(image.Rect(0, 0, 513, 1))); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]int{"?filter=pixelate": 400, "?filter=oil": 400, "": 400} {
		w := httptest.NewRecorder()
		// This source now fits admission; a header alone is malformed input.
		Handle(w, httptest.NewRequest("POST", "/api/transform"+query, bytes.NewReader(input.Bytes()[:33])))
		if w.Code != want {
			t.Fatalf("%s: %d, want %d", query, w.Code, want)
		}
	}
}

func TestHandleAutoResize(t *testing.T) {
	for _, tc := range []struct {
		format, filter string
		w, h, dw, dh   int
	}{
		{"jpeg", "edge", 1024, 768, 1024, 768},
		{"png", "edge", 2048, 512, 1024, 256},
		{"png", "edge", 13, 7, 13, 7},
		{"jpeg", "pixelate", 1024, 768, 1024, 768},
		{"jpeg", "pixelate", 768, 1024, 768, 1024},
		{"png", "grayscale", 2048, 512, 1024, 256},
		{"png", "pixelate", 2048, 512, 1024, 256},
		{"png", "pixelate", 1025, 1023, 1024, 1022},
		{"png", "pixelate", 13, 7, 13, 7},
		{"jpeg", "oil", 1024, 768, 1024, 768},
		{"png", "oil", 2048, 512, 1024, 256},
		{"png", "oil", 13, 7, 13, 7},
	} {
		var input bytes.Buffer
		src := image.NewGray(image.Rect(0, 0, tc.w, tc.h))
		if tc.format == "png" {
			if err := png.Encode(&input, src); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := jpeg.Encode(&input, src, nil); err != nil {
				t.Fatal(err)
			}
		}
		w := httptest.NewRecorder()
		Handle(w, httptest.NewRequest("POST", "/api/transform?filter="+tc.filter, bytes.NewReader(input.Bytes())))
		if w.Code != 200 {
			t.Fatalf("%+v: status %d", tc, w.Code)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
		if err != nil || cfg.Width != tc.dw || cfg.Height != tc.dh {
			t.Fatalf("%+v: %v, %v", tc, cfg, err)
		}
		if bytes.Count(inputBuffer, []byte{0}) != len(inputBuffer) {
			t.Fatal("source bytes retained")
		}
	}
}

func TestEdgeDefaultRoughness(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 17, 13))
	for y := 0; y < 13; y++ {
		for x := 0; x < 17; x++ {
			src.Pix[y*src.Stride+x] = uint8(x*37 + y*71)
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, src); err != nil {
		t.Fatal(err)
	}
	results := make(map[string][]byte)
	for _, query := range []string{"filter=edge", "filter=edge&roughness=2", "filter=edge&roughness=0"} {
		w := httptest.NewRecorder()
		Handle(w, httptest.NewRequest("POST", "/api/transform?"+query, bytes.NewReader(input.Bytes())))
		if w.Code != 200 {
			t.Fatalf("%s: status %d", query, w.Code)
		}
		results[query] = bytes.Clone(w.Body.Bytes())
	}
	if !bytes.Equal(results["filter=edge"], results["filter=edge&roughness=2"]) {
		t.Fatal("omitted roughness must match explicit 2")
	}
	if bytes.Equal(results["filter=edge"], results["filter=edge&roughness=0"]) {
		t.Fatal("explicit zero must remain unsmoothed")
	}
}
