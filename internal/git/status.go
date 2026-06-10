package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrDirtyWorktree = errors.New("dirty git worktree")

type DirtyWorktreeError struct {
	Root   string
	Status string
}

func (e *DirtyWorktreeError) Error() string {
	message := fmt.Sprintf("dirty git worktree at %s; commit or stash changes, or pass --allow-dirty", e.Root)
	if strings.TrimSpace(e.Status) == "" {
		return message
	}
	return message + ":\n" + strings.TrimRight(e.Status, "\n")
}

func (e *DirtyWorktreeError) Unwrap() error {
	return ErrDirtyWorktree
}

func (e *DirtyWorktreeError) ExitCode() int {
	return 1
}

// RequireCleanWorktree verifies root is a git worktree and rejects tracked or untracked changes.
func RequireCleanWorktree(ctx context.Context, root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("check git status: empty root path")
	}

	inside, err := gitOutput(ctx, root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return fmt.Errorf("verify git repository at %s: %w", root, err)
	}
	if strings.TrimSpace(string(inside)) != "true" {
		return fmt.Errorf("verify git repository at %s: not inside a git worktree", root)
	}

	status, err := gitOutput(ctx, root, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status at %s: %w", root, err)
	}
	if len(bytes.TrimSpace(status)) > 0 {
		return &DirtyWorktreeError{Root: root, Status: string(status)}
	}
	return nil
}

func gitOutput(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
