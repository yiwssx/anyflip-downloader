package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// These are conservative admission guards, not OS-enforced volume quotas.
	// Keep the existing per-job limit and the CLI defaults unchanged.
	webVolumeSoftLimit int64 = 4 << 30
	webMinimumFreeBytes int64 = 512 << 20
)

var errWebCapacityExceeded = errors.New("web job volume capacity unavailable")

// managedVolumeUsage counts only application-owned PDFs, image workspaces and
// temporary PDF staging directories. Unknown files are not counted as owned,
// never traversed or removed; filesystem free-space checks still see them.
func managedVolumeUsage(root string) (int64, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, fmt.Errorf("read web job volume: %w", err)
	}
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		_, suffix, jobOwned := jobArtifactID(name)
		stage := strings.HasPrefix(name, ".anyflip-pdf-stage-") && len(name) > len(".anyflip-pdf-stage-")
		if !jobOwned && !stage {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			return 0, fmt.Errorf("inspect managed volume entry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("managed volume contains unexpected symlink")
		}
		if (jobOwned && suffix == ".pdf") && !info.Mode().IsRegular() {
			return 0, fmt.Errorf("managed PDF is not a regular file")
		}
		if (jobOwned && suffix == ".images" || stage) && !info.IsDir() {
			return 0, fmt.Errorf("managed workspace is not a directory")
		}
		err = filepath.WalkDir(path, func(_ string, item fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := item.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("managed volume contains unexpected symlink")
			}
			if info.Mode().IsRegular() {
				if info.Size() < 0 || info.Size() > webVolumeSoftLimit-total {
					return errWebCapacityExceeded
				}
				total += info.Size()
			} else if !info.IsDir() {
				return fmt.Errorf("managed volume contains unexpected file type")
			}
			return nil
		})
		if err != nil {
			return 0, fmt.Errorf("measure managed volume: %w", err)
		}
	}
	return total, nil
}

// checkWebAdmission accounts for all retained managed artifacts plus a
// reservation for every already-running job and the candidate job. This
// admission guard is deliberately conservative; jobs can temporarily exceed
// their checkpointed per-job budget and only a host quota is a hard ceiling.
func checkWebAdmission(used int64, free uint64, active int) error {
	if used < 0 || active < 0 || active >= 2 {
		return errWebCapacityExceeded
	}
	reserved := int64(active+1) * webMaxJobBytes
	if used > webVolumeSoftLimit-reserved || free < uint64(reserved+webMinimumFreeBytes) {
		return errWebCapacityExceeded
	}
	return nil
}

func (s *webServer) reserveWebCapacity() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	used, err := managedVolumeUsage(s.root)
	if err != nil {
		return fmt.Errorf("%w: %v", errWebCapacityExceeded, err)
	}
	free, err := availableJobVolumeBytes(s.root)
	if err != nil {
		return fmt.Errorf("%w: cannot inspect filesystem", errWebCapacityExceeded)
	}
	if err := checkWebAdmission(used, free, s.activeReservations); err != nil {
		return err
	}
	s.activeReservations++
	return nil
}

func (s *webServer) releaseWebCapacity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeReservations--
}
