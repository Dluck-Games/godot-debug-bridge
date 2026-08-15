package debug

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveCommandScreenshot(t *testing.T) {
	f, _, err := ParseFlags([]string{"--delay", "3", "--count", "2", "--scale", "2"})
	if err != nil {
		t.Fatal(err)
	}
	got := ResolveCommand("screenshot", nil, f)
	want := map[string]any{"cmd": "screenshot", "args": map[string]any{
		"delay": 3.0, "count": 2, "scale": 2.0,
	}}
	if got.Type != RouteBridge || !reflect.DeepEqual(got.Payload, want) {
		t.Fatalf("route = %#v, want payload %#v", got, want)
	}
}

func TestResolveCommandRecord(t *testing.T) {
	f, _, err := ParseFlags([]string{"--seconds", "10", "--fps", "4", "--output", "/tmp/play.mov"})
	if err != nil {
		t.Fatal(err)
	}
	got := ResolveCommand("record", nil, f)
	if got.Type != RouteRecord || got.Options == nil || got.Options.Seconds != 10 || got.Options.FPS != 4 {
		t.Fatalf("route = %#v", got)
	}
	if bad := ResolveCommand("record", []string{"extra"}, Flags{}); bad.Type != RouteError {
		t.Fatalf("record positional route = %#v", bad)
	}
}

func TestResolveCommandFetchAndScriptValidation(t *testing.T) {
	fetch := ResolveCommand("fetch", []string{"cap_123"}, Flags{})
	want := map[string]any{"cmd": "fetch", "args": map[string]any{"capture_id": "cap_123"}}
	if !reflect.DeepEqual(fetch.Payload, want) {
		t.Fatalf("fetch payload = %#v", fetch.Payload)
	}
	if got := ResolveCommand("fetch", nil, Flags{}); got.Type != RouteError {
		t.Fatalf("missing fetch id route = %#v", got)
	}
	script := ResolveCommand("script", []string{"inspect.gd"}, Flags{})
	if script.Type != RouteScript || script.Path != "inspect.gd" {
		t.Fatalf("script route = %#v", script)
	}
	if got := ResolveCommand("script", nil, Flags{}); got.Type != RouteError {
		t.Fatalf("missing script route = %#v", got)
	}
}

func TestResolveCommandInput(t *testing.T) {
	got := ResolveCommand("input", []string{"hold", "move_right", "1.5"}, Flags{})
	want := map[string]any{"op": "hold", "action": "move_right", "duration": 1.5}
	if got.Type != RouteBridge || !reflect.DeepEqual(got.Payload["args"], want) {
		t.Fatalf("input route = %#v", got)
	}
	if bad := ResolveCommand("input", []string{"hold"}, Flags{}); bad.Type != RouteError {
		t.Fatalf("invalid input route = %#v", bad)
	}
}

func TestResolveCommandForwardsProjectVocabularyUnchanged(t *testing.T) {
	for _, command := range []string{"spawn", "time", "day", "get", "perf", "custom-command"} {
		args := []string{"one", "two"}
		got := ResolveCommand(command, args, Flags{})
		want := map[string]any{"cmd": command, "args": args}
		if got.Type != RouteBridge || !reflect.DeepEqual(got.Payload, want) {
			t.Errorf("%s route = %#v, want %#v", command, got, want)
		}
	}
}

func TestResolveCommandConsoleWrapperAndScreenshot(t *testing.T) {
	f, _, err := ParseFlags([]string{"--screenshot", "--width", "1280"})
	if err != nil {
		t.Fatal(err)
	}
	got := ResolveCommand("console", []string{"echo", "hello"}, f)
	if got.Payload["cmd"] != "echo" || !reflect.DeepEqual(got.Payload["args"], []string{"hello"}) {
		t.Fatalf("console route = %#v", got)
	}
	if _, ok := got.Payload["screenshot"]; !ok {
		t.Fatalf("console route missing screenshot: %#v", got)
	}
	if empty := ResolveCommand("console", nil, Flags{}); empty.Type != RouteError {
		t.Fatalf("empty console route = %#v", empty)
	}
}

func TestResolveCommandLifecycleMigrationHints(t *testing.T) {
	for command, hint := range map[string]string{
		"boot": "gdbg run game", "teardown": "gdbg stop", "reimport": "gdbg reimport",
	} {
		got := ResolveCommand(command, nil, Flags{})
		if got.Type != RouteMigrate || !strings.Contains(got.Message, hint) {
			t.Errorf("%s route = %#v", command, got)
		}
	}
}
