package perf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiff(t *testing.T) {
	dir := t.TempDir()
	file := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	a := file("a", `{"frame_ms":{"mean":2},"custom_monitors":{"test/hits":{"max":0}}}`)
	b := file("b", `{"frame_ms":{"mean":1},"fps":{"mean":60}}`)
	rows, err := Diff(a, b)
	if err != nil || len(rows) != 3 {
		t.Fatalf("%v %v", rows, err)
	}
	if r := rows[0]; r.Metric != "custom_monitors.test/hits.max" || r.After != nil || r.Delta != nil || *r.Before != 0 {
		t.Fatal(r)
	}
	if rows[1].Before != nil || *rows[1].After != 60 || *rows[2].Delta != -1 {
		t.Fatal(rows)
	}
	if _, err := Diff(a, file("bad", "{}")); err == nil {
		t.Fatal("accepted invalid report")
	}
}
