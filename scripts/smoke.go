// Run with: go run scripts/smoke.go [base URL]
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"image-filter-playground/internal/imageproc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	base := "http://127.0.0.1:3000"
	if len(os.Args) > 1 {
		base = os.Args[1]
	}
	client := &http.Client{Timeout: 10 * time.Second}
	var total time.Duration
	successes := 0
	uiResp, err := client.Get(base + "/")
	if err != nil {
		return err
	}
	uiBody, err := io.ReadAll(uiResp.Body)
	uiResp.Body.Close()
	if err != nil {
		return err
	}
	if uiResp.StatusCode != 200 || uiResp.Header.Get("Content-Type") != "text/html; charset=utf-8" || !bytes.Contains(uiBody, []byte("Image Filter Playground")) {
		return fmt.Errorf("GET /: status %d content-type %q", uiResp.StatusCode, uiResp.Header.Get("Content-Type"))
	}
	fmt.Printf("GET /: 200, HTML playground UI (%d bytes)\n", len(uiBody))
	for _, format := range []string{"png", "jpeg"} {
		img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
		for y := 0; y < 1024; y++ {
			for x := 0; x < 1024; x++ {
				img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
			}
		}
		var input bytes.Buffer
		var err error
		if format == "png" {
			err = png.Encode(&input, img)
		} else {
			err = jpeg.Encode(&input, img, nil)
		}
		if err != nil {
			return err
		}
		start := time.Now()
		encodedInput := input.Bytes()
		resp, err := client.Post(base+"/api/transform", "image/"+format, &input)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		elapsed := time.Since(start)
		total += elapsed
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" {
			return fmt.Errorf("%s: %s: %s", format, resp.Status, data)
		}
		output, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if output.Bounds() != img.Bounds() {
			return fmt.Errorf("unexpected dimensions: %v", output.Bounds())
		}
		for y := 0; y < 1024; y++ {
			for x := 0; x < 1024; x++ {
				r, g, b, _ := output.At(x, y).RGBA()
				if r != g || g != b {
					return fmt.Errorf("output is not grayscale")
				}
			}
		}
		fmt.Printf("%s: 200, grayscale JPEG 1024x1024, %d bytes, latency %s\n", format, len(data), elapsed)
		successes++
		// Both filters now preserve an admitted 1024x1024 source.
		decodedInput, err := imageproc.Prepare(encodedInput, imageproc.MaxDimension)
		if err != nil {
			return err
		}
		if decodedInput.Bounds() != image.Rect(0, 0, 1024, 1024) {
			return fmt.Errorf("unexpected resized reference dimensions: %v", decodedInput.Bounds())
		}
		for _, level := range []int{-1, 0, 1, 2, 3, 4} {
			gray := imageproc.Grayscale(decodedInput)
			effectiveLevel := level
			if effectiveLevel < 0 {
				effectiveLevel = 2
			}
			edgeImage, err := imageproc.EdgeDetectionWithRoughness(gray, effectiveLevel)
			if err != nil {
				return err
			}
			query := "?filter=edge"
			if level >= 0 {
				query += "&roughness=" + strconv.Itoa(level)
			}
			edgeReference, err := imageproc.JPEG(edgeImage)
			if err != nil {
				return err
			}
			started := time.Now()
			resp, err := client.Post(base+"/api/transform"+query, "image/"+format, bytes.NewReader(encodedInput))
			if err != nil {
				return err
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			elapsed := time.Since(started)
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Content-Disposition") != `attachment; filename="edge.jpg"` || !bytes.Equal(data, edgeReference) {
				return fmt.Errorf("%s edge roughness %d: mismatched response status %d", format, level, resp.StatusCode)
			}
			total += elapsed
			successes++
			fmt.Printf("%s: 200, edge roughness %d (-1=omitted), native-reference JPEG match, latency %s\n", format, level, elapsed)
		}
		for _, blockSize := range []int{2, 16, 128} {
			filtered, err := imageproc.Pixelate(decodedInput, blockSize)
			if err != nil {
				return err
			}
			reference, err := imageproc.JPEG(filtered)
			if err != nil {
				return err
			}
			start := time.Now()
			query := "?filter=pixelate"
			if blockSize != imageproc.DefaultBlockSize {
				query += "&block_size=" + strconv.Itoa(blockSize)
			}
			resp, err := client.Post(base+"/api/transform"+query, "image/"+format, bytes.NewReader(encodedInput))
			if err != nil {
				return err
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			elapsed := time.Since(start)
			total += elapsed
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Content-Disposition") != `attachment; filename="pixelate.jpg"` || !bytes.Equal(data, reference) {
				return fmt.Errorf("%s pixelate block %d: status %d or mismatched output/headers", format, blockSize, resp.StatusCode)
			}
			fmt.Printf("%s: 200, pixelate block %d, native-reference JPEG match, latency %s\n", format, blockSize, elapsed)
			successes++
		}
		for _, tc := range []struct {
			query                string
			brushSize, roughness int
		}{
			{"?filter=oil", imageproc.DefaultBrushSize, imageproc.DefaultRoughness},
			{"?filter=oil&brush_size=1&roughness=0", 1, 0},
			{"?filter=oil&brush_size=6&roughness=4", 6, 4},
		} {
			filtered, err := imageproc.OilPaint(decodedInput, tc.brushSize, tc.roughness)
			if err != nil {
				return err
			}
			reference, err := imageproc.JPEG(filtered)
			if err != nil {
				return err
			}
			start := time.Now()
			resp, err := client.Post(base+"/api/transform"+tc.query, "image/"+format, bytes.NewReader(encodedInput))
			if err != nil {
				return err
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			elapsed := time.Since(start)
			total += elapsed
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Content-Disposition") != `attachment; filename="oil.jpg"` || !bytes.Equal(data, reference) {
				return fmt.Errorf("%s oil %s: status %d or mismatched output/headers", format, tc.query, resp.StatusCode)
			}
			fmt.Printf("%s: 200, oil %s, native-reference JPEG match, latency %s\n", format, tc.query, elapsed)
			successes++
		}
	}
	wide := image.NewNRGBA(image.Rect(0, 0, 2048, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 2048; x++ {
			wide.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var wideInput bytes.Buffer
	if err := png.Encode(&wideInput, wide); err != nil {
		return err
	}
	resized, err := imageproc.ResizeToFit(wide, imageproc.MaxDimension)
	if err != nil {
		return err
	}
	for _, filter := range []string{"grayscale", "pixelate", "edge", "oil"} {
		var expected image.Image
		switch filter {
		case "pixelate":
			expected, err = imageproc.Pixelate(resized, imageproc.DefaultBlockSize)
			if err != nil {
				return err
			}
		case "oil":
			expected, err = imageproc.OilPaint(resized, imageproc.DefaultBrushSize, imageproc.DefaultRoughness)
			if err != nil {
				return err
			}
		default:
			gray := imageproc.Grayscale(resized)
			expected = gray
			if filter == "edge" {
				expected, err = imageproc.EdgeDetectionWithRoughness(gray, 2)
				if err != nil {
					return err
				}
			}
		}
		reference, err := imageproc.JPEG(expected)
		if err != nil {
			return err
		}
		start := time.Now()
		resp, err := client.Post(base+"/api/transform?filter="+filter, "image/png", bytes.NewReader(wideInput.Bytes()))
		if err != nil {
			return err
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		elapsed := time.Since(start)
		total += elapsed
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Cache-Control") != "no-store" || !bytes.Equal(data, reference) {
			return fmt.Errorf("%s wide source: status %d or mismatched output/headers", filter, resp.StatusCode)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 1024 || cfg.Height != 256 {
			return fmt.Errorf("%s wide resize: %v, %v", filter, cfg, err)
		}
		fmt.Printf("%s: 200, source 2048x512 resized to 1024x256, native-reference JPEG match, latency %s\n", filter, elapsed)
		successes++
	}
	var big bytes.Buffer
	if err := png.Encode(&big, image.NewGray(image.Rect(0, 0, imageproc.MaxInputDimension+1, 1))); err != nil {
		return err
	}
	var pixelateOversized bytes.Buffer
	if err := png.Encode(&pixelateOversized, image.NewGray(image.Rect(0, 0, imageproc.MaxInputDimension+1, 1))); err != nil {
		return err
	}
	for _, tc := range []struct {
		method, path string
		data         []byte
		want         int
	}{
		{"POST", "/", nil, 405},
		{"GET", "/api/transform", nil, 405},
		{"POST", "/missing", nil, 404},
		{"POST", "/api/transform", []byte("invalid"), 400},
		{"POST", "/api/transform", big.Bytes(), 413},
		{"POST", "/api/transform", make([]byte, (2<<20)+1), 413},
		{"POST", "/api/transform?filter=other", nil, 400},
		{"POST", "/api/transform?filter=edge&block_size=16", nil, 400},
		{"POST", "/api/transform?filter=edge&roughness=-1", nil, 400},
		{"POST", "/api/transform?filter=edge&roughness=5", nil, 400},
		{"POST", "/api/transform?filter=edge&roughness=1.5", nil, 400},
		{"POST", "/api/transform?filter=edge&roughness=1&roughness=2", nil, 400},
		{"POST", "/api/transform?filter=pixelate&roughness=1", nil, 400},
		{"POST", "/api/transform?filter=pixelate&block_size=1", nil, 400},
		{"POST", "/api/transform?filter=pixelate&block_size=129", nil, 400},
		{"POST", "/api/transform?filter=pixelate&filter=grayscale", nil, 400},
		{"POST", "/api/transform?filter=pixelate", pixelateOversized.Bytes()[:33], 413},
		{"POST", "/api/transform?filter=pixelate", []byte("invalid"), 400},
		{"POST", "/api/transform?filter=oil&brush_size=0", nil, 400},
		{"POST", "/api/transform?filter=oil&brush_size=7", nil, 400},
		{"POST", "/api/transform?filter=oil&roughness=5", nil, 400},
		{"POST", "/api/transform?filter=oil&block_size=16", nil, 400},
		{"POST", "/api/transform?filter=edge&brush_size=4", nil, 400},
	} {
		req, err := http.NewRequest(tc.method, base+tc.path, bytes.NewReader(tc.data))
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			return fmt.Errorf("%s %s: %d, want %d", tc.method, tc.path, resp.StatusCode, tc.want)
		}
		fmt.Printf("%s %s: %d as expected\n", tc.method, tc.path, resp.StatusCode)
	}
	fmt.Printf("Success latency total (%d requests): %s; target %s; end-to-end HTTP, not CPU time\n", successes, total, base)
	return nil
}
