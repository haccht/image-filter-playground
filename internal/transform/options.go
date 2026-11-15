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
)

type options struct {
	filter    string
	blockSize int
	roughness int
}

func parseOptions(rawQuery string) (options, error) {
	opts := options{filter: filterGrayscale, blockSize: imageproc.DefaultBlockSize}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return opts, errors.New("invalid query parameters")
	}
	for key, values := range query {
		if (key != "filter" && key != "block_size" && key != "roughness") || len(values) != 1 {
			return opts, errors.New("expected single filter, block_size and roughness parameters")
		}
	}
	if filter := query.Get("filter"); filter != "" {
		opts.filter = filter
	}
	switch opts.filter {
	case filterGrayscale, filterPixelate, filterEdge:
	default:
		return opts, errors.New("filter must be grayscale, pixelate or edge")
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
	if opts.filter == filterEdge {
		opts.roughness = imageproc.DefaultRoughness
	}
	if values, ok := query["roughness"]; ok {
		if opts.filter != filterEdge {
			return opts, errors.New("roughness requires filter=edge")
		}
		level, err := strconv.Atoi(values[0])
		if err != nil || level < 0 || level > imageproc.MaxRoughness {
			return opts, imageproc.ErrRoughness
		}
		opts.roughness = level
	}
	return opts, nil
}
