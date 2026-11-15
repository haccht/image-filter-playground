package imageproc

import (
	"image"
	"image/color"
)

// ResizeToFit downsizes to a longest-side limit without upscaling or cropping.
// Output dimensions use integer flooring, with a minimum of one pixel per side.
// Each output pixel averages its disjoint source rectangle onto white. This
// box filter uses one output buffer and no full-size intermediate RGB image.
func ResizeToFit(src image.Image, maxDimension int) (image.Image, error) {
	return resizeToFit(src, maxDimension, false)
}

// GrayscaleToFit fuses downsampling and grayscale conversion, avoiding a large
// intermediate RGBA image. Pixels match Grayscale(ResizeToFit(src, limit)).
func GrayscaleToFit(src image.Image, maxDimension int) (image.Image, error) {
	return resizeToFit(src, maxDimension, true)
}

func resizeToFit(src image.Image, maxDimension int, grayscale bool) (image.Image, error) {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if maxDimension < 1 || maxDimension > MaxDimension || w < 1 || h < 1 {
		return nil, ErrDimensions
	}
	if w <= maxDimension && h <= maxDimension {
		if grayscale {
			return Grayscale(src), nil
		}
		return src, nil
	}
	dw, dh := maxDimension, max(1, h*maxDimension/w)
	if h > w {
		dw, dh = max(1, w*maxDimension/h), maxDimension
	}
	var gray *image.Gray
	var rgb *image.RGBA
	if grayscale {
		gray = image.NewGray(image.Rect(0, 0, dw, dh))
	} else {
		rgb = image.NewRGBA(image.Rect(0, 0, dw, dh))
	}
	for y := 0; y < dh; y++ {
		y0, y1 := y*h/dh, (y+1)*h/dh
		for x := 0; x < dw; x++ {
			x0, x1 := x*w/dw, (x+1)*w/dw
			var red, green, blue uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, a := rgbaAt(src, bounds.Min.X+sx, bounds.Min.Y+sy)
					white := uint32(0xffff) - a
					red += uint64(r + white)
					green += uint64(g + white)
					blue += uint64(b + white)
				}
			}
			divisor := uint64((x1-x0)*(y1-y0)) * 257
			r, g, b := uint32(red/divisor), uint32(green/divisor), uint32(blue/divisor)
			if grayscale {
				luminance := (19595*r*257 + 38470*g*257 + 7471*b*257 + (1 << 15)) >> 24
				gray.Pix[y*gray.Stride+x] = uint8(luminance)
			} else {
				rgb.SetRGBA(x, y, color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255})
			}
		}
	}
	if grayscale {
		return gray, nil
	}
	return rgb, nil
}
