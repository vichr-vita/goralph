package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"goralph/internal/db"
	"goralph/internal/db/sqlc"
	"goralph/internal/prd"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

func newPRDCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prd",
		Short: "Manage PRD JSON files",
	}

	cmd.AddCommand(newPRDValidateCommand())
	cmd.AddCommand(newPRDImportCommand())
	cmd.AddCommand(newPRDExportCommand())

	return cmd
}

func newPRDValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a PRD JSON array file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prd.ValidateFile(args[0]); err != nil {
				return err
			}
			if jsonOutputFromContext(cmd.Context()) {
				return writeJSON(cmd, fileActionOutput{OK: true, Action: "validate", File: args[0]})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "validated %s\n", args[0])
			return err
		},
	}
}

func newPRDImportCommand() *cobra.Command {
	var replace bool
	var appendMode bool

	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import PRD tasks into the current project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if replace && appendMode {
				return errors.New("--replace and --append cannot be used together")
			}

			items, err := prd.LoadFile(args[0])
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

			mode := importModeDefault
			if replace {
				mode = importModeReplace
			}
			if appendMode {
				mode = importModeAppend
			}

			result, err := importPRDItems(cmd.Context(), settings.DBPath, project.ID, items, mode, cmd)
			if err != nil {
				return err
			}
			if jsonOutputFromContext(cmd.Context()) {
				return writeJSON(cmd, prdImportOutput{OK: true, Imported: result.Imported, Mode: result.Mode, File: args[0]})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "imported %d tasks (%s) from %s\n", result.Imported, result.Mode, args[0])
			return err
		},
	}
	cmd.Flags().BoolVar(&replace, "replace", false, "replace current project tasks before import")
	cmd.Flags().BoolVar(&appendMode, "append", false, "append imported tasks without deleting current tasks")

	return cmd
}

func newPRDExportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "export [file]",
		Short: "Export current project tasks as PRD JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			items, err := exportPRDItems(cmd.Context(), settings.DBPath, project.ID)
			if err != nil {
				return err
			}
			data, err := json.MarshalIndent(items, "", "  ")
			if err != nil {
				return fmt.Errorf("encode PRD export: %w", err)
			}
			data = append(data, '\n')

			if len(args) == 0 {
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}
			if err := os.WriteFile(args[0], data, 0o600); err != nil {
				return err
			}
			if jsonOutputFromContext(cmd.Context()) {
				return writeJSON(cmd, fileActionOutput{OK: true, Action: "export", File: args[0]})
			}
			return nil
		},
	}
}

func isPRDValidateCommand(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Name() != "validate" {
		return false
	}
	parent := cmd.Parent()
	return parent != nil && parent.Name() == "prd"
}

type importMode string

const (
	importModeDefault importMode = "default"
	importModeReplace importMode = "replace"
	importModeAppend  importMode = "append"
)

type importResult struct {
	Imported int
	Mode     importMode
}

type prdImportOutput struct {
	OK       bool       `json:"ok"`
	Imported int        `json:"imported"`
	Mode     importMode `json:"mode"`
	File     string     `json:"file"`
}

func importPRDItems(ctx context.Context, dbPath string, projectID int64, items []prd.Item, mode importMode, cmd *cobra.Command) (importResult, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return importResult{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return importResult{}, fmt.Errorf("begin import transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	queries := sqlc.New(database).WithTx(tx)
	existing, err := queries.CountTasksByProject(ctx, projectID)
	if err != nil {
		return importResult{}, fmt.Errorf("count current tasks: %w", err)
	}

	if existing > 0 && mode == importModeDefault {
		if !stdinIsTerminal(cmd) {
			return importResult{}, errors.New("current project already has tasks; rerun with --replace or --append")
		}
		confirmed, err := confirmReplace(cmd, existing)
		if err != nil {
			return importResult{}, err
		}
		if !confirmed {
			return importResult{}, errors.New("import cancelled")
		}
		mode = importModeReplace
	}

	if mode == importModeDefault {
		mode = importModeReplace
	}

	if existing > 0 && mode == importModeReplace {
		if err := queries.DeleteTaskStepsByProject(ctx, projectID); err != nil {
			return importResult{}, fmt.Errorf("delete current task steps: %w", err)
		}
		if err := queries.DeleteTasksByProject(ctx, projectID); err != nil {
			return importResult{}, fmt.Errorf("delete current tasks: %w", err)
		}
	}

	for index, item := range items {
		status := string(db.TaskStatusPending)
		if item.Passes {
			status = string(db.TaskStatusPassed)
		}
		task, err := queries.CreateTask(ctx, sqlc.CreateTaskParams{
			ProjectID:   projectID,
			Category:    item.Category,
			Description: item.Description,
			Status:      status,
		})
		if err != nil {
			return importResult{}, fmt.Errorf("insert task %d: %w", index, err)
		}
		for stepIndex, step := range item.Steps {
			if _, err := queries.CreateTaskStep(ctx, sqlc.CreateTaskStepParams{
				TaskID:      task.ID,
				Position:    int64(stepIndex + 1),
				Description: step,
			}); err != nil {
				return importResult{}, fmt.Errorf("insert task %d step %d: %w", index, stepIndex, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return importResult{}, fmt.Errorf("commit PRD import: %w", err)
	}
	committed = true

	return importResult{Imported: len(items), Mode: mode}, nil
}

func exportPRDItems(ctx context.Context, dbPath string, projectID int64) ([]prd.Item, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	rows, err := sqlc.New(database).ListTaskPRDRowsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list PRD tasks: %w", err)
	}

	tasks := make([]prd.Task, 0)
	taskIndexByID := make(map[int64]int)
	for _, row := range rows {
		index, ok := taskIndexByID[row.ID]
		if !ok {
			index = len(tasks)
			taskIndexByID[row.ID] = index
			tasks = append(tasks, prd.Task{
				Category:    row.Category,
				Description: row.Description,
				Steps:       []string{},
				Status:      row.Status,
			})
		}
		if row.StepDescription.Valid {
			tasks[index].Steps = append(tasks[index].Steps, row.StepDescription.String)
		}
	}

	items, err := prd.ExportTasks(tasks)
	if err != nil {
		return nil, fmt.Errorf("map PRD tasks: %w", err)
	}
	return items, nil
}

func stdinIsTerminal(cmd *cobra.Command) bool {
	file, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func confirmReplace(cmd *cobra.Command, existing int64) (bool, error) {
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "current project has %d tasks; replace them? [Y/n] ", existing); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "" || answer == "y" || answer == "yes", nil
}
