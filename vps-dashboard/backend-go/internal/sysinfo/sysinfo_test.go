package sysinfo

import (
	"context"
	"testing"
	"time"
)

func TestSnapshotBasic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snap, err := Capture(ctx)
	if err != nil {
		t.Skipf("Snapshot unavailable in this environment: %v", err)
	}

	if snap.CPU.Cores <= 0 {
		t.Errorf("CPU.Cores: got %d, want > 0", snap.CPU.Cores)
	}
	if snap.Memory.Total == 0 {
		t.Errorf("Memory.Total: got 0, want > 0")
	}
	if snap.Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}
}

// TestParseNetDev verifies the /proc/net/dev parser used by the network
// fallback path. The sample below is a trimmed real /proc/net/dev with
// loopback + a single ethernet interface; we expect the parser to return
// both names and pull the correct receive/transmit byte/packet counts
// from the documented column positions.
func TestParseNetDev(t *testing.T) {
	const sample = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  100      2    0    0    0     0          0         0      100       2    0    0    0     0       0          0
  eth0: 214457818 166221 0    0    0     0          0         0      2590250  45142 0    0    0     0       0          0
`

	stats, err := parseNetDev([]byte(sample))
	if err != nil {
		t.Fatalf("parseNetDev: unexpected error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("parseNetDev: got %d entries, want 2", len(stats))
	}

	byName := map[string]int{}
	for i, s := range stats {
		byName[s.Name] = i
	}
	loIdx, ok := byName["lo"]
	if !ok {
		t.Fatalf("parseNetDev: missing lo")
	}
	ethIdx, ok := byName["eth0"]
	if !ok {
		t.Fatalf("parseNetDev: missing eth0")
	}

	if got, want := stats[loIdx].BytesRecv, uint64(100); got != want {
		t.Errorf("lo.BytesRecv: got %d, want %d", got, want)
	}
	if got, want := stats[ethIdx].BytesRecv, uint64(214457818); got != want {
		t.Errorf("eth0.BytesRecv: got %d, want %d", got, want)
	}
	if got, want := stats[ethIdx].BytesSent, uint64(2590250); got != want {
		t.Errorf("eth0.BytesSent: got %d, want %d", got, want)
	}
	if got, want := stats[ethIdx].PacketsRecv, uint64(166221); got != want {
		t.Errorf("eth0.PacketsRecv: got %d, want %d", got, want)
	}
	if got, want := stats[ethIdx].PacketsSent, uint64(45142); got != want {
		t.Errorf("eth0.PacketsSent: got %d, want %d", got, want)
	}
}

// TestParseNetDevMalformed exercises the defensive paths in parseNetDev:
// too few lines, missing colon, and short field counts must all be
// rejected or skipped without panicking.
func TestParseNetDevMalformed(t *testing.T) {
	cases := map[string]string{
		"too few lines":     "Inter-| Receive\n",
		"only headers":      "Inter-| Receive\n face | bytes\n",
		"line without colon": "Inter-| Receive\n face | bytes\nthis is not a valid row\n",
		"short fields":      "Inter-| Receive\n face | bytes\n  eth0: 1 2 3\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			stats, err := parseNetDev([]byte(input))
			// "too few lines" is expected to error; the others should
			// return without error but produce no usable entries.
			if name == "too few lines" {
				if err == nil {
					t.Fatalf("expected error for %q, got %d stats", name, len(stats))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", name, err)
			}
			if len(stats) != 0 {
				t.Errorf("expected 0 stats for %q, got %d", name, len(stats))
			}
		})
	}
}

