package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindRootReturnsNearestGitRoot(t *testing.T) {
	outer := filepath.Join(t.TempDir(), "outer")
	inner := filepath.Join(outer, "nested", "inner")
	start := filepath.Join(inner, "pkg")
	if err := os.MkdirAll(filepath.Join(outer, ".git"), 0o755); err != nil {
		t.Fatalf("create outer git root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0o755); err != nil {
		t.Fatalf("create inner git root: %v", err)
	}
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatalf("create start: %v", err)
	}

	got, err := FindRoot(start)
	if err != nil {
		t.Fatalf("find root: %v", err)
	}
	if got != inner {
		t.Fatalf("root = %q, want %q", got, inner)
	}
}

func TestFindRootAcceptsGitFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktree")
	start := filepath.Join(root, "pkg")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatalf("create start: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: ../repo/.git/worktrees/worktree\n"), 0o600); err != nil {
		t.Fatalf("write git file: %v", err)
	}

	got, err := FindRoot(start)
	if err != nil {
		t.Fatalf("find root: %v", err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}

func TestFindRootReportsMissingGitRoot(t *testing.T) {
	_, err := FindRoot(t.TempDir())
	if !errors.Is(err, ErrRootNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrRootNotFound)
	}
}
