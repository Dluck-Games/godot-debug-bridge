package godot

import "testing"

func TestImportFailureMarkerRejectsGodotScriptErrors(t *testing.T) {
	for _, output := range []string{
		"SCRIPT ERROR: Parse Error: unexpected token",
		"SCRIPT ERROR: Compile Error: failed dependency",
		"ERROR: Failed to load script res://main.gd",
	} {
		if importFailureMarker(output) == "" {
			t.Fatalf("expected failure marker for %q", output)
		}
	}
}

func TestImportFailureMarkerAllowsOrdinaryImportOutput(t *testing.T) {
	if marker := importFailureMarker("Godot Engine v4\n[DONE] first_scan_filesystem\n"); marker != "" {
		t.Fatalf("unexpected marker %q", marker)
	}
}
