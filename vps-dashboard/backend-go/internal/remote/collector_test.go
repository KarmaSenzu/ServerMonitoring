package remote_test

import (
	"context"
	"testing"

	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/remote"
)

func TestParseMetrics(t *testing.T) {
	output := `cpu_usage=42.5
load1=0.52
load5=0.48
load15=0.50
mem_total=8589934592
mem_used=4294967296
mem_percent=50.0
disk_total=10737418240
disk_used=8053063680
disk_percent=75.0
net_bytes_sent=1024000
net_bytes_recv=2048000
uptime=3600
`
	m := remote.ParseMetrics(output)

	if m.CPUUsage != 42.5 {
		t.Errorf("cpu_usage: %f", m.CPUUsage)
	}
	if m.CPULoad1 != 0.52 {
		t.Errorf("load1: %f", m.CPULoad1)
	}
	if m.MemTotal != 8589934592 {
		t.Errorf("mem_total: %f", m.MemTotal)
	}
	if m.MemPercent != 50.0 {
		t.Errorf("mem_percent: %f", m.MemPercent)
	}
	if m.DiskPercent != 75.0 {
		t.Errorf("disk_percent: %f", m.DiskPercent)
	}
	if m.NetBytesSent != 1024000 {
		t.Errorf("net_bytes_sent: %f", m.NetBytesSent)
	}
	if m.Uptime != 3600 {
		t.Errorf("uptime: %f", m.Uptime)
	}
}

func TestParseMetricsPartial(t *testing.T) {
	output := `cpu_usage=99.9
mem_total=1000000
`
	m := remote.ParseMetrics(output)

	if m.CPUUsage != 99.9 {
		t.Errorf("cpu_usage: %f", m.CPUUsage)
	}
	if m.MemTotal != 1000000 {
		t.Errorf("mem_total: %f", m.MemTotal)
	}
	if m.DiskPercent != 0 {
		t.Errorf("disk_percent should default to 0: %f", m.DiskPercent)
	}
}

func TestParseMetricsEmpty(t *testing.T) {
	m := remote.ParseMetrics("")
	if m.CPUUsage != 0 || m.MemTotal != 0 {
		t.Errorf("expected all zeros for empty output")
	}
}

func TestParseMetricsGarbage(t *testing.T) {
	output := `garbage line
cpu_usage=abc
mem_total=1000
not_an_assignment
`
	m := remote.ParseMetrics(output)
	if m.MemTotal != 1000 {
		t.Errorf("mem_total: %f", m.MemTotal)
	}
	if m.CPUUsage != 0 {
		t.Errorf("cpu_usage should be 0 for non-numeric: %f", m.CPUUsage)
	}
}

// TestCollectorErrorOnNilEngine verifies that collecting with no SSH
// engine produces a metric with an error field rather than a panic.
func TestCollectorErrorOnNilEngine(t *testing.T) {
	c := remote.NewCollector(nil)

	// Defer a recover so a panic becomes a failure, not a crash.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Collect panicked: %v", r)
		}
	}()

	m := c.Collect(context.Background(), models.Server{
		ID:           "srv-1",
		Name:         "test",
		Hostname:     "127.0.0.1",
		SSHPort:      22,
		SSHUsername:  "deploy",
		CredentialType: models.ServerCredentialSSHKey,
		CredentialRef:  "missing",
	})

	if m.Error == "" {
		t.Error("expected error for nil engine")
	}
	if m.ServerID != "srv-1" {
		t.Errorf("server_id: %q", m.ServerID)
	}
}
