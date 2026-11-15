package imageproc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"testing"
)

func TestResizeToFit(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 13, 22))
	colors := []color.NRGBA{
		{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255},
		{R: 255, G: 255, B: 255, A: 255}, {A: 255}, {R: 255, G: 255, B: 255, A: 255},
	}
	for i, c := range colors {
		src.SetNRGBA(10+i%3, 20+i/3, c)
	}
	before := append([]byte(nil), src.Pix...)
	for _, input := range []image.Image{src, genericImage{src}} {
		got, err := ResizeToFit(input, 2)
		if err != nil || got.Bounds() != image.Rect(0, 0, 2, 1) {
			t.Fatalf("%T: bounds %v, %v", input, got, err)
		}
		for x, want := range []color.RGBA{{R: 255, G: 127, B: 127, A: 255}, {R: 63, G: 127, B: 127, A: 255}} {
			if actual := color.RGBAModel.Convert(got.At(x, 0)); actual != want {
				t.Fatalf("pixel %d: got %v, want %v", x, actual, want)
			}
		}
	}
	if !bytes.Equal(src.Pix, before) {
		t.Fatal("source was mutated")
	}
	if same, err := ResizeToFit(src, 16); err != nil || same != src {
		t.Fatal("small source was unnecessarily copied or enlarged")
	}
	for _, tc := range []struct{ w, h, limit, dw, dh int }{
		{1024, 768, 512, 512, 384}, {768, 1024, 512, 384, 512},
		{2048, 512, 1024, 1024, 256}, {4096, 1, 512, 512, 1},
		{1, 4096, 512, 1, 512}, {513, 257, 512, 512, 256},
		{1, 1, 512, 1, 1},
	} {
		got, err := ResizeToFit(image.NewGray(image.Rect(0, 0, tc.w, tc.h)), tc.limit)
		if err != nil || got.Bounds().Dx() != tc.dw || got.Bounds().Dy() != tc.dh {
			t.Fatalf("%+v: %v, %v", tc, got, err)
		}
	}
	alpha := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	alpha.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	got, err := ResizeToFit(alpha, 1)
	if err != nil || color.RGBAModel.Convert(got.At(0, 0)) != (color.RGBA{R: 255, G: 191, B: 191, A: 255}) {
		t.Fatalf("alpha compositing before averaging: %v, %v", got, err)
	}
	for _, limit := range []int{0, -1, MaxDimension + 1} {
		if _, err := ResizeToFit(src, limit); !errors.Is(err, ErrDimensions) {
			t.Fatal("invalid output limit admitted", limit, err)
		}
	}
	if _, err := ResizeToFit(image.NewRGBA(image.Rectangle{}), 512); !errors.Is(err, ErrDimensions) {
		t.Fatal("empty image admitted", err)
	}
}

func TestSourceAdmission(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		got, err := Prepare(encoded(t, format, 1024, 768), 512)
		if err != nil || got.Bounds() != image.Rect(0, 0, 512, 384) {
			t.Fatalf("%s: %v, %v", format, got, err)
		}
	}
	for _, tc := range []struct{ w, h int }{{4097, 1}, {1, 4097}, {1025, 1024}} {
		header := encoded(t, "png", tc.w, tc.h)[:33]
		if _, _, err := InspectSource(header); !errors.Is(err, ErrSourceDimensions) {
			t.Fatalf("oversized header %+v: %v", tc, err)
		}
	}
	// A legal 16-bit RGBA header exceeds the decode estimate before any pixel
	// allocation. The IDAT is deliberately absent.
	header := append([]byte(nil), encoded(t, "png", 1024, 1024)[:33]...)
	header[24], header[25] = 16, 6
	binary.BigEndian.PutUint32(header[29:], crc32.ChecksumIEEE(header[12:29]))
	if _, _, err := InspectSource(header); !errors.Is(err, ErrDecodeBudget) {
		t.Fatal("16-bit budget not enforced:", err)
	}
	header[24], header[28] = 8, 1
	binary.BigEndian.PutUint32(header[29:], crc32.ChecksumIEEE(header[12:29]))
	if _, _, err := InspectSource(header); !errors.Is(err, ErrDecodeBudget) {
		t.Fatal("interlace budget not enforced:", err)
	}
	if _, _, err := InspectSource(make([]byte, MaxBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	if _, err := Prepare(encoded(t, "png", 2, 2), 0); !errors.Is(err, ErrDimensions) {
		t.Fatal(err)
	}
}

func TestJPEGFrameBounds(t *testing.T) {
	for _, data := range [][]byte{nil, {0xff}, {0xff, 0xd8}, {0xff, 0xd8, 0xff}, {0xff, 0xd8, 0xff, 0xe0, 0, 1}, {0xff, 0xd8, 0xff, 0xe0, 0xff, 0xff}, {0xff, 0xd8, 0xff, 0xda, 0, 2}} {
		if _, _, err := jpegFrame(data); !errors.Is(err, ErrInvalid) {
			t.Fatalf("malformed JPEG header accepted: %x", data)
		}
	}
	for _, progressive := range []bool{false, true} {
		marker := byte(0xc0)
		if progressive {
			marker = 0xc2
		}
		// Minimal frame traits after an APP segment and a marker-fill byte.
		data := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 2, 0xff, 0xff, marker, 0, 8, 8, 0, 1, 0, 1, 3}
		got, components, err := jpegFrame(data)
		if err != nil || got != progressive || components != 3 {
			t.Fatalf("frame traits: %v %d %v", got, components, err)
		}
	}
}

func TestGrayscaleToFitMatchesColorPipeline(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(10, 20, 23, 29))
	for y := 20; y < 29; y++ {
		for x := 10; x < 23; x++ {
			src.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x * 2345), G: uint16(y * 1753), B: uint16(x * y * 151), A: uint16((x + y) * 1301)})
		}
	}
	for _, limit := range []int{1, 2, 8, 16} {
		resized, err := ResizeToFit(src, limit)
		if err != nil {
			t.Fatal(err)
		}
		want := Grayscale(resized)
		got, err := GrayscaleToFit(src, limit)
		if err != nil {
			t.Fatal(err)
		}
		gray, ok := got.(*image.Gray)
		if !ok || gray.Bounds() != want.Bounds() || !bytes.Equal(gray.Pix, want.Pix) {
			t.Fatalf("limit %d: fused grayscale differs from reference", limit)
		}
	}
}
