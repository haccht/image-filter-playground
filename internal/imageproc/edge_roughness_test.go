package imageproc

import (
	"bytes"
	"image"
	"testing"
)

func TestRoughnessBlurReference(t *testing.T) {
	for _, size := range []image.Point{{1, 1}, {2, 9}, {9, 2}, {7, 11}, {31, 27}} {
		parent := image.NewGray(image.Rect(4, 7, 4+size.X+3, 7+size.Y+3))
		src := parent.SubImage(image.Rect(5, 8, 5+size.X, 8+size.Y)).(*image.Gray)
		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				src.Pix[y*src.Stride+x] = uint8(x*37 + y*71)
			}
		}
		before := bytes.Clone(parent.Pix)
		for level := 0; level <= MaxRoughness; level++ {
			wantSource := image.NewGray(image.Rect(0, 0, size.X, size.Y))
			radius := level * 2
			for y := 0; y < size.Y; y++ {
				for x := 0; x < size.X; x++ {
					sum, count := 0, 0
					for sy := max(0, y-radius); sy < min(size.Y, y+radius+1); sy++ {
						for sx := max(0, x-radius); sx < min(size.X, x+radius+1); sx++ {
							sum += int(src.GrayAt(src.Rect.Min.X+sx, src.Rect.Min.Y+sy).Y)
							count++
						}
					}
					wantSource.Pix[y*wantSource.Stride+x] = uint8(sum / count)
				}
			}
			if level > 0 && !bytes.Equal(blurGray(src, radius).Pix, wantSource.Pix) {
				t.Fatalf("blur mismatch size %v level %d", size, level)
			}
			got, err := EdgeDetectionWithRoughness(src, level)
			if err != nil || !bytes.Equal(got.Pix, EdgeDetection(wantSource).Pix) {
				t.Fatalf("edge mismatch size %v level %d: %v", size, level, err)
			}
		}
		if !bytes.Equal(parent.Pix, before) {
			t.Fatal("source modified")
		}
	}
	for _, level := range []int{-1, 5} {
		if _, err := EdgeDetectionWithRoughness(nil, level); err != ErrRoughness {
			t.Fatal("invalid level accepted")
		}
		if _, err := PrepareEdgeWithRoughness([]byte("invalid"), level); err != ErrRoughness {
			t.Fatal("option must fail before decoding")
		}
	}
}

func TestRoughnessSuppressesFineDetail(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 33, 33))
	for y := 0; y < 33; y++ {
		for x := 0; x < 33; x++ {
			if x%4 < 2 {
				src.Pix[y*src.Stride+x] = 255
			}
		}
	}
	base := EdgeDetection(src)
	sum := func(pixels []byte) int {
		n := 0
		for _, p := range pixels {
			n += int(p)
		}
		return n
	}
	for level := 1; level <= MaxRoughness; level++ {
		got, err := EdgeDetectionWithRoughness(src, level)
		if err != nil || sum(got.Pix) >= sum(base.Pix) {
			t.Fatalf("fine detail not reduced at level %d", level)
		}
	}
}

func TestPrepareEdgeWithRoughnessMatchesReference(t *testing.T) {
	data := encoded(t, "png", 19, 15)
	grayImg, err := PrepareGrayscale(data)
	if err != nil {
		t.Fatal(err)
	}
	gray := grayImg.(*image.Gray)
	for level := 0; level <= MaxRoughness; level++ {
		want, err := EdgeDetectionWithRoughness(gray, level)
		if err != nil {
			t.Fatal(err)
		}
		gotImg, err := PrepareEdgeWithRoughness(data, level)
		if err != nil {
			t.Fatal(err)
		}
		got := gotImg.(*image.Gray)
		if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("level %d: PrepareEdgeWithRoughness output mismatch", level)
		}
	}
}
