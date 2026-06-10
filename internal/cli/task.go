package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	TaskID    *int64 `json:"task_id,omitempty"`
	RunID     *int64 `json:"run_id,omitempty"`
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
	cmd.AddCommand(newTaskAddCommand())
	cmd.AddCommand(newTaskUpdateCommand())
	cmd.AddCommand(newTaskLifecycleCommand("start", db.TaskStatusInProgress, "Set a task in progress", false))
	cmd.AddCommand(newTaskLifecycleCommand("pass", db.TaskStatusPassed, "Set a task passed", false))
	cmd.AddCommand(newTaskLifecycleCommand("fail", db.TaskStatusFailed, "Set a task failed", true))
	cmd.AddCommand(newTaskLifecycleCommand("block", db.TaskStatusBlocked, "Set a task blocked", true))
	cmd.AddCommand(newTaskLifecycleCommand("unblock", db.TaskStatusPending, "Set a task pending", false))

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
			id, err := parseTaskID(args[0])
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

			task, err := showTask(cmd.Context(), settings.DBPath, project.ID, id)
			if err != nil {
				return err
			}
			return writeTask(cmd, task)
		},
	}
}

func newTaskAddCommand() *cobra.Command {
	var category string
	var description string
	var steps []string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a task",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(category) == "" {
				return errors.New("task category cannot be empty")
			}
			if strings.TrimSpace(description) == "" {
				return errors.New("task description cannot be empty")
			}
			if err := validateTaskSteps(steps); err != nil {
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

			task, err := addTask(cmd.Context(), settings.DBPath, project.ID, category, description, steps)
			if err != nil {
				return err
			}
			return writeTask(cmd, task)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "task category")
	cmd.Flags().StringVar(&description, "description", "", "task description")
	cmd.Flags().StringArrayVar(&steps, "step", nil, "task step; repeat to preserve order")
	_ = cmd.MarkFlagRequired("category")
	_ = cmd.MarkFlagRequired("description")

	return cmd
}

func newTaskUpdateCommand() *cobra.Command {
	var category string
	var description string
	var status string
	var progressReport string
	var steps []string

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("category") && strings.TrimSpace(category) == "" {
				return errors.New("task category cannot be empty")
			}
			if cmd.Flags().Changed("description") && strings.TrimSpace(description) == "" {
				return errors.New("task description cannot be empty")
			}
			if cmd.Flags().Changed("status") {
				if err := validateTaskStatus(status); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("step") {
				if err := validateTaskSteps(steps); err != nil {
					return err
				}
			}

			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			updates := taskUpdates{
				Category:              category,
				CategoryChanged:       cmd.Flags().Changed("category"),
				Description:           description,
				DescriptionChanged:    cmd.Flags().Changed("description"),
				Status:                status,
				StatusChanged:         cmd.Flags().Changed("status"),
				ProgressReport:        progressReport,
				ProgressReportChanged: cmd.Flags().Changed("progress_report"),
				Steps:                 steps,
				StepsChanged:          cmd.Flags().Changed("step"),
			}
			task, err := updateTask(cmd.Context(), settings.DBPath, project.ID, id, updates)
			if err != nil {
				return err
			}
			return writeTask(cmd, task)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "task category")
	cmd.Flags().StringVar(&description, "description", "", "task description")
	cmd.Flags().StringArrayVar(&steps, "step", nil, "replace task steps; repeat to preserve order")
	cmd.Flags().StringVar(&status, "status", "", "task status")
	cmd.Flags().StringVar(&progressReport, "progress_report", "", "task progress report")

	return cmd
}

func newTaskLifecycleCommand(name string, status db.TaskStatus, short string, requireReason bool) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   name + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			reason = strings.TrimSpace(reason)
			if requireReason && reason == "" {
				return errors.New("reason cannot be empty")
			}

			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			updates := taskUpdates{
				Status:                 string(status),
				StatusChanged:          true,
				ProgressSummary:        reason,
				ProgressSummaryChanged: requireReason,
			}
			task, err := updateTask(cmd.Context(), settings.DBPath, project.ID, id, updates)
			if err != nil {
				return err
			}
			return writeTask(cmd, task)
		},
	}
	if requireReason {
		cmd.Flags().StringVar(&reason, "reason", "", "progress reason")
		_ = cmd.MarkFlagRequired("reason")
	}

	return cmd
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

func addTask(ctx context.Context, dbPath string, projectID int64, category string, description string, steps []string) (taskOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return taskOutput{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return taskOutput{}, fmt.Errorf("begin task add transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := sqlc.New(database).WithTx(tx)
	task, err := queries.CreateTask(ctx, sqlc.CreateTaskParams{
		ProjectID:   projectID,
		Category:    category,
		Description: description,
		Status:      string(db.TaskStatusPending),
	})
	if err != nil {
		return taskOutput{}, fmt.Errorf("insert task: %w", err)
	}
	if err := replaceTaskSteps(ctx, queries, task.ID, steps); err != nil {
		return taskOutput{}, err
	}
	out, err := loadTaskDetails(ctx, queries, task)
	if err != nil {
		return taskOutput{}, err
	}
	if err := tx.Commit(); err != nil {
		return taskOutput{}, fmt.Errorf("commit task add: %w", err)
	}
	committed = true
	return out, nil
}

type taskUpdates struct {
	Category               string
	CategoryChanged        bool
	Description            string
	DescriptionChanged     bool
	Status                 string
	StatusChanged          bool
	ProgressReport         string
	ProgressReportChanged  bool
	ProgressSummary        string
	ProgressSummaryChanged bool
	Steps                  []string
	StepsChanged           bool
}

func (u taskUpdates) changed() bool {
	return u.CategoryChanged || u.DescriptionChanged || u.StatusChanged || u.ProgressReportChanged || u.ProgressSummaryChanged || u.StepsChanged
}

func updateTask(ctx context.Context, dbPath string, projectID int64, taskID int64, updates taskUpdates) (taskOutput, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return taskOutput{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return taskOutput{}, fmt.Errorf("begin task update transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := sqlc.New(database).WithTx(tx)
	task, err := queries.GetTaskByProjectAndID(ctx, sqlc.GetTaskByProjectAndIDParams{
		ProjectID: projectID,
		ID:        taskID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return taskOutput{}, fmt.Errorf("task %d not found", taskID)
		}
		return taskOutput{}, fmt.Errorf("load task %d: %w", taskID, err)
	}

	if !updates.changed() {
		out, err := loadTaskDetails(ctx, queries, task)
		if err != nil {
			return taskOutput{}, err
		}
		if err := tx.Commit(); err != nil {
			return taskOutput{}, fmt.Errorf("commit task update: %w", err)
		}
		committed = true
		return out, nil
	}

	if updates.CategoryChanged {
		task.Category = updates.Category
	}
	if updates.DescriptionChanged {
		task.Description = updates.Description
	}
	if updates.StatusChanged {
		task.Status = updates.Status
	}
	if updates.ProgressReportChanged {
		task.ProgressReport = updates.ProgressReport
	}

	updated, err := queries.UpdateTask(ctx, sqlc.UpdateTaskParams{
		Category:       task.Category,
		Description:    task.Description,
		Status:         task.Status,
		ProgressReport: task.ProgressReport,
		ProjectID:      projectID,
		ID:             taskID,
	})
	if err != nil {
		return taskOutput{}, fmt.Errorf("update task %d: %w", taskID, err)
	}
	if updates.StepsChanged {
		if err := queries.DeleteTaskStepsByTask(ctx, taskID); err != nil {
			return taskOutput{}, fmt.Errorf("delete task %d steps: %w", taskID, err)
		}
		if err := replaceTaskSteps(ctx, queries, taskID, updates.Steps); err != nil {
			return taskOutput{}, err
		}
	}
	if updates.ProgressSummaryChanged {
		if _, err := queries.CreateProgress(ctx, sqlc.CreateProgressParams{
			ProjectID: projectID,
			TaskID:    sql.NullInt64{Int64: taskID, Valid: true},
			Summary:   updates.ProgressSummary,
		}); err != nil {
			return taskOutput{}, fmt.Errorf("record task %d progress: %w", taskID, err)
		}
	}

	out, err := loadTaskDetails(ctx, queries, updated)
	if err != nil {
		return taskOutput{}, err
	}
	if err := tx.Commit(); err != nil {
		return taskOutput{}, fmt.Errorf("commit task update: %w", err)
	}
	committed = true
	return out, nil
}

func replaceTaskSteps(ctx context.Context, queries *sqlc.Queries, taskID int64, steps []string) error {
	for index, step := range steps {
		if _, err := queries.CreateTaskStep(ctx, sqlc.CreateTaskStepParams{
			TaskID:      taskID,
			Position:    int64(index + 1),
			Description: step,
		}); err != nil {
			return fmt.Errorf("insert task %d step %d: %w", taskID, index, err)
		}
	}
	return nil
}

func parseTaskID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid task id %q", value)
	}
	return id, nil
}

func validateTaskSteps(steps []string) error {
	for index, step := range steps {
		if strings.TrimSpace(step) == "" {
			return fmt.Errorf("task step %d cannot be empty", index+1)
		}
	}
	return nil
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
		out.LatestProgress = append(out.LatestProgress, progressOutputFromRow(item))
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
