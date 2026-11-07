package monitor

import (
	"context"
	"testing"

	"github.com/nugget-sh/nugget/agent/internal/store"
)

func TestCollectMachineMetrics(t *testing.T) {
	metrics, err := CollectMachineMetrics()
	if err != nil {
		t.Fatalf("CollectMachineMetrics failed: %v", err)
	}

	if metrics.TotalDiskBytes == 0 {
		t.Errorf("Expected TotalDiskBytes > 0")
	}
	if metrics.TotalRAMBytes == 0 {
		t.Errorf("Expected TotalRAMBytes > 0")
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		secs     uint64
		expected string
	}{
		{45, "45s"},
		{125, "2m 5s"},
		{3665, "1h 1m 5s"},
		{90061, "1d 1h 1m"},
	}

	for _, tt := range tests {
		actual := FormatUptime(tt.secs)
		if actual != tt.expected {
			t.Errorf("FormatUptime(%d) = %s; want %s", tt.secs, actual, tt.expected)
		}
	}
}

func TestCollectAllApps(t *testing.T) {
	apps := []*store.Application{
		{
			ID:   "app_01",
			Name: "test-app",
		},
	}

	res := CollectAllApps(context.Background(), nil, apps)
	if len(res) != 1 {
		t.Fatalf("Expected 1 app metric, got %d", len(res))
	}
	if res[0].Status != "stopped" {
		t.Errorf("Expected status stopped for app with no container, got %s", res[0].Status)
	}
}

