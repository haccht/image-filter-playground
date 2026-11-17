package imageproc

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strconv"
	"testing"
)

func naiveOilPaint(src image.Image, brushSize, roughness int) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	levels := oilLevels[roughness]
	r8 := make([]uint8, w*h)
	g8 := make([]uint8, w*h)
	b8 := make([]uint8, w*h)
	bins := make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := rgbaAt(src, bounds.Min.X+x, bounds.Min.Y+y)
			white := uint32(0xffff) - a
			rv := uint8((r + white) / 257)
			gv := uint8((g + white) / 257)
			bv := uint8((b + white) / 257)
			idx := y*w + x
			r8[idx], g8[idx], b8[idx] = rv, gv, bv
			bins[idx] = int(quantizeLuminance(rv, gv, bv, levels))
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var hist [maxOilLevels]oilHistBin
			for sy := max(0, y-brushSize); sy < min(h, y+brushSize+1); sy++ {
				for sx := max(0, x-brushSize); sx < min(w, x+brushSize+1); sx++ {
					idx := sy*w + sx
					bin := bins[idx]
					hist[bin].count++
					hist[bin].r += uint16(r8[idx])
					hist[bin].g += uint16(g8[idx])
					hist[bin].b += uint16(b8[idx])
				}
			}
			var best oilHistBin
			for l := 0; l < levels; l++ {
				if hist[l].count > best.count {
					best = hist[l]
				}
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(best.r / best.count),
				G: uint8(best.g / best.count),
				B: uint8(best.b / best.count),
				A: 255,
			})
		}
	}
	return dst
}

func TestOilPaintMatchesNaiveReference(t *testing.T) {
	for _, size := range []image.Point{{1, 1}, {2, 9}, {9, 2}, {7, 11}, {19, 15}} {
		parent := image.NewNRGBA(image.Rect(4, 7, 4+size.X+3, 7+size.Y+3))
		src := parent.SubImage(image.Rect(5, 8, 5+size.X, 8+size.Y)).(*image.NRGBA)
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				src.SetNRGBA(5+x, 8+y, color.NRGBA{
					R: uint8(x*37 + y*13),
					G: uint8(x*19 + y*71),
					B: uint8((x ^ y) * 53),
					A: uint8(128 + (x+y)%128),
				})
			}
		}
		before := bytes.Clone(parent.Pix)
		for brush := MinBrushSize; brush <= MaxBrushSize; brush++ {
			for roughness := 0; roughness <= MaxRoughness; roughness++ {
				want := naiveOilPaint(src, brush, roughness)
				got, err := OilPaint(src, brush, roughness)
				if err != nil {
					t.Fatalf("size %v brush %d roughness %d: %v", size, brush, roughness, err)
				}
				if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
					t.Fatalf("size %v brush %d roughness %d: sliding window differs from naive reference", size, brush, roughness)
				}
			}
		}
		if !bytes.Equal(parent.Pix, before) {
			t.Fatal("source modified")
		}
	}
}

func TestOilPaintValidation(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for _, brush := range []int{-1, 0, MaxBrushSize + 1} {
		if _, err := OilPaint(src, brush, DefaultRoughness); !errors.Is(err, ErrBrushSize) {
			t.Fatalf("brush %d: %v", brush, err)
		}
		if _, err := OilPaintToFit(src, 4, brush, DefaultRoughness); !errors.Is(err, ErrBrushSize) {
			t.Fatalf("brush %d: %v", brush, err)
		}
		if _, err := PrepareOil([]byte("invalid"), brush, DefaultRoughness); !errors.Is(err, ErrBrushSize) {
			t.Fatalf("brush %d must fail before decoding: %v", brush, err)
		}
	}
	for _, roughness := range []int{-1, MaxRoughness + 1} {
		if _, err := OilPaint(src, DefaultBrushSize, roughness); !errors.Is(err, ErrRoughness) {
			t.Fatalf("roughness %d: %v", roughness, err)
		}
		if _, err := OilPaintToFit(src, 4, DefaultBrushSize, roughness); !errors.Is(err, ErrRoughness) {
			t.Fatalf("roughness %d: %v", roughness, err)
		}
		if _, err := PrepareOil([]byte("invalid"), DefaultBrushSize, roughness); !errors.Is(err, ErrRoughness) {
			t.Fatalf("roughness %d must fail before decoding: %v", roughness, err)
		}
	}
	if _, err := OilPaint(image.NewRGBA(image.Rectangle{}), DefaultBrushSize, DefaultRoughness); !errors.Is(err, ErrDimensions) {
		t.Fatalf("empty image: %v", err)
	}
	for _, limit := range []int{0, MaxDimension + 1} {
		if _, err := OilPaintToFit(src, limit, DefaultBrushSize, DefaultRoughness); !errors.Is(err, ErrDimensions) {
			t.Fatalf("limit %d: %v", limit, err)
		}
	}
}

func TestOilPaintToFitMatchesSeparatePipeline(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(10, 20, 23, 29))
	for y := 20; y < 29; y++ {
		for x := 10; x < 23; x++ {
			src.SetNRGBA64(x, y, color.NRGBA64{
				R: uint16(x * 2345),
				G: uint16(y * 1753),
				B: uint16(x * y * 151),
				A: uint16((x + y) * 1301),
			})
		}
	}
	before := bytes.Clone(src.Pix)
	for _, input := range []image.Image{src, genericImage{src}} {
		for _, limit := range []int{1, 2, 8, 16} {
			resized, err := ResizeToFit(input, limit)
			if err != nil {
				t.Fatal(err)
			}
			for _, brush := range []int{1, 4, 6} {
				for _, roughness := range []int{0, 2, 4} {
					want, err := OilPaint(resized, brush, roughness)
					if err != nil {
						t.Fatal(err)
					}
					got, err := OilPaintToFit(input, limit, brush, roughness)
					if err != nil {
						t.Fatal(err)
					}
					if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
						t.Fatalf("%T limit %d brush %d roughness %d: mismatch", input, limit, brush, roughness)
					}
				}
			}
		}
	}
	if !bytes.Equal(src.Pix, before) {
		t.Fatal("source was mutated")
	}
}

func TestPrepareOilMatchesReference(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		for _, dims := range [][2]int{{19, 15}, {1025, 1023}, {2048, 512}} {
			img := image.NewNRGBA(image.Rect(0, 0, dims[0], dims[1]))
			for y := 0; y < dims[1]; y++ {
				for x := 0; x < dims[0]; x++ {
					img.SetNRGBA(x, y, color.NRGBA{
						R: uint8(x*29 + y*7),
						G: uint8(x*11 + y*43),
						B: uint8((x ^ y) * 17),
						A: uint8(180 + (x+y)%76),
					})
				}
			}
			var buf bytes.Buffer
			if format == "png" {
				if err := png.Encode(&buf, img); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := jpeg.Encode(&buf, img, nil); err != nil {
					t.Fatal(err)
				}
			}
			data := buf.Bytes()
			prepared, err := Prepare(data, MaxDimension)
			if err != nil {
				t.Fatal(err)
			}
			for _, brush := range []int{1, 4, 6} {
				for _, roughness := range []int{0, 2, 4} {
					want, err := OilPaint(prepared, brush, roughness)
					if err != nil {
						t.Fatal(err)
					}
					gotImg, err := PrepareOil(data, brush, roughness)
					if err != nil {
						t.Fatal(err)
					}
					got := gotImg.(*image.RGBA)
					if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
						t.Fatalf("%s %v brush %d roughness %d: PrepareOil output mismatch", format, dims, brush, roughness)
					}
				}
			}
		}
	}
}

func BenchmarkOilPaint512(b *testing.B) {
	data := encoded(b, "png", 512, 512)
	src, err := Prepare(data, MaxDimension)
	if err != nil {
		b.Fatal(err)
	}
	for _, brush := range []int{1, 4, 6} {
		b.Run(strconv.Itoa(brush), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				result, err := OilPaint(src, brush, DefaultRoughness)
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
