// Package tunnel discovers Cloudflare tunnels by reading
// /etc/cloudflared/*.yml configs, querying systemd for service status,
// and scraping the cloudflared metrics endpoint for live stream counts.
//
// All shell-out work goes through safeexec; YAML is parsed with
// gopkg.in/yaml.v3 so config files are never interpolated as shell.
package tunnel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"

	"vps-dashboard-api/internal/safeexec"
)

// ErrInvalidTunnelService is returned by Restart when the requested
// systemd unit name does not match the cloudflared* allowlist.
var ErrInvalidTunnelService = errors.New("tunnel: invalid service name")

var serviceNameRE = regexp.MustCompile(`^cloudflared(-[a-zA-Z0-9_.-]+)?$`)

// IngressRule mirrors a single entry under the cloudflared `ingress:` list.
// Catchall is true when the entry has no hostname (the YAML default route).
type IngressRule struct {
	Hostname string `json:"hostname"`
	Service  string `json:"service"`
	Path     string `json:"path,omitempty"`
	Catchall bool   `json:"catchall"`
}

// Tunnel is the JSON-friendly view of a single cloudflared instance.
type Tunnel struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	ConfigPath      string        `json:"configPath"`
	ServiceName     string        `json:"serviceName"`
	ActiveState     string        `json:"activeState"`
	SubState        string        `json:"subState"`
	MainPID         int           `json:"mainPid"`
	StartedAt       time.Time     `json:"startedAt"`
	Uptime          string        `json:"uptime"`
	ActiveStreams   int           `json:"activeStreams"`
	LatencyMs       int           `json:"latency_ms"`
	Ingress         []IngressRule `json:"ingress"`
	CredentialsFile string        `json:"credentialsFile,omitempty"`
	Hostname        string        `json:"hostname,omitempty"`
}

// Service holds environment-specific configuration: where to look for
// cloudflared YAML files, and which localhost ports to scrape for the
// metrics endpoint.
type Service struct {
	Logger       zerolog.Logger
	ConfigDir    string
	MetricsPorts []int
}

// NewService returns a Service with sensible defaults for a Linux VPS.
func NewService(l zerolog.Logger) *Service {
	return &Service{
		Logger:       l,
		ConfigDir:    "/etc/cloudflared",
		MetricsPorts: []int{20241, 20242},
	}
}

// rawConfig matches the subset of cloudflared YAML we care about.
type rawConfig struct {
	Tunnel          string           `yaml:"tunnel"`
	CredentialsFile string           `yaml:"credentials-file"`
	Ingress         []map[string]any `yaml:"ingress"`
}

// configEntry is one discovered cloudflared config file together with
// the deployment name we want it to surface as. nameOverride is empty
// for legacy flat-layout files (where the name is derived from the
// filename); for per-project subdir layouts it is the subdir name.
type configEntry struct {
	path         string
	nameOverride string
}

// List discovers every cloudflared config in s.ConfigDir, parses each
// one, and decorates it with systemd state and live metrics. If the
// directory does not exist (e.g. on a dev mac) it returns an empty list,
// not an error.
//
// Two layouts are supported simultaneously:
//
//  1. Flat (legacy): /etc/cloudflared/*.yml — every YAML at the root is
//     one tunnel. config.yml is the "default" deployment.
//  2. Per-project (multi-tunnel): /etc/cloudflared/<name>/config.yml —
//     each subdir is one tunnel, surfaced with Name=<subdir>. This is
//     used when several project repos each contribute their own
//     cloudflared-config/ via bind mount.
func (s *Service) List(ctx context.Context) ([]Tunnel, error) {
	if s.ConfigDir == "" {
		return []Tunnel{}, nil
	}

	if _, err := os.Stat(s.ConfigDir); err != nil {
		if os.IsNotExist(err) {
			return []Tunnel{}, nil
		}
		return nil, fmt.Errorf("tunnel: stat config dir: %w", err)
	}

	entries, err := s.gatherConfigEntries()
	if err != nil {
		return nil, err
	}

	out := make([]Tunnel, 0, len(entries))
	for _, e := range entries {
		t, err := parseConfigFile(e.path)
		if err != nil {
			s.Logger.Warn().Err(err).Str("path", e.path).Msg("tunnel.parse_failed")
			continue
		}
		if e.nameOverride != "" {
			t.Name = e.nameOverride
			t.ServiceName = "cloudflared-" + e.nameOverride
		}
		s.decorateWithSystemd(ctx, &t)
		t.ActiveStreams = s.scrapeActiveStreams(ctx)
		t.LatencyMs = s.measureLatencyMs(ctx)
		out = append(out, t)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// gatherConfigEntries scans ConfigDir for both layouts and returns the
// merged list of files to parse. Duplicates (same absolute path
// reachable via both scans) are de-duped. Hidden subdirs (starting with
// "." or "_") are skipped so a marker dir like "_global" can hold
// shared credentials without showing up as a tunnel.
func (s *Service) gatherConfigEntries() ([]configEntry, error) {
	var entries []configEntry
	seen := make(map[string]struct{})

	// Layout 1: flat *.yml / *.yaml at the root.
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		hits, err := filepath.Glob(filepath.Join(s.ConfigDir, pattern))
		if err != nil {
			return nil, fmt.Errorf("tunnel: glob %s: %w", pattern, err)
		}
		for _, h := range hits {
			base := filepath.Base(h)
			if strings.Contains(base, ".bak") {
				continue
			}
			if _, ok := seen[h]; ok {
				continue
			}
			seen[h] = struct{}{}
			entries = append(entries, configEntry{path: h})
		}
	}

	// Layout 2: per-project subdirs with config.yml inside.
	subs, err := os.ReadDir(s.ConfigDir)
	if err != nil {
		// Non-fatal: keep flat-layout matches we already have.
		s.Logger.Warn().Err(err).Str("dir", s.ConfigDir).Msg("tunnel.readdir_failed")
		return entries, nil
	}
	for _, sub := range subs {
		if !sub.IsDir() {
			continue
		}
		name := sub.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		for _, candidate := range []string{"config.yml", "config.yaml"} {
			full := filepath.Join(s.ConfigDir, name, candidate)
			if _, statErr := os.Stat(full); statErr != nil {
				continue
			}
			if _, ok := seen[full]; ok {
				break
			}
			seen[full] = struct{}{}
			entries = append(entries, configEntry{path: full, nameOverride: name})
			break
		}
	}

	return entries, nil
}

// Restart asks systemd to restart the named cloudflared unit. The
// service name is allowlist-validated to prevent injection through the
// HTTP API. This is exposed for Wave 2; no route is registered yet.
func (s *Service) Restart(ctx context.Context, serviceName string) error {
	if !serviceNameRE.MatchString(serviceName) {
		return ErrInvalidTunnelService
	}
	_, stderr, err := safeexec.Run(ctx, "systemctl", "restart", serviceName)
	if err != nil {
		return fmt.Errorf("systemctl restart %s: %w (stderr=%q)", serviceName, err, stderr)
	}
	return nil
}

// parseConfigFile reads a single cloudflared YAML config and turns it
// into a Tunnel with all of the static fields populated. It does not
// touch systemctl or the metrics endpoint.
func parseConfigFile(path string) (Tunnel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Tunnel{}, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg rawConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Tunnel{}, fmt.Errorf("parse %s: %w", path, err)
	}

	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	name := stem
	serviceName := "cloudflared-" + stem
	if base == "config.yml" || base == "config.yaml" {
		name = "default"
		serviceName = "cloudflared"
	}

	t := Tunnel{
		ID:              strings.TrimSpace(cfg.Tunnel),
		Name:            name,
		ConfigPath:      path,
		ServiceName:     serviceName,
		ActiveStreams:   -1,
		LatencyMs:       -1,
		CredentialsFile: strings.TrimSpace(cfg.CredentialsFile),
	}

	for _, rule := range cfg.Ingress {
		ir := buildIngressRule(rule)
		t.Ingress = append(t.Ingress, ir)
		if t.Hostname == "" && !ir.Catchall && ir.Hostname != "" {
			t.Hostname = ir.Hostname
		}
	}

	return t, nil
}

func buildIngressRule(m map[string]any) IngressRule {
	var rule IngressRule
	if v, ok := stringField(m, "hostname"); ok {
		rule.Hostname = v
	}
	if v, ok := stringField(m, "service"); ok {
		rule.Service = v
	}
	if v, ok := stringField(m, "path"); ok {
		rule.Path = v
	}
	rule.Catchall = rule.Hostname == ""
	return rule
}

func stringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s), true
	case fmt.Stringer:
		return strings.TrimSpace(s.String()), true
	default:
		return strings.TrimSpace(fmt.Sprint(s)), true
	}
}

// decorateWithSystemd best-effort populates ActiveState/SubState/MainPID/
// StartedAt/Uptime by calling `systemctl show`. Any failure (systemctl
// missing, unit missing, etc.) leaves the fields at their zero values
// and sets ActiveState to "unknown".
func (s *Service) decorateWithSystemd(ctx context.Context, t *Tunnel) {
	stdout, _, err := safeexec.Run(ctx, "systemctl", "show", t.ServiceName,
		"--property=ActiveState,SubState,MainPID,ActiveEnterTimestamp")
	if err != nil {
		t.ActiveState = "unknown"
		return
	}

	props := parseSystemctlShow(stdout)
	t.ActiveState = props["ActiveState"]
	t.SubState = props["SubState"]
	if t.ActiveState == "" {
		t.ActiveState = "unknown"
	}
	if pid, perr := strconv.Atoi(props["MainPID"]); perr == nil && pid > 0 {
		t.MainPID = pid
	}
	if ts := props["ActiveEnterTimestamp"]; ts != "" {
		if started, perr := time.Parse("Mon 2006-01-02 15:04:05 MST", ts); perr == nil {
			t.StartedAt = started
			if t.ActiveState == "active" {
				t.Uptime = humanizeDuration(time.Since(started))
			}
		}
	}
}

func parseSystemctlShow(s string) map[string]string {
	out := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		out[line[:idx]] = strings.TrimSpace(line[idx+1:])
	}
	return out
}

// humanizeDuration produces a compact human-readable form of d, picking
// the two most-significant non-zero units (e.g. "3h 12m", "5d 1h").
func humanizeDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	totalSec := int64(d.Seconds())
	days := totalSec / 86400
	hours := (totalSec % 86400) / 3600
	mins := (totalSec % 3600) / 60
	secs := totalSec % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case mins > 0:
		return fmt.Sprintf("%dm %ds", mins, secs)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

// scrapeActiveStreams probes each configured metrics port and parses
// `cloudflared_tunnel_active_streams` out of the prometheus exposition
// format. Returns -1 when no port answers in time.
func (s *Service) scrapeActiveStreams(ctx context.Context) int {
	client := &http.Client{Timeout: 1 * time.Second}
	for _, port := range s.MetricsPorts {
		url := fmt.Sprintf("http://127.0.0.1:%d/metrics", port)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		val, ok := readActiveStreams(resp.Body)
		_ = resp.Body.Close()
		if ok {
			return val
		}
	}
	return -1
}

func readActiveStreams(r interface {
	Read(p []byte) (int, error)
}) (int, bool) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "cloudflared_tunnel_active_streams") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		raw := fields[len(fields)-1]
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return int(v), true
		}
	}
	return 0, false
}

// MeasureLatency probes http://127.0.0.1:<port>/ready (which cloudflared
// exposes on the metrics endpoint) and returns the round-trip duration.
// On failure it returns -1ns and a wrapped error.
func (s *Service) MeasureLatency(ctx context.Context, port int) (time.Duration, error) {
	if port <= 0 || port > 65535 {
		return time.Duration(-1), fmt.Errorf("tunnel: invalid port %d", port)
	}
	probeCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/ready", port)
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return time.Duration(-1), fmt.Errorf("tunnel: build req: %w", err)
	}
	client := &http.Client{Timeout: 1 * time.Second}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return time.Duration(-1), fmt.Errorf("tunnel: probe %s: %w", url, err)
	}
	rtt := time.Since(start)
	_ = resp.Body.Close()
	return rtt, nil
}

// measureLatencyMs walks the configured MetricsPorts and returns the
// first reachable round-trip in milliseconds. It returns -1 when no
// port answers in time.
func (s *Service) measureLatencyMs(ctx context.Context) int {
	for _, port := range s.MetricsPorts {
		if rtt, err := s.MeasureLatency(ctx, port); err == nil && rtt >= 0 {
			ms := int(rtt / time.Millisecond)
			if ms < 0 {
				ms = 0
			}
			return ms
		}
	}
	return -1
}
