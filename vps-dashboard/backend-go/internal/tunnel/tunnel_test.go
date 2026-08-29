package tunnel

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rs/zerolog"
)

const configYML = `tunnel: 11111111-2222-3333-4444-555555555555
credentials-file: /etc/cloudflared/creds.json
ingress:
  - hostname: app.example.com
    service: http://localhost:3000
  - hostname: api.example.com
    service: http://localhost:3001
    path: /v1
  - service: http_status:404
`

const dashboardYML = `tunnel: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
credentials-file: /etc/cloudflared/dash.json
ingress:
  - hostname: dash.example.com
    service: http://localhost:4000
  - service: http_status:404
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func nopLogger() zerolog.Logger {
	return zerolog.New(io.Discard)
}

func testContext() context.Context {
	return context.Background()
}

func TestParseConfigFileDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	writeFile(t, path, configYML)

	got, err := parseConfigFile(path)
	if err != nil {
		t.Fatalf("parseConfigFile: %v", err)
	}
	if got.Name != "default" {
		t.Errorf("Name: got %q want default", got.Name)
	}
	if got.ServiceName != "cloudflared" {
		t.Errorf("ServiceName: got %q want cloudflared", got.ServiceName)
	}
	if got.ID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("ID: got %q", got.ID)
	}
	if got.CredentialsFile != "/etc/cloudflared/creds.json" {
		t.Errorf("CredentialsFile: got %q", got.CredentialsFile)
	}
	if len(got.Ingress) != 3 {
		t.Fatalf("Ingress len: got %d want 3", len(got.Ingress))
	}
	if got.Ingress[0].Hostname != "app.example.com" {
		t.Errorf("Ingress[0].Hostname: got %q", got.Ingress[0].Hostname)
	}
	if got.Ingress[0].Service != "http://localhost:3000" {
		t.Errorf("Ingress[0].Service: got %q", got.Ingress[0].Service)
	}
	if got.Ingress[0].Catchall {
		t.Errorf("Ingress[0].Catchall should be false")
	}
	if got.Ingress[1].Path != "/v1" {
		t.Errorf("Ingress[1].Path: got %q want /v1", got.Ingress[1].Path)
	}
	if !got.Ingress[2].Catchall {
		t.Errorf("Ingress[2].Catchall should be true")
	}
	if got.Hostname != "app.example.com" {
		t.Errorf("Hostname (first non-catchall): got %q", got.Hostname)
	}
	if got.ActiveStreams != -1 {
		t.Errorf("ActiveStreams default: got %d want -1", got.ActiveStreams)
	}
}

func TestParseConfigFileNamed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dashboard.yml")
	writeFile(t, path, dashboardYML)

	got, err := parseConfigFile(path)
	if err != nil {
		t.Fatalf("parseConfigFile: %v", err)
	}
	if got.Name != "dashboard" {
		t.Errorf("Name: got %q want dashboard", got.Name)
	}
	if got.ServiceName != "cloudflared-dashboard" {
		t.Errorf("ServiceName: got %q want cloudflared-dashboard", got.ServiceName)
	}
	if len(got.Ingress) != 2 {
		t.Fatalf("Ingress len: got %d want 2", len(got.Ingress))
	}
	if !got.Ingress[1].Catchall {
		t.Errorf("Ingress[1].Catchall expected true")
	}
}

func TestServiceListMissingDirIsEmpty(t *testing.T) {
	s := NewService(nopLogger())
	s.ConfigDir = "/path/that/does/not/exist/we/promise"
	tunnels, err := s.List(testContext())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tunnels) != 0 {
		t.Errorf("expected empty list, got %d", len(tunnels))
	}
}

func TestServiceListSkipsBakFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.yml"), configYML)
	writeFile(t, filepath.Join(dir, "config.yml.bak"), configYML)
	writeFile(t, filepath.Join(dir, "old.bak.yml"), configYML)

	s := NewService(nopLogger())
	s.ConfigDir = dir
	// No metrics ports so the http scrape short-circuits to -1.
	s.MetricsPorts = nil

	tunnels, err := s.List(testContext())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel (the .bak ones must be skipped), got %d", len(tunnels))
	}
	if tunnels[0].Name != "default" {
		t.Errorf("Name: got %q want default", tunnels[0].Name)
	}
}

func TestParseSystemctlShow(t *testing.T) {
	in := "ActiveState=active\nSubState=running\nMainPID=1234\nActiveEnterTimestamp=Mon 2026-05-12 03:04:05 UTC\n"
	got := parseSystemctlShow(in)
	if got["ActiveState"] != "active" {
		t.Errorf("ActiveState: got %q", got["ActiveState"])
	}
	if got["MainPID"] != "1234" {
		t.Errorf("MainPID: got %q", got["MainPID"])
	}
	if got["ActiveEnterTimestamp"] != "Mon 2026-05-12 03:04:05 UTC" {
		t.Errorf("ActiveEnterTimestamp: got %q", got["ActiveEnterTimestamp"])
	}
}

func TestMeasureLatencyAgainstHTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	if host != "127.0.0.1" {
		t.Skipf("httptest bound to %s, not 127.0.0.1; MeasureLatency hard-codes loopback", host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("port atoi: %v", err)
	}

	s := NewService(nopLogger())
	rtt, err := s.MeasureLatency(testContext(), port)
	if err != nil {
		t.Fatalf("MeasureLatency: %v", err)
	}
	if rtt < 0 {
		t.Fatalf("expected non-negative RTT, got %v", rtt)
	}
}

func TestMeasureLatencyUnreachable(t *testing.T) {
	s := NewService(nopLogger())
	// Pick a high port that almost certainly isn't bound.
	rtt, err := s.MeasureLatency(testContext(), 1)
	if err == nil {
		t.Fatalf("expected error for unreachable port, got rtt=%v", rtt)
	}
	if rtt >= 0 {
		t.Fatalf("expected -1 sentinel, got %v", rtt)
	}
}
