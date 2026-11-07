//go:build !linux && !darwin

package sysinfo

func getDiskSpace(path string) (uint64, uint64, error) {
	return 0, 0, nil
}

