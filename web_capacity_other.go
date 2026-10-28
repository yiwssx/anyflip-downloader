//go:build !linux

package main

import "errors"

// The supported Web deployment target is Ubuntu/Linux. Refuse unknown disk
// capacity on other OSes rather than silently bypassing the admission guard.
// The cross-platform CLI remains unaffected.
func availableJobVolumeBytes(_ string) (uint64, error) {
	return 0, errors.New("Web capacity admission requires Linux")
}
