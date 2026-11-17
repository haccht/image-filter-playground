package transform

import (
	"errors"
	"net/url"
	"strconv"

	"image-filter-playground/internal/imageproc"
)

const (
	filterGrayscale = "grayscale"
	filterPixelate  = "pixelate"
	filterEdge      = "edge"
	filterOil       = "oil"
)

type options struct {
	filter    string
	blockSize int
	roughness int
	brushSize int
}

func parseOptions(rawQuery string) (options, error) {
	opts := options{
		filter:    filterGrayscale,
		blockSize: imageproc.DefaultBlockSize,
		brushSize: imageproc.DefaultBrushSize,
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return opts, errors.New("invalid query parameters")
	}
	for key, values := range query {
		if (key != "filter" && key != "block_size" && key != "roughness" && key != "brush_size") || len(values) != 1 {
			return opts, errors.New("expected single filter, block_size, roughness and brush_size parameters")
		}
	}
	if filter := query.Get("filter"); filter != "" {
		opts.filter = filter
	}
	switch opts.filter {
	case filterGrayscale, filterPixelate, filterEdge, filterOil:
	default:
		return opts, errors.New("filter must be grayscale, pixelate, edge or oil")
	}
	if values, ok := query["block_size"]; ok {
		if opts.filter != filterPixelate {
			return opts, errors.New("block_size requires filter=pixelate")
		}
		size, err := strconv.Atoi(values[0])
		if err != nil || size < imageproc.MinBlockSize || size > imageproc.MaxBlockSize {
			return opts, imageproc.ErrBlockSize
		}
		opts.blockSize = size
	}
	if opts.filter == filterEdge || opts.filter == filterOil {
		opts.roughness = imageproc.DefaultRoughness
	}
	if values, ok := query["roughness"]; ok {
		if opts.filter != filterEdge && opts.filter != filterOil {
			return opts, errors.New("roughness requires filter=edge or filter=oil")
		}
		level, err := strconv.Atoi(values[0])
		if err != nil || level < 0 || level > imageproc.MaxRoughness {
			return opts, imageproc.ErrRoughness
		}
		opts.roughness = level
	}
	if values, ok := query["brush_size"]; ok {
		if opts.filter != filterOil {
			return opts, errors.New("brush_size requires filter=oil")
		}
		size, err := strconv.Atoi(values[0])
		if err != nil || size < imageproc.MinBrushSize || size > imageproc.MaxBrushSize {
			return opts, imageproc.ErrBrushSize
		}
		opts.brushSize = size
	}
	return opts, nil
}
