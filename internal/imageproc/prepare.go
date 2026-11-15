package imageproc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
)

const (
	MaxInputDimension = 4096
	MaxInputPixels    = 1 << 20
	MaxDecodeBytes    = 4 << 20
)

var (
	ErrSourceDimensions = errors.New("source exceeds 4096 pixels per side or 1048576 total pixels")
	ErrDecodeBudget     = errors.New("source exceeds safe decoding budget; reduce image dimensions")
)

// InspectSource bounds source dimensions and estimated decoder work before any
// full image allocation. The estimate is a conservative admission policy, not
// a measurement or hard limit on total Go/host/Wasm memory.
func InspectSource(data []byte) (image.Config, string, error) {
	if len(data) > MaxBytes {
		return image.Config{}, "", ErrTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return image.Config{}, "", ErrInvalid
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxInputDimension || cfg.Height > MaxInputDimension || uint64(cfg.Width)*uint64(cfg.Height) > MaxInputPixels {
		return cfg, format, ErrSourceDimensions
	}
	bytesPerPixel := uint64(4)
	if format == "png" {
		// Cover alpha/tRNS expansion as RGBA even for grayscale/paletted PNG.
		if len(data) < 33 {
			return cfg, format, ErrInvalid
		}
		if data[24] == 16 {
			bytesPerPixel = 8
		}
		// Adam7 holds the final image plus temporary pass images.
		if data[28] == 1 {
			bytesPerPixel *= 2
		}
	} else {
		progressive, components, err := jpegFrame(data)
		if err != nil {
			return cfg, format, err
		}
		if components == 4 {
			// Allow for the additional CMYK/YCCK conversion buffer.
			bytesPerPixel = 8
		}
		if progressive {
			// Coefficients persist across scans in addition to decoded pixels.
			bytesPerPixel *= 4
		}
	}
	if uint64(cfg.Width)*uint64(cfg.Height)*bytesPerPixel > MaxDecodeBytes {
		return cfg, format, ErrDecodeBudget
	}
	return cfg, format, nil
}

// Prepare validates and decodes the source, then fits it to the filter's budget.
func Prepare(data []byte, maxDimension int) (image.Image, error) {
	if maxDimension < 1 || maxDimension > MaxDimension {
		return nil, ErrDimensions
	}
	src, err := decodeSource(data)
	if err != nil {
		return nil, err
	}
	return ResizeToFit(src, maxDimension)
}

// PrepareGrayscale avoids a full-size intermediate color image when resizing.
func PrepareGrayscale(data []byte) (image.Image, error) {
	src, err := decodeSource(data)
	if err != nil {
		return nil, err
	}
	return GrayscaleToFit(src, MaxDimension)
}

// PreparePixelate shares grayscale's processing limit and avoids a full-size
// intermediate RGBA image even for sources that need downsampling.
func PreparePixelate(data []byte, blockSize int) (image.Image, error) {
	if blockSize < MinBlockSize || blockSize > MaxBlockSize {
		return nil, ErrBlockSize
	}
	src, err := decodeSource(data)
	if err != nil {
		return nil, err
	}
	return PixelateToFit(src, MaxDimension, blockSize)
}

func decodeSource(data []byte) (image.Image, error) {
	cfg, format, err := InspectSource(data)
	if err != nil {
		return nil, err
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format {
		return nil, ErrInvalid
	}
	if img.Bounds().Dx() != cfg.Width || img.Bounds().Dy() != cfg.Height {
		return nil, ErrInvalid
	}
	return img, nil
}

// jpegFrame reads bounded marker segments before scan data. DecodeConfig has
// already validated the header; this reads only the frame's allocation traits.
func jpegFrame(data []byte) (bool, int, error) {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return false, 0, ErrInvalid
	}
	for pos := 2; pos < len(data); {
		if data[pos] != 0xff {
			return false, 0, ErrInvalid
		}
		for pos < len(data) && data[pos] == 0xff {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		if marker == 0xda || marker == 0xd9 || pos+2 > len(data) {
			break
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			continue
		}
		length := int(binary.BigEndian.Uint16(data[pos:]))
		if length < 2 || length > len(data)-pos {
			break
		}
		if marker == 0xc0 || marker == 0xc1 || marker == 0xc2 {
			if length < 8 {
				break
			}
			return marker == 0xc2, int(data[pos+7]), nil
		}
		pos += length
	}
	return false, 0, ErrInvalid
}
