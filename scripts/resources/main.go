// Command resources measures the unmodified Spin component using local OTLP.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"image-filter-playground/internal/imageproc"
)

type collector struct {
	sync.Mutex
	metrics map[string]histogram
	err     error
}

func (c *collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.URL.Path != "/v1/metrics" {
		http.NotFound(w, r)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err == nil && r.Header.Get("Content-Encoding") != "" {
		err = fmt.Errorf("unsupported OTLP compression")
	}
	var metrics map[string]histogram
	if err == nil {
		metrics, err = decodeMetrics(data)
	}
	c.Lock()
	if err != nil {
		c.err = err
	} else {
		for name, h := range metrics {
			if h.Count >= c.metrics[name].Count {
				c.metrics[name] = h
			}
		}
	}
	c.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	// Empty ExportMetricsServiceResponse is a successful OTLP acknowledgement.
}

type logBuffer struct {
	sync.Mutex
	data []byte
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	if len(b.data) < 1<<20 {
		b.data = append(b.data, p...)
	}
	return len(p), nil
}
func (b *logBuffer) String() string { b.Lock(); defer b.Unlock(); return string(b.data) }

type observation struct {
	Requests           []requestResult `json:"requests"`
	PeakClientRequests int             `json:"peak_client_requests"`
	Name               string          `json:"name"`
	InputBytes         int             `json:"input_bytes,omitempty"`
	ExpectedStatus     int             `json:"expected_status,omitempty"`
	Statuses           []int           `json:"statuses"`
	LatencyMS          []float64       `json:"latency_ms"`
	CPU                *histogram      `json:"guest_active_seconds,omitempty"`
	Memory             *histogram      `json:"linear_memory_bytes,omitempty"`
	Error              string          `json:"error,omitempty"`
	SpinLogs           string          `json:"spin_logs,omitempty"`
}

type report struct {
	Roughness           int           `json:"roughness,omitempty"`
	Edge                bool          `json:"edge,omitempty"`
	Oil                 bool          `json:"oil,omitempty"`
	OilBrushSize        int           `json:"oil_brush_size,omitempty"`
	PixelateBlockSize   int           `json:"pixelate_block_size,omitempty"`
	Mode                string        `json:"mode"`
	ClientConcurrency   int           `json:"client_concurrency"`
	InstanceConcurrency int           `json:"instance_concurrency"`
	IdleTimeout         string        `json:"idle_timeout"`
	RecordedAt          time.Time     `json:"recorded_at"`
	Platform            string        `json:"platform"`
	GoVersion           string        `json:"go_version"`
	SpinVersion         string        `json:"spin_version"`
	WasmSHA256          string        `json:"wasm_sha256"`
	MemoryMiB           int           `json:"instance_memory_limit_mib"`
	Repeats             int           `json:"repeats"`
	ReuseCount          int           `json:"instance_reuse_count"`
	Notes               string        `json:"notes"`
	Cases               []observation `json:"cases"`
}

func measure(work workload, references map[string][32]byte, memoryMiB int, address string, cfg settings) (obs observation) {
	repeats := len(work.requests)
	obs = observation{Name: work.name}
	if cfg.mode == "isolated" {
		obs.InputBytes = len(work.requests[0].data)
		obs.ExpectedStatus = work.requests[0].status
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		obs.Error = err.Error()
		return
	}
	c := &collector{metrics: make(map[string]histogram)}
	server := &http.Server{Handler: c, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	logs := &logBuffer{}
	args := []string{"up", "--listen", address, "--disable-cache", "--max-instance-reuse-count", fmt.Sprint(cfg.reuse), "--max-instance-concurrent-reuse-count", fmt.Sprint(cfg.instanceConcurrency), "--idle-instance-timeout", cfg.idle}
	if memoryMiB > 0 {
		args = append(args, "--max-instance-memory", fmt.Sprint(memoryMiB<<20))
	}
	cmd := exec.Command("spin", args...)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "OTEL_") && !strings.HasPrefix(env, "SPIN_") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env,
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://"+listener.Addr().String()+"/v1/metrics",
		"OTEL_EXPORTER_OTLP_METRICS_PROTOCOL=http/protobuf",
		"OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative",
		"OTEL_METRIC_EXPORT_INTERVAL=100")
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		obs.Error = err.Error()
		return
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
			<-done
		}
		if obs.Error != "" {
			obs.SpinLogs = logs.String()
		}
	}()
	deadline := time.Now().Add(30 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), "Serving http://") {
			ready = true
			break
		}
		select {
		case err := <-done:
			done <- err
			obs.Error = fmt.Sprintf("Spin startup failed: %v: %s", err, logs.String())
			return
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		obs.Error = "Spin startup timeout: " + logs.String()
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	url := "http://" + address + "/api/transform"
	if cfg.edge {
		url += fmt.Sprintf("?filter=edge&roughness=%d", cfg.roughness)
	} else if cfg.oil {
		url += fmt.Sprintf("?filter=oil&brush_size=%d&roughness=%d", cfg.brushSize, cfg.roughness)
	} else if cfg.blockSize != 0 {
		url += fmt.Sprintf("?filter=pixelate&block_size=%d", cfg.blockSize)
	}
	obs.Requests, obs.PeakClientRequests = sendRequests(client, url, work.requests, references, cfg.workers)
	for _, req := range obs.Requests {
		obs.Statuses = append(obs.Statuses, req.Status)
		obs.LatencyMS = append(obs.LatencyMS, req.LatencyMS)
		if req.Error != "" {
			obs.Error = "one or more requests failed validation"
		}
	}
	// Reused instances emit metrics only after idle eviction. Allow that to finish
	// before interpreting counts; these histograms are per lifetime, not per request.
	settledAfter := time.Now().Add(2 * time.Second)
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.Lock()
		cpu, cpuOK := c.metrics["spin.component_cpu_time"]
		memory, memoryOK := c.metrics["spin.component_memory_used"]
		err := c.err
		c.Unlock()
		if err != nil {
			obs.Error = "OTLP: " + err.Error()
			return
		}
		if cpuOK && memoryOK && time.Now().After(settledAfter) && validCounts(cpu.Count, memory.Count, repeats, cfg.mode) {
			obs.CPU = &cpu
			obs.Memory = &memory
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	obs.Error += "; missing or unexpected OTLP sample counts: " + logs.String()
	return
}

func run() error {
	repeats := flag.Int("repeats", 3, "requests per fixture (1–20)")
	mode := flag.String("mode", "isolated", "isolated, reuse, or concurrent")
	concurrency := flag.Int("concurrency", 16, "client and instance concurrency in concurrent mode (2–16)")
	memory := flag.Int("memory-mib", 0, "Spin per-instance memory cap in MiB (0 = no cap)")
	address := flag.String("listen", "127.0.0.1:3000", "Spin loopback listen address")
	blockSize := flag.Int("pixelate-block", 0, "pixelate block size, 2–128 (0 = grayscale)")
	roughness := flag.Int("roughness", 0, "edge/oil smoothing or coarseness level, 0–4 (requires -edge or -oil)")
	edge := flag.Bool("edge", false, "measure Sobel edge detection instead of grayscale")
	oil := flag.Bool("oil", false, "measure oil paint filter instead of grayscale")
	brushSize := flag.Int("brush-size", 0, "oil brush size, 1–6 (requires -oil; default 4)")
	fixtureName := flag.String("fixture", "", "run only the named fixture (empty = all)")
	flag.Parse()
	cfg, err := configuration(*mode, *concurrency)
	if err != nil {
		return err
	}
	if *blockSize != 0 && (*blockSize < imageproc.MinBlockSize || *blockSize > imageproc.MaxBlockSize) {
		return imageproc.ErrBlockSize
	}
	cfg.blockSize = *blockSize
	cfg.edge = *edge
	cfg.oil = *oil
	cfg.roughness = *roughness
	cfg.brushSize = *brushSize
	if cfg.oil && cfg.brushSize == 0 {
		cfg.brushSize = imageproc.DefaultBrushSize
	}
	if !cfg.oil && *brushSize != 0 {
		return fmt.Errorf("brush-size requires -oil")
	}
	if cfg.oil && (cfg.brushSize < imageproc.MinBrushSize || cfg.brushSize > imageproc.MaxBrushSize) {
		return imageproc.ErrBrushSize
	}
	if cfg.roughness < 0 || cfg.roughness > imageproc.MaxRoughness || (cfg.roughness != 0 && !cfg.edge && !cfg.oil) {
		return fmt.Errorf("roughness must be 0–4 and requires -edge or -oil")
	}
	if (cfg.edge && cfg.blockSize != 0) || (cfg.oil && cfg.blockSize != 0) || (cfg.edge && cfg.oil) {
		return fmt.Errorf("edge, oil, and pixelate-block are mutually exclusive")
	}
	host, _, err := net.SplitHostPort(*address)
	if err != nil || host != "127.0.0.1" || *repeats < 1 || *repeats > 20 || *memory < 0 || *memory > 1024 {
		return fmt.Errorf("invalid options; use loopback, 1–20 repeats, and 0–1024 MiB")
	}
	// Refuse an occupied port; never send the corpus to an unrelated server.
	probe, err := net.Listen("tcp", *address)
	if err != nil {
		return fmt.Errorf("listen address unavailable: %w", err)
	}
	probe.Close()
	wasm, err := os.ReadFile("main.wasm")
	if err != nil {
		return err
	}
	version, err := exec.Command("spin", "--version").Output()
	if err != nil {
		return err
	}
	cases, err := fixtures()
	if err != nil {
		return err
	}
	if *fixtureName != "" {
		var selected []fixture
		for _, f := range cases {
			if f.name == *fixtureName {
				selected = append(selected, f)
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("unknown fixture %q", *fixtureName)
		}
		cases = selected
	}
	for i, f := range cases {
		_, _, err := imageproc.InspectSource(f.data)
		if errors.Is(err, imageproc.ErrSourceDimensions) || errors.Is(err, imageproc.ErrDecodeBudget) || errors.Is(err, imageproc.ErrTooLarge) {
			cases[i].status = http.StatusRequestEntityTooLarge
		} else if err != nil {
			cases[i].status = http.StatusBadRequest
		}
	}
	references, err := referenceHashes(cases, cfg)
	if err != nil {
		return err
	}
	rep := report{Mode: cfg.mode, ClientConcurrency: cfg.workers, InstanceConcurrency: cfg.instanceConcurrency, IdleTimeout: cfg.idle, RecordedAt: time.Now().UTC(), Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), SpinVersion: strings.TrimSpace(string(version)), WasmSHA256: fmt.Sprintf("%x", sha256.Sum256(wasm)), MemoryMiB: *memory, Repeats: *repeats, ReuseCount: cfg.reuse,
		Notes: "Local synthetic corpus; metrics describe instance lifetimes (multiple requests when reused); guest active seconds use Spin wall-clock call hooks, not OS CPU accounting; linear memory excludes host RSS; client overlap does not prove concurrent execution within one instance; not production or exhaustive worst-case evidence."}
	failed := false
	rep.PixelateBlockSize = cfg.blockSize
	rep.Edge = cfg.edge
	rep.Oil = cfg.oil
	rep.OilBrushSize = cfg.brushSize
	rep.Roughness = cfg.roughness
	for _, work := range workloads(cases, *repeats, cfg.mode, cfg.blockSize) {
		obs := measure(work, references, *memory, *address, cfg)
		fmt.Fprintf(os.Stderr, "%s: statuses=%v error=%s\n", work.name, obs.Statuses, obs.Error)
		rep.Cases = append(rep.Cases, obs)
		if obs.Error != "" {
			failed = true
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		return err
	}
	if failed {
		return fmt.Errorf("resource checks failed; inspect the JSON report")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
