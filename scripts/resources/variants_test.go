package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestVariantIdentity(t *testing.T) {
	manifestData, err := variantData.ReadFile("testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]struct {
		Bytes  int    `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != len(variants) {
		t.Fatal("fixture manifest count differs")
	}
	for _, v := range variants {
		t.Run(v.file, func(t *testing.T) {
			data, err := variantData.ReadFile("testdata/" + v.file)
			if err != nil {
				t.Fatal(err)
			}
			entry := manifest[v.file]
			if len(data) != entry.Bytes || fmt.Sprintf("%x", sha256.Sum256(data)) != entry.SHA256 {
				t.Fatal("fixture identity changed")
			}
			if strings.HasSuffix(v.file, ".png") {
				depth := byte(8)
				if strings.Contains(v.file, "rgba16") {
					depth = 16
				}
				if len(data) < 33 || !bytes.Equal(data[24:29], []byte{depth, 6, 0, 0, 1}) {
					t.Fatal("expected Adam7 RGBA PNG with declared depth")
				}
			} else {
				frame := progressiveFrame(data)
				sampling := byte(0x22)
				if strings.Contains(v.file, "444") {
					sampling = 0x11
				}
				if len(frame) < 15 || frame[0] != 8 || frame[5] != 3 || frame[7] != sampling || bytes.Count(data, []byte{0xff, 0xda}) < 2 {
					t.Fatal("expected 8-bit, three-component progressive JPEG with multiple scans")
				}
			}
		})
	}
}

// Read marker segments before the first scan; SOF2 identifies progressive DCT.
func progressiveFrame(data []byte) []byte {
	if len(data) < 2 || !bytes.Equal(data[:2], []byte{0xff, 0xd8}) {
		return nil
	}
	for pos := 2; pos+4 <= len(data); {
		if data[pos] != 0xff || data[pos+1] == 0xda {
			return nil
		}
		length := int(binary.BigEndian.Uint16(data[pos+2:]))
		if length < 2 || pos+2+length > len(data) {
			return nil
		}
		if data[pos+1] == 0xc2 {
			return data[pos+4 : pos+2+length]
		}
		pos += 2 + length
	}
	return nil
}
