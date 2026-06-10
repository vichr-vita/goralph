package pi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vichr-vita/goralph/internal/runner"
)

func TestNewDefaultsToPiCommandAndPromptArg(t *testing.T) {
	r := New("", nil)
	if r.Command() != DefaultCommand {
		t.Fatalf("command = %q, want %q", r.Command(), DefaultCommand)
	}
	if !reflect.DeepEqual(r.Args(), []string{"-p"}) {
		t.Fatalf("args = %#v, want prompt flag", r.Args())
	}
}

func TestRunExecutesConfiguredCommandAndReturnsMetadata(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "args.txt")
	script := writeScript(t, "capture.sh", "#!/bin/sh\nfor arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > \"$GORALPH_ARGS_FILE\"\nprintf 'runner stdout\\n'\nprintf 'runner stderr\\n' >&2\n")
	r := New(script, []string{"-p", "--model", "test-model"})

	result, err := r.Run(context.Background(), runner.Request{
		Prompt: "hello prompt",
		Env:    []string{"GORALPH_ARGS_FILE=" + argsPath},
		Quiet:  true,
	})
	if err != nil {
		t.Fatalf("run pi: %v", err)
	}

	gotArgs := strings.Split(strings.TrimSpace(readFile(t, argsPath)), "\n")
	wantArgs := []string{"-p", "--model", "test-model", "hello prompt"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}

	metadata := result.Metadata
	if metadata.RunnerName != Name {
		t.Fatalf("runner name = %q, want %q", metadata.RunnerName, Name)
	}
	if metadata.Command != script {
		t.Fatalf("command = %q, want %q", metadata.Command, script)
	}
	if !reflect.DeepEqual(metadata.Args, []string{"-p", "--model", "test-model"}) {
		t.Fatalf("metadata args = %#v", metadata.Args)
	}
	if metadata.PID <= 0 {
		t.Fatalf("pid = %d, want positive", metadata.PID)
	}
	if metadata.Host == "" {
		t.Fatalf("host empty")
	}
	if metadata.StartedAt.IsZero() || metadata.FinishedAt.IsZero() || metadata.FinishedAt.Before(metadata.StartedAt) {
		t.Fatalf("bad timestamps: started=%s finished=%s", metadata.StartedAt, metadata.FinishedAt)
	}
	if metadata.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", metadata.ExitCode)
	}
	if metadata.ExitError != "" {
		t.Fatalf("exit error = %q, want empty", metadata.ExitError)
	}
	if result.Stdout != "runner stdout\n" {
		t.Fatalf("stdout = %q, want runner stdout", result.Stdout)
	}
	if result.Stderr != "runner stderr\n" {
		t.Fatalf("stderr = %q, want runner stderr", result.Stderr)
	}
}

func TestRunInteractiveLaunchesTUIWithoutPrintFlag(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "interactive-args.txt")
	script := writeScript(t, "interactive.sh", "#!/bin/sh\nfor arg in \"$@\"; do printf '%s\\n' \"$arg\"; done > \"$GORALPH_ARGS_FILE\"\nprintf 'interactive stdout\\n'\nprintf 'interactive stderr\\n' >&2\n")
	r := New(script, []string{"--provider", "test-provider", "--print", "--model", "test-model", "-p"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	result, err := r.Run(context.Background(), runner.Request{
		Prompt:      "initial prompt",
		Env:         []string{"GORALPH_ARGS_FILE=" + argsPath},
		Interactive: true,
		Stdout:      &stdout,
		Stderr:      &stderr,
	})
	if err != nil {
		t.Fatalf("run pi interactive: %v", err)
	}

	gotArgs := strings.Split(strings.TrimSpace(readFile(t, argsPath)), "\n")
	wantArgs := []string{"--provider", "test-provider", "--model", "test-model", "initial prompt"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
	if result.Stdout != "interactive stdout\n" {
		t.Fatalf("stdout = %q, want interactive stdout", result.Stdout)
	}
	if result.Stderr != "interactive stderr\n" {
		t.Fatalf("stderr = %q, want interactive stderr", result.Stderr)
	}
	if stdout.String() != "interactive stdout\n" {
		t.Fatalf("live stdout = %q, want interactive stdout", stdout.String())
	}
	if stderr.String() != "interactive stderr\n" {
		t.Fatalf("live stderr = %q, want interactive stderr", stderr.String())
	}
}

func TestRunStreamsByDefaultAndQuietSuppressesLiveOutput(t *testing.T) {
	script := writeScript(t, "stream.sh", "#!/bin/sh\nprintf 'live stdout\\n'\nprintf 'live stderr\\n' >&2\n")
	r := New(script, []string{"-p"})

	var liveStdout bytes.Buffer
	var liveStderr bytes.Buffer
	result, err := r.Run(context.Background(), runner.Request{Prompt: "stream prompt", Stdout: &liveStdout, Stderr: &liveStderr})
	if err != nil {
		t.Fatalf("run streaming pi: %v", err)
	}
	if liveStdout.String() != result.Stdout || liveStderr.String() != result.Stderr {
		t.Fatalf("live output = (%q, %q), result = (%q, %q)", liveStdout.String(), liveStderr.String(), result.Stdout, result.Stderr)
	}

	liveStdout.Reset()
	liveStderr.Reset()
	result, err = r.Run(context.Background(), runner.Request{Prompt: "quiet prompt", Quiet: true, Stdout: &liveStdout, Stderr: &liveStderr})
	if err != nil {
		t.Fatalf("run quiet pi: %v", err)
	}
	if liveStdout.Len() != 0 || liveStderr.Len() != 0 {
		t.Fatalf("quiet live output = (%q, %q), want empty", liveStdout.String(), liveStderr.String())
	}
	if result.Stdout != "live stdout\n" || result.Stderr != "live stderr\n" {
		t.Fatalf("quiet captured output = (%q, %q)", result.Stdout, result.Stderr)
	}
}

func TestRunDiscoversSessionFileMetadata(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	projectDir := filepath.Join(sessionDir, sessionProjectDir(workDir))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	sessionFile := filepath.Join(projectDir, "2026-06-10T00-00-00-000Z_019eb1ce-4932-713d-937c-d5be0d1b8d5e.jsonl")
	script := writeScript(t, "session.sh", "#!/bin/sh\nprintf 'session output\\n'\nprintf '{}' > \"$GORALPH_SESSION_FILE\"\n")
	r := New(script, []string{"-p", "--session-dir", sessionDir})

	result, err := r.Run(context.Background(), runner.Request{
		Prompt:  "session prompt",
		WorkDir: workDir,
		Env:     []string{"GORALPH_SESSION_FILE=" + sessionFile},
		Quiet:   true,
	})
	if err != nil {
		t.Fatalf("run pi session: %v", err)
	}
	if result.Metadata.SessionPath != sessionFile {
		t.Fatalf("session path = %q, want %q", result.Metadata.SessionPath, sessionFile)
	}
	if result.Metadata.SessionID != "019eb1ce-4932-713d-937c-d5be0d1b8d5e" {
		t.Fatalf("session id = %q", result.Metadata.SessionID)
	}
}

func TestRunRecordsNonZeroExitMetadata(t *testing.T) {
	script := writeScript(t, "fail.sh", "#!/bin/sh\nprintf 'failed stdout\\n'\nprintf 'failed stderr\\n' >&2\nexit 7\n")
	r := New(script, []string{"-p"})

	result, err := r.Run(context.Background(), runner.Request{Prompt: "fail prompt", Quiet: true})
	if err == nil {
		t.Fatalf("run pi error nil, want non-zero exit error")
	}
	if result.Metadata.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.Metadata.ExitCode)
	}
	if result.Metadata.ExitError == "" {
		t.Fatalf("exit error empty, want wait error")
	}
	if result.Stdout != "failed stdout\n" {
		t.Fatalf("stdout = %q, want failed stdout", result.Stdout)
	}
	if result.Stderr != "failed stderr\n" {
		t.Fatalf("stderr = %q, want failed stderr", result.Stderr)
	}
}

func TestRunReportsMissingCommand(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "missing-runner"), nil)

	_, err := r.Run(context.Background(), runner.Request{Prompt: "prompt"})
	if err == nil {
		t.Fatalf("run pi error nil, want command error")
	}
	if !IsCommandNotFound(err) && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want command not found", err)
	}
}

func writeScript(t *testing.T, name string, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
