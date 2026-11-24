package remoteprof

import (
	"bytes"
	"encoding/hex"
	"io"
	"math"
	"reflect"
	"testing"
)

func TestCapturedEnvelope(t *testing.T) {
	// A set_pid envelope from the 4.7.1 loopback probe with PID replaced by 42.
	packet, err := hex.DecodeString("300000001c0000000300000004000000070000007365745f7069640002000000010000001c00000001000000020000002a000000")
	if err != nil {
		t.Fatal(err)
	}
	b, err := readPacket(bytes.NewReader(packet))
	if err != nil {
		t.Fatal(err)
	}
	d := decoder{data: b}
	want := []any{"set_pid", int64(1), []any{int64(42)}}
	if got := d.value(0); !reflect.DeepEqual(got, want) || d.err != nil || len(d.data) != 0 {
		t.Fatalf("got %v (%v)", got, d.err)
	}
	var buf bytes.Buffer
	if err := writePacket(&buf, want); err != nil || !bytes.Equal(buf.Bytes(), packet) {
		t.Fatalf("encoded %x (%v)", buf.Bytes(), err)
	}
}
func TestVariants(t *testing.T) {
	values := []any{nil, false, true, int64(-27), int64(1) << 48, 3.5, stringName("area"), "区域", []any{int64(3), nil}, packed{Header: packedString, Count: 2, Strings: []string{"", "test"}}}
	for _, kind := range []uint32{29, 30, 31, 32, 33, 35, 36, 37, 38, 36 | wide} {
		values = append(values, packed{Header: kind, Count: 3, Data: bytes.Repeat([]byte{7}, 3*packedWidth(kind))})
	}
	for _, v := range values {
		b, err := encode(nil, v)
		if err != nil {
			t.Fatal(err)
		}
		d := decoder{data: b}
		if got := d.value(0); !reflect.DeepEqual(got, v) || d.err != nil || len(d.data) != 0 {
			t.Fatalf("round trip %v: got %v, %v", v, got, d.err)
		}
	}
	d := decoder{data: wire.AppendUint32(wire.AppendUint32(nil, kindFloat), math.Float32bits(1.25))}
	if got := d.value(0); got != 1.25 {
		t.Fatal(got)
	}
}
func TestTruncatedData(t *testing.T) {
	b, _ := encode(nil, []any{"metric", int64(1) << 40})
	for i := range len(b) {
		d := decoder{data: b[:i]}
		d.value(0)
		if d.err == nil {
			t.Fatalf("accepted truncation at %d", i)
		}
	}
	_, err := readPacket(bytes.NewReader([]byte{10, 0, 0, 0, 1}))
	if err != io.ErrUnexpectedEOF {
		t.Fatal(err)
	}
}
