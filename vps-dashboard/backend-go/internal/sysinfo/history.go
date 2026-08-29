// Package sysinfo also provides an in-memory ring buffer Recorder that
// captures CPU/memory/disk/network samples on a fixed cadence so the API
// can answer "what did the system look like in the last hour?" without
// hitting persistent storage.
package sysinfo

import (
	"context"
	"sync"
	"time"
)

// Sample is a single timestamped scalar metric. CPU/memory/disk all use
// this shape; network is recorded separately (NetSample) because it has
// per-interface counters.
type Sample struct {
	Timestamp    time.Time `json:"timestamp"`
	UsagePercent float64   `json:"usage_percent,omitempty"`
	Load1        float64   `json:"load1,omitempty"`
	UsedPercent  float64   `json:"used_percent,omitempty"`
	UsedBytes    uint64    `json:"used_bytes,omitempty"`
}

// CPUSample is a CPU-flavoured Sample alias. The struct shape is
// shared with Sample so JSON consumers see the same fields; this alias
// exists so call sites can be self-documenting.
type CPUSample = Sample

// MemSample, DiskSample share the same shape.
type MemSample = Sample

// DiskSample also reuses the Sample shape.
type DiskSample = Sample

// NetSample captures per-interface counters at a single tick.
type NetSample struct {
	Timestamp    time.Time `json:"timestamp"`
	PerInterface []NetIO   `json:"per_interface"`
}

// History is the snapshot of all rings, optionally truncated to a window
// supplied by the caller.
type History struct {
	CPU       []Sample    `json:"cpu"`
	Memory    []Sample    `json:"memory"`
	Disk      []Sample    `json:"disk"`
	Network   []NetSample `json:"network"`
	Capacity  int         `json:"capacity"`
	GeneratedAt time.Time `json:"generated_at"`
}

// Recorder captures and retains recent metric samples. It is safe for
// concurrent Tick + Snapshot calls.
type Recorder struct {
	mu       sync.Mutex
	capacity int
	cpu      []Sample
	memory   []Sample
	disk     []Sample
	network  []NetSample
	// nowFn lets tests inject a deterministic clock. In production it
	// defaults to time.Now.
	nowFn func() time.Time
	// captureFn lets tests inject a fake metrics source. In production
	// it defaults to defaultCapture which calls the real sysinfo.* funcs.
	captureFn func(ctx context.Context, now time.Time) (Sample, Sample, Sample, NetSample, error)
}

// NewRecorder returns a Recorder with the given ring capacity. A
// capacity <= 0 falls back to 1 (no negative or zero rings allowed).
func NewRecorder(capacity int) *Recorder {
	if capacity <= 0 {
		capacity = 1
	}
	return &Recorder{
		capacity:  capacity,
		nowFn:     time.Now,
		captureFn: defaultCapture,
	}
}

// Tick captures one snapshot of CPU/memory/disk/network and appends it
// to each ring. When a ring is full the oldest sample is evicted.
// Errors during capture are logged-via-return-value: the function never
// panics; partial samples are still appended where possible.
func (r *Recorder) Tick(ctx context.Context) {
	now := r.nowFn().UTC()
	cpuS, memS, diskS, netS, _ := r.captureFn(ctx, now)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cpu = appendBounded(r.cpu, cpuS, r.capacity)
	r.memory = appendBounded(r.memory, memS, r.capacity)
	r.disk = appendBounded(r.disk, diskS, r.capacity)
	r.network = appendNetBounded(r.network, netS, r.capacity)
}

// Snapshot returns a copy of the current rings, truncated to entries
// whose Timestamp is within the given window from "now". A window of
// zero or negative is treated as "return everything".
func (r *Recorder) Snapshot(window time.Duration) *History {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := &History{
		Capacity:    r.capacity,
		GeneratedAt: r.nowFn().UTC(),
	}
	cutoff := time.Time{}
	if window > 0 {
		cutoff = out.GeneratedAt.Add(-window)
	}

	out.CPU = filterAfter(r.cpu, cutoff)
	out.Memory = filterAfter(r.memory, cutoff)
	out.Disk = filterAfter(r.disk, cutoff)
	out.Network = filterNetAfter(r.network, cutoff)
	return out
}

// Len returns the count of samples in each ring, useful for tests.
func (r *Recorder) Len() (cpu, mem, disk, net int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cpu), len(r.memory), len(r.disk), len(r.network)
}

// HasSamples reports whether at least one tick has been recorded.
func (r *Recorder) HasSamples() bool {
	cpu, mem, disk, net := r.Len()
	return cpu > 0 || mem > 0 || disk > 0 || net > 0
}

func appendBounded(buf []Sample, s Sample, cap int) []Sample {
	if cap <= 0 {
		return buf
	}
	if len(buf) >= cap {
		// Drop the oldest by shifting in place. This stays O(N) but the
		// expected cap is in the low hundreds, so the cost is negligible
		// vs. the allocation savings of a fixed slice.
		copy(buf, buf[1:])
		buf = buf[:len(buf)-1]
	}
	return append(buf, s)
}

func appendNetBounded(buf []NetSample, s NetSample, cap int) []NetSample {
	if cap <= 0 {
		return buf
	}
	if len(buf) >= cap {
		copy(buf, buf[1:])
		buf = buf[:len(buf)-1]
	}
	return append(buf, s)
}

func filterAfter(in []Sample, cutoff time.Time) []Sample {
	if cutoff.IsZero() {
		out := make([]Sample, len(in))
		copy(out, in)
		return out
	}
	out := make([]Sample, 0, len(in))
	for _, s := range in {
		if s.Timestamp.Equal(cutoff) || s.Timestamp.After(cutoff) {
			out = append(out, s)
		}
	}
	return out
}

func filterNetAfter(in []NetSample, cutoff time.Time) []NetSample {
	if cutoff.IsZero() {
		out := make([]NetSample, len(in))
		copy(out, in)
		return out
	}
	out := make([]NetSample, 0, len(in))
	for _, s := range in {
		if s.Timestamp.Equal(cutoff) || s.Timestamp.After(cutoff) {
			out = append(out, s)
		}
	}
	return out
}

// defaultCapture is the production capture function. It calls the
// real sysinfo collectors and translates their output into ring
// samples. Errors are best-effort: a failed sub-call yields a zeroed
// sample with the captured timestamp.
func defaultCapture(ctx context.Context, now time.Time) (Sample, Sample, Sample, NetSample, error) {
	cpuS := Sample{Timestamp: now}
	memS := Sample{Timestamp: now}
	diskS := Sample{Timestamp: now}
	netS := NetSample{Timestamp: now}

	if v, err := CPU(ctx); err == nil {
		cpuS.UsagePercent = v.UsagePercent
		cpuS.Load1 = v.Load1
	}
	if v, err := Memory(ctx); err == nil {
		memS.UsedPercent = v.UsedPercent
		memS.UsedBytes = v.Used
	}
	if v, err := Disk(ctx, diskRootPath()); err == nil {
		diskS.UsedPercent = v.UsedPercent
		diskS.UsedBytes = v.Used
	}
	if v, err := Network(ctx); err == nil {
		netS.PerInterface = v
	}
	return cpuS, memS, diskS, netS, nil
}
