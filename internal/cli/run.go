package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"goralph/internal/db"
	"goralph/internal/db/sqlc"
	"goralph/internal/loop"
	"goralph/internal/runner"
	"goralph/internal/runner/pi"

	"github.com/spf13/cobra"
)

type runOutput struct {
	ID            int64            `json:"id"`
	TaskID        *int64           `json:"task_id,omitempty"`
	RunnerName    string           `json:"runner_name"`
	RunnerVersion string           `json:"runner_version"`
	RunnerModel   string           `json:"runner_model"`
	SessionID     string           `json:"session_id"`
	SessionPath   string           `json:"session_path"`
	Status        string           `json:"status"`
	ExitCode      *int64           `json:"exit_code,omitempty"`
	ExitSignal    string           `json:"exit_signal,omitempty"`
	ExitError     string           `json:"exit_error,omitempty"`
	PID           *int64           `json:"pid,omitempty"`
	Host          string           `json:"host"`
	HeartbeatAt   string           `json:"heartbeat_at,omitempty"`
	StartedAt     string           `json:"started_at,omitempty"`
	FinishedAt    string           `json:"finished_at,omitempty"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
	Progress      []progressOutput `json:"progress"`
}

func newRunCommand() *cobra.Command {
	var quiet bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect runner sessions",
	}
	cmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "suppress live runner output")
	cmd.AddCommand(newRunOneCommand(&quiet))
	cmd.AddCommand(newRunAllCommand(&quiet))
	cmd.AddCommand(newRunShowCommand())

	return cmd
}

func newRunOneCommand(quiet *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "one",
		Short: "Run one eligible task turn",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			run, ran, err := executeAgentTurn(cmd.Context(), settings.DBPath, project, settings.RunnerCommand, settings.RunnerArgs, *quiet, nil, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if !ran {
				_, printErr := fmt.Fprintln(cmd.OutOrStdout(), "No eligible task")
				return printErr
			}
			if err != nil {
				return err
			}
			return writeRun(cmd, run)
		},
	}
}

func newRunAllCommand(quiet *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "all",
		Short: "Run eligible task turns until none remain",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			runs := 0
			seen := map[int64]struct{}{}
			for {
				run, ran, err := executeAgentTurn(cmd.Context(), settings.DBPath, project, settings.RunnerCommand, settings.RunnerArgs, *quiet, seen, cmd.OutOrStdout(), cmd.ErrOrStderr())
				if !ran {
					if runs == 0 {
						_, printErr := fmt.Fprintln(cmd.OutOrStdout(), "No eligible task")
						return printErr
					}
					return nil
				}
				runs++
				if err != nil {
					return err
				}
				if err := writeRun(cmd, run); err != nil {
					return err
				}
				if run.TaskID == nil {
					return nil
				}
			}
		},
	}
}

func executeAgentTurn(ctx context.Context, dbPath string, project sqlc.Project, runnerCommand string, runnerArgs []string, quiet bool, seen map[int64]struct{}, stdout io.Writer, stderr io.Writer) (runOutput, bool, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return runOutput{}, false, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	selection, err := loop.SelectEligibleTask(ctx, queries, project.ID)
	if err != nil {
		return runOutput{}, false, err
	}
	if !selection.HasTask {
		return runOutput{}, false, nil
	}
	if seen != nil {
		if _, ok := seen[selection.Task.ID]; ok {
			return runOutput{}, false, nil
		}
		seen[selection.Task.ID] = struct{}{}
	}

	host, _ := os.Hostname()
	started, err := queries.CreateRun(ctx, sqlc.CreateRunParams{
		ProjectID:  project.ID,
		TaskID:     sql.NullInt64{Int64: selection.Task.ID, Valid: true},
		RunnerName: pi.Name,
		Host:       host,
	})
	if err != nil {
		return runOutput{}, true, fmt.Errorf("create run: %w", err)
	}

	result, runErr := pi.New(runnerCommand, runnerArgs).Run(ctx, runner.Request{
		Prompt:  promptForTask(selection.Task),
		WorkDir: project.RootPath,
		Quiet:   quiet,
		Stdout:  stdout,
		Stderr:  stderr,
	})
	metadata := result.Metadata
	if metadata.RunnerName == "" {
		metadata.RunnerName = started.RunnerName
	}
	if metadata.Host == "" {
		metadata.Host = started.Host
	}
	if runErr != nil && metadata.ExitError == "" {
		metadata.ExitError = runErr.Error()
	}

	finished, finishErr := queries.FinishRun(ctx, sqlc.FinishRunParams{
		RunnerName:    metadata.RunnerName,
		RunnerVersion: metadata.RunnerVersion,
		RunnerModel:   metadata.RunnerModel,
		SessionID:     metadata.SessionID,
		SessionPath:   metadata.SessionPath,
		Status:        statusForRun(ctx, metadata, runErr),
		ExitCode:      nullInt(int64(metadata.ExitCode), metadata.ExitCode >= 0),
		ExitSignal:    nullString(metadata.ExitSignal),
		ExitError:     nullString(metadata.ExitError),
		Pid:           nullInt(int64(metadata.PID), metadata.PID > 0),
		Host:          metadata.Host,
		ProjectID:     project.ID,
		ID:            started.ID,
	})
	if finishErr != nil {
		return runOutputFromRow(started), true, fmt.Errorf("finish run %d: %w", started.ID, finishErr)
	}
	if runErr != nil {
		return runOutputFromRow(finished), true, runErr
	}
	return runOutputFromRow(finished), true, nil
}

func promptForTask(task sqlc.Task) string {
	return fmt.Sprintf("Task ID: %d\nCategory: %s\nDescription: %s\n", task.ID, task.Category, task.Description)
}

func statusForRun(ctx context.Context, metadata runner.Metadata, err error) string {
	if ctx.Err() != nil {
		return "cancelled"
	}
	if err != nil || metadata.ExitSignal != "" || metadata.ExitError != "" || metadata.ExitCode != 0 {
		return "failed"
	}
	return "succeeded"
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullInt(value int64, valid bool) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: valid}
}

func newRunShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseRunID(args[0])
			if err != nil {
				return err
			}

			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			run, err := showRun(cmd.Context(), settings.DBPath, project.ID, id)
			if err != nil {
				return err
			}
			return writeRun(cmd, run)
		},
	}
}

func parseRunID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid run id %q", value)
	}
	return id, nil
}

func showRun(ctx context.Context, dbPath string, projectID int64, runID int64) (runOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return runOutput{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	run, err := queries.GetRunByProjectAndID(ctx, sqlc.GetRunByProjectAndIDParams{ProjectID: projectID, ID: runID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return runOutput{}, fmt.Errorf("run %d not found", runID)
		}
		return runOutput{}, fmt.Errorf("load run %d: %w", runID, err)
	}
	progressRows, err := queries.ListProgressByRun(ctx, sqlc.ListProgressByRunParams{ProjectID: projectID, RunID: sql.NullInt64{Int64: runID, Valid: true}})
	if err != nil {
		return runOutput{}, fmt.Errorf("list run progress: %w", err)
	}

	out := runOutputFromRow(run)
	out.Progress = make([]progressOutput, 0, len(progressRows))
	for _, row := range progressRows {
		out.Progress = append(out.Progress, progressOutputFromRow(row))
	}
	return out, nil
}

func runOutputFromRow(row sqlc.Run) runOutput {
	out := runOutput{
		ID:            row.ID,
		RunnerName:    row.RunnerName,
		RunnerVersion: row.RunnerVersion,
		RunnerModel:   row.RunnerModel,
		SessionID:     row.SessionID,
		SessionPath:   row.SessionPath,
		Status:        row.Status,
		Host:          row.Host,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if row.TaskID.Valid {
		out.TaskID = &row.TaskID.Int64
	}
	if row.ExitCode.Valid {
		out.ExitCode = &row.ExitCode.Int64
	}
	if row.ExitSignal.Valid {
		out.ExitSignal = row.ExitSignal.String
	}
	if row.ExitError.Valid {
		out.ExitError = row.ExitError.String
	}
	if row.Pid.Valid {
		out.PID = &row.Pid.Int64
	}
	if row.HeartbeatAt.Valid {
		out.HeartbeatAt = row.HeartbeatAt.String
	}
	if row.StartedAt.Valid {
		out.StartedAt = row.StartedAt.String
	}
	if row.FinishedAt.Valid {
		out.FinishedAt = row.FinishedAt.String
	}
	return out
}

func writeRun(cmd *cobra.Command, run runOutput) error {
	if jsonOutputFromContext(cmd.Context()) {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(run)
	}
	return writeRunText(cmd, run)
}

func writeRunText(cmd *cobra.Command, run runOutput) error {
	task := "(none)"
	if run.TaskID != nil {
		task = fmt.Sprintf("%d", *run.TaskID)
	}
	exitCode := "(none)"
	if run.ExitCode != nil {
		exitCode = fmt.Sprintf("%d", *run.ExitCode)
	}
	pid := "(none)"
	if run.PID != nil {
		pid = fmt.Sprintf("%d", *run.PID)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Run %d\n  Task: %s\n  Runner: %s\n  Version: %s\n  Model: %s\n  Status: %s\n  PID: %s\n  Host: %s\n  Exit code: %s\n  Exit signal: %s\n  Exit error: %s\n  Session ID: %s\n  Session file: %s\n  Heartbeat: %s\n  Started: %s\n  Finished: %s\n  Created: %s\n  Updated: %s\n", run.ID, task, run.RunnerName, run.RunnerVersion, run.RunnerModel, run.Status, pid, run.Host, exitCode, emptyValue(run.ExitSignal), emptyValue(run.ExitError), emptyValue(run.SessionID), emptyValue(run.SessionPath), emptyValue(run.HeartbeatAt), emptyValue(run.StartedAt), emptyValue(run.FinishedAt), run.CreatedAt, run.UpdatedAt); err != nil {
		return err
	}
	if len(run.Progress) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "  Progress: (none)")
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  Progress:"); err != nil {
		return err
	}
	for _, progress := range run.Progress {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "    - %s\n", progress.Summary); err != nil {
			return err
		}
	}
	return nil
}

func emptyValue(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
