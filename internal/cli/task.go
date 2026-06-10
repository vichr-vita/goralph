package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"goralph/internal/db"
	"goralph/internal/db/sqlc"

	"github.com/spf13/cobra"
)

const latestProgressLimit = 5

type taskOutput struct {
	ID             int64            `json:"id"`
	Category       string           `json:"category"`
	Description    string           `json:"description"`
	Status         string           `json:"status"`
	ProgressReport string           `json:"progress_report"`
	CreatedAt      string           `json:"created_at"`
	UpdatedAt      string           `json:"updated_at"`
	Steps          []taskStepOutput `json:"steps"`
	LatestProgress []progressOutput `json:"latest_progress"`
}

type taskStepOutput struct {
	ID          int64  `json:"id"`
	Position    int64  `json:"position"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type progressOutput struct {
	ID        int64  `json:"id"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func newTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "List and inspect tasks",
	}

	cmd.AddCommand(newTaskListCommand())
	cmd.AddCommand(newTaskShowCommand())

	return cmd
}

func newTaskListCommand() *cobra.Command {
	var status string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List current project tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}
			if status != "" {
				if err := validateTaskStatus(status); err != nil {
					return err
				}
			}

			tasks, err := listTasks(cmd.Context(), settings.DBPath, project.ID, status)
			if err != nil {
				return err
			}
			return writeTasks(cmd, tasks)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by task status")

	return cmd
}

func newTaskShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("invalid task id %q", args[0])
			}

			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			task, err := showTask(cmd.Context(), settings.DBPath, project.ID, id)
			if err != nil {
				return err
			}
			return writeTask(cmd, task)
		},
	}
}

func listTasks(ctx context.Context, dbPath string, projectID int64, status string) ([]taskOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	var rows []sqlc.Task
	if status == "" {
		rows, err = queries.ListTasksByProject(ctx, projectID)
	} else {
		rows, err = queries.ListTasksByProjectAndStatus(ctx, sqlc.ListTasksByProjectAndStatusParams{
			ProjectID: projectID,
			Status:    status,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	outputs := make([]taskOutput, 0, len(rows))
	for _, row := range rows {
		out, err := loadTaskDetails(ctx, queries, row)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}
	return outputs, nil
}

func showTask(ctx context.Context, dbPath string, projectID int64, taskID int64) (taskOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return taskOutput{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	queries := sqlc.New(database)
	row, err := queries.GetTaskByProjectAndID(ctx, sqlc.GetTaskByProjectAndIDParams{
		ProjectID: projectID,
		ID:        taskID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return taskOutput{}, fmt.Errorf("task %d not found", taskID)
		}
		return taskOutput{}, fmt.Errorf("load task %d: %w", taskID, err)
	}
	return loadTaskDetails(ctx, queries, row)
}

func loadTaskDetails(ctx context.Context, queries *sqlc.Queries, task sqlc.Task) (taskOutput, error) {
	steps, err := queries.ListTaskStepsByTask(ctx, task.ID)
	if err != nil {
		return taskOutput{}, fmt.Errorf("list task %d steps: %w", task.ID, err)
	}
	progress, err := queries.ListLatestProgressByTask(ctx, sqlc.ListLatestProgressByTaskParams{
		TaskID: sql.NullInt64{Int64: task.ID, Valid: true},
		Limit:  latestProgressLimit,
	})
	if err != nil {
		return taskOutput{}, fmt.Errorf("list task %d progress: %w", task.ID, err)
	}

	out := taskOutput{
		ID:             task.ID,
		Category:       task.Category,
		Description:    task.Description,
		Status:         task.Status,
		ProgressReport: task.ProgressReport,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
		Steps:          make([]taskStepOutput, 0, len(steps)),
		LatestProgress: make([]progressOutput, 0, len(progress)),
	}
	for _, step := range steps {
		out.Steps = append(out.Steps, taskStepOutput{
			ID:          step.ID,
			Position:    step.Position,
			Description: step.Description,
			CreatedAt:   step.CreatedAt,
			UpdatedAt:   step.UpdatedAt,
		})
	}
	for _, item := range progress {
		out.LatestProgress = append(out.LatestProgress, progressOutput{
			ID:        item.ID,
			Summary:   item.Summary,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}
	return out, nil
}

func writeTasks(cmd *cobra.Command, tasks []taskOutput) error {
	if jsonOutputFromContext(cmd.Context()) {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(tasks)
	}

	if len(tasks) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No tasks")
		return err
	}
	for index, task := range tasks {
		if index > 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
		}
		if err := writeTaskText(cmd, task); err != nil {
			return err
		}
	}
	return nil
}

func writeTask(cmd *cobra.Command, task taskOutput) error {
	if jsonOutputFromContext(cmd.Context()) {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(task)
	}
	return writeTaskText(cmd, task)
}

func writeTaskText(cmd *cobra.Command, task taskOutput) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Task %d\n  Category: %s\n  Description: %s\n  Status: %s\n  Progress report: %s\n  Created: %s\n  Updated: %s\n", task.ID, task.Category, task.Description, task.Status, task.ProgressReport, task.CreatedAt, task.UpdatedAt); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  Steps:"); err != nil {
		return err
	}
	if len(task.Steps) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "    (none)"); err != nil {
			return err
		}
	} else {
		for _, step := range task.Steps {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "    %d. %s [%s]\n", step.Position, step.Description, step.UpdatedAt); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  Latest progress:"); err != nil {
		return err
	}
	if len(task.LatestProgress) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "    (none)")
		return err
	}
	for _, item := range task.LatestProgress {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "    - %s: %s\n", item.CreatedAt, item.Summary); err != nil {
			return err
		}
	}
	return nil
}
