package imageproc

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestEdgeDetection(t *testing.T) {
	tests := []struct {
		name   string
		pixels []byte
		want   byte
	}{
		{"constant", []byte{20, 20, 20, 20, 20, 20, 20, 20, 20}, 0},
		{"horizontal gradient", []byte{0, 10, 20, 0, 10, 20, 0, 10, 20}, 80},
		{"vertical gradient", []byte{0, 0, 0, 10, 10, 10, 20, 20, 20}, 80},
		{"diagonal gradient", []byte{0, 10, 20, 10, 20, 30, 20, 30, 40}, 160},
		{"negative gradient", []byte{20, 10, 0, 20, 10, 0, 20, 10, 0}, 80},
		{"clip strong edge", []byte{0, 0, 255, 0, 0, 255, 0, 0, 255}, 255},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent := image.NewGray(image.Rect(4, 7, 10, 13))
			src := parent.SubImage(image.Rect(5, 8, 8, 11)).(*image.Gray)
			for y := 0; y < 3; y++ {
				copy(src.Pix[y*src.Stride:y*src.Stride+3], tc.pixels[y*3:y*3+3])
			}
			before := bytes.Clone(parent.Pix)
			got := EdgeDetection(src)
			want := []byte{0, 0, 0, 0, tc.want, 0, 0, 0, 0}
			if got.Bounds() != image.Rect(0, 0, 3, 3) || !bytes.Equal(got.Pix, want) {
				t.Fatalf("got %v %v, want %v", got.Bounds(), got.Pix, want)
			}
			if !bytes.Equal(parent.Pix, before) {
				t.Fatal("source modified")
			}
		})
	}
	for _, r := range []image.Rectangle{image.Rect(0, 0, 1, 1), image.Rect(0, 0, 2, 8), image.Rect(0, 0, 8, 2)} {
		src := image.NewGray(r)
		for i := range src.Pix {
			src.Pix[i] = 255
		}
		got := EdgeDetection(src)
		if !bytes.Equal(got.Pix, make([]byte, len(got.Pix))) {
			t.Fatal("small image must be black")
		}
	}
}

func TestPrepareEdge(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2048, 512))
	// Fully transparent colors must composite to uniform white, with no edges.
	for y := 0; y < 512; y++ {
		for x := 0; x < 2048; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), A: 0})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, src); err != nil {
		t.Fatal(err)
	}
	got, err := PrepareEdge(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	gray := got.(*image.Gray)
	if gray.Bounds() != image.Rect(0, 0, 1024, 256) || !bytes.Equal(gray.Pix, make([]byte, len(gray.Pix))) {
		t.Fatal("unexpected dimensions or transparency edges")
	}
	if _, err := PrepareEdge([]byte("bad")); err == nil {
		t.Fatal("invalid image accepted")
	}
}
