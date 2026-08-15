package debug

import (
	"reflect"
	"testing"
)

func TestFormatResult_String(t *testing.T) {
	got := FormatResult("done")
	if !reflect.DeepEqual(got, Formatted{Stdout: []string{"done"}}) {
		t.Errorf("got %#v", got)
	}
}

func TestFormatResult_ScalarResult(t *testing.T) {
	got := FormatResult(map[string]any{"result": "ok"})
	if !reflect.DeepEqual(got, Formatted{Stdout: []string{"ok"}}) {
		t.Errorf("got %#v", got)
	}

	gotNum := FormatResult(map[string]any{"result": float64(42)})
	if !reflect.DeepEqual(gotNum, Formatted{Stdout: []string{"42"}}) {
		t.Errorf("got %#v", gotNum)
	}
}

func TestFormatResult_ArrayResult(t *testing.T) {
	got := FormatResult(map[string]any{"result": []any{"/tmp/a.png", "/tmp/b.png"}})
	want := Formatted{Stdout: []string{"/tmp/a.png", "/tmp/b.png"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestFormatResult_NullResultSkipped(t *testing.T) {
	got := FormatResult(map[string]any{"result": nil})
	if !reflect.DeepEqual(got, Formatted{}) {
		t.Errorf("got %#v, want empty", got)
	}
}

func TestFormatResult_CaptureIDAndPending(t *testing.T) {
	got := FormatResult(map[string]any{"capture_id": "cap_123"})
	if !reflect.DeepEqual(got, Formatted{Stdout: []string{"capture_id:cap_123"}}) {
		t.Errorf("got %#v", got)
	}

	gotPending := FormatResult(map[string]any{"status": "pending", "progress": "1/3"})
	wantPending := Formatted{Stdout: []string{"status:pending progress:1/3"}}
	if !reflect.DeepEqual(gotPending, wantPending) {
		t.Errorf("got %#v, want %#v", gotPending, wantPending)
	}
}

func TestFormatResult_ErrorPayload(t *testing.T) {
	got := FormatResult(map[string]any{"status": "error", "message": "boom", "result": nil})
	want := Formatted{Stderr: []string{"Error: boom"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestIsFailureResult(t *testing.T) {
	if !IsFailureResult(map[string]any{"status": "error", "message": "boom"}) {
		t.Error("error status should be a failure")
	}
	if !IsFailureResult("TIMEOUT") {
		t.Error("TIMEOUT string should be a failure")
	}
	if !IsFailureResult("CRASH\nstack") {
		t.Error("CRASH string should be a failure")
	}
	if IsFailureResult("READY") {
		t.Error("READY should not be a failure")
	}
	if IsFailureResult(map[string]any{"result": "ok"}) {
		t.Error("ok result should not be a failure")
	}
	if IsFailureResult(map[string]any{"status": "pending"}) {
		t.Error("pending should not be a failure")
	}
}
