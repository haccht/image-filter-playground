package imageproc

import (
	"errors"
	"image"
)

const (
	DefaultRoughness = 2
	MaxRoughness     = 4
)

var ErrRoughness = errors.New("roughness must be an integer between 0 and 4")

// EdgeDetectionWithRoughness smooths grayscale pixels before Sobel detection.
// Level zero is byte-identical to EdgeDetection. Levels 1..4 use box radii
// 2, 4, 6, 8 in resized-image pixels. The source is never modified.
func EdgeDetectionWithRoughness(src *image.Gray, roughness int) (*image.Gray, error) {
	if roughness < 0 || roughness > MaxRoughness {
		return nil, ErrRoughness
	}
	if roughness == 0 {
		return EdgeDetection(src), nil
	}
	return EdgeDetection(blurGray(src, 2*roughness)), nil
}

// blurGray averages the clipped square neighborhood, rounding down once.
// Sliding column sums keep work linear in pixel count without a full-size
// integral image or separate horizontal/vertical image buffers.
func blurGray(src *image.Gray, radius int) *image.Gray {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	if w == 0 || h == 0 {
		return dst
	}
	columns := make([]int, w)
	for y := 0; y < min(h, radius); y++ {
		for x := 0; x < w; x++ {
			columns[x] += int(src.Pix[y*src.Stride+x])
		}
	}
	for y := 0; y < h; y++ {
		if row := y + radius; row < h {
			for x := 0; x < w; x++ {
				columns[x] += int(src.Pix[row*src.Stride+x])
			}
		}
		if row := y - radius - 1; row >= 0 {
			for x := 0; x < w; x++ {
				columns[x] -= int(src.Pix[row*src.Stride+x])
			}
		}
		rows := min(h, y+radius+1) - max(0, y-radius)
		sum := 0
		for x := 0; x < min(w, radius); x++ {
			sum += columns[x]
		}
		for x := 0; x < w; x++ {
			if right := x + radius; right < w {
				sum += columns[right]
			}
			if left := x - radius - 1; left >= 0 {
				sum -= columns[left]
			}
			count := rows * (min(w, x+radius+1) - max(0, x-radius))
			dst.Pix[y*dst.Stride+x] = uint8(sum / count)
		}
	}
	return dst
}

// PrepareEdgeWithRoughness validates options before decoding and shares the
// existing source admission, white compositing, and 1024-pixel resize path.
func PrepareEdgeWithRoughness(data []byte, roughness int) (image.Image, error) {
	if roughness < 0 || roughness > MaxRoughness {
		return nil, ErrRoughness
	}
	img, err := PrepareGrayscale(data)
	if err != nil {
		return nil, err
	}
	gray := img.(*image.Gray)
	if roughness == 0 {
		return EdgeDetection(gray), nil
	}
	blurred := blurGray(gray, 2*roughness)
	// Reuse the freshly allocated gray buffer as the Sobel output destination.
	clear(gray.Pix)
	edgeDetectionInto(gray, blurred)
	return gray, nil
}
