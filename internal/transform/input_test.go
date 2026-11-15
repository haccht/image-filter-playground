package transform

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"image-filter-playground/internal/imageproc"
)

type terminalReader struct {
	data []byte
	err  error
}

func (r terminalReader) Read(p []byte) (int, error) { return copy(p, r.data), r.err }

func TestReadInput(t *testing.T) {
	buffer := make([]byte, imageproc.MaxBytes+1)
	for _, size := range []int{0, 1, imageproc.MaxBytes, imageproc.MaxBytes + 1} {
		input := bytes.Repeat([]byte{42}, size)
		got, err := readInput(bytes.NewReader(input), buffer)
		if size > imageproc.MaxBytes {
			var limit *http.MaxBytesError
			if !errors.As(err, &limit) {
				t.Fatalf("size %d: %v", size, err)
			}
		} else if err != nil || !bytes.Equal(got, input) {
			t.Fatalf("size %d: %d bytes, %v", size, len(got), err)
		}
	}
	for _, failure := range []error{io.ErrUnexpectedEOF, errors.New("stream failed")} {
		if _, err := readInput(terminalReader{[]byte("partial"), failure}, buffer); !errors.Is(err, failure) {
			t.Fatalf("read failure lost: %v", err)
		}
	}
	got, err := readInput(terminalReader{[]byte("last bytes"), io.EOF}, buffer)
	if err != nil || string(got) != "last bytes" {
		t.Fatalf("final bytes lost: %q, %v", got, err)
	}
	if _, err := readInput(terminalReader{}, buffer); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("stalled reader: %v", err)
	}
}
