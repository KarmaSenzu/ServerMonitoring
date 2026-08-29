// Package discovery correlates Docker, PM2, and Cloudflare tunnel
// listings into a single "what's running on this VPS" snapshot, plus
// adoption candidates that the operator can promote into the projects
// registry.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"vps-dashboard-api/internal/docker"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/pm2"
	"vps-dashboard-api/internal/tunnel"
)

// Service correlates the live process / container / tunnel state into a
// Snapshot suitable for the dashboard.
type Service struct {
	Logger zerolog.Logger
	Docker *docker.Service
	Tunnel *tunnel.Service
	PM2    *pm2.Service
}

// NewService constructs a Service. Any of the dependencies may be nil
// (in which case the corresponding source is skipped at capture time).
func NewService(l zerolog.Logger, d *docker.Service, t *tunnel.Service, p *pm2.Service) *Service {
	return &Service{Logger: l, Docker: d, Tunnel: t, PM2: p}
}

// Snapshot is the discovery payload returned over the API.
type Snapshot struct {
	Containers []docker.Container `json:"containers"`
	PM2Apps    []pm2.Process      `json:"pm2_apps"`
	Tunnels    []tunnel.Tunnel    `json:"tunnels"`
	Candidates []Candidate        `json:"candidates"`
	Errors     []string           `json:"errors"`
	Timestamp  time.Time          `json:"timestamp"`
}

// Candidate is an adoption suggestion produced by correlating sources.
type Candidate struct {
	SuggestedName  string   `json:"suggested_name"`
	Domain         string   `json:"domain"`
	Port           int      `json:"port"`
	ContainerName  string   `json:"container_name"`
	PM2Name        string   `json:"pm2_name"`
	TunnelService  string   `json:"tunnel_service"`
	HealthURL      string   `json:"health_url"`
	Sources        []string `json:"sources"`
	Confidence     int      `json:"confidence"`
	AlreadyAdopted bool     `json:"already_adopted"`
	AdoptedAs      string   `json:"adopted_as"`
	Reason         string   `json:"reason"`
}

// Capture queries every source in parallel, then folds the results into
// candidates and an aggregate error list. Per-source failures degrade
// gracefully into errs entries; the function only returns a non-nil
// error if loading the projects registry itself fails.
func (s *Service) Capture(ctx context.Context, repo *models.ProjectRepo) (Snapshot, error) {
	var (
		containers []docker.Container
		pm2s       []pm2.Process
		tunnels    []tunnel.Tunnel
		errs       []string
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if s.Docker == nil {
			return nil
		}
		out, err := s.Docker.List(gctx)
		if err != nil {
			if isDockerUnavailable(err) {
				errs = append(errs, "docker_unavailable")
				return nil
			}
			s.Logger.Warn().Err(err).Msg("discovery.docker_list_failed")
			errs = append(errs, "docker_error")
			return nil
		}
		containers = out
		return nil
	})

	g.Go(func() error {
		if s.PM2 == nil {
			return nil
		}
		out, err := s.PM2.List(gctx)
		if err != nil {
			if isPM2Unavailable(err) {
				errs = append(errs, "pm2_unavailable")
				return nil
			}
			s.Logger.Warn().Err(err).Msg("discovery.pm2_list_failed")
			errs = append(errs, "pm2_error")
			return nil
		}
		pm2s = out
		return nil
	})

	g.Go(func() error {
		if s.Tunnel == nil {
			return nil
		}
		out, err := s.Tunnel.List(gctx)
		if err != nil {
			s.Logger.Warn().Err(err).Msg("discovery.tunnel_list_failed")
			errs = append(errs, "tunnel_unavailable")
			return nil
		}
		tunnels = out
		return nil
	})

	if err := g.Wait(); err != nil {
		return Snapshot{}, fmt.Errorf("discovery: capture: %w", err)
	}

	if containers == nil {
		containers = []docker.Container{}
	}
	if pm2s == nil {
		pm2s = []pm2.Process{}
	}
	if tunnels == nil {
		tunnels = []tunnel.Tunnel{}
	}
	if errs == nil {
		errs = []string{}
	}

	projects, err := repo.List(ctx, models.ProjectFilter{})
	if err != nil {
		return Snapshot{}, fmt.Errorf("discovery: projects list: %w", err)
	}

	candidates := buildCandidates(containers, pm2s, tunnels, projects)

	sort.SliceStable(candidates, func(i, j int) bool {
		ci, cj := candidates[i], candidates[j]
		if ci.AlreadyAdopted != cj.AlreadyAdopted {
			return !ci.AlreadyAdopted
		}
		if ci.Confidence != cj.Confidence {
			return ci.Confidence > cj.Confidence
		}
		return strings.ToLower(ci.SuggestedName) < strings.ToLower(cj.SuggestedName)
	})

	return Snapshot{
		Containers: containers,
		PM2Apps:    pm2s,
		Tunnels:    tunnels,
		Candidates: candidates,
		Errors:     errs,
		Timestamp:  time.Now().UTC(),
	}, nil
}

// buildCandidates is the pure correlation function. It takes the four
// raw inputs and returns a deduplicated list of adoption candidates.
func buildCandidates(
	containers []docker.Container,
	pm2s []pm2.Process,
	tunnels []tunnel.Tunnel,
	projects []models.Project,
) []Candidate {
	out := make([]Candidate, 0, 8)

	// Track which sources are already represented so we can emit
	// orphans for everything that wasn't picked up by a tunnel rule.
	usedContainers := make(map[string]struct{})
	usedPM2 := make(map[string]struct{})

	// Phase 1: tunnel-driven candidates.
	for _, tn := range tunnels {
		for _, rule := range tn.Ingress {
			host := strings.TrimSpace(strings.ToLower(rule.Hostname))
			if host == "" {
				continue
			}
			svcHost, svcPort, ok := parseTunnelService(rule.Service)
			if !ok {
				// Even without a parseable service URL we still want a
				// "tunnel-only" candidate keyed off the hostname.
				cand := Candidate{
					SuggestedName: slugFromHostname(host),
					Domain:        host,
					TunnelService: tn.ServiceName,
					Sources:       []string{"tunnel"},
					Confidence:    25,
					Reason:        "tunnel ingress for " + host,
				}
				out = append(out, cand)
				continue
			}

			port := svcPort
			cand := Candidate{
				SuggestedName: slugFromHostname(host),
				Domain:        host,
				Port:          port,
				TunnelService: tn.ServiceName,
				Sources:       []string{"tunnel"},
				Confidence:    25,
			}

			// Try to match a container exposing the same host port.
			if port > 0 {
				if c, ok := matchContainerByPort(containers, port); ok {
					cand.ContainerName = c.Name
					cand.Sources = appendUnique(cand.Sources, "docker")
					usedContainers[strings.ToLower(c.Name)] = struct{}{}
				}
			}

			// Try to match a pm2 app whose name or cwd matches the slug.
			slug := cand.SuggestedName
			if p, ok := matchPM2BySlug(pm2s, slug); ok {
				cand.PM2Name = p.Name
				cand.Sources = appendUnique(cand.Sources, "pm2")
				usedPM2[strings.ToLower(p.Name)] = struct{}{}
			}

			cand.Confidence = scoreSources(cand.Sources, host != "" && svcHost != "")
			cand.Reason = reasonFor(cand)
			out = append(out, cand)
			_ = svcHost
		}
	}

	// Phase 2: orphan running containers with at least one published port.
	for _, c := range containers {
		if _, ok := usedContainers[strings.ToLower(c.Name)]; ok {
			continue
		}
		if !isContainerRunning(c) {
			continue
		}
		ports := extractPublishedPort(c.Ports)
		if len(ports) == 0 {
			continue
		}
		port := ports[0]
		out = append(out, Candidate{
			SuggestedName: c.Name,
			Port:          port,
			ContainerName: c.Name,
			Sources:       []string{"docker"},
			Confidence:    40,
			Reason:        fmt.Sprintf("orphan container %q on port %d", c.Name, port),
		})
		usedContainers[strings.ToLower(c.Name)] = struct{}{}
	}

	// Phase 3: orphan pm2 apps that are online.
	for _, p := range pm2s {
		if _, ok := usedPM2[strings.ToLower(p.Name)]; ok {
			continue
		}
		if !strings.EqualFold(p.Status, "online") {
			continue
		}
		out = append(out, Candidate{
			SuggestedName: p.Name,
			PM2Name:       p.Name,
			Sources:       []string{"pm2"},
			Confidence:    40,
			Reason:        fmt.Sprintf("orphan pm2 app %q", p.Name),
		})
		usedPM2[strings.ToLower(p.Name)] = struct{}{}
	}

	// Phase 4: mark adoption status against the projects registry.
	for i := range out {
		markAdopted(&out[i], projects)
	}

	return out
}

// scoreSources turns the source set into a confidence number per the
// spec: 100 for all three, 75 for two, 50 for one (when the host
// matched), 25 for tunnel-only with no domain match.
func scoreSources(sources []string, domainParsed bool) int {
	count := 0
	hasTunnel := false
	for _, s := range sources {
		switch s {
		case "tunnel":
			hasTunnel = true
		case "docker", "pm2":
			// counted below
		}
		count++
	}
	switch count {
	case 3:
		return 100
	case 2:
		return 75
	case 1:
		if hasTunnel && !domainParsed {
			return 25
		}
		return 50
	}
	return 0
}

func reasonFor(c Candidate) string {
	parts := make([]string, 0, 4)
	if c.Domain != "" {
		parts = append(parts, "tunnel "+c.Domain)
	}
	if c.ContainerName != "" {
		parts = append(parts, "container "+c.ContainerName)
	}
	if c.PM2Name != "" {
		parts = append(parts, "pm2 "+c.PM2Name)
	}
	if len(parts) == 0 {
		return "no signals"
	}
	return strings.Join(parts, " + ")
}

// markAdopted sets AlreadyAdopted/AdoptedAs when the candidate's
// identifiers (or domain) match an existing project row.
func markAdopted(c *Candidate, projects []models.Project) {
	for _, p := range projects {
		if c.ContainerName != "" && strings.EqualFold(p.ContainerName, c.ContainerName) {
			c.AlreadyAdopted = true
			c.AdoptedAs = p.ID
			return
		}
		if c.PM2Name != "" && strings.EqualFold(p.PM2Name, c.PM2Name) {
			c.AlreadyAdopted = true
			c.AdoptedAs = p.ID
			return
		}
		if c.TunnelService != "" && strings.EqualFold(p.TunnelService, c.TunnelService) {
			c.AlreadyAdopted = true
			c.AdoptedAs = p.ID
			return
		}
		if c.Domain != "" && p.Domain != "" && strings.EqualFold(p.Domain, c.Domain) {
			c.AlreadyAdopted = true
			c.AdoptedAs = p.ID
			return
		}
	}
}

// parseTunnelService accepts shapes like:
//
//	http://localhost:3000
//	https://app.example.com
//	http://127.0.0.1:8080
//	http://my-host
//
// It returns (host, port, ok). Implicit ports default to 80 (http) or
// 443 (https). Non-http schemes (hello_world, http_status:404) are
// rejected with ok=false.
func parseTunnelService(s string) (string, int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, false
	}
	if !strings.HasPrefix(strings.ToLower(s), "http://") &&
		!strings.HasPrefix(strings.ToLower(s), "https://") {
		return "", 0, false
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", 0, false
	}
	host := u.Hostname()
	if host == "" {
		return "", 0, false
	}
	port := 0
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n <= 65535 {
			port = n
		}
	} else {
		switch strings.ToLower(u.Scheme) {
		case "http":
			port = 80
		case "https":
			port = 443
		}
	}
	return strings.ToLower(host), port, true
}

// matchContainerByPort returns the first container whose published
// host-side port matches the given port.
func matchContainerByPort(containers []docker.Container, port int) (docker.Container, bool) {
	for _, c := range containers {
		for _, p := range extractPublishedPort(c.Ports) {
			if p == port {
				return c, true
			}
		}
	}
	return docker.Container{}, false
}

// matchPM2BySlug looks for a pm2 process whose name or basename(cwd)
// matches the slug, case-insensitive.
func matchPM2BySlug(pm2s []pm2.Process, slug string) (pm2.Process, bool) {
	if slug == "" {
		return pm2.Process{}, false
	}
	want := strings.ToLower(slug)
	for _, p := range pm2s {
		if strings.EqualFold(p.Name, slug) {
			return p, true
		}
		if p.Cwd != "" {
			base := strings.ToLower(filepath.Base(p.Cwd))
			if base == want {
				return p, true
			}
		}
	}
	return pm2.Process{}, false
}

// portRE matches the host-side port in a Docker `Ports` column entry.
// Examples it must handle:
//
//	0.0.0.0:8080->80/tcp
//	:::8080->80/tcp
//	127.0.0.1:5432->5432/tcp
var portRE = regexp.MustCompile(`(?:^|[\s:\[])(\d{2,5})->\d`)

// extractPublishedPort parses the comma-separated `Ports` string Docker
// emits and returns the slice of host-side published ports as ints.
func extractPublishedPort(ports string) []int {
	if strings.TrimSpace(ports) == "" {
		return nil
	}
	out := make([]int, 0, 4)
	seen := make(map[int]struct{}, 4)
	for _, segment := range strings.Split(ports, ",") {
		seg := strings.TrimSpace(segment)
		if seg == "" {
			continue
		}
		matches := portRE.FindAllStringSubmatch(seg, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			n, err := strconv.Atoi(m[1])
			if err != nil || n <= 0 || n > 65535 {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	return out
}

// slugFromHostname turns a DNS hostname into a short, slug-safe name.
//
//	api.example.com   -> api
//	www.example.com   -> example
//	my-app.example.io -> my-app
//	""                -> ""
func slugFromHostname(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	labels := strings.Split(host, ".")
	pick := labels[0]
	if (pick == "" || pick == "www") && len(labels) > 1 {
		pick = labels[1]
	}
	return slugify(pick)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	// Collapse repeated dashes and trim edges.
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return strings.Trim(out, "-")
}

func isContainerRunning(c docker.Container) bool {
	state := strings.ToLower(strings.TrimSpace(c.State))
	if state == "" {
		return strings.HasPrefix(strings.ToLower(c.Status), "up")
	}
	return state == "running" || state == "up"
}

func appendUnique(xs []string, x string) []string {
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}

func isDockerUnavailable(err error) bool {
	if err == nil {
		return false
	}
	// docker.ErrDockerUnavailable is a sentinel; check both via errors.Is
	// and a string fallback in case of wrapping by upstream callers.
	if isErr(err, docker.ErrDockerUnavailable) {
		return true
	}
	return strings.Contains(err.Error(), "docker: not installed")
}

func isPM2Unavailable(err error) bool {
	if err == nil {
		return false
	}
	if isErr(err, pm2.ErrPM2Unavailable) {
		return true
	}
	return strings.Contains(err.Error(), "pm2: not installed")
}

// isErr is a tiny shim over errors.Is so callers don't need to import
// errors directly just for the helpers above.
func isErr(err, target error) bool {
	return errors.Is(err, target)
}
