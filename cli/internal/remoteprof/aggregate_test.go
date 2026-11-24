package remoteprof

import (
	"strconv"
	"testing"
)

func sendMessage(t *testing.T, c *collector, name string, args ...any) error {
	t.Helper()
	b, err := encode(nil, []any{name, 1, args})
	if err != nil {
		t.Fatal(err)
	}
	return c.message(b)
}

func TestPhaseLimits(t *testing.T) {
	c := newCollector()
	for i := range maxPhases {
		name := strconv.Itoa(i)
		for _, op := range []string{"gdbg:phase_start", "gdbg:phase_end"} {
			if err := sendMessage(t, c, op, name); err != nil {
				t.Fatal(err)
			}
		}
	}
	if sendMessage(t, c, "gdbg:phase_start", "extra") == nil || len(c.stack) != 0 {
		t.Fatal("accepted phase beyond limit")
	}
	if sendMessage(t, c, "servers:profile_frame", 1, 0, 0, 0, 0, 0, 0, 0) == nil || len(c.phases) != maxPhases {
		t.Fatal("implicit default phase bypassed limit")
	}
	for range maxStackDepth {
		if err := sendMessage(t, c, "gdbg:phase_start", "0"); err != nil {
			t.Fatal(err)
		}
	}
	if sendMessage(t, c, "gdbg:phase_start", "0") == nil || len(c.stack) != maxStackDepth {
		t.Fatal("accepted phase beyond stack limit")
	}
}

func TestSignatureLimit(t *testing.T) {
	c := newCollector()
	for id := range maxSignatures - 1 {
		if err := sendMessage(t, c, "servers:function_signature", "function", id); err != nil {
			t.Fatal(err)
		}
	}
	frame := func(id int) error {
		return sendMessage(t, c, "servers:profile_frame", 1, 0, 0, 0, 0, 0, 0, 5, id, 1, 0, 0, 0)
	}
	if err := frame(maxSignatures - 1); err != nil {
		t.Fatal(err)
	}
	if err := sendMessage(t, c, "servers:function_signature", "late signature", maxSignatures-1); err != nil {
		t.Fatal(err)
	}
	if sendMessage(t, c, "servers:function_signature", "extra", maxSignatures) == nil || frame(maxSignatures) == nil {
		t.Fatal("accepted signature beyond limit")
	}
	p := c.phases["default"]
	if len(c.signatures) != maxSignatures || len(p.scripts) != 1 || p.ServerFrames != 1 {
		t.Fatal("rejected frame modified aggregation")
	}
}

func TestPhaseAggregation(t *testing.T) {
	c := newCollector()
	send := func(name string, args ...any) {
		t.Helper()
		if err := sendMessage(t, c, name, args...); err != nil {
			t.Fatal(err)
		}
	}
	send("servers:function_signature", "res://actor.gd::5::update", 7)
	frame := []any{1, 0.01, 0.002, 0.001, 0.016, 0.003, 1, "physics", 4, "step", 0.001, "step", 0.002, 5, 7, 4, 0.002, 0.003, 0.001}
	send("servers:profile_frame", frame...)
	send("gdbg:phase_start", "loading")
	send("visual:profile_frame", 2, 9, "start", 1.0, 2.0, "mesh", 1.5, 2.2, "end", 3.0, 4.0)
	send("gdbg:phase_end", "loading")
	send("servers:profile_frame", 2, 0, 0, 0, 0, 0, 0, 0)
	// Reject the whole frame when the last record is incomplete.
	b, _ := encode(nil, []any{"servers:profile_frame", 1, frame[:len(frame)-1]})
	if c.message(b) == nil {
		t.Fatal("accepted partial frame")
	}
	phases := c.finish()
	p := phases["default"]
	if p.ScriptFrames != 1 || p.ServerFrames != 2 || p.Servers["physics"]["step"] != 1.5 || len(p.Scripts) != 1 {
		t.Fatalf("phase %+v", p)
	}
	if s := p.Scripts[0]; s != (Script{Signature: "res://actor.gd::5::update", Calls: 2, Self: 1, Total: 1.5, Max: 2}) {
		t.Fatal(s)
	}
	v := phases["loading"]
	if v.VisualFrames != 1 || v.Visual["mesh"].CPU != 1.5 || v.ServerFrames != 0 {
		t.Fatal(v)
	}
}
