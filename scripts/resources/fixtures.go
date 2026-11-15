package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"

	"image-filter-playground/internal/imageproc"
)

type fixture struct {
	name    string
	data    []byte
	status  int
	chunked bool
}

func fixtures() ([]fixture, error) {
	bounds := image.Rect(0, 0, 1024, 1024)
	noise := image.NewRGBA(bounds)
	gray := image.NewGray(bounds)
	rgba16 := image.NewNRGBA64(bounds)
	rgba8 := image.NewNRGBA(bounds)
	rng := rand.New(rand.NewPCG(1, 2))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			noise.SetRGBA(x, y, color.RGBA{R: uint8(rng.Uint32()), G: uint8(rng.Uint32()), B: uint8(rng.Uint32()), A: 255})
			gray.SetGray(x, y, color.Gray{Y: uint8(rng.Uint32())})
			rgba16.SetNRGBA64(x, y, color.NRGBA64{R: uint16(x * 64), G: uint16(y * 64), B: uint16((x + y) * 32), A: uint16(x * 64)})
			rgba8.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: uint8(x + 64)})
		}
	}
	var jpg, png8, png16, oversized bytes.Buffer
	if err := jpeg.Encode(&jpg, noise, &jpeg.Options{Quality: 95}); err != nil {
		return nil, err
	}
	if err := png.Encode(&png8, gray); err != nil {
		return nil, err
	}
	if err := png.Encode(&png16, rgba16); err != nil {
		return nil, err
	}
	if err := png.Encode(&oversized, image.NewGray(image.Rect(0, 0, imageproc.MaxInputDimension+1, 1))); err != nil {
		return nil, err
	}
	// A valid ancillary PNG chunk makes an exact-2-MiB upload without increasing
	// the decoded dimensions. It exercises bounded body buffering near the limit.
	base := png8.Bytes()
	pad := make([]byte, imageproc.MaxBytes-len(base))
	binary.BigEndian.PutUint32(pad, uint32(len(pad)-12))
	copy(pad[4:8], "paDd")
	binary.BigEndian.PutUint32(pad[len(pad)-4:], crc32.ChecksumIEEE(pad[4:len(pad)-4]))
	padded := append([]byte{}, base[:len(base)-12]...)
	padded = append(padded, pad...)
	padded = append(padded, base[len(base)-12:]...)
	corrupt := append([]byte{}, png16.Bytes()...)
	corrupt[len(corrupt)/2] ^= 0xff
	cases := []fixture{
		{"jpeg-noise-1024", jpg.Bytes(), 200, false},
		{"png-gray-noise-1024", png8.Bytes(), 200, false},
		{"png-rgba16-1024", png16.Bytes(), 200, false},
		{"png-exact-2mib", padded, 200, false},
		{"png-corrupt", corrupt, 400, false},
		{"jpeg-truncated", jpg.Bytes()[:jpg.Len()/2], 400, false},
		{"png-oversized-header", oversized.Bytes()[:33], 413, false},
		{"stream-over-2mib", make([]byte, imageproc.MaxBytes+1), 413, true},
	}
	for _, v := range variants {
		data, err := variantData.ReadFile("testdata/" + v.file)
		if err != nil {
			return nil, err
		}
		cases = append(cases, fixture{v.file, data, 200, false})
	}
	wide := image.NewRGBA(image.Rect(0, 0, 2048, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 2048; x++ {
			wide.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	var wideJPEG bytes.Buffer
	if err := jpeg.Encode(&wideJPEG, wide, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	cases = append(cases, fixture{"jpeg-wide-2048x512", wideJPEG.Bytes(), 200, false})
	var rgba8PNG bytes.Buffer
	if err := png.Encode(&rgba8PNG, rgba8); err != nil {
		return nil, err
	}
	cases = append(cases, fixture{"png-rgba8-1024", rgba8PNG.Bytes(), 200, false})
	nearSquare := image.NewNRGBA(image.Rect(0, 0, 1025, 1023))
	for y := 0; y < 1023; y++ {
		for x := 0; x < 1025; x++ {
			nearSquare.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 128})
		}
	}
	var nearPNG bytes.Buffer
	if err := png.Encode(&nearPNG, nearSquare); err != nil {
		return nil, err
	}
	cases = append(cases, fixture{"png-rgba8-1025x1023", nearPNG.Bytes(), 200, false})
	return cases, nil
}
