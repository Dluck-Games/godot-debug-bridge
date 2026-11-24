// Package perf compares the numeric fields of bridge performance reports.
package perf

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

type Change struct {
	Metric string   `json:"metric"`
	Before *float64 `json:"before"`
	After  *float64 `json:"after"`
	Delta  *float64 `json:"delta"`
}

func readReport(path string) (map[string]float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err = json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if _, ok := root["frame_ms"].(map[string]any); !ok {
		return nil, fmt.Errorf("%s: expected a frame_ms report", path)
	}
	metrics := map[string]float64{}
	var flatten func(string, map[string]any)
	flatten = func(prefix string, obj map[string]any) {
		for key, v := range obj {
			switch value := v.(type) {
			case float64:
				metrics[prefix+key] = value
			case map[string]any:
				flatten(prefix+key+".", value)
			}
		}
	}
	flatten("", root)
	return metrics, nil
}

func Diff(before, after string) ([]Change, error) {
	a, err := readReport(before)
	if err != nil {
		return nil, err
	}
	b, err := readReport(after)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	rows := make([]Change, 0, len(keys))
	for _, k := range keys {
		row := Change{Metric: k}
		x, xok := a[k]
		y, yok := b[k]
		if xok {
			row.Before = &x
		}
		if yok {
			row.After = &y
		}
		if xok && yok {
			d := y - x
			row.Delta = &d
		}
		rows = append(rows, row)
	}
	return rows, nil
}
