//go:build !linux && !darwin

package monitor

import "time"

// CollectMachineMetrics fallback for unsupported platforms.
func CollectMachineMetrics() (*MachineMetrics, error) {
	return &MachineMetrics{
		CollectedAt: time.Now().UTC(),
	}, nil
}

