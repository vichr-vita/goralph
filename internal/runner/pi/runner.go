package pi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/vichr-vita/goralph/internal/runner"
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
	runnerArgs := r.Args()
	args := commandArgs(runnerArgs, req)
	cmd := exec.CommandContext(ctx, r.command, args...)
	cmd.Dir = req.WorkDir
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), req.Env...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdoutW := req.Stdout
	if stdoutW == nil {
		stdoutW = os.Stdout
	}
	stderrW := req.Stderr
	if stderrW == nil {
		stderrW = os.Stderr
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if !req.Quiet {
		cmd.Stdout = io.MultiWriter(stdoutW, &stdout)
		cmd.Stderr = io.MultiWriter(stderrW, &stderr)
	}
	if req.Interactive {
		cmd.Stdin = os.Stdin
	}

	metadata := runner.Metadata{
		RunnerName: Name,
		Command:    r.command,
		Args:       runnerArgs,
		Host:       hostName(),
		StartedAt:  startedAt,
		ExitCode:   -1,
	}

	if err := cmd.Start(); err != nil {
		metadata.FinishedAt = time.Now()
		metadata.ExitError = err.Error()
		return runner.Result{Metadata: metadata, Stdout: stdout.String(), Stderr: stderr.String()}, fmt.Errorf("start pi runner: %w", err)
	}
	metadata.PID = cmd.Process.Pid
	if req.OnStart != nil {
		req.OnStart(metadata)
	}

	err := cmd.Wait()
	metadata.FinishedAt = time.Now()
	metadata.ExitCode, metadata.ExitSignal = exitOutcome(cmd.ProcessState)
	metadata.SessionID, metadata.SessionPath = discoverSessionMetadata(startedAt, args, req)
	result := runner.Result{Metadata: metadata, Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		metadata.ExitError = err.Error()
		result.Metadata = metadata
		return result, fmt.Errorf("wait for pi runner: %w", err)
	}

	return result, nil
}

func commandArgs(args []string, req runner.Request) []string {
	if req.Interactive {
		args = withoutPrintArgs(args)
	}
	return append(slices.Clone(args), req.Prompt)
}

func withoutPrintArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "-p" || arg == "--print" {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
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

func discoverSessionMetadata(startedAt time.Time, args []string, req runner.Request) (string, string) {
	if hasArg(args, "--no-session") {
		return "", ""
	}
	if sessionRef := argValue(args, "--session"); sessionRef != "" {
		if path := cleanSessionPath(sessionRef); path != "" {
			return sessionIDFromPath(path), path
		}
		return sessionRef, ""
	}

	sessionDir := argValue(args, "--session-dir")
	if sessionDir == "" {
		sessionDir = envValue(req.Env, "PI_CODING_AGENT_SESSION_DIR")
	}
	if sessionDir == "" {
		sessionDir = os.Getenv("PI_CODING_AGENT_SESSION_DIR")
	}
	if sessionDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", ""
		}
		sessionDir = filepath.Join(home, ".pi", "agent", "sessions")
	}

	workDir := req.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return "", ""
		}
	}
	if abs, err := filepath.Abs(workDir); err == nil {
		workDir = abs
	}

	projectDir := filepath.Join(sessionDir, sessionProjectDir(workDir))
	path := newestSessionFile(projectDir, startedAt)
	if path == "" {
		path = newestSessionFile(sessionDir, startedAt)
	}
	return sessionIDFromPath(path), path
}

func hasArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func argValue(args []string, name string) string {
	prefix := name + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(arg, prefix))
		}
		if arg == name && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
	}
	return ""
}

func envValue(env []string, name string) string {
	prefix := name + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func cleanSessionPath(ref string) string {
	if strings.HasSuffix(ref, ".jsonl") || strings.ContainsRune(ref, os.PathSeparator) {
		if abs, err := filepath.Abs(ref); err == nil {
			return abs
		}
		return ref
	}
	return ""
}

func sessionProjectDir(workDir string) string {
	trimmed := strings.Trim(filepath.ToSlash(workDir), "/")
	if trimmed == "" {
		return "--"
	}
	return "--" + strings.ReplaceAll(trimmed, "/", "-") + "--"
}

func newestSessionFile(root string, startedAt time.Time) string {
	var newestPath string
	var newestMod time.Time
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().Before(startedAt.Add(-time.Second)) {
			return nil
		}
		if newestPath == "" || info.ModTime().After(newestMod) {
			newestPath = path
			newestMod = info.ModTime()
		}
		return nil
	})
	return newestPath
}

func sessionIDFromPath(path string) string {
	if path == "" {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if index := strings.LastIndex(base, "_"); index >= 0 && index+1 < len(base) {
		return base[index+1:]
	}
	return base
}

var _ runner.Runner = (*Runner)(nil)

// IsCommandNotFound reports whether an error came from an unavailable runner command.
func IsCommandNotFound(err error) bool {
	var pathErr *exec.Error
	return errors.As(err, &pathErr) && errors.Is(pathErr.Err, exec.ErrNotFound)
}
