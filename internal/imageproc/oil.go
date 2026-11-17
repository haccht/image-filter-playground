package imageproc

import (
	"errors"
	"image"
)

const (
	MinBrushSize     = 1
	MaxBrushSize     = 6
	DefaultBrushSize = 4
	maxOilLevels     = 32
)

var (
	ErrBrushSize = errors.New("brush_size must be an integer between 1 and 6")
	oilLevels    = [MaxRoughness + 1]int{32, 24, 16, 12, 8}
)

type oilHistBin struct {
	count, r, g, b uint16
}

// OilPaint applies a luminance-histogram oil painting filter with neighborhood
// radius brushSize (1..6) and tonal coarseness roughness (0..4). Transparency
// is composited onto white. The source image, including nonzero-origin
// subimages, is never modified.
func OilPaint(src image.Image, brushSize, roughness int) (*image.RGBA, error) {
	if brushSize < MinBrushSize || brushSize > MaxBrushSize {
		return nil, ErrBrushSize
	}
	if roughness < 0 || roughness > MaxRoughness {
		return nil, ErrRoughness
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 1 || h < 1 {
		return nil, ErrDimensions
	}
	dst := copyWhiteCompositedRGBA(src)
	oilPaintInPlace(dst, brushSize, oilLevels[roughness])
	return dst, nil
}

// OilPaintToFit matches OilPaint(ResizeToFit(src, maxDimension), brushSize, roughness)
// while allocating only a single output RGBA buffer.
func OilPaintToFit(src image.Image, maxDimension, brushSize, roughness int) (*image.RGBA, error) {
	if brushSize < MinBrushSize || brushSize > MaxBrushSize {
		return nil, ErrBrushSize
	}
	if roughness < 0 || roughness > MaxRoughness {
		return nil, ErrRoughness
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if maxDimension < 1 || maxDimension > MaxDimension || w < 1 || h < 1 {
		return nil, ErrDimensions
	}
	if w <= maxDimension && h <= maxDimension {
		return OilPaint(src, brushSize, roughness)
	}
	resized, err := resizeToFit(src, maxDimension, false)
	if err != nil {
		return nil, err
	}
	dst := resized.(*image.RGBA)
	oilPaintInPlace(dst, brushSize, oilLevels[roughness])
	return dst, nil
}

// PrepareOil validates options before decoding, shares the existing admission
// and 1024-pixel resize path, and reuses the decoded 4-byte pixel buffer in
// place when the source is already an unscaled RGBA or NRGBA image.
func PrepareOil(data []byte, brushSize, roughness int) (image.Image, error) {
	if brushSize < MinBrushSize || brushSize > MaxBrushSize {
		return nil, ErrBrushSize
	}
	if roughness < 0 || roughness > MaxRoughness {
		return nil, ErrRoughness
	}
	dst, err := decodeAndPrepareOil(data)
	if err != nil {
		return nil, err
	}
	oilPaintInPlace(dst, brushSize, oilLevels[roughness])
	return dst, nil
}

func decodeAndPrepareOil(data []byte) (*image.RGBA, error) {
	src, err := decodeSource(data)
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w > MaxDimension || h > MaxDimension {
		dw, dh := MaxDimension, max(1, h*MaxDimension/w)
		if h > w {
			dw, dh = max(1, w*MaxDimension/h), MaxDimension
		}
		switch img := src.(type) {
		case *image.RGBA:
			if img.Rect.Min == (image.Point{}) {
				return downsampleBufferInPlace(img.Pix, img.Stride, w, h, dw, dh, true), nil
			}
		case *image.NRGBA:
			if img.Rect.Min == (image.Point{}) {
				return downsampleBufferInPlace(img.Pix, img.Stride, w, h, dw, dh, false), nil
			}
		}
		resized, err := resizeToFit(src, MaxDimension, false)
		if err != nil {
			return nil, err
		}
		return resized.(*image.RGBA), nil
	}
	// Reuse the freshly decoded 4-byte-per-pixel buffer in place when possible
	// to avoid a second 4 MiB allocation on 1024x1024 PNG inputs.
	switch img := src.(type) {
	case *image.RGBA:
		if img.Rect.Min == (image.Point{}) {
			compositeWhiteInPlace(img.Pix, img.Stride, w, h, true)
			return img, nil
		}
	case *image.NRGBA:
		if img.Rect.Min == (image.Point{}) {
			compositeWhiteInPlace(img.Pix, img.Stride, w, h, false)
			return &image.RGBA{Pix: img.Pix, Stride: img.Stride, Rect: img.Rect}, nil
		}
	}
	return copyWhiteCompositedRGBA(src), nil
}

func copyWhiteCompositedRGBA(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := dst.Pix[y*dst.Stride:]
		for x := 0; x < w; x++ {
			r, g, b, a := rgbaAt(src, bounds.Min.X+x, bounds.Min.Y+y)
			white := uint32(0xffff) - a
			i := x * 4
			row[i] = uint8((r + white) / 257)
			row[i+1] = uint8((g + white) / 257)
			row[i+2] = uint8((b + white) / 257)
			row[i+3] = 255
		}
	}
	return dst
}

func compositeWhiteInPlace(pix []uint8, stride, w, h int, premultiplied bool) {
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+w*4]
		for x := 0; x < w; x++ {
			i := x * 4
			var r, g, b, a uint32
			if premultiplied {
				r = uint32(row[i]) * 0x101
				g = uint32(row[i+1]) * 0x101
				b = uint32(row[i+2]) * 0x101
				a = uint32(row[i+3]) * 0x101
			} else {
				a8 := uint32(row[i+3])
				r = uint32(row[i]) * 0x101 * a8 / 0xff
				g = uint32(row[i+1]) * 0x101 * a8 / 0xff
				b = uint32(row[i+2]) * 0x101 * a8 / 0xff
				a = a8 * 0x101
			}
			white := uint32(0xffff) - a
			row[i] = uint8((r + white) / 257)
			row[i+1] = uint8((g + white) / 257)
			row[i+2] = uint8((b + white) / 257)
			row[i+3] = 255
		}
	}
}

// downsampleBufferInPlace box-filters an unshifted 4-byte-per-pixel source into
// the prefix of its own pixel slice. Because dw <= w, dh <= h, and stride >= 4*w,
// each destination offset y*(4*dw)+4*x is at or behind the top-left source pixel
// y0*stride+4*x0 of its disjoint averaging box.
func downsampleBufferInPlace(pix []uint8, stride, w, h, dw, dh int, premultiplied bool) *image.RGBA {
	dstStride := dw * 4
	for y := 0; y < dh; y++ {
		y0, y1 := y*h/dh, (y+1)*h/dh
		dstRow := pix[y*dstStride : (y+1)*dstStride]
		for x := 0; x < dw; x++ {
			x0, x1 := x*w/dw, (x+1)*w/dw
			var red, green, blue uint64
			for sy := y0; sy < y1; sy++ {
				srcRow := pix[sy*stride:]
				for sx := x0; sx < x1; sx++ {
					si := sx * 4
					var r, g, b, a uint32
					if premultiplied {
						r = uint32(srcRow[si]) * 0x101
						g = uint32(srcRow[si+1]) * 0x101
						b = uint32(srcRow[si+2]) * 0x101
						a = uint32(srcRow[si+3]) * 0x101
					} else {
						a8 := uint32(srcRow[si+3])
						r = uint32(srcRow[si]) * 0x101 * a8 / 0xff
						g = uint32(srcRow[si+1]) * 0x101 * a8 / 0xff
						b = uint32(srcRow[si+2]) * 0x101 * a8 / 0xff
						a = a8 * 0x101
					}
					white := uint32(0xffff) - a
					red += uint64(r + white)
					green += uint64(g + white)
					blue += uint64(b + white)
				}
			}
			divisor := uint64((x1-x0)*(y1-y0)) * 257
			di := x * 4
			dstRow[di] = uint8(red / divisor)
			dstRow[di+1] = uint8(green / divisor)
			dstRow[di+2] = uint8(blue / divisor)
			dstRow[di+3] = 255
		}
	}
	return &image.RGBA{
		Pix:    pix[:dh*dstStride],
		Stride: dstStride,
		Rect:   image.Rect(0, 0, dw, dh),
	}
}

func quantizeLuminance(r, g, b uint8, levels int) uint8 {
	lum := (19595*uint32(r)*257 + 38470*uint32(g)*257 + 7471*uint32(b)*257 + (1 << 15)) >> 24
	return uint8((lum * uint32(levels)) >> 8)
}

// oilPaintInPlace slides a (2*radius+1)x(2*radius+1) window across dst using
// per-column luminance histograms and a (radius+1)-row ring buffer so that no
// second full-frame image buffer is allocated. As each row enters the window,
// its quantized luminance bin is cached in the pixel's alpha byte; when the
// row leaves the window, it is overwritten with the completed opaque RGBA row.
func oilPaintInPlace(dst *image.RGBA, radius, levels int) {
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	if w == 0 || h == 0 {
		return
	}
	cols := make([]oilHistBin, w*levels)
	rowBytes := w * 4
	ringSlots := radius + 1
	ring := make([]uint8, ringSlots*rowBytes)

	addRowToCols := func(y int) {
		row := dst.Pix[y*dst.Stride : y*dst.Stride+rowBytes]
		for x := 0; x < w; x++ {
			i := x * 4
			r, g, b := row[i], row[i+1], row[i+2]
			q := quantizeLuminance(r, g, b, levels)
			row[i+3] = q
			bin := &cols[x*levels+int(q)]
			bin.count++
			bin.r += uint16(r)
			bin.g += uint16(g)
			bin.b += uint16(b)
		}
	}

	for y := 0; y < min(h, radius); y++ {
		addRowToCols(y)
	}

	var win [maxOilLevels]oilHistBin
	for y := 0; y < h; y++ {
		if addRow := y + radius; addRow < h {
			addRowToCols(addRow)
		}

		clear(win[:levels])
		for x := 0; x < min(w, radius); x++ {
			col := cols[x*levels : x*levels+levels]
			for l := 0; l < levels; l++ {
				win[l].count += col[l].count
				win[l].r += col[l].r
				win[l].g += col[l].g
				win[l].b += col[l].b
			}
		}

		outRow := ring[(y%ringSlots)*rowBytes : (y%ringSlots+1)*rowBytes]
		for x := 0; x < w; x++ {
			if right := x + radius; right < w {
				col := cols[right*levels : right*levels+levels]
				for l := 0; l < levels; l++ {
					win[l].count += col[l].count
					win[l].r += col[l].r
					win[l].g += col[l].g
					win[l].b += col[l].b
				}
			}
			if left := x - radius - 1; left >= 0 {
				col := cols[left*levels : left*levels+levels]
				for l := 0; l < levels; l++ {
					win[l].count -= col[l].count
					win[l].r -= col[l].r
					win[l].g -= col[l].g
					win[l].b -= col[l].b
				}
			}
			var best oilHistBin
			for l := 0; l < levels; l++ {
				if win[l].count > best.count {
					best = win[l]
				}
			}
			i := x * 4
			outRow[i] = uint8(best.r / best.count)
			outRow[i+1] = uint8(best.g / best.count)
			outRow[i+2] = uint8(best.b / best.count)
			outRow[i+3] = 255
		}

		if subRow := y - radius; subRow >= 0 {
			row := dst.Pix[subRow*dst.Stride : subRow*dst.Stride+rowBytes]
			for x := 0; x < w; x++ {
				i := x * 4
				bin := &cols[x*levels+int(row[i+3])]
				bin.count--
				bin.r -= uint16(row[i])
				bin.g -= uint16(row[i+1])
				bin.b -= uint16(row[i+2])
			}
			saved := ring[(subRow%ringSlots)*rowBytes : (subRow%ringSlots+1)*rowBytes]
			copy(row, saved)
		}
	}

	for y := max(0, h-radius); y < h; y++ {
		row := dst.Pix[y*dst.Stride : y*dst.Stride+rowBytes]
		saved := ring[(y%ringSlots)*rowBytes : (y%ringSlots+1)*rowBytes]
		copy(row, saved)
	}
}
