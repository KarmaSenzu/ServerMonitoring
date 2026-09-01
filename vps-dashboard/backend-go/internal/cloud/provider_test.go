package cloud_test

import (
	"context"
	"testing"

	"vps-dashboard-api/internal/cloud"
)

func TestManualProvider(t *testing.T) {
	instances := []cloud.Instance{
		{ID: "i-001", Name: "web-01", PublicIP: "1.2.3.4", PrivateIP: "10.0.0.1", State: "running"},
		{ID: "i-002", Name: "db-01", PublicIP: "5.6.7.8", PrivateIP: "10.0.0.2", State: "running"},
	}
	p := cloud.NewManualProvider(instances)

	if p.Name() != "manual" {
		t.Errorf("Name: %q", p.Name())
	}

	all, err := p.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListInstances: got %d", len(all))
	}
	if all[0].Provider != "manual" {
		t.Errorf("Provider: %q", all[0].Provider)
	}

	one, err := p.GetInstance(context.Background(), "i-001")
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if one.Name != "web-01" {
		t.Errorf("Name: %q", one.Name)
	}

	_, err = p.GetInstance(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
}

func TestRegistry(t *testing.T) {
	reg := cloud.NewRegistry()
	reg.Register(cloud.NewManualProvider(nil))

	if reg.Get("manual") == nil {
		t.Fatal("manual provider not found")
	}
	if reg.Get("aws") != nil {
		t.Fatal("aws should not be registered")
	}

	names := reg.Names()
	if len(names) != 1 || names[0] != "manual" {
		t.Errorf("Names: %v", names)
	}
}

func TestRegistryListAll(t *testing.T) {
	reg := cloud.NewRegistry()
	reg.Register(cloud.NewManualProvider([]cloud.Instance{
		{ID: "i-1", Name: "alpha", PublicIP: "1.1.1.1", State: "running"},
	}))

	results, errs := reg.ListAll(context.Background())
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(results) != 1 {
		t.Fatalf("providers: %d", len(results))
	}
	if len(results["manual"]) != 1 {
		t.Fatalf("manual instances: %d", len(results["manual"]))
	}
}
