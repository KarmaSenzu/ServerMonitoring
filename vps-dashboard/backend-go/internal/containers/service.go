// Package containers implements remote container fleet management
// (PROJECT ARCHITECTURE.md §14-§15) for Docker and Podman on registered
// servers, accessed over SSH in an agentless manner.
//
// The service detects the runtime (docker or podman) on each probe so
// the caller does not need to know which engine a host runs. Output is
// normalised to a uniform Container shape.
package containers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/ssh"
)

// Container is the engine-agnostic container shape returned for both
// Docker and Podman. Podman's JSON differs slightly (Id vs ID, array
// Names, array-of-objects Ports); we normalise everything here.
type Container struct {
	ID      string `json:"id"`
	ShortID string `json:"short_id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
	Engine  string `json:"engine"` // "docker" | "podman"
}

// Service manages containers on remote servers via SSH.
type Service struct {
	Engine *ssh.Service
}

// NewService constructs a remote container service bound to the SSH
// engine.
func NewService(engine *ssh.Service) *Service {
	return &Service{Engine: engine}
}

// listCommand detects the container runtime and lists containers in
// one SSH round-trip. Sentinel lines let us tolerate MOTD/banner noise.
const listCommand = `runtime=""
if command -v docker >/dev/null 2>&1; then runtime="docker"
elif command -v podman >/dev/null 2>&1; then runtime="podman"
fi
if [ -z "$runtime" ]; then echo "__PURPLE_NONE__"; exit 0; fi
echo "__PURPLE_ENGINE__${runtime}"
$runtime ps -a --format '{{json .}}'`

// ListByServer runs the container probe on a registered server and
// returns the detected engine plus the container list.
func (s *Service) ListByServer(ctx context.Context, server models.Server) (string, []Container, error) {
	result, err := s.Engine.RunCommand(ctx, server, listCommand)
	if err != nil {
		return "", nil, fmt.Errorf("containers: list: %w", err)
	}
	engine, containers, err := ParseListOutput(result.Stdout)
	if err != nil {
		return "", nil, fmt.Errorf("containers: parse: %w", err)
	}
	return engine, containers, nil
}

// StartByServer starts a container on a registered server.
func (s *Service) StartByServer(ctx context.Context, server models.Server, name string) error {
	cmd := actionCommand("start", name, 0)
	result, err := s.Engine.RunCommand(ctx, server, cmd)
	if err != nil {
		return fmt.Errorf("containers: start: %w", err)
	}
	if strings.Contains(result.Stdout, "__PURPLE_NONE__") {
		return ErrNoContainerRuntime
	}
	return nil
}

// StopByServer stops a container on a registered server.
func (s *Service) StopByServer(ctx context.Context, server models.Server, name string, timeoutSec int) error {
	cmd := actionCommand("stop", name, timeoutSec)
	result, err := s.Engine.RunCommand(ctx, server, cmd)
	if err != nil {
		return fmt.Errorf("containers: stop: %w", err)
	}
	if strings.Contains(result.Stdout, "__PURPLE_NONE__") {
		return ErrNoContainerRuntime
	}
	return nil
}

// RestartByServer restarts a container on a registered server.
func (s *Service) RestartByServer(ctx context.Context, server models.Server, name string, timeoutSec int) error {
	cmd := actionCommand("restart", name, timeoutSec)
	result, err := s.Engine.RunCommand(ctx, server, cmd)
	if err != nil {
		return fmt.Errorf("containers: restart: %w", err)
	}
	if strings.Contains(result.Stdout, "__PURPLE_NONE__") {
		return ErrNoContainerRuntime
	}
	return nil
}

// LogsByServer fetches container logs from a registered server.
func (s *Service) LogsByServer(ctx context.Context, server models.Server, name string, tail int) (string, string, error) {
	if tail < 1 {
		tail = 200
	}
	if tail > 5000 {
		tail = 5000
	}
	cmd := fmt.Sprintf(`r=$(command -v docker 2>/dev/null || command -v podman 2>/dev/null); [ -n "$r" ] && $r logs --tail %d %s 2>&1 || echo "__PURPLE_NONE__"`, tail, shellEscape(name))
	result, err := s.Engine.RunCommand(ctx, server, cmd)
	if err != nil {
		return "", "", fmt.Errorf("containers: logs: %w", err)
	}
	if strings.Contains(result.Stdout, "__PURPLE_NONE__") {
		return "", "", ErrNoContainerRuntime
	}
	return result.Stdout, result.Stderr, nil
}

// ErrNoContainerRuntime is returned when neither docker nor podman is
// installed on the remote host.
var ErrNoContainerRuntime = fmt.Errorf("containers: no docker or podman runtime found")

// actionCommand builds a start/stop/restart command that auto-detects
// the runtime inline.
func actionCommand(action, name string, timeoutSec int) string {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	escaped := shellEscape(name)
	switch action {
	case "start":
		return fmt.Sprintf(`r=$(command -v docker 2>/dev/null || command -v podman 2>/dev/null); [ -n "$r" ] && $r start %s || echo "__PURPLE_NONE__"`, escaped)
	case "stop":
		return fmt.Sprintf(`r=$(command -v docker 2>/dev/null || command -v podman 2>/dev/null); [ -n "$r" ] && $r stop -t %d %s || echo "__PURPLE_NONE__"`, timeoutSec, escaped)
	case "restart":
		return fmt.Sprintf(`r=$(command -v docker 2>/dev/null || command -v podman 2>/dev/null); [ -n "$r" ] && $r restart -t %d %s || echo "__PURPLE_NONE__"`, timeoutSec, escaped)
	}
	return ""
}

// shellEscape quotes a string for safe inclusion in an SSH command.
func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// ParseListOutput parses the raw output of the container probe command
// into the detected engine and a list of Containers. Non-JSON lines
// (MOTD banners) are silently skipped.
func ParseListOutput(stdout string) (engine string, containers []Container, err error) {
	lines := strings.Split(stdout, "\n")
	started := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "__PURPLE_NONE__" {
			return "", nil, nil
		}
		if strings.HasPrefix(line, "__PURPLE_ENGINE__") {
			engine = strings.TrimPrefix(line, "__PURPLE_ENGINE__")
			started = true
			continue
		}
		if !started {
			continue
		}
		c, perr := parseContainerJSON(line)
		if perr != nil {
			continue
		}
		c.Engine = engine
		containers = append(containers, c)
	}

	return engine, containers, nil
}

// rawContainer tolerates both Docker and Podman JSON shapes.
type rawContainer struct {
	ID    string   `json:"ID"`     // Docker
	Id    string   `json:"Id"`     // Podman
	Names string   `json:"Names"`  // Docker (string)
	Name  string   `json:"Name"`   // Podman (sometimes)
	Image string   `json:"Image"`
	State string   `json:"State"`
	Status string  `json:"Status"`
	Ports string   `json:"Ports"`
}

func parseContainerJSON(line string) (Container, error) {
	var raw rawContainer
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return Container{}, err
	}
	id := raw.ID
	if id == "" {
		id = raw.Id
	}
	name := raw.Names
	if name == "" {
		name = raw.Name
	}
	short := id
	if len(short) >= 12 {
		short = short[:12]
	}
	return Container{
		ID:      id,
		ShortID: short,
		Name:    name,
		Image:   raw.Image,
		State:   raw.State,
		Status:  raw.Status,
		Ports:   raw.Ports,
	}, nil
}
