package main

import "embed"

//go:embed testdata/*.jpg testdata/*.png testdata/manifest.json
var variantData embed.FS

var variants = []struct {
	file          string
	width, height int
}{
	{"progressive-1024x1024-420.jpg", 1024, 1024},
	{"progressive-1024x1024-444.jpg", 1024, 1024},
	{"progressive-257x129-420.jpg", 257, 129},
	{"progressive-1x1-420.jpg", 1, 1},
	{"adam7-rgba16-1024x1024.png", 1024, 1024},
	{"adam7-rgba16-513x257.png", 513, 257},
	{"adam7-rgba16-1x1.png", 1, 1},
	{"adam7-rgba8-1024x1024.png", 1024, 1024},
	{"progressive-512x512-444.jpg", 512, 512},
	{"adam7-rgba16-512x512.png", 512, 512},
	{"adam7-rgba8-512x512.png", 512, 512},
}
