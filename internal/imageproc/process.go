// Package imageproc validates and transforms bounded JPEG and PNG images.
package imageproc

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
)

const (
	MaxBytes     = 2 << 20
	MaxDimension = 1024
)

var (
	ErrTooLarge   = errors.New("image exceeds 2 MiB")
	ErrDimensions = errors.New("image dimensions must be between 1 and 1024 pixels")
	ErrInvalid    = errors.New("invalid JPEG or PNG image")
)

// Grayscale composites transparency onto white and converts luminance.
func Grayscale(src image.Image) *image.Gray {
	bounds := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			r, g, b, a := rgbaAt(src, bounds.Min.X+x, bounds.Min.Y+y)
			white := uint32(0xffff) - a
			// Use the same integer luminance weights as color.GrayModel.
			luminance := (19595*(r+white) + 38470*(g+white) + 7471*(b+white) + (1 << 15)) >> 24
			dst.SetGray(x, y, color.Gray{Y: uint8(luminance)})
		}
	}
	return dst
}

// rgbaAt avoids boxing a color into an interface for every decoded pixel.
// Keep the generic fallback for callers using other image implementations.
func rgbaAt(src image.Image, x, y int) (r, g, b, a uint32) {
	switch src := src.(type) {
	case *image.NRGBA:
		return src.NRGBAAt(x, y).RGBA()
	case *image.NRGBA64:
		return src.NRGBA64At(x, y).RGBA()
	case *image.RGBA:
		return src.RGBAAt(x, y).RGBA()
	case *image.RGBA64:
		return src.RGBA64At(x, y).RGBA()
	case *image.Gray:
		return src.GrayAt(x, y).RGBA()
	case *image.Gray16:
		return src.Gray16At(x, y).RGBA()
	case *image.YCbCr:
		return src.YCbCrAt(x, y).RGBA()
	case *image.CMYK:
		return src.CMYKAt(x, y).RGBA()
	default:
		return src.At(x, y).RGBA()
	}
}

// JPEG encodes an image at a fixed quality with no source metadata.
func JPEG(src image.Image) ([]byte, error) {
	var output bytes.Buffer
	if err := jpeg.Encode(&output, src, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
