// Package paths finds the local directories used by the CLI.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// CacheDir returns the directory for downloaded content.
func CacheDir() string {
	if v := os.Getenv("DEVOPS_DAILY_CACHE_DIR"); v != "" {
		return v
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(".", ".cache", "devops-daily")
	}
	return filepath.Join(dir, "devops-daily")
}

// StateDir returns the directory for local progress.
func StateDir() string {
	if v := os.Getenv("DEVOPS_DAILY_STATE_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "linux" {
		if v := os.Getenv("XDG_STATE_HOME"); v != "" {
			return filepath.Join(v, "devops-daily")
		}
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".local", "state", "devops-daily")
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", ".config", "devops-daily")
	}
	return filepath.Join(dir, "devops-daily")
}
