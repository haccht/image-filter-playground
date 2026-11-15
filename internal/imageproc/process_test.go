package imageproc

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encoded(t testing.TB, format string, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	var b bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPrepareDecodeValidation(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		t.Run(format, func(t *testing.T) {
			for _, size := range []int{1, 256, 1024} {
				if _, err := PrepareGrayscale(encoded(t, format, size, size)); err != nil {
					t.Fatal(err)
				}
			}
			for _, dims := range [][2]int{{4097, 1}, {1, 4097}, {1025, 1024}} {
				if _, err := PrepareGrayscale(encoded(t, format, dims[0], dims[1])); !errors.Is(err, ErrSourceDimensions) {
					t.Fatalf("dimensions: %v", err)
				}
			}
			data := encoded(t, format, 2, 2)
			if _, err := PrepareGrayscale(data[:len(data)/2]); !errors.Is(err, ErrInvalid) {
				t.Fatalf("truncated: %v", err)
			}
		})
	}
	for _, data := range [][]byte{nil, []byte("GIF89a"), []byte("invalid")} {
		if _, err := PrepareGrayscale(data); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid: %v", err)
		}
	}
	if _, err := PrepareGrayscale(make([]byte, MaxBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	// An oversized IHDR is rejected even when the PNG body is missing.
	data := encoded(t, "png", 4097, 1)
	if _, err := PrepareGrayscale(data[:33]); !errors.Is(err, ErrSourceDimensions) {
		t.Fatalf("header-only oversized: %v", err)
	}
}

func TestGrayscale(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 13, 21))
	src.SetNRGBA(10, 20, color.NRGBA{R: 255, A: 255})
	src.SetNRGBA(11, 20, color.NRGBA{R: 255, A: 128})
	got := Grayscale(src)
	if got.Bounds() != image.Rect(0, 0, 3, 1) {
		t.Fatal(got.Bounds())
	}
	for x, want := range []uint8{76, 165, 255} {
		if value := got.GrayAt(x, 0).Y; value != want {
			t.Errorf("pixel %d: %d, want %d", x, value, want)
		}
	}
	output, err := JPEG(got)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(output))
	if err != nil || decoded.Bounds() != got.Bounds() {
		t.Fatalf("JPEG roundtrip: %v", err)
	}
}

func BenchmarkTransform1024(b *testing.B) {
	data := encoded(b, "png", 1024, 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		img, err := PrepareGrayscale(data)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := JPEG(img); err != nil {
			b.Fatal(err)
		}
	}
}

// genericImage hides concrete pixel accessors to exercise the fallback.
type genericImage struct{ image.Image }

func TestGrayscaleDecodedTypes(t *testing.T) {
	bounds := image.Rect(10, 20, 14, 22)
	sources := []image.Image{
		image.NewNRGBA(bounds), image.NewNRGBA64(bounds),
		image.NewRGBA(bounds), image.NewRGBA64(bounds),
		image.NewGray(bounds), image.NewGray16(bounds),
		image.NewYCbCr(bounds, image.YCbCrSubsampleRatio420),
		image.NewCMYK(bounds),
	}
	colors := []color.Color{
		color.NRGBA64{R: 50000, G: 12000, B: 30000, A: 65535},
		color.NRGBA64{R: 50000, G: 12000, B: 30000, A: 32768},
		color.NRGBA64{R: 50000, G: 12000, B: 30000, A: 0},
		color.NRGBA64{R: 65535, G: 65535, B: 65535, A: 65535},
	}
	for _, src := range sources {
		if setter, ok := src.(interface{ Set(int, int, color.Color) }); ok {
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x, c := range colors {
					setter.Set(bounds.Min.X+x, y, c)
				}
			}
		} else {
			ycbcr := src.(*image.YCbCr)
			for i := range ycbcr.Y {
				ycbcr.Y[i] = uint8(i * 31)
			}
			for i := range ycbcr.Cb {
				ycbcr.Cb[i] = 70
				ycbcr.Cr[i] = 200
			}
		}
		for _, input := range []image.Image{src, genericImage{src}} {
			got := Grayscale(input)
			for y := 0; y < bounds.Dy(); y++ {
				for x := 0; x < bounds.Dx(); x++ {
					r, g, b, a := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
					white := uint32(0xffff) - a
					want := color.GrayModel.Convert(color.RGBA64{R: uint16(r + white), G: uint16(g + white), B: uint16(b + white), A: 0xffff}).(color.Gray)
					if got.GrayAt(x, y) != want {
						t.Fatalf("%T (%d,%d): got %v, want %v", input, x, y, got.GrayAt(x, y), want)
					}
				}
			}
		}
	}
}
