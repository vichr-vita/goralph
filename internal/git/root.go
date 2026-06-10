package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrRootNotFound = errors.New("git root not found")

// FindRoot walks upward from start and returns the nearest directory containing .git.
func FindRoot(start string) (string, error) {
	if start == "" {
		return "", errors.New("find git root: empty start path")
	}

	current, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve start path: %w", err)
	}

	info, err := os.Stat(current)
	if err != nil {
		return "", fmt.Errorf("stat start path: %w", err)
	}
	if !info.IsDir() {
		current = filepath.Dir(current)
	}

	for {
		if isGitRoot(current) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", ErrRootNotFound
		}
		current = parent
	}
}

func isGitRoot(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}
