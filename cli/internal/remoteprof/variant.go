// Package remoteprof reads Godot's length-prefixed binary debugger messages.
package remoteprof

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	kindNil        = 0
	kindBool       = 1
	kindInt        = 2
	kindFloat      = 3
	kindString     = 4
	kindStringName = 21
	kindArray      = 28
	packedByte     = 29
	packedString   = 34
	wide           = 1 << 16
	maxPacket      = 16 << 20
)

var wire = binary.LittleEndian

type stringName string

// Packed values retain their exact element representation (including vector
// components and precision flags). PackedStringArray has variable-width elements.
type packed struct {
	Header  uint32
	Count   uint32
	Data    []byte
	Strings []string
}

func packedWidth(header uint32) int {
	realSize := 4
	if header&wide != 0 {
		realSize = 8
	}
	switch header & 255 {
	case 29:
		return 1
	case 30, 32:
		return 4
	case 31, 33:
		return 8
	case 35:
		return 2 * realSize
	case 36:
		return 3 * realSize
	case 37:
		return 16 // Color always uses four float32 components.
	case 38:
		return 4 * realSize
	}
	return 0
}

func appendBlob(dst, data []byte) []byte {
	dst = append(dst, data...)
	return append(dst, make([]byte, (-len(data))&3)...)
}
func appendText(dst []byte, s string) []byte {
	return appendBlob(wire.AppendUint32(dst, uint32(len(s))), []byte(s))
}
func encode(dst []byte, v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return wire.AppendUint32(dst, kindNil), nil
	case bool:
		var n uint32
		if x {
			n = 1
		}
		return wire.AppendUint32(wire.AppendUint32(dst, kindBool), n), nil
	case int:
		return encode(dst, int64(x))
	case int64:
		if x == int64(int32(x)) {
			return wire.AppendUint32(wire.AppendUint32(dst, kindInt), uint32(x)), nil
		}
		return wire.AppendUint64(wire.AppendUint32(dst, kindInt|wide), uint64(x)), nil
	case float64:
		return wire.AppendUint64(wire.AppendUint32(dst, kindFloat|wide), math.Float64bits(x)), nil
	case string:
		return appendText(wire.AppendUint32(dst, kindString), x), nil
	case stringName:
		return appendText(wire.AppendUint32(dst, kindStringName), string(x)), nil
	case []any:
		dst = wire.AppendUint32(wire.AppendUint32(dst, kindArray), uint32(len(x)))
		for _, elem := range x {
			var err error
			dst, err = encode(dst, elem)
			if err != nil {
				return nil, err
			}
		}
		return dst, nil
	case packed:
		dst = wire.AppendUint32(dst, x.Header)
		if x.Header == packedString {
			dst = wire.AppendUint32(dst, uint32(len(x.Strings)))
			for _, s := range x.Strings {
				dst = appendText(dst, s+"\x00")
			}
			return dst, nil
		}
		width := packedWidth(x.Header)
		if width == 0 || uint64(x.Count)*uint64(width) != uint64(len(x.Data)) {
			return nil, errors.New("invalid packed array")
		}
		return appendBlob(wire.AppendUint32(dst, x.Count), x.Data), nil
	default:
		return nil, fmt.Errorf("unsupported variant %T", v)
	}
}

type decoder struct {
	data []byte
	err  error
}

func (d *decoder) take(n int) []byte {
	if n < 0 || n > len(d.data) {
		d.err = io.ErrUnexpectedEOF
		return nil
	}
	b := d.data[:n]
	d.data = d.data[n:]
	return b
}
func (d *decoder) word() uint32 {
	b := d.take(4)
	if b == nil {
		return 0
	}
	return wire.Uint32(b)
}
func (d *decoder) long() uint64 {
	b := d.take(8)
	if b == nil {
		return 0
	}
	return wire.Uint64(b)
}
func (d *decoder) blob(n int) []byte { b := d.take(n); d.take((-n) & 3); return b }
func (d *decoder) text() string      { return string(d.blob(int(d.word()))) }
func (d *decoder) value(depth int) any {
	if depth > 64 {
		d.err = errors.New("variant nesting limit exceeded")
		return nil
	}
	h := d.word()
	k := h & 255
	switch k {
	case kindNil:
		return nil
	case kindBool:
		return d.word() != 0
	case kindInt:
		if h&wide != 0 {
			return int64(d.long())
		}
		return int64(int32(d.word()))
	case kindFloat:
		if h&wide != 0 {
			return math.Float64frombits(d.long())
		}
		return float64(math.Float32frombits(d.word()))
	case kindString:
		return d.text()
	case kindStringName:
		return stringName(d.text())
	case kindArray, packedString:
		if h != k {
			d.err = fmt.Errorf("unsupported array flags %#x", h)
			return nil
		}
		n := int(d.word())
		if n < 0 || n > len(d.data)/4 {
			d.err = io.ErrUnexpectedEOF
			return nil
		}
		if k == packedString {
			p := packed{Header: h, Count: uint32(n), Strings: make([]string, 0, n)}
			for i := 0; i < n && d.err == nil; i++ {
				p.Strings = append(p.Strings, strings.TrimSuffix(d.text(), "\x00"))
			}
			return p
		}
		a := make([]any, 0, n)
		for i := 0; i < n && d.err == nil; i++ {
			a = append(a, d.value(depth+1))
		}
		return a
	default:
		w := packedWidth(h)
		if w == 0 {
			d.err = fmt.Errorf("unsupported variant kind %d", k)
			return nil
		}
		count := d.word()
		if uint64(count)*uint64(w) > uint64(len(d.data)) {
			d.err = io.ErrUnexpectedEOF
			return nil
		}
		return packed{Header: h, Count: count, Data: d.blob(int(count) * w)}
	}
}

func writePacket(w io.Writer, message []any) error {
	b, err := encode(make([]byte, 4), message)
	if err != nil {
		return err
	}
	wire.PutUint32(b, uint32(len(b)-4))
	_, err = w.Write(b)
	return err
}
func readPacket(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := wire.Uint32(hdr[:])
	if n == 0 || n > maxPacket {
		return nil, fmt.Errorf("invalid debugger packet size %d", n)
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return b, err
}
