package remoteprof

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

const (
	maxPhases     = 256
	maxStackDepth = 64
	maxSignatures = 65536
)

type Script struct {
	Signature string  `json:"signature"`
	Calls     float64 `json:"calls_per_frame"`
	Self      float64 `json:"self_ms_mean"`
	Total     float64 `json:"total_ms_mean"`
	Max       float64 `json:"self_ms_max"`
}
type Region struct {
	CPU float64 `json:"cpu_ms_mean"`
	GPU float64 `json:"gpu_ms_mean"`
}
type Phase struct {
	ScriptFrames int                           `json:"script_frames"`
	ServerFrames int                           `json:"server_frames"`
	VisualFrames int                           `json:"visual_frames"`
	Scripts      []Script                      `json:"scripts"`
	Servers      map[string]map[string]float64 `json:"servers"`
	Visual       map[string]Region             `json:"visual"`
	scripts      map[int64]Script
}
type collector struct {
	phases     map[string]*Phase
	stack      []string
	signatures map[int64]string
}

func newCollector() *collector {
	return &collector{phases: map[string]*Phase{}, signatures: map[int64]string{}}
}
func (c *collector) phase(name string) (*Phase, error) {
	p := c.phases[name]
	if p == nil {
		if len(c.phases) >= maxPhases {
			return nil, errors.New("remote profiler phase limit exceeded")
		}
		p = &Phase{scripts: map[int64]Script{}, Scripts: []Script{}, Servers: map[string]map[string]float64{}, Visual: map[string]Region{}}
		c.phases[name] = p
	}
	return p, nil
}

// The official frame payloads are flat arrays. Counts inside a server or script
// section specify the number of Variant slots, not the number of records.
type fields struct {
	v   []any
	err error
}

func (f *fields) next() any {
	if len(f.v) == 0 {
		f.err = errors.New("short debugger payload")
		return nil
	}
	v := f.v[0]
	f.v = f.v[1:]
	return v
}
func (f *fields) integer() int64 {
	v, ok := f.next().(int64)
	if !ok || v < 0 {
		f.err = errors.New("expected nonnegative integer")
	}
	return v
}
func (f *fields) text() string {
	switch v := f.next().(type) {
	case string:
		return v
	case stringName:
		return string(v)
	}
	f.err = errors.New("expected string")
	return ""
}
func (f *fields) number() float64 {
	switch v := f.next().(type) {
	case float64:
		if !math.IsInf(v, 0) && !math.IsNaN(v) {
			return v
		}
	case int64:
		return float64(v)
	}
	f.err = errors.New("expected finite number")
	return 0
}
func (f *fields) count(width int64) int64 {
	n := f.integer()
	if n > int64(len(f.v)) || n%width != 0 {
		f.err = errors.New("invalid section length")
		return 0
	}
	return n / width
}
func (f *fields) done() error {
	if f.err != nil {
		return f.err
	}
	if len(f.v) != 0 {
		return errors.New("trailing debugger fields")
	}
	return nil
}

func (c *collector) message(b []byte) error {
	d := decoder{data: b}
	if d.word() != kindArray || d.word() != 3 {
		return errors.New("expected debugger envelope")
	}
	name, ok := d.value(0).(string)
	if !ok {
		return errors.New("missing debugger message name")
	}
	switch name {
	case "servers:function_signature", "gdbg:phase_start", "gdbg:phase_end", "servers:profile_frame", "visual:profile_frame":
	default:
		return nil
	}
	if _, ok := d.value(0).(int64); !ok {
		return errors.New("missing debugger thread id")
	}
	args, ok := d.value(0).([]any)
	if d.err != nil {
		return d.err
	}
	if !ok || len(d.data) != 0 {
		return errors.New("invalid debugger arguments")
	}
	f := fields{v: args}
	switch name {
	case "servers:function_signature":
		s := f.text()
		id := f.integer()
		if err := f.done(); err != nil {
			return err
		}
		if _, exists := c.signatures[id]; !exists && len(c.signatures) >= maxSignatures {
			return errors.New("remote profiler signature limit exceeded")
		}
		c.signatures[id] = s
	case "gdbg:phase_start", "gdbg:phase_end":
		s := f.text()
		if err := f.done(); err != nil {
			return err
		}
		if name == "gdbg:phase_start" {
			if len(c.stack) >= maxStackDepth {
				return errors.New("remote profiler phase stack limit exceeded")
			}
			if _, err := c.phase(s); err != nil {
				return err
			}
			c.stack = append(c.stack, s)
		} else if n := len(c.stack); n > 0 && c.stack[n-1] == s {
			c.stack = c.stack[:n-1]
		}
	case "servers:profile_frame", "visual:profile_frame":
		phaseName := "default"
		if n := len(c.stack); n > 0 {
			phaseName = c.stack[n-1]
		}
		p, err := c.phase(phaseName)
		if err != nil {
			return err
		}
		if name == "servers:profile_frame" {
			return p.serverFrame(&f, c.signatures)
		}
		return p.visualFrame(&f)
	}
	return nil
}

func (p *Phase) serverFrame(f *fields, signatures map[int64]string) error {
	f.integer() // frame number
	for range 5 {
		f.number()
	} // frame, process, physics, physics frame and script seconds
	servers := map[string]map[string]float64{}
	for n := f.count(1); n > 0 && f.err == nil; n-- {
		srv := f.text()
		times := servers[srv]
		if times == nil {
			times = map[string]float64{}
			servers[srv] = times
		}
		for m := f.count(2); m > 0; m-- {
			key := f.text()
			times[key] += 1000 * f.number()
		}
	}
	scripts := map[int64]Script{}
	// A frame can introduce an ID before its signature message arrives.
	newIDs := 0
	for n := f.count(5); n > 0; n-- {
		id := f.integer()
		s, seen := scripts[id]
		if _, known := signatures[id]; !known && !seen {
			newIDs++
			if len(signatures)+newIDs > maxSignatures {
				return errors.New("remote profiler signature limit exceeded")
			}
		}
		s.Calls += float64(f.integer())
		s.Self += 1000 * f.number()
		s.Total += 1000 * f.number()
		f.number()
		scripts[id] = s
	}
	if err := f.done(); err != nil {
		return err
	}
	p.ServerFrames++
	if len(scripts) > 0 {
		p.ScriptFrames++
	}
	for srv, times := range servers {
		if p.Servers[srv] == nil {
			p.Servers[srv] = map[string]float64{}
		}
		for k, v := range times {
			p.Servers[srv][k] += v
		}
	}
	for id, s := range scripts {
		if _, known := signatures[id]; !known {
			signatures[id] = ""
		}
		sum := p.scripts[id]
		sum.Calls += s.Calls
		sum.Self += s.Self
		sum.Total += s.Total
		sum.Max = max(sum.Max, s.Self)
		p.scripts[id] = sum
	}
	return nil
}
func (p *Phase) visualFrame(f *fields) error {
	f.integer()
	areas := map[string]Region{}
	var prev Region
	var previous string
	for n := f.count(3); n > 0; n-- {
		name := f.text()
		point := Region{f.number(), f.number()}
		if previous != "" {
			r := areas[previous]
			r.CPU += point.CPU - prev.CPU
			r.GPU += point.GPU - prev.GPU
			areas[previous] = r
		}
		prev = point
		previous = name
	}
	if err := f.done(); err != nil {
		return err
	}
	p.VisualFrames++
	for name, r := range areas {
		s := p.Visual[name]
		s.CPU += r.CPU
		s.GPU += r.GPU
		p.Visual[name] = s
	}
	return nil
}

// finish converts the accumulated sums once, after all packets have arrived.
func (c *collector) finish() map[string]*Phase {
	if len(c.phases) == 0 {
		c.phase("default")
	}
	for _, p := range c.phases {
		frames := float64(p.ServerFrames)
		for id, s := range p.scripts {
			s.Signature = c.signatures[id]
			if s.Signature == "" {
				s.Signature = fmt.Sprintf("signature #%d", id)
			}
			s.Calls /= frames
			s.Self /= frames
			s.Total /= frames
			p.Scripts = append(p.Scripts, s)
		}
		sort.Slice(p.Scripts, func(i, j int) bool {
			a, b := p.Scripts[i], p.Scripts[j]
			if a.Self == b.Self {
				return a.Signature < b.Signature
			}
			return a.Self > b.Self
		})
		for _, times := range p.Servers {
			for k := range times {
				times[k] /= frames
			}
		}
		for k, r := range p.Visual {
			r.CPU /= float64(p.VisualFrames)
			r.GPU /= float64(p.VisualFrames)
			p.Visual[k] = r
		}
	}
	return c.phases
}
