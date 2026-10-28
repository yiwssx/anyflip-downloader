//go:build linux

package main

import (
	"syscall"
)

// Bavail excludes filesystem blocks reserved for privileged processes. The
// container runs as a non-root user; Bfree would overstate usable capacity.
func availableJobVolumeBytes(root string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return 0, err
	}
	size := uint64(stat.Bsize)
	if size != 0 && stat.Bavail > ^uint64(0)/size {
		return ^uint64(0), nil
	}
	return stat.Bavail * size, nil
}
