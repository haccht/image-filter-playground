package imageproc

import (
	"errors"
	"image"
	"image/color"
)

const (
	MinBlockSize     = 2
	MaxBlockSize     = 128
	DefaultBlockSize = 16
)

var ErrBlockSize = errors.New("block_size must be an integer between 2 and 128")

// Pixelate averages each block after compositing transparency onto white.
// The returned image keeps only one color per block. JPEG can read this view
// directly, without allocating a full-size RGB output or boxing every pixel.
func Pixelate(src image.Image, blockSize int) (image.Image, error) {
	if blockSize < MinBlockSize || blockSize > MaxBlockSize {
		return nil, ErrBlockSize
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	cols, rows := (width+blockSize-1)/blockSize, (height+blockSize-1)/blockSize
	dst := &pixelatedImage{
		rect: image.Rect(0, 0, width, height), blockSize: blockSize, cols: cols,
		colors: make([]color.RGBA, cols*rows),
	}
	for by := 0; by < rows; by++ {
		for bx := 0; bx < cols; bx++ {
			x0, y0 := bx*blockSize, by*blockSize
			x1, y1 := min(x0+blockSize, width), min(y0+blockSize, height)
			var red, green, blue uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, b, a := rgbaAt(src, bounds.Min.X+x, bounds.Min.Y+y)
					white := uint32(0xffff) - a
					red += uint64(r + white)
					green += uint64(g + white)
					blue += uint64(b + white)
				}
			}
			// Average in 16-bit space, then truncate to an 8-bit opaque color.
			divisor := uint64((x1-x0)*(y1-y0)) * 257
			dst.colors[by*cols+bx] = color.RGBA{
				R: uint8(red / divisor), G: uint8(green / divisor), B: uint8(blue / divisor), A: 255,
			}
		}
	}
	return dst, nil
}

// PixelateToFit matches Pixelate(ResizeToFit(src, limit), blockSize) without
// allocating the intermediate RGBA image. Averaged resized pixels are rounded
// to 8 bits before block averaging, preserving the separate pipeline's result.
func PixelateToFit(src image.Image, maxDimension, blockSize int) (image.Image, error) {
	if blockSize < MinBlockSize || blockSize > MaxBlockSize {
		return nil, ErrBlockSize
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if maxDimension < 1 || maxDimension > MaxDimension || w < 1 || h < 1 {
		return nil, ErrDimensions
	}
	if w <= maxDimension && h <= maxDimension {
		return Pixelate(src, blockSize)
	}
	dw, dh := maxDimension, max(1, h*maxDimension/w)
	if h > w {
		dw, dh = max(1, w*maxDimension/h), maxDimension
	}
	cols, rows := (dw+blockSize-1)/blockSize, (dh+blockSize-1)/blockSize
	dst := &pixelatedImage{
		rect: image.Rect(0, 0, dw, dh), blockSize: blockSize, cols: cols,
		colors: make([]color.RGBA, cols*rows),
	}
	for by := 0; by < rows; by++ {
		for bx := 0; bx < cols; bx++ {
			x0, y0 := bx*blockSize, by*blockSize
			x1, y1 := min(x0+blockSize, dw), min(y0+blockSize, dh)
			var blockRed, blockGreen, blockBlue uint64
			for y := y0; y < y1; y++ {
				sy0, sy1 := y*h/dh, (y+1)*h/dh
				for x := x0; x < x1; x++ {
					sx0, sx1 := x*w/dw, (x+1)*w/dw
					var red, green, blue uint64
					for sy := sy0; sy < sy1; sy++ {
						for sx := sx0; sx < sx1; sx++ {
							r, g, b, a := rgbaAt(src, bounds.Min.X+sx, bounds.Min.Y+sy)
							white := uint32(0xffff) - a
							red += uint64(r + white)
							green += uint64(g + white)
							blue += uint64(b + white)
						}
					}
					divisor := uint64((sx1-sx0)*(sy1-sy0)) * 257
					blockRed += red / divisor
					blockGreen += green / divisor
					blockBlue += blue / divisor
				}
			}
			count := uint64((x1 - x0) * (y1 - y0))
			dst.colors[by*cols+bx] = color.RGBA{R: uint8(blockRed / count), G: uint8(blockGreen / count), B: uint8(blockBlue / count), A: 255}
		}
	}
	return dst, nil
}

type pixelatedImage struct {
	rect      image.Rectangle
	blockSize int
	cols      int
	colors    []color.RGBA
}

func (p *pixelatedImage) ColorModel() color.Model { return color.RGBAModel }
func (p *pixelatedImage) Bounds() image.Rectangle { return p.rect }
func (p *pixelatedImage) At(x, y int) color.Color {
	if !image.Pt(x, y).In(p.rect) {
		return color.RGBA{}
	}
	return &p.colors[(y/p.blockSize)*p.cols+x/p.blockSize]
}
