package imageproc

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strconv"
	"testing"
)

func TestPixelate(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 13, 23))
	colors := []color.NRGBA{
		{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255},
		{R: 255, G: 255, B: 255, A: 255}, {A: 255}, {R: 255, G: 255, B: 255, A: 255},
		{R: 128, G: 128, B: 128, A: 255}, {}, {R: 255, A: 128},
	}
	for i, c := range colors {
		src.SetNRGBA(10+i%3, 20+i/3, c)
	}
	before := append([]byte(nil), src.Pix...)
	got, err := Pixelate(src, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 3, 3) {
		t.Fatal(got.Bounds())
	}
	wants := []color.RGBA{
		{R: 127, G: 127, B: 63, A: 255}, {R: 127, G: 127, B: 63, A: 255}, {R: 127, G: 127, B: 255, A: 255},
		{R: 127, G: 127, B: 63, A: 255}, {R: 127, G: 127, B: 63, A: 255}, {R: 127, G: 127, B: 255, A: 255},
		{R: 191, G: 191, B: 191, A: 255}, {R: 191, G: 191, B: 191, A: 255}, {R: 255, G: 127, B: 127, A: 255},
	}
	for i, want := range wants {
		if actual := color.RGBAModel.Convert(got.At(i%3, i/3)); actual != want {
			t.Fatalf("pixel %d: got %v, want %v", i, actual, want)
		}
	}
	if _, _, _, a := got.At(-1, 0).RGBA(); a != 0 {
		t.Fatal("out-of-bounds pixel is opaque")
	}
	if !bytes.Equal(src.Pix, before) {
		t.Fatal("source was modified")
	}
	large, err := Pixelate(src, 128)
	if err != nil {
		t.Fatal(err)
	}
	if actual := color.RGBAModel.Convert(large.At(2, 2)); actual != (color.RGBA{R: 155, G: 141, B: 141, A: 255}) {
		t.Fatal("partial whole-image block average:", actual)
	}
	encoded, err := JPEG(got)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(encoded))
	if err != nil || decoded.Bounds() != got.Bounds() {
		t.Fatalf("JPEG roundtrip: %v", err)
	}
	for _, size := range []int{-1, 0, 1, 129} {
		if _, err := Pixelate(src, size); !errors.Is(err, ErrBlockSize) {
			t.Fatalf("block size %d: %v", size, err)
		}
	}
}

func TestPixelateSinglePixel(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(3, 4, 4, 5))
	src.SetNRGBA64(3, 4, color.NRGBA64{R: 65535, A: 65535})
	for _, input := range []image.Image{src, genericImage{src}} {
		got, err := Pixelate(input, 16)
		if err != nil || color.RGBAModel.Convert(got.At(0, 0)) != (color.RGBA{R: 255, A: 255}) {
			t.Fatalf("%T: %v, %v", input, got, err)
		}
	}
}

func TestPixelateToFitMatchesSeparatePipeline(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(10, 20, 23, 29))
	for y := 20; y < 29; y++ {
		for x := 10; x < 23; x++ {
			src.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x * 2345), G: uint16(y * 1753), B: uint16(x * y * 151), A: uint16((x + y) * 1301)})
		}
	}
	before := append([]byte(nil), src.Pix...)
	for _, input := range []image.Image{src, genericImage{src}} {
		for _, limit := range []int{1, 2, 8, 16} {
			resized, err := ResizeToFit(input, limit)
			if err != nil {
				t.Fatal(err)
			}
			for _, size := range []int{2, 3, 16, 128} {
				want, err := Pixelate(resized, size)
				if err != nil {
					t.Fatal(err)
				}
				got, err := PixelateToFit(input, limit, size)
				if err != nil {
					t.Fatal(err)
				}
				if got.Bounds() != want.Bounds() {
					t.Fatal("wrong output bounds")
				}
				for y := 0; y < got.Bounds().Dy(); y++ {
					for x := 0; x < got.Bounds().Dx(); x++ {
						actual := color.RGBAModel.Convert(got.At(x, y))
						expected := color.RGBAModel.Convert(want.At(x, y))
						if actual != expected {
							t.Fatalf("%T limit %d block %d at %d,%d: got %v want %v", input, limit, size, x, y, actual, expected)
						}
					}
				}
			}
		}
	}
	if !bytes.Equal(src.Pix, before) {
		t.Fatal("source was mutated")
	}
	for _, limit := range []int{0, MaxDimension + 1} {
		if _, err := PixelateToFit(src, limit, 16); !errors.Is(err, ErrDimensions) {
			t.Fatal(err)
		}
	}
	if _, err := PixelateToFit(src, 16, 1); !errors.Is(err, ErrBlockSize) {
		t.Fatal(err)
	}
	if _, err := PreparePixelate(nil, 1); !errors.Is(err, ErrBlockSize) {
		t.Fatal(err)
	}
	if _, err := PixelateToFit(image.NewGray(image.Rectangle{}), 16, 16); !errors.Is(err, ErrDimensions) {
		t.Fatal(err)
	}
}

func BenchmarkPixelate512(b *testing.B) {
	data := encoded(b, "png", 512, 512)
	src, err := Prepare(data, MaxDimension)
	if err != nil {
		b.Fatal(err)
	}
	for _, size := range []int{2, 16, 128} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				result, err := Pixelate(src, size)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := JPEG(result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPixelateToFit1024(b *testing.B) {
	src := image.NewNRGBA(image.Rect(0, 0, 1025, 1023))
	for y := 0; y < 1023; y++ {
		for x := 0; x < 1025; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 128})
		}
	}
	for _, size := range []int{2, 16, 128} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				result, err := PixelateToFit(src, MaxDimension, size)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := JPEG(result); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
