package discovery

import (
	"testing"

	"vps-dashboard-api/internal/docker"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/pm2"
	"vps-dashboard-api/internal/tunnel"
)

func TestSlugFromHostname(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"api.example.com", "api"},
		{"www.example.com", "example"},
		{"my-app.example.com", "my-app"},
		{"foo_bar.example.com", "foo-bar"},
		{"", ""},
		{"plainhost", "plainhost"},
		{"WWW.Example.com", "example"},
	}
	for _, tc := range cases {
		got := slugFromHostname(tc.in)
		if got != tc.want {
			t.Errorf("slugFromHostname(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractPublishedPort(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"0.0.0.0:8080->80/tcp, :::8080->80/tcp", []int{8080}},
		{"127.0.0.1:5432->5432/tcp", []int{5432}},
		{"", nil},
		{"0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp", []int{80, 443}},
	}
	for _, tc := range cases {
		got := extractPublishedPort(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("extractPublishedPort(%q) = %v want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("extractPublishedPort(%q)[%d] = %d want %d", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestParseTunnelService(t *testing.T) {
	cases := []struct {
		in       string
		host     string
		port     int
		ok       bool
	}{
		{"http://localhost:3000", "localhost", 3000, true},
		{"http://127.0.0.1:8080", "127.0.0.1", 8080, true},
		{"https://app.example.com", "app.example.com", 443, true},
		{"http://my-host", "my-host", 80, true},
		{"hello_world", "", 0, false},
		{"http_status:404", "", 0, false},
		{"", "", 0, false},
	}
	for _, tc := range cases {
		host, port, ok := parseTunnelService(tc.in)
		if ok != tc.ok || host != tc.host || port != tc.port {
			t.Errorf("parseTunnelService(%q) = (%q, %d, %v) want (%q, %d, %v)",
				tc.in, host, port, ok, tc.host, tc.port, tc.ok)
		}
	}
}

func TestBuildCandidatesFullMatch(t *testing.T) {
	containers := []docker.Container{
		{Name: "my-app", State: "running", Status: "Up 2 hours", Ports: "0.0.0.0:3000->3000/tcp"},
	}
	pm2s := []pm2.Process{
		{Name: "my-app", Status: "online", Cwd: "/srv/my-app"},
	}
	tunnels := []tunnel.Tunnel{
		{
			ServiceName: "cloudflared-app",
			Ingress: []tunnel.IngressRule{
				{Hostname: "my-app.example.com", Service: "http://localhost:3000"},
			},
		},
	}

	got := buildCandidates(containers, pm2s, tunnels, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d (%+v)", len(got), got)
	}
	c := got[0]
	if c.Confidence != 100 {
		t.Errorf("confidence = %d want 100", c.Confidence)
	}
	if c.SuggestedName != "my-app" {
		t.Errorf("suggested_name = %q want my-app", c.SuggestedName)
	}
	if c.ContainerName != "my-app" || c.PM2Name != "my-app" {
		t.Errorf("identifiers wrong: %+v", c)
	}
	if c.TunnelService != "cloudflared-app" {
		t.Errorf("tunnel_service = %q", c.TunnelService)
	}
	if c.Domain != "my-app.example.com" {
		t.Errorf("domain = %q", c.Domain)
	}
}

func TestBuildCandidatesTunnelPlusContainer(t *testing.T) {
	containers := []docker.Container{
		{Name: "api-svc", State: "running", Status: "Up", Ports: "0.0.0.0:4000->4000/tcp"},
	}
	pm2s := []pm2.Process{}
	tunnels := []tunnel.Tunnel{
		{
			ServiceName: "cloudflared",
			Ingress: []tunnel.IngressRule{
				{Hostname: "api.example.com", Service: "http://127.0.0.1:4000"},
			},
		},
	}

	got := buildCandidates(containers, pm2s, tunnels, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d (%+v)", len(got), got)
	}
	c := got[0]
	if c.Confidence != 75 {
		t.Errorf("confidence = %d want 75 (tunnel+docker)", c.Confidence)
	}
	if c.ContainerName != "api-svc" {
		t.Errorf("container_name = %q want api-svc", c.ContainerName)
	}
}

func TestBuildCandidatesTunnelOnlyNoMatch(t *testing.T) {
	tunnels := []tunnel.Tunnel{
		{
			ServiceName: "cloudflared",
			Ingress: []tunnel.IngressRule{
				{Hostname: "lonely.example.com", Service: "http_status:404"},
			},
		},
	}
	got := buildCandidates(nil, nil, tunnels, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d (%+v)", len(got), got)
	}
	c := got[0]
	if c.Confidence != 25 {
		t.Errorf("confidence = %d want 25", c.Confidence)
	}
	if len(c.Sources) != 1 || c.Sources[0] != "tunnel" {
		t.Errorf("sources = %v want [tunnel]", c.Sources)
	}
}

func TestBuildCandidatesOrphanContainer(t *testing.T) {
	containers := []docker.Container{
		{Name: "orph", State: "running", Status: "Up", Ports: "0.0.0.0:9000->9000/tcp"},
	}
	got := buildCandidates(containers, nil, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	c := got[0]
	if c.Confidence != 40 {
		t.Errorf("confidence = %d want 40", c.Confidence)
	}
	if c.ContainerName != "orph" || c.Port != 9000 {
		t.Errorf("orphan container fields wrong: %+v", c)
	}
}

func TestBuildCandidatesOrphanContainerNoPortsSkipped(t *testing.T) {
	containers := []docker.Container{
		{Name: "noports", State: "running", Status: "Up", Ports: ""},
	}
	got := buildCandidates(containers, nil, nil, nil)
	if len(got) != 0 {
		t.Errorf("expected 0 candidates for container without ports, got %+v", got)
	}
}

func TestBuildCandidatesOrphanPM2(t *testing.T) {
	pm2s := []pm2.Process{
		{Name: "worker", Status: "online", Cwd: "/srv/worker"},
		{Name: "stopped", Status: "stopped"},
	}
	got := buildCandidates(nil, pm2s, nil, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate (online only), got %d (%+v)", len(got), got)
	}
	c := got[0]
	if c.Confidence != 40 {
		t.Errorf("confidence = %d want 40", c.Confidence)
	}
	if c.PM2Name != "worker" {
		t.Errorf("pm2_name = %q", c.PM2Name)
	}
}

func TestBuildCandidatesAlreadyAdoptedByContainerName(t *testing.T) {
	containers := []docker.Container{
		{Name: "my-app", State: "running", Status: "Up", Ports: "0.0.0.0:3000->3000/tcp"},
	}
	projects := []models.Project{
		{ID: "p1", Name: "My App", ContainerName: "my-app"},
	}
	got := buildCandidates(containers, nil, nil, projects)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	if !got[0].AlreadyAdopted || got[0].AdoptedAs != "p1" {
		t.Errorf("not marked adopted: %+v", got[0])
	}
}

func TestBuildCandidatesAlreadyAdoptedByDomain(t *testing.T) {
	tunnels := []tunnel.Tunnel{
		{
			ServiceName: "cloudflared",
			Ingress: []tunnel.IngressRule{
				{Hostname: "shop.example.com", Service: "http://localhost:3000"},
			},
		},
	}
	projects := []models.Project{
		{ID: "p2", Name: "Shop", Domain: "shop.example.com"},
	}
	got := buildCandidates(nil, nil, tunnels, projects)
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	if !got[0].AlreadyAdopted || got[0].AdoptedAs != "p2" {
		t.Errorf("not marked adopted by domain: %+v", got[0])
	}
}
