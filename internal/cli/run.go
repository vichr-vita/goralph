package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"goralph/internal/db"
	"goralph/internal/db/sqlc"
	gitrepo "goralph/internal/git"
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
	var allowDirty bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Inspect runner sessions",
	}
	cmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "suppress live runner output")
	cmd.PersistentFlags().BoolVar(&allowDirty, "allow-dirty", false, "run agents without requiring a clean git worktree")
	cmd.AddCommand(newRunOneCommand(&quiet, &allowDirty))
	cmd.AddCommand(newRunAllCommand(&quiet, &allowDirty))
	cmd.AddCommand(newRunListCommand())
	cmd.AddCommand(newRunShowCommand())
	cmd.AddCommand(newRunOpenCommand())
	cmd.AddCommand(newRunExportCommand())

	return cmd
}

func newRunOneCommand(quiet *bool, allowDirty *bool) *cobra.Command {
	var taskID int64
	cmd := &cobra.Command{
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
			if err := requireCleanWorktreeBeforeAgent(cmd.Context(), project, *allowDirty); err != nil {
				return err
			}

			var forcedTaskID *int64
			if cmd.Flags().Changed("task") {
				if taskID <= 0 {
					return fmt.Errorf("invalid task id %q", strconv.FormatInt(taskID, 10))
				}
				forcedTaskID = &taskID
			}

			run, ran, _, err := executeAgentTurn(cmd.Context(), settings.DBPath, project, settings.RunnerCommand, settings.RunnerArgs, settings.FeedbackCommands, *quiet, nil, forcedTaskID, false, cmd.OutOrStdout(), cmd.ErrOrStderr())
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
	cmd.Flags().Int64Var(&taskID, "task", 0, "target task id")
	return cmd
}

func newRunAllCommand(quiet *bool, allowDirty *bool) *cobra.Command {
	var continueOnBlocked bool
	var maxTurns int

	cmd := &cobra.Command{
		Use:   "all",
		Short: "Run eligible task turns until none remain",
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxTurns < 0 {
				return fmt.Errorf("invalid max turns %d", maxTurns)
			}
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}
			if err := requireCleanWorktreeBeforeAgent(cmd.Context(), project, *allowDirty); err != nil {
				return err
			}

			runs := 0
			for {
				stop, err := inspectRunAllStop(cmd.Context(), settings.DBPath, project.ID, continueOnBlocked)
				if err != nil {
					return err
				}
				switch stop.kind {
				case runAllStopAllPassed:
					_, printErr := fmt.Fprintln(cmd.OutOrStdout(), "All tasks passed")
					return printErr
				case runAllStopBlockedOrFailed:
					return fmt.Errorf("blocked or failed tasks remain: %s", strings.Join(stop.descriptions, ", "))
				case runAllStopNoEligible:
					if runs == 0 {
						_, printErr := fmt.Fprintln(cmd.OutOrStdout(), "No eligible task")
						return printErr
					}
					return nil
				}
				if maxTurns > 0 && runs >= maxTurns {
					_, printErr := fmt.Fprintf(cmd.OutOrStdout(), "Max turns reached (%d)\n", maxTurns)
					return printErr
				}
				if err := requireCleanWorktreeBeforeAgent(cmd.Context(), project, *allowDirty); err != nil {
					return err
				}

				run, ran, complete, err := executeAgentTurn(cmd.Context(), settings.DBPath, project, settings.RunnerCommand, settings.RunnerArgs, settings.FeedbackCommands, *quiet, nil, nil, continueOnBlocked, cmd.OutOrStdout(), cmd.ErrOrStderr())
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
				if complete {
					return nil
				}
				if run.TaskID == nil {
					return nil
				}
			}
		},
	}
	cmd.Flags().BoolVar(&continueOnBlocked, "continue-on-blocked", false, "continue running pending tasks when blocked or failed tasks exist")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 0, "maximum agent turns to run (0 means unlimited)")
	return cmd
}

func requireCleanWorktreeBeforeAgent(ctx context.Context, project sqlc.Project, allowDirty bool) error {
	if allowDirty {
		return nil
	}
	return gitrepo.RequireCleanWorktree(ctx, project.RootPath)
}

func executeAgentTurn(ctx context.Context, dbPath string, project sqlc.Project, runnerCommand string, runnerArgs []string, feedbackCommands []string, quiet bool, seen map[int64]struct{}, forcedTaskID *int64, pendingOnly bool, stdout io.Writer, stderr io.Writer) (runOutput, bool, bool, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return runOutput{}, false, false, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	promptContract := loop.PromptContract{
		ProjectName:      project.Name,
		ProjectRootPath:  project.RootPath,
		FeedbackCommands: feedbackCommands,
	}
	beforeStatuses := map[int64]string{}
	runTaskID := sql.NullInt64{}

	if forcedTaskID != nil {
		task, err := queries.GetTaskByProjectAndID(ctx, sqlc.GetTaskByProjectAndIDParams{ProjectID: project.ID, ID: *forcedTaskID})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return runOutput{}, true, false, fmt.Errorf("task %d not found", *forcedTaskID)
			}
			return runOutput{}, true, false, fmt.Errorf("load task %d: %w", *forcedTaskID, err)
		}
		promptTask, err := promptTaskFromRow(ctx, queries, task)
		if err != nil {
			return runOutput{}, true, false, err
		}
		promptContract.ForcedTask = &promptTask
		beforeStatuses[task.ID] = task.Status
		runTaskID = sql.NullInt64{Int64: task.ID, Valid: true}
	} else {
		eligible, err := loop.SelectEligibleTasks(ctx, queries, project.ID)
		if err != nil {
			return runOutput{}, false, false, err
		}
		eligible = filterSeenTasks(eligible, seen)
		if pendingOnly {
			eligible = filterPendingTasks(eligible)
		}
		if len(eligible) == 0 {
			return runOutput{}, false, false, nil
		}

		promptTasks := make([]loop.PromptTask, 0, len(eligible))
		for _, task := range eligible {
			promptTask, err := promptTaskFromRow(ctx, queries, task)
			if err != nil {
				return runOutput{}, true, false, err
			}
			promptTasks = append(promptTasks, promptTask)
			beforeStatuses[task.ID] = task.Status
		}
		promptContract.EligibleTasks = promptTasks
	}
	prompt := loop.GenerateAgentPrompt(promptContract)

	host, _ := os.Hostname()
	started, err := queries.CreateRun(ctx, sqlc.CreateRunParams{
		ProjectID:  project.ID,
		TaskID:     runTaskID,
		RunnerName: pi.Name,
		Host:       host,
	})
	if err != nil {
		return runOutput{}, true, false, fmt.Errorf("create run: %w", err)
	}

	result, runErr := pi.New(runnerCommand, runnerArgs).Run(ctx, runner.Request{
		Prompt:  prompt,
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
	complete := hasCompletePromise(result.Stdout) || hasCompletePromise(result.Stderr)
	selectedTaskID, verifyErr := int64(0), error(nil)
	if !complete {
		selectedTaskID, verifyErr = verifyAgentTaskUpdate(ctx, queries, project.ID, started.ID, beforeStatuses)
	}
	if verifyErr != nil {
		if runErr == nil {
			runErr = verifyErr
		}
	} else if selectedTaskID != 0 {
		started, err = queries.SetRunTaskID(ctx, sqlc.SetRunTaskIDParams{
			TaskID:    sql.NullInt64{Int64: selectedTaskID, Valid: true},
			ProjectID: project.ID,
			ID:        started.ID,
		})
		if err != nil {
			if runErr == nil {
				runErr = fmt.Errorf("set run %d task: %w", started.ID, err)
			}
		} else if seen != nil {
			seen[selectedTaskID] = struct{}{}
		}
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
		return runOutputFromRow(started), true, complete, fmt.Errorf("finish run %d: %w", started.ID, finishErr)
	}
	if err := verifyFinishedRunMetadata(finished); err != nil {
		return runOutputFromRow(finished), true, complete, err
	}
	if runErr != nil {
		return runOutputFromRow(finished), true, complete, runErr
	}
	return runOutputFromRow(finished), true, complete, nil
}

type runAllStopKind string

type runAllStop struct {
	kind         runAllStopKind
	descriptions []string
}

const (
	runAllStopNone            runAllStopKind = ""
	runAllStopAllPassed       runAllStopKind = "all_passed"
	runAllStopBlockedOrFailed runAllStopKind = "blocked_or_failed"
	runAllStopNoEligible      runAllStopKind = "no_eligible"
)

func inspectRunAllStop(ctx context.Context, dbPath string, projectID int64, continueOnBlocked bool) (runAllStop, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return runAllStop{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	tasks, err := sqlc.New(database).ListTasksByProject(ctx, projectID)
	if err != nil {
		return runAllStop{}, fmt.Errorf("list tasks: %w", err)
	}
	if len(tasks) == 0 {
		return runAllStop{kind: runAllStopNoEligible}, nil
	}

	allPassed := true
	hasPending := false
	blockedOrFailed := make([]string, 0)
	for _, task := range tasks {
		if task.Status != string(db.TaskStatusPassed) {
			allPassed = false
		}
		switch task.Status {
		case string(db.TaskStatusPending):
			hasPending = true
		case string(db.TaskStatusBlocked), string(db.TaskStatusFailed):
			blockedOrFailed = append(blockedOrFailed, fmt.Sprintf("%d %s", task.ID, task.Status))
		}
	}
	if allPassed {
		return runAllStop{kind: runAllStopAllPassed}, nil
	}
	if !continueOnBlocked && len(blockedOrFailed) > 0 {
		return runAllStop{kind: runAllStopBlockedOrFailed, descriptions: blockedOrFailed}, nil
	}
	if !hasPending {
		return runAllStop{kind: runAllStopNoEligible}, nil
	}
	return runAllStop{kind: runAllStopNone}, nil
}

func filterSeenTasks(tasks []sqlc.Task, seen map[int64]struct{}) []sqlc.Task {
	if len(seen) == 0 {
		return tasks
	}
	filtered := make([]sqlc.Task, 0, len(tasks))
	for _, task := range tasks {
		if _, ok := seen[task.ID]; !ok {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func filterPendingTasks(tasks []sqlc.Task) []sqlc.Task {
	filtered := make([]sqlc.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Status == string(db.TaskStatusPending) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func hasCompletePromise(output string) bool {
	return strings.Contains(output, "<promise>COMPLETE</promise>")
}

func verifyAgentTaskUpdate(ctx context.Context, queries *sqlc.Queries, projectID int64, runID int64, beforeStatuses map[int64]string) (int64, error) {
	candidates := map[int64]struct{}{}
	progress, err := queries.ListProgressByRun(ctx, sqlc.ListProgressByRunParams{ProjectID: projectID, RunID: sql.NullInt64{Int64: runID, Valid: true}})
	if err != nil {
		return 0, fmt.Errorf("verify run %d progress: %w", runID, err)
	}
	for _, item := range progress {
		if !item.TaskID.Valid {
			continue
		}
		if _, ok := beforeStatuses[item.TaskID.Int64]; ok {
			candidates[item.TaskID.Int64] = struct{}{}
		}
	}

	tasks, err := queries.ListTasksByProject(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("verify run %d task updates: %w", runID, err)
	}
	for _, task := range tasks {
		before, ok := beforeStatuses[task.ID]
		if ok && task.Status != before {
			candidates[task.ID] = struct{}{}
		}
	}

	if len(candidates) == 0 {
		return 0, fmt.Errorf("run %d completed without updating an eligible task", runID)
	}
	if len(candidates) > 1 {
		return 0, fmt.Errorf("run %d updated multiple eligible tasks", runID)
	}
	for taskID := range candidates {
		return taskID, nil
	}
	return 0, fmt.Errorf("run %d selected task could not be determined", runID)
}

func verifyFinishedRunMetadata(run sqlc.Run) error {
	missing := make([]string, 0, 4)
	if run.Host == "" {
		missing = append(missing, "host")
	}
	if !run.HeartbeatAt.Valid {
		missing = append(missing, "heartbeat_at")
	}
	if !run.StartedAt.Valid {
		missing = append(missing, "started_at")
	}
	if !run.FinishedAt.Valid {
		missing = append(missing, "finished_at")
	}
	if len(missing) > 0 {
		return fmt.Errorf("run %d missing metadata: %s", run.ID, strings.Join(missing, ", "))
	}
	return nil
}

func promptTaskFromRow(ctx context.Context, queries *sqlc.Queries, task sqlc.Task) (loop.PromptTask, error) {
	steps, err := queries.ListTaskStepsByTask(ctx, task.ID)
	if err != nil {
		return loop.PromptTask{}, fmt.Errorf("list task %d steps: %w", task.ID, err)
	}
	progress, err := queries.ListLatestProgressByTask(ctx, sqlc.ListLatestProgressByTaskParams{
		TaskID: sql.NullInt64{Int64: task.ID, Valid: true},
		Limit:  latestProgressLimit,
	})
	if err != nil {
		return loop.PromptTask{}, fmt.Errorf("list task %d progress: %w", task.ID, err)
	}

	out := loop.PromptTask{
		ID:             task.ID,
		Category:       task.Category,
		Description:    task.Description,
		Status:         task.Status,
		ProgressReport: task.ProgressReport,
		Steps:          make([]string, 0, len(steps)),
		LatestProgress: make([]loop.PromptProgress, 0, len(progress)),
	}
	for _, step := range steps {
		out.Steps = append(out.Steps, step.Description)
	}
	for _, item := range progress {
		out.LatestProgress = append(out.LatestProgress, loop.PromptProgress{
			Summary:   item.Summary,
			CreatedAt: item.CreatedAt,
		})
	}
	return out, nil
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

func newRunListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List runs for this project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			runs, err := listRuns(cmd.Context(), settings.DBPath, project.ID)
			if err != nil {
				return err
			}
			return writeRunList(cmd, runs)
		},
	}
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

func newRunOpenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "open <id>",
		Short: "Open a run session with pi",
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
			ref, err := runSessionRef(run)
			if err != nil {
				return err
			}
			return runPiCommand(cmd.Context(), project, settings.RunnerCommand, []string{"--session", ref}, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func newRunExportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "export <id> [file]",
		Short: "Export a run session with pi",
		Args:  cobra.RangeArgs(1, 2),
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
			ref, err := runSessionRef(run)
			if err != nil {
				return err
			}
			piArgs := []string{"--export", ref}
			if len(args) == 2 {
				piArgs = append(piArgs, args[1])
			}
			return runPiCommand(cmd.Context(), project, settings.RunnerCommand, piArgs, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
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

func listRuns(ctx context.Context, dbPath string, projectID int64) ([]runOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	rows, err := sqlc.New(database).ListRunsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	runs := make([]runOutput, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, runOutputFromRow(row))
	}
	return runs, nil
}

func showRun(ctx context.Context, dbPath string, projectID int64, runID int64) (runOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return runOutput{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	run, err := loadRunRow(ctx, queries, projectID, runID)
	if err != nil {
		return runOutput{}, err
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

func loadRunRow(ctx context.Context, queries *sqlc.Queries, projectID int64, runID int64) (sqlc.Run, error) {
	run, err := queries.GetRunByProjectAndID(ctx, sqlc.GetRunByProjectAndIDParams{ProjectID: projectID, ID: runID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sqlc.Run{}, fmt.Errorf("run %d not found", runID)
		}
		return sqlc.Run{}, fmt.Errorf("load run %d: %w", runID, err)
	}
	return run, nil
}

func runSessionRef(run runOutput) (string, error) {
	if run.SessionPath != "" {
		return run.SessionPath, nil
	}
	if run.SessionID != "" {
		return run.SessionID, nil
	}
	return "", fmt.Errorf("run %d has no stored session metadata", run.ID)
}

func runPiCommand(ctx context.Context, project sqlc.Project, command string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = project.RootPath
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s failed: %w", command, strings.Join(args, " "), err)
	}
	return nil
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

func writeRunList(cmd *cobra.Command, runs []runOutput) error {
	if jsonOutputFromContext(cmd.Context()) {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(runs)
	}
	if len(runs) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No runs")
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "ID\tTASK\tSTATUS\tRUNNER\tMODEL\tSESSION\tSTARTED\tFINISHED"); err != nil {
		return err
	}
	for _, run := range runs {
		task := "-"
		if run.TaskID != nil {
			task = fmt.Sprintf("%d", *run.TaskID)
		}
		session := run.SessionID
		if session == "" {
			session = run.SessionPath
		}
		if session == "" {
			session = "-"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", run.ID, task, run.Status, run.RunnerName, emptyValue(run.RunnerModel), session, emptyValue(run.StartedAt), emptyValue(run.FinishedAt)); err != nil {
			return err
		}
	}
	return nil
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
