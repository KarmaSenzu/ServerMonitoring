// Package sysinfo collects system metrics (CPU, memory, disk, host, network)
// using gopsutil. It exposes small, JSON-friendly structs and a Snapshot
// aggregator suitable for HTTP responses.
package sysinfo

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
)

// CPUInfo describes CPU configuration and current load.
type CPUInfo struct {
	Cores        int     `json:"cores"`
	Model        string  `json:"model"`
	UsagePercent float64 `json:"usagePercent"`
	Load1        float64 `json:"load1"`
	Load5        float64 `json:"load5"`
	Load15       float64 `json:"load15"`
}

// MemoryInfo describes virtual memory usage.
type MemoryInfo struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"usedPercent"`
}

// DiskInfo describes filesystem usage for a single path.
type DiskInfo struct {
	Path        string  `json:"path"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"usedPercent"`
}

// HostInfo describes basic host identity and uptime.
type HostInfo struct {
	Hostname        string `json:"hostname"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
	KernelVersion   string `json:"kernelVersion"`
	KernelArch      string `json:"kernelArch"`
	Uptime          uint64 `json:"uptime"`
	BootTime        uint64 `json:"bootTime"`
}

// NetIO describes per-interface network counters.
type NetIO struct {
	Name        string `json:"name"`
	BytesSent   uint64 `json:"bytesSent"`
	BytesRecv   uint64 `json:"bytesRecv"`
	PacketsSent uint64 `json:"packetsSent"`
	PacketsRecv uint64 `json:"packetsRecv"`
	ErrIn       uint64 `json:"errIn"`
	ErrOut      uint64 `json:"errOut"`
}

// Snapshot is a one-shot aggregate of all metrics at a moment in time.
//
// Errors holds any per-metric failures Capture tolerated while building the
// snapshot, formatted as "<metric>: <error>". When empty (the happy path)
// the field is omitted from JSON via omitempty so the wire format stays
// quiet for normal responses.
type Snapshot struct {
	CPU       CPUInfo    `json:"cpu"`
	Memory    MemoryInfo `json:"memory"`
	Disk      DiskInfo   `json:"disk"`
	Host      HostInfo   `json:"host"`
	Network   []NetIO    `json:"network"`
	Timestamp time.Time  `json:"timestamp"`
	Errors    []string   `json:"errors,omitempty"`
}

// CPU returns a populated CPUInfo. Errors fetching individual fields are
// surfaced; load averages on platforms that do not support them (Windows)
// degrade to zeros instead of failing.
func CPU(ctx context.Context) (CPUInfo, error) {
	out := CPUInfo{}

	cores, err := cpu.Counts(true)
	if err != nil {
		return out, fmt.Errorf("sysinfo: cpu count: %w", err)
	}
	out.Cores = cores

	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		out.Model = infos[0].ModelName
	}

	pct, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return out, fmt.Errorf("sysinfo: cpu percent: %w", err)
	}
	if len(pct) > 0 {
		out.UsagePercent = pct[0]
	} else if perCore, err := cpu.PercentWithContext(ctx, 0, true); err == nil && len(perCore) > 0 {
		var sum float64
		for _, v := range perCore {
			sum += v
		}
		out.UsagePercent = sum / float64(len(perCore))
	}

	if avg, err := load.Avg(); err == nil && avg != nil {
		out.Load1 = avg.Load1
		out.Load5 = avg.Load5
		out.Load15 = avg.Load15
	}

	return out, nil
}

// Memory returns virtual memory statistics.
func Memory(ctx context.Context) (MemoryInfo, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return MemoryInfo{}, fmt.Errorf("sysinfo: virtual memory: %w", err)
	}
	return MemoryInfo{
		Total:       vm.Total,
		Used:        vm.Used,
		Free:        vm.Free,
		Available:   vm.Available,
		UsedPercent: vm.UsedPercent,
	}, nil
}

// Disk returns filesystem usage for path. An empty path defaults to "/".
//
// When the API runs inside a container with the host root bind-mounted at
// "/host" (see SYSTEM_ROOT_PATH / diskRootPath), gopsutil reports usage for
// that bind mount but its u.Path field is the container-side path
// ("/host", "/host/var", ...). For the JSON response we strip the "/host"
// prefix so consumers always see logical host paths ("/", "/var", ...).
func Disk(ctx context.Context, path string) (DiskInfo, error) {
	if path == "" {
		path = "/"
	}
	u, err := disk.UsageWithContext(ctx, path)
	if err != nil {
		return DiskInfo{}, fmt.Errorf("sysinfo: disk usage %q: %w", path, err)
	}
	return DiskInfo{
		Path:        stripHostPrefix(u.Path),
		Total:       u.Total,
		Used:        u.Used,
		Free:        u.Free,
		UsedPercent: u.UsedPercent,
	}, nil
}

// diskRootPath returns the path used for the primary disk usage report.
// Honors SYSTEM_ROOT_PATH (e.g. "/host" when running in a container with
// the host root bind-mounted there). Falls back to "/" when the env var
// is unset, empty, or points at a path that does not exist on the
// running filesystem.
func diskRootPath() string {
	if p := os.Getenv("SYSTEM_ROOT_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/"
}

// stripHostPrefix removes a leading "/host" segment from p so JSON
// responses report the logical host path rather than the container-side
// bind mount. "/host" itself collapses to "/".
func stripHostPrefix(p string) string {
	if p == "/host" {
		return "/"
	}
	if strings.HasPrefix(p, "/host/") {
		return strings.TrimPrefix(p, "/host")
	}
	return p
}

// Host returns identity and uptime information about the machine.
func Host(ctx context.Context) (HostInfo, error) {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return HostInfo{}, fmt.Errorf("sysinfo: host info: %w", err)
	}
	return HostInfo{
		Hostname:        info.Hostname,
		Platform:        info.Platform,
		PlatformVersion: info.PlatformVersion,
		KernelVersion:   info.KernelVersion,
		KernelArch:      info.KernelArch,
		Uptime:          info.Uptime,
		BootTime:        info.BootTime,
	}, nil
}

// Network returns per-interface counters, excluding the loopback ("lo")
// and any interface with all-zero counters.
//
// gopsutil reads /proc/net/dev (or $HOST_PROC/net/dev) directly. Inside a
// container with /host/proc bind-mounted, the "net" subdirectory is a
// symlink that resolves to "self/net" relative to the caller's PID
// namespace, which points at the container's network namespace rather
// than the host's. When that read fails we fall back to parsing
// /host/proc/1/net/dev manually so the host's interfaces are still
// reported.
func Network(ctx context.Context) ([]NetIO, error) {
	stats, err := psnet.IOCountersWithContext(ctx, true)
	if err != nil {
		fallback, ferr := readNetDevFallback()
		if ferr != nil || len(fallback) == 0 {
			return nil, fmt.Errorf("sysinfo: net counters: %w", err)
		}
		stats = fallback
	}
	out := make([]NetIO, 0, len(stats))
	for _, s := range stats {
		if s.Name == "lo" {
			continue
		}
		if s.BytesSent == 0 && s.BytesRecv == 0 &&
			s.PacketsSent == 0 && s.PacketsRecv == 0 &&
			s.Errin == 0 && s.Errout == 0 {
			continue
		}
		out = append(out, NetIO{
			Name:        s.Name,
			BytesSent:   s.BytesSent,
			BytesRecv:   s.BytesRecv,
			PacketsSent: s.PacketsSent,
			PacketsRecv: s.PacketsRecv,
			ErrIn:       s.Errin,
			ErrOut:      s.Errout,
		})
	}
	return out, nil
}

// readNetDevFallback parses a /proc/net/dev-formatted file from a list of
// candidate paths, returning counters from the first readable one. Used
// when gopsutil cannot resolve /proc/net/dev (typically due to the
// /host/proc/net symlink resolving inside the caller's PID namespace).
func readNetDevFallback() ([]psnet.IOCountersStat, error) {
	paths := []string{
		"/host/proc/1/net/dev", // host PID 1 net namespace (most accurate)
		"/host/proc/net/dev",   // direct (may work if symlink resolves)
		"/proc/net/dev",        // container's own (last resort)
	}
	var lastErr error
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}
		stats, perr := parseNetDev(data)
		if perr != nil {
			lastErr = perr
			continue
		}
		if len(stats) > 0 {
			return stats, nil
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no readable net/dev found")
}

// parseNetDev parses /proc/net/dev format. The file looks like:
//
//	Inter-|   Receive                                                |  Transmit
//	 face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed
//	  eth0:  1234   56    0    0    0    0       0          0      9876   54   0    0    0    0    0       0
//
// Field order after the colon (16 numeric columns):
//
//	0=bytes_recv 1=packets_recv 2=errs_in   3=drop_in
//	4=fifo_in    5=frame        6=compress  7=multicast
//	8=bytes_sent 9=packets_sent 10=errs_out 11=drop_out
//	12=fifo_out  13=colls       14=carrier  15=compress_out
func parseNetDev(data []byte) ([]psnet.IOCountersStat, error) {
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		return nil, fmt.Errorf("malformed net/dev: %d lines", len(lines))
	}
	out := make([]psnet.IOCountersStat, 0, len(lines)-2)
	for _, line := range lines[2:] { // skip the two header lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		fields := strings.Fields(line[colon+1:])
		if len(fields) < 16 {
			continue
		}
		bytesRecv, _ := strconv.ParseUint(fields[0], 10, 64)
		packetsRecv, _ := strconv.ParseUint(fields[1], 10, 64)
		errsIn, _ := strconv.ParseUint(fields[2], 10, 64)
		bytesSent, _ := strconv.ParseUint(fields[8], 10, 64)
		packetsSent, _ := strconv.ParseUint(fields[9], 10, 64)
		errsOut, _ := strconv.ParseUint(fields[10], 10, 64)
		out = append(out, psnet.IOCountersStat{
			Name:        name,
			BytesRecv:   bytesRecv,
			PacketsRecv: packetsRecv,
			Errin:       errsIn,
			BytesSent:   bytesSent,
			PacketsSent: packetsSent,
			Errout:      errsOut,
		})
	}
	return out, nil
}

// Capture fetches all metrics in parallel and returns them as a single
// Snapshot. Per-metric failures are tolerated: if a sub-call returns an
// error (e.g. /proc/net/dev unreadable inside a restricted container),
// the affected field is left at its zero value, the error is appended to
// Snapshot.Errors as "<metric>: <error>", and a warning is logged. This
// keeps /system/stats useful even when one collector is broken.
//
// Capture itself only returns a non-nil error when the supplied context is
// cancelled or its deadline is exceeded; in that case the partial
// Snapshot is still returned so the caller can inspect what was gathered
// before cancellation.
//
// Note: the spec asked for a function named Snapshot, but Go forbids a
// type and function sharing a name in the same package, so the function
// is exposed as Capture and the data type stays Snapshot.
func Capture(ctx context.Context) (Snapshot, error) {
	var (
		mu   sync.Mutex
		snap = Snapshot{Timestamp: time.Now().UTC()}
		wg   sync.WaitGroup
	)

	record := func(name string, err error) {
		if err == nil {
			return
		}
		log.Printf("sysinfo: %s collector failed (tolerated): %v", name, err)
		mu.Lock()
		snap.Errors = append(snap.Errors, fmt.Sprintf("%s: %s", name, err))
		mu.Unlock()
	}

	wg.Add(5)

	go func() {
		defer wg.Done()
		v, err := CPU(ctx)
		mu.Lock()
		snap.CPU = v
		mu.Unlock()
		record("cpu", err)
	}()

	go func() {
		defer wg.Done()
		v, err := Memory(ctx)
		mu.Lock()
		snap.Memory = v
		mu.Unlock()
		record("memory", err)
	}()

	go func() {
		defer wg.Done()
		v, err := Disk(ctx, diskRootPath())
		mu.Lock()
		snap.Disk = v
		mu.Unlock()
		record("disk", err)
	}()

	go func() {
		defer wg.Done()
		v, err := Host(ctx)
		mu.Lock()
		snap.Host = v
		mu.Unlock()
		record("host", err)
	}()

	go func() {
		defer wg.Done()
		v, err := Network(ctx)
		mu.Lock()
		snap.Network = v
		mu.Unlock()
		record("network", err)
	}()

	wg.Wait()

	if err := ctx.Err(); err != nil {
		return snap, err
	}
	return snap, nil
}
