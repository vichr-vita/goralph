package pi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"goralph/internal/runner"
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

	result, err := r.Run(context.Background(), runner.Request{
		Prompt:      "initial prompt",
		Env:         []string{"GORALPH_ARGS_FILE=" + argsPath},
		Interactive: true,
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
}

func TestRunRecordsNonZeroExitMetadata(t *testing.T) {
	script := writeScript(t, "fail.sh", "#!/bin/sh\nprintf 'failed stdout\\n'\nprintf 'failed stderr\\n' >&2\nexit 7\n")
	r := New(script, []string{"-p"})

	result, err := r.Run(context.Background(), runner.Request{Prompt: "fail prompt"})
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
