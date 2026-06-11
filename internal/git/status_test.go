package git

import (
	"strings"
	"testing"
)

func TestDirtyWorktreeErrorIncludesRecoveryGuidance(t *testing.T) {
	err := (&DirtyWorktreeError{
		Root:   "/work/repo",
		Status: " M file.go\n?? new.txt\n",
	}).Error()

	for _, want := range []string{
		"dirty git worktree at /work/repo",
		" M file.go",
		"?? new.txt",
		"git status --short",
		"commit/stash/revert remaining changes",
		"rerunning goralph",
	} {
		if !strings.Contains(err, want) {
			t.Fatalf("dirty worktree error missing %q:\n%s", want, err)
		}
	}
}
