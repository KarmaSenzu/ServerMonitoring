// Package cloud implements cloud discovery for the Infrastructure
// Platform (PROJECT ARCHITECTURE.md §23-§24, Phase 9).
//
// The architecture follows the spec:
//
//	Cloud Provider → Provider Adapter → Discovery → Server Registry
//
// A narrow Provider interface isolates each cloud's logic. Discovered
// instances are never auto-managed: the user must explicitly approve
// them into the Server Registry (§24 Cloud Discovery Safety).
package cloud

import (
	"context"
	"fmt"
	"strings"
)

// Instance is a discovered cloud VM/container instance.
type Instance struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`    // aws, gcp, azure, hetzner, manual
	Region     string `json:"region"`
	Type       string `json:"type"`        // instance type (e.g. t3.micro)
	State      string `json:"state"`       // running, stopped, terminated
	PublicIP   string `json:"public_ip"`
	PrivateIP  string `json:"private_ip"`
	Tags       map[string]string `json:"tags"`
}

// Provider is the cloud provider abstraction (§23).
type Provider interface {
	// Name returns the provider identifier (aws, gcp, ...).
	Name() string

	// ListInstances discovers all instances visible to the configured
	// credentials.
	ListInstances(ctx context.Context) ([]Instance, error)

	// GetInstance returns a single instance by its cloud provider ID.
	GetInstance(ctx context.Context, id string) (*Instance, error)
}

// Registry holds configured providers by name.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider to the registry.
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Get returns the provider with the given name, or nil.
func (r *Registry) Get(name string) Provider {
	return r.providers[strings.ToLower(name)]
}

// Names returns all registered provider names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// ListAll discovers instances across all registered providers. Errors
// from individual providers are collected per-provider, not fatal.
func (r *Registry) ListAll(ctx context.Context) (map[string][]Instance, map[string]string) {
	results := make(map[string][]Instance)
	errors := make(map[string]string)
	for name, p := range r.providers {
		instances, err := p.ListInstances(ctx)
		if err != nil {
			errors[name] = err.Error()
			continue
		}
		results[name] = instances
	}
	return results, errors
}

// ManualProvider reads instances from static configuration (env vars
// or a JSON file). It is the default provider for environments without
// cloud API access — on-premise servers, homelab, or testing.
type ManualProvider struct {
	instances []Instance
}

// NewManualProvider constructs a ManualProvider from a static list.
func NewManualProvider(instances []Instance) *ManualProvider {
	for i := range instances {
		instances[i].Provider = "manual"
	}
	return &ManualProvider{instances: instances}
}

func (p *ManualProvider) Name() string { return "manual" }

func (p *ManualProvider) ListInstances(ctx context.Context) ([]Instance, error) {
	out := make([]Instance, len(p.instances))
	copy(out, p.instances)
	return out, nil
}

func (p *ManualProvider) GetInstance(ctx context.Context, id string) (*Instance, error) {
	for _, inst := range p.instances {
		if inst.ID == id {
			out := inst
			return &out, nil
		}
	}
	return nil, fmt.Errorf("manual: instance %s not found", id)
}

// ImportCandidate is an instance selected by the user for import
// into the Server Registry. The handler converts this into a
// models.Server row with status "unknown" (§24: discovery does not
// auto-grant management authorization).
type ImportCandidate struct {
	InstanceID string `json:"instance_id"`
	Provider   string `json:"provider"`
	Name       string `json:"server_name"`     // user-editable name
	Hostname   string `json:"hostname"`         // usually public_ip
	SSHPort    int    `json:"ssh_port"`
	SSHUsername string `json:"ssh_username"`
	Environment string `json:"environment"`
	Tags       []string `json:"tags"`
}
