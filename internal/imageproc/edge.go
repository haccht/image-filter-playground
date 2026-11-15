package imageproc

import "image"

// EdgeDetection applies 3x3 Sobel kernels to grayscale pixels. The L1 gradient
// magnitude is clipped to 255, producing bright edges on black. The outermost
// pixels remain black; images smaller than 3 pixels on either side are black.
// The source, including nonzero-origin subimages, is never modified.
func EdgeDetection(src *image.Gray) *image.Gray {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	edgeDetectionInto(dst, src)
	return dst
}

// edgeDetectionInto writes 3x3 Sobel edge magnitudes from src into a
// zero-origin destination image of the same dimensions.
func edgeDetectionInto(dst, src *image.Gray) {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	for y := 1; y < h-1; y++ {
		top := src.Pix[(y-1)*src.Stride:]
		mid := src.Pix[y*src.Stride:]
		bottom := src.Pix[(y+1)*src.Stride:]
		for x := 1; x < w-1; x++ {
			a, b, c := int(top[x-1]), int(top[x]), int(top[x+1])
			d, f := int(mid[x-1]), int(mid[x+1])
			g, h, i := int(bottom[x-1]), int(bottom[x]), int(bottom[x+1])
			gx := -a + c - 2*d + 2*f - g + i
			gy := -a - 2*b - c + g + 2*h + i
			if gx < 0 {
				gx = -gx
			}
			if gy < 0 {
				gy = -gy
			}
			dst.Pix[y*dst.Stride+x] = uint8(min(255, gx+gy))
		}
	}
}

// PrepareEdge uses default roughness with shared admission and grayscale resize.
func PrepareEdge(data []byte) (image.Image, error) {
	return PrepareEdgeWithRoughness(data, DefaultRoughness)
}
