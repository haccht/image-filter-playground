package main

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWorkloads(t *testing.T) {
	cases := []fixture{{name: "first", status: 400}, {name: "png-rgba8-1024", status: 200}}
	for _, mode := range []string{"isolated", "reuse", "concurrent"} {
		cfg, err := configuration(mode, 4)
		if err != nil {
			t.Fatal(err)
		}
		work := workloads(cases, 3, mode, 0)
		if mode == "isolated" {
			if len(work) != 2 || len(work[0].requests) != 3 || cfg.reuse != 1 {
				t.Fatal(work, cfg)
			}
		} else {
			if len(work[0].requests) != 6 || work[0].requests[0].name != "first" || work[0].requests[1].name != "png-rgba8-1024" {
				t.Fatal(work)
			}
			if mode == "concurrent" && (len(work) != 2 || len(work[1].requests) != 6 || cfg.workers != 4) {
				t.Fatal(work, cfg)
			}
		}
	}
	if _, err := configuration("other", 4); err == nil {
		t.Fatal("invalid mode accepted")
	}
	if _, err := configuration("concurrent", 17); err == nil {
		t.Fatal("unbounded concurrency accepted")
	}
	for _, tc := range []struct {
		cpu, mem uint64
		n        int
		mode     string
		want     bool
	}{
		{3, 3, 3, "isolated", true}, {2, 2, 3, "isolated", false},
		{1, 1, 24, "reuse", true}, {24, 24, 24, "reuse", false},
		{2, 2, 24, "concurrent", true}, {0, 0, 24, "concurrent", false},
		{1, 2, 24, "concurrent", false}, {25, 25, 24, "concurrent", false},
	} {
		if got := validCounts(tc.cpu, tc.mem, tc.n, tc.mode); got != tc.want {
			t.Fatalf("counts %+v: %v", tc, got)
		}
	}
}

func TestConcurrentResultIdentity(t *testing.T) {
	// Force overlap and reverse response order without depending on a TCP listener.
	var entered sync.WaitGroup
	entered.Add(4)
	outputs := map[string][]byte{"a": []byte("JPEG-A"), "b": []byte("JPEG-B"), "c": []byte("JPEG-C"), "d": []byte("JPEG-D")}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		entered.Done()
		entered.Wait()
		time.Sleep(time.Duration('d'-body[0]) * time.Millisecond)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/jpeg"}, "Cache-Control": {"no-store"}}, Body: io.NopCloser(bytes.NewReader(outputs[string(body)]))}, nil
	})
	cases := []fixture{}
	refs := map[string][32]byte{}
	for _, name := range []string{"a", "b", "c", "d"} {
		cases = append(cases, fixture{name: name, data: []byte(name), status: 200})
		refs[name] = sha256.Sum256(outputs[name])
	}
	result, peak := sendRequests(&http.Client{Transport: transport}, "http://127.0.0.1/api/transform", cases, refs, 4)
	if peak != 4 {
		t.Fatalf("peak=%d", peak)
	}
	for i, r := range result {
		if r.Fixture != cases[i].name || r.Error != "" {
			t.Fatalf("result %d: %+v", i, r)
		}
	}
	// A different grayscale JPEG from another request must not be accepted.
	wrong := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/jpeg"}, "Cache-Control": {"no-store"}}, Body: io.NopCloser(bytes.NewReader(outputs["b"]))}, nil
	})}
	got := request(wrong, "http://127.0.0.1/api/transform", cases[0], refs["a"])
	if got.Error == "" {
		t.Fatal("cross-request output accepted")
	}
}
