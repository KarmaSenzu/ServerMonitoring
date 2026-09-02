package remote

import (
	"context"
	"fmt"
	"strings"
	"time"

	"vps-dashboard-api/internal/models"
)

// DiscoveryResult holds auto-detected services and tunnels from a remote server.
// This is collected via a single SSH command (like Purple's approach) —
// no manual input needed from the user.
type DiscoveryResult struct {
	// PM2 processes
	PM2Processes []PM2Process `json:"pm2_processes,omitempty"`

	// Docker containers
	DockerContainers []DockerContainerInfo `json:"docker_containers,omitempty"`

	// SSH tunnels (reverse port forwards seen in /proc/net/tcp)
	SSHTunnels []SSHTunnelInfo `json:"ssh_tunnels,omitempty"`

	// Running services (systemd units that are active)
	SystemdServices []SystemdService `json:"systemd_services,omitempty"`

	// Listening ports
	ListeningPorts []PortInfo `json:"listening_ports,omitempty"`

	// System info
	Hostname string `json:"hostname"`
	Kernel   string `json:"kernel"`
	OSName   string `json:"os_name"`
}

// PM2Process represents a PM2-managed process.
type PM2Process struct {
	Name      string `json:"name"`
	Status    string `json:"status"`    // online/stopped/errored
	Restarts  int    `json:"restarts"`
	Uptime    string `json:"uptime"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	ExecMode  string `json:"exec_mode"`
	Script    string `json:"script"`
}

// DockerContainerInfo represents a Docker container on the remote host.
type DockerContainerInfo struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
	State   string `json:"state"`
}

// SSHTunnelInfo represents an active SSH tunnel.
type SSHTunnelInfo struct {
	LocalPort  int    `json:"local_port"`
	RemoteHost string `json:"remote_host"`
	RemotePort int    `json:"remote_port"`
	Type       string `json:"type"` // L=local, R=remote, D=dynamic
}

// SystemdService represents a systemd service.
type SystemdService struct {
	Name   string `json:"name"`
	Status string `json:"status"` // active/inactive/failed
	Type   string `json:"type"`  // service/socket/timer
}

// PortInfo represents a listening port.
type PortInfo struct {
	Port    int    `json:"port"`
	Address string `json:"address"`
	Process string `json:"process"`
	Proto   string `json:"proto"` // tcp/udp
}

// discoveryCommand is a single SSH command that collects ALL service info
// from a remote server in one round-trip (like Purple's approach).
// It auto-detects: PM2, Docker, SSH tunnels, systemd services, listening ports.
// Each section is separated by a delimiter so the parser can split them.
const discoveryCommand = `echo "=PM2="; ` +
	// PM2: list all processes in machine-readable format
	`pm2 jlist 2>/dev/null || echo '{}'; ` +
	`echo "=DOCKER="; ` +
	// Docker: list containers
	`docker ps -a --format '{{.Names}}|{{.Image}}|{{.Status}}|{{.Ports}}|{{.State}}' 2>/dev/null || echo ''; ` +
	`echo "=TUNNELS="; ` +
	// SSH tunnels: check for reverse port forwards in process list
	`ps aux 2>/dev/null | grep -E 'ssh.*-R|ssh.*-L|ssh.*-D' | grep -v grep || echo ''; ` +
	`echo "=SYSTEMD="; ` +
	// Systemd: list active services
	`systemctl list-units --type=service --state=running --no-pager --no-legend 2>/dev/null | head -20 || echo ''; ` +
	`echo "=PORTS="; ` +
	// Listening ports
	`ss -tlnp 2>/dev/null | tail -n +2 | awk '{print $4, $6}' || netstat -tlnp 2>/dev/null | tail -n +2 | awk '{print $4, $6}' || echo ''; ` +
	`echo "=SYSINFO="; ` +
	// System info
	`echo "hostname=$(hostname 2>/dev/null || echo '')"; ` +
	`echo "kernel=$(uname -r 2>/dev/null || echo '')"; ` +
	`echo "os_name=$(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | head -1 | cut -d'"' -f2 || uname -sr 2>/dev/null || echo '')"; ` +
	`echo "=END="`

// Discover collects service info from a remote server via SSH.
// This is called after a successful metrics collection to auto-detect
// PM2, Docker, tunnels, and systemd services — so users don't need
// to manually input anything.
func (c *Collector) Discover(ctx context.Context, server models.Server) (*DiscoveryResult, error) {
	discoverCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	result, err := c.Engine.RunCommand(discoverCtx, server, discoveryCommand)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}

	return parseDiscovery(result.Stdout), nil
}

// parseDiscovery parses the SSH command output into structured data.
// Sections are delimited by "=SECTION=" markers.
func parseDiscovery(stdout string) *DiscoveryResult {
	result := &DiscoveryResult{}

	// Split output into sections
	sections := splitDiscoverySections(stdout)

	for section, data := range sections {
		switch section {
		case "PM2":
			result.PM2Processes = parsePM2(data)
		case "DOCKER":
			result.DockerContainers = parseDocker(data)
		case "TUNNELS":
			result.SSHTunnels = parseTunnels(data)
		case "SYSTEMD":
			result.SystemdServices = parseSystemd(data)
		case "PORTS":
			result.ListeningPorts = parsePorts(data)
		case "SYSINFO":
			info := parseSysInfo(data)
			result.Hostname = info["hostname"]
			result.Kernel = info["kernel"]
			result.OSName = info["os_name"]
		}
	}

	return result
}

// splitDiscoverySections splits the output into sections by =MARKER= delimiters.
func splitDiscoverySections(stdout string) map[string]string {
	sections := make(map[string]string)
	currentSection := ""
	var currentData strings.Builder

	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "=") && strings.HasSuffix(trimmed, "=") && len(trimmed) > 2 {
			// Save previous section
			if currentSection != "" {
				sections[currentSection] = currentData.String()
			}
			// Start new section
			currentSection = strings.Trim(trimmed, "=")
			currentData.Reset()
		} else {
			currentData.WriteString(line + "\n")
		}
	}
	// Save last section
	if currentSection != "" {
		sections[currentSection] = currentData.String()
	}

	return sections
}

// parsePM2 parses PM2 jlist output into process info.
func parsePM2(data string) []PM2Process {
	data = strings.TrimSpace(data)
	if data == "" || data == "{}" {
		return nil
	}

	// PM2 jlist returns a JSON array — but we do a simple parse
	// to avoid importing encoding/json here. Each process has:
	// "name":"...", "pm2_env":{"status":"...", "restart_time":N, ...}
	var processes []PM2Process

	// Simple line-based extraction (not full JSON parsing)
	// In production, use encoding/json
	lines := strings.Split(data, "\"name\":")
	for i := 1; i < len(lines); i++ {
		parts := strings.SplitN(lines[i], "\"", 3)
		if len(parts) >= 2 {
			name := parts[1]
			processes = append(processes, PM2Process{
				Name:   name,
				Status: "online",
			})
		}
	}

	return processes
}

// parseDocker parses docker ps --format output.
func parseDocker(data string) []DockerContainerInfo {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}

	var containers []DockerContainerInfo
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) >= 5 {
			containers = append(containers, DockerContainerInfo{
				Name:   strings.TrimSpace(parts[0]),
				Image:  strings.TrimSpace(parts[1]),
				Status: strings.TrimSpace(parts[2]),
				Ports:  strings.TrimSpace(parts[3]),
				State:  strings.TrimSpace(parts[4]),
			})
		}
	}

	return containers
}

// parseTunnels parses ssh tunnel process lines.
func parseTunnels(data string) []SSHTunnelInfo {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}

	var tunnels []SSHTunnelInfo
	// Parse ssh -L/-R/-D lines
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Extract port info from ssh command line
		// This is a simplified parser — real implementation would be more robust
		if strings.Contains(line, "-L") || strings.Contains(line, "-R") || strings.Contains(line, "-D") {
			tunnels = append(tunnels, SSHTunnelInfo{
				Type: "active",
			})
		}
	}

	return tunnels
}

// parseSystemd parses systemctl list-units output.
func parseSystemd(data string) []SystemdService {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}

	var services []SystemdService
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: name.service loaded active running description
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			services = append(services, SystemdService{
				Name:   fields[0],
				Status: fields[3],
				Type:   "service",
			})
		}
	}

	// Limit to top 20 services
	if len(services) > 20 {
		services = services[:20]
	}

	return services
}

// parsePorts parses ss/netstat listening port output.
func parsePorts(data string) []PortInfo {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}

	var ports []PortInfo
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Extract port from address (e.g., 0.0.0.0:8080 or *:8080)
			addr := fields[0]
			procInfo := fields[1]
			port := extractPort(addr)
			if port > 0 {
				ports = append(ports, PortInfo{
					Port:    port,
					Address: addr,
					Process: procInfo,
					Proto:   "tcp",
				})
			}
		}
	}

	return ports
}

// parseSysInfo parses key=value system info lines.
func parseSysInfo(data string) map[string]string {
	kv := make(map[string]string)
	for _, line := range strings.Split(data, "\n") {
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
	return kv
}

// extractPort extracts a port number from an address string.
func extractPort(addr string) int {
	idx := strings.LastIndexByte(addr, ':')
	if idx < 0 {
		return 0
	}
	port := 0
	for i := idx + 1; i < len(addr); i++ {
		c := addr[i]
		if c < '0' || c > '9' {
			break
		}
		port = port*10 + int(c-'0')
	}
	return port
}
