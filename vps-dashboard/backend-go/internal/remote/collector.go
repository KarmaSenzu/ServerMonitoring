// Package remote implements agentless remote monitoring: collecting
// CPU, memory, disk, load, network and uptime metrics from registered
// servers over SSH (PROJECT ARCHITECTURE.md §9, §12, Phase 3).
//
// The collector runs a single portable shell command per server that
// emits key=value lines, then parses the output into a ServerMetric.
// This follows Purple's agentless philosophy: no custom agent on the
// remote host, only the standard /proc and /sys interfaces.
package remote

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/ssh"
)

// metricsCommand is a single portable shell script that collects all
// essential system metrics in one SSH round-trip. It uses only
// coreutils/procps utilities available on any Linux/macOS system.
// Output is key=value, one per line; lines without '=' are ignored.
//
// In addition to metrics, it also collects system info (OS, arch,
// hostname) that is used to auto-populate the server registry.
//
// IMPORTANT: This script must NOT use `set -e` because some commands
// may fail on certain systems (e.g., `free` not installed, /proc
// not available on macOS). Each command has its own error handling.
const metricsCommand = `# CPU usage
idle=$(top -bn1 2>/dev/null | awk '/Cpu\(s\)/{print $8}' | head -1)
if [ -z "$idle" ]; then idle=100; fi
echo "cpu_usage=$(awk -v i="$idle" 'BEGIN{print 100-i}')

# Load averages
echo "load1=$(cat /proc/loadavg 2>/dev/null | cut -d' ' -f1 || sysctl -n vm.loadavg 2>/dev/null | awk '{print $1}')"
echo "load5=$(cat /proc/loadavg 2>/dev/null | cut -d' ' -f2 || sysctl -n vm.loadavg 2>/dev/null | awk '{print $2}')"
echo "load15=$(cat /proc/loadavg 2>/dev/null | cut -d' ' -f3 || sysctl -n vm.loadavg 2>/dev/null | awk '{print $3}')"

# Memory
free -b 2>/dev/null | awk '/Mem:/{print "mem_total="$2; print "mem_used="$3}'
echo "mem_percent=$(free 2>/dev/null | awk '/Mem:/{if($2>0) printf "%.1f", $3/$2*100}')"

# Disk
df -B1 / 2>/dev/null | awk 'NR==2{print "disk_total="$2; print "disk_used="$3; if($2>0) printf "disk_percent=%.1f\n", $3/$2*100}'

# Network bytes
rx=0
tx=0
if [ -r /proc/net/dev ]; then
  while read iface rest; do
    case "$iface" in
      lo:|lo) continue;;
    esac
    r=$(echo "$rest" | awk '{print $1}')
    t=$(echo "$rest" | awk '{print $9}')
    if [ -n "$r" ] && [ -n "$t" ]; then
      rx=$((rx + r))
      tx=$((tx + t))
    fi
  done < /proc/net/dev
fi
echo "net_bytes_recv=$rx"
echo "net_bytes_sent=$tx"

# Uptime
echo "uptime=$(cat /proc/uptime 2>/dev/null | cut -d' ' -f1 || echo 0)"

# System info (auto-detected)
echo "os_name=$(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | head -1 | cut -d'"' -f2 || uname -sr 2>/dev/null || echo '')"
echo "architecture=$(uname -m 2>/dev/null || echo '')"
echo "resolved_hostname=$(hostname 2>/dev/null || cat /etc/hostname 2>/dev/null || echo '')"
echo "kernel=$(uname -r 2>/dev/null || echo '')"`

// Collector gathers metrics from a single remote server.
type Collector struct {
	Engine *ssh.Service
}

// NewCollector constructs a Collector bound to the SSH engine.
func NewCollector(engine *ssh.Service) *Collector {
	return &Collector{Engine: engine}
}

// Collect runs the metrics command on the given server and parses the
// output. A failed SSH session still produces a ServerMetric with an
// Error field so the monitoring loop can record the failure.
//
// It also captures system info (OS, architecture, hostname) that the
// engine can use to auto-populate the server registry.
func (c *Collector) Collect(ctx context.Context, server models.Server) models.ServerMetric {
	now := time.Now().UTC()
	if c.Engine == nil {
		return models.ServerMetric{
			ServerID:  server.ID,
			Timestamp: now,
			Error:     "ssh engine not configured",
		}
	}
	result, err := c.Engine.RunCommand(ctx, server, metricsCommand)
	if err != nil {
		return models.ServerMetric{
			ServerID:  server.ID,
			Timestamp: now,
			Error:     err.Error(),
		}
	}
	// Non-zero exit code but we still got partial output: parse what
	// we have and record the error.
	parseErr := ""
	if result.Stderr != "" && result.ExitCode != 0 {
		parseErr = fmt.Sprintf("exit %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}

	m := parseMetrics(result.Stdout)
	m.ServerID = server.ID
	m.Timestamp = now
	m.RawStdout = result.Stdout
	if parseErr != "" {
		m.Error = parseErr
	}
	return m
}

// SystemInfo holds auto-detected server metadata collected via SSH.
// This is populated from the same SSH command as metrics, so no
// extra round-trip is needed.
type SystemInfo struct {
	OperatingSystem string
	Architecture    string
	Hostname        string
	Kernel          string
}

// ParseSystemInfo extracts system info from the metrics command output.
// Called by the engine to auto-populate the server registry.
func ParseSystemInfo(stdout string) SystemInfo {
	kv := make(map[string]string, 4)
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		kv[k] = v
	}
	return SystemInfo{
		OperatingSystem: kv["os_name"],
		Architecture:    kv["architecture"],
		Hostname:        kv["resolved_hostname"],
		Kernel:          kv["kernel"],
	}
}

// parseMetrics turns key=value lines into a ServerMetric.
func parseMetrics(stdout string) models.ServerMetric {
	kv := make(map[string]string, 16)
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		kv[k] = v
	}
	return models.ServerMetric{
		CPUUsage:     parseFloat(kv, "cpu_usage"),
		CPULoad1:     parseFloat(kv, "load1"),
		CPULoad5:     parseFloat(kv, "load5"),
		CPULoad15:    parseFloat(kv, "load15"),
		MemTotal:     parseFloat(kv, "mem_total"),
		MemUsed:      parseFloat(kv, "mem_used"),
		MemPercent:   parseFloat(kv, "mem_percent"),
		DiskTotal:    parseFloat(kv, "disk_total"),
		DiskUsed:     parseFloat(kv, "disk_used"),
		DiskPercent:  parseFloat(kv, "disk_percent"),
		NetBytesSent: parseFloat(kv, "net_bytes_sent"),
		NetBytesRecv: parseFloat(kv, "net_bytes_recv"),
		Uptime:       parseFloat(kv, "uptime"),
	}
}

// ParseMetrics is the exported wrapper for testing.
func ParseMetrics(stdout string) models.ServerMetric {
	return parseMetrics(stdout)
}

// parseFloat extracts a float64 from the map, defaulting to 0.
func parseFloat(m map[string]string, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}
