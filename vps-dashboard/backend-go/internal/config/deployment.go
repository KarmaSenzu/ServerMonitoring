package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeploymentMode describes how the application is running:
//   - "docker": running inside a container, with /host/proc etc. bind-mounted
//   - "host":   running directly on the host (systemd service, bare metal)
//   - "dev":    local development (go run)
type DeploymentMode string

const (
	ModeDocker DeploymentMode = "docker"
	ModeHost   DeploymentMode = "host"
	ModeDev    DeploymentMode = "dev"
)

// DetectDeploymentMode determines how the binary is running by checking
// for Docker-specific markers and the presence of /host/proc.
//
// This affects:
//   - HOST_PROC path for gopsutil metrics (container reads /host/proc,
//     host reads /proc directly)
//   - Docker socket path (container uses proxy, host uses direct socket)
//   - PM2 socket location
//   - Which features are available (degradation in UI)
func DetectDeploymentMode() DeploymentMode {
	// Dev mode: explicit override
	if os.Getenv("ENV") == "development" || os.Getenv("VPSDASH_DEV") == "1" {
		return ModeDev
	}

	// Docker mode: /.dockerenv marker OR HOST_PROC=/host/proc set
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return ModeDocker
	}
	if os.Getenv("HOST_PROC") == "/host/proc" {
		return ModeDocker
	}

	// Host mode: running directly on the host
	return ModeHost
}

// HostProcPath returns the path to /proc for system metrics collection.
// In Docker mode, this is /host/proc (bind-mounted from host).
// In host mode, this is /proc (direct kernel access).
func HostProcPath(mode DeploymentMode) string {
	if env := os.Getenv("HOST_PROC"); env != "" {
		return env
	}
	switch mode {
	case ModeDocker:
		return "/host/proc"
	default:
		return "/proc"
	}
}

// HostSysPath returns the path to /sys for system info.
func HostSysPath(mode DeploymentMode) string {
	if env := os.Getenv("HOST_SYS"); env != "" {
		return env
	}
	switch mode {
	case ModeDocker:
		return "/host/sys"
	default:
		return "/sys"
	}
}

// HostEtcPath returns the path to /etc for host info (hostname, OS release).
func HostEtcPath(mode DeploymentMode) string {
	if env := os.Getenv("HOST_ETC"); env != "" {
		return env
	}
	switch mode {
	case ModeDocker:
		return "/host/etc"
	default:
		return "/etc"
	}
}

// SystemRootPath returns the root path for disk usage calculations.
// In Docker mode, this is /host (to measure host filesystem).
// In host mode, this is / (root filesystem).
func SystemRootPath(mode DeploymentMode) string {
	if env := os.Getenv("SYSTEM_ROOT_PATH"); env != "" {
		return env
	}
	switch mode {
	case ModeDocker:
		return "/host"
	default:
		return "/"
	}
}

// DockerHostURI returns the Docker socket URI.
// Priority:
//  1. DOCKER_HOST env var (explicit override)
//  2. Docker mode: tcp://dockerproxy:2375 (compose service)
//  3. Host mode: unix:///var/run/docker.sock (direct)
func DockerHostURI(mode DeploymentMode) string {
	if env := os.Getenv("DOCKER_HOST"); env != "" {
		return env
	}
	switch mode {
	case ModeDocker:
		return "tcp://dockerproxy:2375"
	default:
		// Check if socket exists; if not, return empty (Docker disabled)
		if _, err := os.Stat("/var/run/docker.sock"); err == nil {
			return "unix:///var/run/docker.sock"
		}
		return "" // Docker not available
	}
}

// PM2Home returns the PM2 daemon socket directory.
// Priority:
//  1. PM2_HOME env var
//  2. Docker mode: /home/app/.pm2 (compose mount)
//  3. Host mode: ~/.pm2 (user home)
func PM2Home(mode DeploymentMode) string {
	if env := os.Getenv("PM2_HOME"); env != "" {
		return env
	}
	switch mode {
	case ModeDocker:
		return "/home/app/.pm2"
	default:
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".pm2")
		}
		return ""
	}
}

// ModeFeatures describes which features are available in the current mode.
// This is used by the frontend to show/hide panels and degrade gracefully.
type ModeFeatures struct {
	HostMetrics    bool   `json:"hostMetrics"`
	DockerFleet    bool   `json:"dockerFleet"`
	PM2Monitor     bool   `json:"pm2Monitor"`
	DockerSocket   string `json:"dockerSocket,omitempty"` // Available socket URI
	Mode           string `json:"mode"`
	ProcPath       string `json:"procPath,omitempty"`
}

// DetectFeatures returns the available features for the current deployment mode.
func DetectFeatures(mode DeploymentMode) ModeFeatures {
	f := ModeFeatures{
		Mode:     string(mode),
		ProcPath: HostProcPath(mode),
	}

	// Host metrics: always available (we can always read /proc or /host/proc)
	f.HostMetrics = true

	// Docker fleet: check socket availability
	socketURI := DockerHostURI(mode)
	if socketURI != "" {
		// Verify socket is actually accessible
		if strings.HasPrefix(socketURI, "unix://") {
			socketPath := strings.TrimPrefix(socketURI, "unix://")
			if _, err := os.Stat(socketPath); err == nil {
				f.DockerFleet = true
				f.DockerSocket = socketURI
			}
		} else {
			// TCP proxy — assume available (can't easily verify without dialing)
			f.DockerFleet = true
			f.DockerSocket = socketURI
		}
	}

	// PM2 monitor: check socket directory
	pm2Home := PM2Home(mode)
	if pm2Home != "" {
		// Check if PM2 daemon socket exists
		sockPath := filepath.Join(pm2Home, "pubsub.sock")
		if _, err := os.Stat(sockPath); err == nil {
			f.PM2Monitor = true
		}
		// Also check for the main RPC socket
		rpcSock := filepath.Join(pm2Home, "rpc.sock")
		if _, err := os.Stat(rpcSock); err == nil {
			f.PM2Monitor = true
		}
	}

	return f
}

// SetGopsutilEnv sets the HOST_PROC, HOST_SYS, HOST_ETC environment
// variables that gopsutil reads, based on the deployment mode. This must
// be called BEFORE any gopsutil functions are invoked.
//
// In Docker mode these are typically set by the compose file, but when
// running as a single binary we set them programmatically.
func SetGopsutilEnv(mode DeploymentMode) error {
	if err := os.Setenv("HOST_PROC", HostProcPath(mode)); err != nil {
		return fmt.Errorf("set HOST_PROC: %w", err)
	}
	if err := os.Setenv("HOST_SYS", HostSysPath(mode)); err != nil {
		return fmt.Errorf("set HOST_SYS: %w", err)
	}
	if err := os.Setenv("HOST_ETC", HostEtcPath(mode)); err != nil {
		return fmt.Errorf("set HOST_ETC: %w", err)
	}
	return nil
}
