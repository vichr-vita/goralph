package pi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"syscall"
	"time"

	"goralph/internal/runner"
)

const (
	// Name is the stable runner name stored for Pi-backed runs.
	Name = "pi"

	// DefaultCommand is used when no custom runner command is configured.
	DefaultCommand = "pi"
)

var defaultArgs = []string{"-p"}

// Runner executes prompts through the Pi CLI.
type Runner struct {
	command string
	args    []string
}

// New creates a Pi-backed runner. Empty command and nil args use Pi defaults.
func New(command string, args []string) *Runner {
	if command == "" {
		command = DefaultCommand
	}
	if args == nil {
		args = defaultArgs
	}
	return &Runner{
		command: command,
		args:    slices.Clone(args),
	}
}

// Command returns the executable used by the runner.
func (r *Runner) Command() string {
	return r.command
}

// Args returns configured arguments added before the prompt.
func (r *Runner) Args() []string {
	return slices.Clone(r.args)
}

// Run executes Pi with configured args and appends the prompt as the final argument.
func (r *Runner) Run(ctx context.Context, req runner.Request) (runner.Result, error) {
	startedAt := time.Now()
	args := append(r.Args(), req.Prompt)
	cmd := exec.CommandContext(ctx, r.command, args...)
	cmd.Dir = req.WorkDir
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), req.Env...)
	}

	metadata := runner.Metadata{
		RunnerName: Name,
		Command:    r.command,
		Args:       r.Args(),
		Host:       hostName(),
		StartedAt:  startedAt,
		ExitCode:   -1,
	}

	if err := cmd.Start(); err != nil {
		metadata.FinishedAt = time.Now()
		metadata.ExitError = err.Error()
		return runner.Result{Metadata: metadata}, fmt.Errorf("start pi runner: %w", err)
	}
	metadata.PID = cmd.Process.Pid

	err := cmd.Wait()
	metadata.FinishedAt = time.Now()
	metadata.ExitCode, metadata.ExitSignal = exitOutcome(cmd.ProcessState)
	if err != nil {
		metadata.ExitError = err.Error()
		return runner.Result{Metadata: metadata}, fmt.Errorf("wait for pi runner: %w", err)
	}

	return runner.Result{Metadata: metadata}, nil
}

func hostName() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}

func exitOutcome(state *os.ProcessState) (int, string) {
	if state == nil {
		return -1, ""
	}
	status, ok := state.Sys().(syscall.WaitStatus)
	if !ok {
		return state.ExitCode(), ""
	}
	if status.Signaled() {
		return -1, status.Signal().String()
	}
	return status.ExitStatus(), ""
}

var _ runner.Runner = (*Runner)(nil)

// IsCommandNotFound reports whether an error came from an unavailable runner command.
func IsCommandNotFound(err error) bool {
	var pathErr *exec.Error
	return errors.As(err, &pathErr) && errors.Is(pathErr.Err, exec.ErrNotFound)
}
