package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"math"
	"testing"

	"image-filter-playground/internal/imageproc"
)

func message(number int, payload []byte) []byte {
	result := binary.AppendUvarint(nil, uint64(number<<3|2))
	result = binary.AppendUvarint(result, uint64(len(payload)))
	return append(result, payload...)
}
func fixed(number int, value uint64) []byte {
	return binary.LittleEndian.AppendUint64(binary.AppendUvarint(nil, uint64(number<<3|1)), value)
}
func otlp(name, component string, temporality byte, count uint64) []byte {
	attr := append(message(1, []byte("component_id")), message(2, message(1, []byte(component)))...)
	point := append(fixed(4, count), fixed(5, math.Float64bits(0.6))...)
	point = append(point, fixed(11, math.Float64bits(0.1))...)
	point = append(point, fixed(12, math.Float64bits(0.3))...)
	point = append(point, message(9, attr)...)
	histogram := append(message(1, point), 16, temporality)
	metric := append(message(1, []byte(name)), message(9, histogram)...)
	return message(1, message(2, message(2, metric)))
}

func TestDecodeMetrics(t *testing.T) {
	data := append(otlp("spin.component_cpu_time", "transform", 2, 3), otlp("spin.component_memory_used", "transform", 2, 3)...)
	got, err := decodeMetrics(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["spin.component_cpu_time"] != (histogram{Count: 3, Sum: 0.6, Min: 0.1, Max: 0.3}) {
		t.Fatalf("%+v", got)
	}
	other, err := decodeMetrics(otlp("spin.component_cpu_time", "other", 2, 3))
	if err != nil || len(other) != 0 {
		t.Fatalf("other component: %v, %v", other, err)
	}
	if _, err := decodeMetrics(otlp("spin.component_cpu_time", "transform", 1, 3)); err == nil {
		t.Fatal("accepted delta metric")
	}
	if _, err := decodeMetrics(otlp("spin.component_cpu_time", "transform", 2, 0)); err == nil {
		t.Fatal("accepted empty histogram")
	}
	duplicate := append(data, data...)
	if _, err := decodeMetrics(duplicate); err == nil {
		t.Fatal("accepted ambiguous series")
	}
	for _, data := range [][]byte{{0}, {0x80}, {0x0a, 100, 1}, {9, 1}, {15}, {8, 0x80}, {13, 1, 2}} {
		if _, err := fields(data); err == nil {
			t.Fatalf("accepted malformed protobuf %x", data)
		}
	}
}

func TestCorpus(t *testing.T) {
	cases, err := fixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range cases {
		t.Run(f.name, func(t *testing.T) {
			// Verify fixture validity independently of the HTTP admission policy.
			img, _, err := image.Decode(bytes.NewReader(f.data))
			if f.status == 200 {
				if err != nil {
					t.Fatal(err)
				}
				width, height := 1024, 1024
				if f.name == "jpeg-wide-2048x512" {
					width, height = 2048, 512
				}
				if f.name == "png-rgba8-1025x1023" {
					width, height = 1025, 1023
				}
				for _, v := range variants {
					if f.name == v.file {
						width, height = v.width, v.height
					}
				}
				if img.Bounds().Dx() != width || img.Bounds().Dy() != height || len(f.data) > imageproc.MaxBytes {
					t.Fatal("invalid valid fixture")
				}
			} else if err == nil {
				t.Fatal("invalid fixture decoded successfully")
			}
			if f.name == "png-exact-2mib" && len(f.data) != imageproc.MaxBytes {
				t.Fatal("not exactly 2 MiB")
			}
			if f.name == "png-rgba16-1024" && !bytes.Equal(f.data[24:26], []byte{16, 6}) {
				t.Fatal("not 16-bit RGBA PNG")
			}
		})
	}
}
