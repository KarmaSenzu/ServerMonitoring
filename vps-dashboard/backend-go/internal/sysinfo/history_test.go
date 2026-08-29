package sysinfo

import (
	"context"
	"testing"
	"time"
)

// fakeCapture builds a captureFn that returns deterministic samples
// based on the supplied sequence of "now" timestamps.
func fakeCapture() func(ctx context.Context, now time.Time) (Sample, Sample, Sample, NetSample, error) {
	return func(_ context.Context, now time.Time) (Sample, Sample, Sample, NetSample, error) {
		return Sample{Timestamp: now, UsagePercent: 12.5, Load1: 0.5},
			Sample{Timestamp: now, UsedPercent: 30.0, UsedBytes: 1024},
			Sample{Timestamp: now, UsedPercent: 40.0, UsedBytes: 2048},
			NetSample{Timestamp: now, PerInterface: []NetIO{{Name: "eth0", BytesSent: 1, BytesRecv: 2}}},
			nil
	}
}

func TestRecorderTickAppendsAllRings(t *testing.T) {
	r := NewRecorder(5)
	r.captureFn = fakeCapture()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	r.nowFn = func() time.Time {
		t := base.Add(time.Duration(calls) * time.Minute)
		calls++
		return t
	}

	for i := 0; i < 3; i++ {
		r.Tick(context.Background())
	}
	cpu, mem, disk, net := r.Len()
	if cpu != 3 || mem != 3 || disk != 3 || net != 3 {
		t.Fatalf("rings: cpu=%d mem=%d disk=%d net=%d, want 3 each", cpu, mem, disk, net)
	}
}

func TestRecorderTickEvictsOldest(t *testing.T) {
	r := NewRecorder(2)
	r.captureFn = fakeCapture()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	r.nowFn = func() time.Time {
		t := base.Add(time.Duration(calls) * time.Minute)
		calls++
		return t
	}

	for i := 0; i < 4; i++ {
		r.Tick(context.Background())
	}

	cpu, mem, disk, net := r.Len()
	if cpu != 2 || mem != 2 || disk != 2 || net != 2 {
		t.Fatalf("rings: cpu=%d mem=%d disk=%d net=%d, want 2 each", cpu, mem, disk, net)
	}

	// Snapshot with a generous window should return both retained samples
	// in chronological order. The first two should have been evicted.
	r.nowFn = func() time.Time { return base.Add(10 * time.Minute) }
	snap := r.Snapshot(0)
	if len(snap.CPU) != 2 {
		t.Fatalf("snap.CPU: got %d want 2", len(snap.CPU))
	}
	wantFirst := base.Add(2 * time.Minute)
	if !snap.CPU[0].Timestamp.Equal(wantFirst) {
		t.Errorf("oldest retained: got %s want %s", snap.CPU[0].Timestamp, wantFirst)
	}
}

func TestRecorderSnapshotWindowing(t *testing.T) {
	r := NewRecorder(10)
	r.captureFn = fakeCapture()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	r.nowFn = func() time.Time {
		t := base.Add(time.Duration(calls) * time.Minute)
		calls++
		return t
	}

	// 6 ticks, one per minute, ending at base + 5m.
	for i := 0; i < 6; i++ {
		r.Tick(context.Background())
	}

	// Pin "now" to base + 5m for the snapshot.
	r.nowFn = func() time.Time { return base.Add(5 * time.Minute) }

	snap := r.Snapshot(2 * time.Minute)
	if len(snap.CPU) != 3 {
		t.Fatalf("expected 3 samples in 2m window, got %d", len(snap.CPU))
	}
	if !snap.CPU[0].Timestamp.Equal(base.Add(3 * time.Minute)) {
		t.Errorf("window first: got %s want %s", snap.CPU[0].Timestamp, base.Add(3*time.Minute))
	}
}

func TestRecorderHasSamples(t *testing.T) {
	r := NewRecorder(3)
	r.captureFn = fakeCapture()
	if r.HasSamples() {
		t.Fatal("expected no samples on a fresh recorder")
	}
	r.Tick(context.Background())
	if !r.HasSamples() {
		t.Fatal("expected HasSamples after a tick")
	}
}
