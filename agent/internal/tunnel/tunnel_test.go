package tunnel

import (
	"context"
	"testing"
)

func TestInspectTunnelUnconfigured(t *testing.T) {
	status := InspectTunnel(context.Background(), nil, "", "apps.nugget.run")
	if status.Configured {
		t.Errorf("Expected Configured=false when no token provided")
	}
	if status.Running {
		t.Errorf("Expected Running=false")
	}
	if status.BaseDomain != "apps.nugget.run" {
		t.Errorf("Expected BaseDomain apps.nugget.run, got %s", status.BaseDomain)
	}
}

func TestInspectTunnelConfigured(t *testing.T) {
	status := InspectTunnel(context.Background(), nil, "mock-token-xyz", "custom.domain.com")
	if !status.Configured {
		t.Errorf("Expected Configured=true")
	}
	if status.BaseDomain != "custom.domain.com" {
		t.Errorf("Expected custom.domain.com, got %s", status.BaseDomain)
	}
}

