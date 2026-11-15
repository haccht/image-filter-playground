package main

import (
	"encoding/binary"
	"fmt"
	"math"
)

// This local-only reader supports the OTLP protobuf histogram subset emitted by
// Spin 4.3.0. Unknown fields are skipped; malformed wire data fails the run.
// Schema: opentelemetry-proto v1.9.0, metrics/v1/metrics.proto.
type field struct {
	number int
	wire   int
	data   []byte
	value  uint64
}

func fields(data []byte) ([]field, error) {
	var result []field
	for len(data) > 0 {
		key, n := binary.Uvarint(data)
		if n <= 0 || key>>3 == 0 {
			return nil, fmt.Errorf("invalid protobuf key")
		}
		data = data[n:]
		f := field{number: int(key >> 3), wire: int(key & 7)}
		switch f.wire {
		case 0:
			v, n := binary.Uvarint(data)
			if n <= 0 {
				return nil, fmt.Errorf("invalid varint")
			}
			f.value = v
			data = data[n:]
		case 1, 5:
			size := 8
			if f.wire == 5 {
				size = 4
			}
			if len(data) < size {
				return nil, fmt.Errorf("truncated fixed field")
			}
			f.data = data[:size]
			data = data[size:]
			if size == 8 {
				f.value = binary.LittleEndian.Uint64(f.data)
			}
		case 2:
			size, n := binary.Uvarint(data)
			if n <= 0 || size > uint64(len(data)-n) {
				return nil, fmt.Errorf("truncated byte field")
			}
			data = data[n:]
			f.data = data[:int(size)]
			data = data[int(size):]
		default:
			return nil, fmt.Errorf("unsupported protobuf wire type %d", f.wire)
		}
		result = append(result, f)
	}
	return result, nil
}

func children(data []byte, number int) ([][]byte, error) {
	fs, err := fields(data)
	if err != nil {
		return nil, err
	}
	var result [][]byte
	for _, f := range fs {
		if f.number == number && f.wire == 2 {
			result = append(result, f.data)
		}
	}
	return result, nil
}

type histogram struct {
	Count uint64  `json:"count"`
	Sum   float64 `json:"sum"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

func decodeMetrics(data []byte) (map[string]histogram, error) {
	result := make(map[string]histogram)
	resources, err := children(data, 1)
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		scopes, err := children(resource, 2)
		if err != nil {
			return nil, err
		}
		for _, scope := range scopes {
			metrics, err := children(scope, 2)
			if err != nil {
				return nil, err
			}
			for _, metric := range metrics {
				fs, err := fields(metric)
				if err != nil {
					return nil, err
				}
				var name string
				var data []byte
				for _, f := range fs {
					if f.number == 1 && f.wire == 2 {
						name = string(f.data)
					}
					if f.number == 9 && f.wire == 2 {
						data = f.data
					}
				}
				if name != "spin.component_cpu_time" && name != "spin.component_memory_used" {
					continue
				}
				fs, err = fields(data)
				if err != nil {
					return nil, err
				}
				var points [][]byte
				cumulative := false
				for _, f := range fs {
					if f.number == 1 && f.wire == 2 {
						points = append(points, f.data)
					}
					if f.number == 2 && f.wire == 0 && f.value == 2 {
						cumulative = true
					}
				}
				if !cumulative {
					return nil, fmt.Errorf("%s is not cumulative", name)
				}
				for _, point := range points {
					fs, err := fields(point)
					if err != nil {
						return nil, err
					}
					var h histogram
					var component string
					hasSum, hasMin, hasMax := false, false, false
					for _, f := range fs {
						switch {
						case f.number == 4 && f.wire == 1:
							h.Count = f.value
						case f.number == 5 && f.wire == 1:
							h.Sum = math.Float64frombits(f.value)
							hasSum = true
						case f.number == 11 && f.wire == 1:
							h.Min = math.Float64frombits(f.value)
							hasMin = true
						case f.number == 12 && f.wire == 1:
							h.Max = math.Float64frombits(f.value)
							hasMax = true
						case f.number == 9 && f.wire == 2:
							kv, err := fields(f.data)
							if err != nil {
								return nil, err
							}
							var key, value string
							for _, k := range kv {
								if k.number == 1 && k.wire == 2 {
									key = string(k.data)
								}
								if k.number == 2 && k.wire == 2 {
									av, err := fields(k.data)
									if err != nil {
										return nil, err
									}
									for _, v := range av {
										if v.number == 1 && v.wire == 2 {
											value = string(v.data)
										}
									}
								}
							}
							if key == "component_id" {
								component = value
							}
						}
					}
					if component != "transform" {
						continue
					}
					if !hasSum || !hasMin || !hasMax || h.Count == 0 || math.IsNaN(h.Sum) || math.IsInf(h.Sum, 0) || h.Min < 0 || h.Max < h.Min || math.IsNaN(h.Min) || math.IsNaN(h.Max) || math.IsInf(h.Max, 0) {
						return nil, fmt.Errorf("incomplete histogram %s", name)
					}
					if _, exists := result[name]; exists {
						return nil, fmt.Errorf("ambiguous histogram %s", name)
					}
					result[name] = h
				}
			}
		}
	}
	return result, nil
}
