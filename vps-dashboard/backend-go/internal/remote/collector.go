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
const metricsCommand = `set -e 2>/dev/null; ` +
	// CPU usage: parse top's idle percentage and invert.
	`idle=$(top -bn1 2>/dev/null | awk '/Cpu\(s\)/{print $8}' | head -1); ` +
	`echo "cpu_usage=$(awk -v i="${idle:-100}" 'BEGIN{print 100-i}')"; ` +
	// Load averages.
	`echo "load1=$(cut -d' ' -f1 /proc/loadavg 2>/dev/null || sysctl -n vm.loadavg 2>/dev/null | cut -d' ' -f1)"; ` +
	`echo "load5=$(cut -d' ' -f2 /proc/loadavg 2>/dev/null || sysctl -n vm.loadavg 2>/dev/null | cut -d' ' -f2)"; ` +
	`echo "load15=$(cut -d' ' -f3 /proc/loadavg 2>/dev/null || sysctl -n vm.loadavg 2>/dev/null | cut -d' ' -f3)"; ` +
	// Memory: free -b gives bytes.
	`free -b 2>/dev/null | awk '/Mem:/{print "mem_total="$2; print "mem_used="$3}'; ` +
	`echo "mem_percent=$(free 2>/dev/null | awk '/Mem:/{if($2>0) printf "%.1f", $3/$2*100}')"; ` +
	// Disk: df for root.
	`df -B1 / 2>/dev/null | awk 'NR==2{print "disk_total="$2; print "disk_used="$3; if($2>0) printf "disk_percent=%.1f\n", $3/$2*100}'; ` +
	// Network: aggregate rx/tx bytes across non-lo interfaces.
	`rx=0; tx=0; ` +
	`for f in /proc/net/dev; do ` +
	`  if [ -r "$f" ]; then ` +
	`    while read iface rest; do ` +
	`      case "$iface" in lo:|lo) continue;; esac; ` +
	`      r=$(echo "$rest" | awk '{print $1}'); t=$(echo "$rest" | awk '{print $9}'); ` +
	`      rx=$((rx + r)); tx=$((tx + t)); ` +
	`    done < "$f"; ` +
	`    echo "net_bytes_recv=$rx"; echo "net_bytes_sent=$tx"; ` +
	`  fi; ` +
	`done; ` +
	// Uptime.
	`echo "uptime=$(cut -d' ' -f1 /proc/uptime 2>/dev/null || sysctl -n kern.boottime 2>/dev/null | awk -F'[ ,]' '{print $4}' | xargs expr $(date +%s) - 2>/dev/null)"; `

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
	if parseErr != "" {
		m.Error = parseErr
	}
	return m
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
