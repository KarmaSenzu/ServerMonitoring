package containers_test

import (
	"testing"

	"vps-dashboard-api/internal/containers"
)

func TestParseListOutputNone(t *testing.T) {
	engine, cs, err := containers.ParseListOutput("__PURPLE_NONE__\n")
	if err != nil {
		t.Fatalf("ParseListOutput: %v", err)
	}
	if engine != "" {
		t.Errorf("engine: %q", engine)
	}
	if len(cs) != 0 {
		t.Errorf("containers: %d", len(cs))
	}
}

func TestParseListOutputDocker(t *testing.T) {
	output := `Welcome to Ubuntu 22.04 (banner noise)
__PURPLE_ENGINE__docker
{"ID":"abc123def456","Names":"nginx","Image":"nginx:latest","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:80->80/tcp"}
{"ID":"ghi789jkl012","Names":"postgres","Image":"postgres:16","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:5432->5432/tcp"}
`
	engine, cs, err := containers.ParseListOutput(output)
	if err != nil {
		t.Fatalf("ParseListOutput: %v", err)
	}
	if engine != "docker" {
		t.Errorf("engine: %q", engine)
	}
	if len(cs) != 2 {
		t.Fatalf("containers: got %d want 2", len(cs))
	}
	if cs[0].Name != "nginx" {
		t.Errorf("cs[0].Name: %q", cs[0].Name)
	}
	if cs[0].Engine != "docker" {
		t.Errorf("cs[0].Engine: %q", cs[0].Engine)
	}
	if cs[0].ShortID != "abc123def456" {
		t.Errorf("cs[0].ShortID: %q", cs[0].ShortID)
	}
	if cs[1].Image != "postgres:16" {
		t.Errorf("cs[1].Image: %q", cs[1].Image)
	}
}

func TestParseListOutputPodman(t *testing.T) {
	// Podman uses "Id" instead of "ID".
	output := `__PURPLE_ENGINE__podman
{"Id":"pod123","Names":"redis","Image":"docker.io/redis:7","State":"running","Status":"Up 1 hour","Ports":""}
`
	engine, cs, err := containers.ParseListOutput(output)
	if err != nil {
		t.Fatalf("ParseListOutput: %v", err)
	}
	if engine != "podman" {
		t.Errorf("engine: %q", engine)
	}
	if len(cs) != 1 {
		t.Fatalf("containers: %d", len(cs))
	}
	if cs[0].ID != "pod123" {
		t.Errorf("ID: %q", cs[0].ID)
	}
	if cs[0].Name != "redis" {
		t.Errorf("Name: %q", cs[0].Name)
	}
	if cs[0].Engine != "podman" {
		t.Errorf("Engine: %q", cs[0].Engine)
	}
}

func TestParseListOutputWithMOTD(t *testing.T) {
	// MOTD lines before the sentinel are skipped.
	output := `Last login: Mon Sep 1 10:00:00 2026
Welcome to Ubuntu!

__PURPLE_ENGINE__docker
{"ID":"abc","Names":"api","Image":"api:latest","State":"running","Status":"Up","Ports":""}
garbage line without json
{"ID":"def","Names":"worker","Image":"worker:latest","State":"exited","Status":"Exited","Ports":""}
`
	engine, cs, err := containers.ParseListOutput(output)
	if err != nil {
		t.Fatalf("ParseListOutput: %v", err)
	}
	if engine != "docker" {
		t.Errorf("engine: %q", engine)
	}
	if len(cs) != 2 {
		t.Fatalf("containers: %d (garbage line should be skipped)", len(cs))
	}
}

func TestParseListOutputEmpty(t *testing.T) {
	engine, cs, err := containers.ParseListOutput("")
	if err != nil {
		t.Fatalf("ParseListOutput: %v", err)
	}
	if engine != "" {
		t.Errorf("engine: %q", engine)
	}
	if len(cs) != 0 {
		t.Errorf("containers: %d", len(cs))
	}
}

func TestParseListOutputShortID(t *testing.T) {
	output := `__PURPLE_ENGINE__docker
{"ID":"verylongid123456789","Names":"test","Image":"test","State":"running","Status":"Up","Ports":""}
`
	_, cs, _ := containers.ParseListOutput(output)
	if len(cs) != 1 {
		t.Fatalf("containers: %d", len(cs))
	}
	if cs[0].ShortID != "verylongid12" {
		t.Errorf("ShortID: %q (want first 12 chars)", cs[0].ShortID)
	}
}
