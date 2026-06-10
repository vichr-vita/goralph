package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"goralph/internal/db"
	"goralph/internal/db/sqlc"

	"github.com/spf13/cobra"
)

func newProgressCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "progress",
		Short: "Log and list project progress",
	}

	cmd.AddCommand(newProgressAddCommand())
	cmd.AddCommand(newProgressListCommand())

	return cmd
}

func newProgressAddCommand() *cobra.Command {
	var taskID int64
	var summary string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a progress entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			summary = strings.TrimSpace(summary)
			if summary == "" {
				return errors.New("progress summary cannot be empty")
			}
			if cmd.Flags().Changed("task") && taskID <= 0 {
				return fmt.Errorf("invalid task id %d", taskID)
			}

			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			progress, err := addProgress(cmd.Context(), settings.DBPath, project.ID, taskID, cmd.Flags().Changed("task"), summary)
			if err != nil {
				return err
			}
			return writeProgress(cmd, progress)
		},
	}
	cmd.Flags().Int64Var(&taskID, "task", 0, "task id")
	cmd.Flags().StringVar(&summary, "summary", "", "progress summary")
	_ = cmd.MarkFlagRequired("summary")

	return cmd
}

func newProgressListCommand() *cobra.Command {
	var taskID int64

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List progress entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("task") && taskID <= 0 {
				return fmt.Errorf("invalid task id %d", taskID)
			}

			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			progress, err := listProgress(cmd.Context(), settings.DBPath, project.ID, taskID, cmd.Flags().Changed("task"))
			if err != nil {
				return err
			}
			return writeProgressList(cmd, progress)
		},
	}
	cmd.Flags().Int64Var(&taskID, "task", 0, "task id filter")

	return cmd
}

func addProgress(ctx context.Context, dbPath string, projectID int64, taskID int64, hasTask bool, summary string) (progressOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return progressOutput{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	taskRef := sql.NullInt64{}
	if hasTask {
		if _, err := queries.GetTaskByProjectAndID(ctx, sqlc.GetTaskByProjectAndIDParams{ProjectID: projectID, ID: taskID}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return progressOutput{}, fmt.Errorf("task %d not found", taskID)
			}
			return progressOutput{}, fmt.Errorf("load task %d: %w", taskID, err)
		}
		taskRef = sql.NullInt64{Int64: taskID, Valid: true}
	}

	runRef := sql.NullInt64{}
	activeRun, err := queries.GetActiveRunByProject(ctx, projectID)
	if err == nil {
		runRef = sql.NullInt64{Int64: activeRun.ID, Valid: true}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return progressOutput{}, fmt.Errorf("load active run: %w", err)
	}

	progress, err := queries.CreateProgress(ctx, sqlc.CreateProgressParams{
		ProjectID: projectID,
		TaskID:    taskRef,
		RunID:     runRef,
		Summary:   summary,
	})
	if err != nil {
		return progressOutput{}, fmt.Errorf("insert progress: %w", err)
	}
	return progressOutputFromRow(progress), nil
}

func listProgress(ctx context.Context, dbPath string, projectID int64, taskID int64, hasTask bool) ([]progressOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	var rows []sqlc.Progress
	if hasTask {
		if _, err := queries.GetTaskByProjectAndID(ctx, sqlc.GetTaskByProjectAndIDParams{ProjectID: projectID, ID: taskID}); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("task %d not found", taskID)
			}
			return nil, fmt.Errorf("load task %d: %w", taskID, err)
		}
		rows, err = queries.ListProgressByProjectAndTask(ctx, sqlc.ListProgressByProjectAndTaskParams{
			ProjectID: projectID,
			TaskID:    sql.NullInt64{Int64: taskID, Valid: true},
		})
	} else {
		rows, err = queries.ListProgressByProject(ctx, projectID)
	}
	if err != nil {
		return nil, fmt.Errorf("list progress: %w", err)
	}

	outputs := make([]progressOutput, 0, len(rows))
	for _, row := range rows {
		outputs = append(outputs, progressOutputFromRow(row))
	}
	return outputs, nil
}

func progressOutputFromRow(row sqlc.Progress) progressOutput {
	out := progressOutput{
		ID:        row.ID,
		Summary:   row.Summary,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if row.TaskID.Valid {
		out.TaskID = &row.TaskID.Int64
	}
	if row.RunID.Valid {
		out.RunID = &row.RunID.Int64
	}
	return out
}

func writeProgress(cmd *cobra.Command, progress progressOutput) error {
	if jsonOutputFromContext(cmd.Context()) {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(progress)
	}
	return writeProgressText(cmd, progress)
}

func writeProgressList(cmd *cobra.Command, progress []progressOutput) error {
	if jsonOutputFromContext(cmd.Context()) {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(progress)
	}
	if len(progress) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No progress")
		return err
	}
	for index, item := range progress {
		if index > 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
		}
		if err := writeProgressText(cmd, item); err != nil {
			return err
		}
	}
	return nil
}

func writeProgressText(cmd *cobra.Command, progress progressOutput) error {
	task := "(none)"
	if progress.TaskID != nil {
		task = fmt.Sprintf("%d", *progress.TaskID)
	}
	run := "(none)"
	if progress.RunID != nil {
		run = fmt.Sprintf("%d", *progress.RunID)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Progress %d\n  Task: %s\n  Run: %s\n  Summary: %s\n  Created: %s\n  Updated: %s\n", progress.ID, task, run, progress.Summary, progress.CreatedAt, progress.UpdatedAt)
	return err
}
