package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// Web-only conservative bounds. CLI defaults remain unchanged.
	webMaxPages = 512
	webMaxJobBytes int64 = 512 << 20
)

var (
	errJobStorageBudgetExceeded = errors.New("job storage budget exceeded")
	errJobPageLimitExceeded = errors.New("job page count limit exceeded")
)

// checkJobStorageBudget counts regular files in the explicitly owned image
// workspace and PDF staging directory. It never follows symlinks and fails
// closed on unexpected entries. Checks occur at page and PDF chunk boundaries;
// this is NOT a hard filesystem quota or a substitute for volume/OS quotas.
func checkJobStorageBudget(limit int64, roots ...string) error {
	if limit <= 0 {
		return nil
	}
	var used int64
	for _, root := range roots {
		err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("unexpected symlink in owned job workspace")
			}
			if info.Mode().IsRegular() {
				size := info.Size()
				if size < 0 || size > limit-used {
					return fmt.Errorf("%w (limit %d bytes)", errJobStorageBudgetExceeded, limit)
				}
				used += size
			} else if !info.IsDir() {
				return fmt.Errorf("unexpected filesystem entry in owned job workspace")
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("measure job workspace: %w", err)
		}
	}
	return nil
}
