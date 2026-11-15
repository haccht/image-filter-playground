package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"image-filter-playground/internal/imageproc"
)

type settings struct {
	mode                                string
	workers, reuse, instanceConcurrency int
	idle                                string
	blockSize                           int
	edge                                bool
	roughness                           int
}

func configuration(mode string, concurrency int) (settings, error) {
	switch mode {
	case "isolated":
		return settings{mode: mode, workers: 1, reuse: 1, instanceConcurrency: 1, idle: "10ms"}, nil
	case "reuse":
		return settings{mode: mode, workers: 1, reuse: 128, instanceConcurrency: 1, idle: "1s"}, nil
	case "concurrent":
		if concurrency < 2 || concurrency > 16 {
			return settings{}, fmt.Errorf("concurrency must be between 2 and 16")
		}
		return settings{mode: mode, workers: concurrency, reuse: 128, instanceConcurrency: concurrency, idle: "1s"}, nil
	default:
		return settings{}, fmt.Errorf("mode must be isolated, reuse, or concurrent")
	}
}

type workload struct {
	name     string
	requests []fixture
}

func workloads(cases []fixture, repeats int, mode string, blockSize int) []workload {
	var result []workload
	if mode == "isolated" {
		for _, f := range cases {
			work := workload{name: f.name}
			for range repeats {
				work.requests = append(work.requests, f)
			}
			result = append(result, work)
		}
		return result
	}
	mixed := workload{name: "mixed-" + mode}
	for range repeats {
		mixed.requests = append(mixed.requests, cases...)
	}
	result = append(result, mixed)
	if mode == "concurrent" {
		// Exercise large decoder allocations separately from the mixed workload.
		names := []string{"png-rgba8-1024", "png-rgba8-1025x1023", "jpeg-wide-2048x512", "progressive-512x512-444.jpg", "adam7-rgba16-512x512.png", "adam7-rgba8-512x512.png"}
		for _, name := range names {
			work := workload{name: name + "-concurrent"}
			for _, f := range cases {
				if f.name == name {
					for range repeats * len(cases) {
						work.requests = append(work.requests, f)
					}
				}
			}
			if len(work.requests) > 0 {
				result = append(result, work)
			}
		}
	}
	return result
}

func referenceHashes(cases []fixture, blockSize int, edge bool, roughness int) (map[string][32]byte, error) {
	result := make(map[string][32]byte)
	for _, f := range cases {
		if f.status != 200 {
			continue
		}
		img, err := imageproc.Prepare(f.data, imageproc.MaxDimension)
		if err != nil {
			return nil, err
		}
		var filtered image.Image
		if blockSize != 0 {
			filtered, err = imageproc.Pixelate(img, blockSize)
			if err != nil {
				return nil, err
			}
		} else {
			gray := imageproc.Grayscale(img)
			filtered = gray
			if edge {
				filtered, err = imageproc.EdgeDetectionWithRoughness(gray, roughness)
				if err != nil {
					return nil, err
				}
			}
		}
		output, err := imageproc.JPEG(filtered)
		if err != nil {
			return nil, err
		}
		result[f.name] = sha256.Sum256(output)
	}
	return result, nil
}

type requestResult struct {
	Fixture        string  `json:"fixture"`
	InputBytes     int     `json:"input_bytes"`
	ExpectedStatus int     `json:"expected_status"`
	Status         int     `json:"status"`
	LatencyMS      float64 `json:"latency_ms"`
	ExpectedSHA256 string  `json:"expected_sha256,omitempty"`
	OutputSHA256   string  `json:"output_sha256,omitempty"`
	Error          string  `json:"error,omitempty"`
}

func request(client *http.Client, url string, f fixture, expected [32]byte) (result requestResult) {
	result = requestResult{Fixture: f.name, InputBytes: len(f.data), ExpectedStatus: f.status}
	var reader io.Reader = bytes.NewReader(f.data)
	if f.chunked {
		reader = struct{ io.Reader }{reader}
	}
	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		result.Error = err.Error()
		return
	}
	start := time.Now()
	defer func() { result.LatencyMS = float64(time.Since(start)) / float64(time.Millisecond) }()
	resp, err := client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return
	}
	defer resp.Body.Close()
	result.Status = resp.StatusCode
	output, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil {
		result.Error = err.Error()
		return
	}
	if resp.StatusCode != f.status {
		result.Error = fmt.Sprintf("status %d, expected %d", resp.StatusCode, f.status)
		return
	}
	if f.status == 200 {
		result.ExpectedSHA256 = fmt.Sprintf("%x", expected)
		result.OutputSHA256 = fmt.Sprintf("%x", sha256.Sum256(output))
		if len(output) > 4<<20 || resp.Header.Get("Content-Type") != "image/jpeg" || result.OutputSHA256 != result.ExpectedSHA256 {
			result.Error = "JPEG differs from the per-fixture native reference"
		}
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		result.Error = "missing no-store"
	}
	return
}

// sendRequests uses bounded waves with a shared start barrier. Each result has
// its original input index, even if the responses arrive out of order.
func sendRequests(client *http.Client, url string, batch []fixture, references map[string][32]byte, workers int) ([]requestResult, int) {
	results := make([]requestResult, len(batch))
	var active, peak atomic.Int32
	for offset := 0; offset < len(batch); offset += workers {
		end := min(offset+workers, len(batch))
		start := make(chan struct{})
		var ready, done sync.WaitGroup
		ready.Add(end - offset)
		done.Add(end - offset)
		for index := offset; index < end; index++ {
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				current := active.Add(1)
				for previous := peak.Load(); current > previous; previous = peak.Load() {
					if peak.CompareAndSwap(previous, current) {
						break
					}
				}
				f := batch[index]
				results[index] = request(client, url, f, references[f.name])
				active.Add(-1)
			}()
		}
		ready.Wait()
		close(start)
		done.Wait()
	}
	return results, int(peak.Load())
}

func validCounts(cpu, memory uint64, requests int, mode string) bool {
	if cpu != memory || cpu == 0 || cpu > uint64(requests) {
		return false
	}
	if mode == "isolated" {
		return cpu == uint64(requests)
	}
	// Fewer lifetimes than requests proves that at least one instance was reused.
	if mode == "reuse" {
		return cpu < uint64(requests)
	}
	return true
}
