package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"goralph/internal/db"
	"goralph/internal/db/sqlc"

	"github.com/spf13/cobra"
)

type feedbackOutput struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Command   string `json:"command"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func newFeedbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feedback",
		Short: "Manage project feedback commands",
	}
	cmd.AddCommand(newFeedbackListCommand())
	cmd.AddCommand(newFeedbackSetCommand())
	cmd.AddCommand(newFeedbackRunCommand())
	return cmd
}

func newFeedbackListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List feedback commands for this project",
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
			commands, err := listFeedbackCommands(cmd.Context(), settings.DBPath, project.ID)
			if err != nil {
				return err
			}
			return writeFeedbackList(cmd, commands)
		},
	}
}

func newFeedbackSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <name> <command>",
		Short: "Set a feedback command for this project",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			command := strings.TrimSpace(args[1])
			if name == "" {
				return errors.New("feedback name cannot be empty")
			}
			if command == "" {
				return errors.New("feedback command cannot be empty")
			}

			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}
			updated, err := upsertFeedbackCommand(cmd.Context(), settings.DBPath, project.ID, name, command)
			if err != nil {
				return err
			}
			return writeFeedback(cmd, updated)
		},
	}
}

func newFeedbackRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run [name]",
		Short: "Run one or all feedback commands for this project",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}

			var commands []sqlc.FeedbackCommand
			if len(args) == 1 {
				name := strings.TrimSpace(args[0])
				if name == "" {
					return errors.New("feedback name cannot be empty")
				}
				feedback, err := getFeedbackCommand(cmd.Context(), settings.DBPath, project.ID, name)
				if err != nil {
					return err
				}
				commands = []sqlc.FeedbackCommand{feedback}
			} else {
				commands, err = listFeedbackCommands(cmd.Context(), settings.DBPath, project.ID)
				if err != nil {
					return err
				}
				if len(commands) == 0 {
					_, printErr := fmt.Fprintln(cmd.OutOrStdout(), "No feedback commands")
					return printErr
				}
			}

			return runFeedbackCommands(cmd.Context(), project.RootPath, commands, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func listFeedbackCommands(ctx context.Context, dbPath string, projectID int64) ([]sqlc.FeedbackCommand, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	commands, err := sqlc.New(database).ListFeedbackCommandsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list feedback commands: %w", err)
	}
	return commands, nil
}

func getFeedbackCommand(ctx context.Context, dbPath string, projectID int64, name string) (sqlc.FeedbackCommand, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return sqlc.FeedbackCommand{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	feedback, err := sqlc.New(database).GetFeedbackCommandByProjectAndName(ctx, sqlc.GetFeedbackCommandByProjectAndNameParams{ProjectID: projectID, Name: name})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sqlc.FeedbackCommand{}, fmt.Errorf("feedback command %q not found", name)
		}
		return sqlc.FeedbackCommand{}, fmt.Errorf("load feedback command %q: %w", name, err)
	}
	return feedback, nil
}

func upsertFeedbackCommand(ctx context.Context, dbPath string, projectID int64, name string, command string) (sqlc.FeedbackCommand, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return sqlc.FeedbackCommand{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	feedback, err := sqlc.New(database).UpsertFeedbackCommand(ctx, sqlc.UpsertFeedbackCommandParams{ProjectID: projectID, Name: name, Command: command})
	if err != nil {
		return sqlc.FeedbackCommand{}, fmt.Errorf("set feedback command %q: %w", name, err)
	}
	return feedback, nil
}

func runFeedbackCommands(ctx context.Context, rootPath string, commands []sqlc.FeedbackCommand, stdout io.Writer, stderr io.Writer) error {
	for _, feedback := range commands {
		if stderr != nil {
			if _, err := fmt.Fprintf(stderr, "Running feedback %s: %s\n", feedback.Name, feedback.Command); err != nil {
				return err
			}
		}
		command := exec.CommandContext(ctx, "sh", "-c", feedback.Command)
		command.Dir = rootPath
		command.Stdout = stdout
		command.Stderr = stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("feedback %q failed: %w", feedback.Name, err)
		}
	}
	return nil
}

func feedbackOutputFromRow(row sqlc.FeedbackCommand) feedbackOutput {
	return feedbackOutput{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Name:      row.Name,
		Command:   row.Command,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func writeFeedbackList(cmd *cobra.Command, commands []sqlc.FeedbackCommand) error {
	if jsonOutputFromContext(cmd.Context()) {
		out := make([]feedbackOutput, 0, len(commands))
		for _, command := range commands {
			out = append(out, feedbackOutputFromRow(command))
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}
	if len(commands) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No feedback commands")
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "NAME\tCOMMAND"); err != nil {
		return err
	}
	for _, command := range commands {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", command.Name, command.Command); err != nil {
			return err
		}
	}
	return nil
}

func writeFeedback(cmd *cobra.Command, command sqlc.FeedbackCommand) error {
	out := feedbackOutputFromRow(command)
	if jsonOutputFromContext(cmd.Context()) {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Feedback command\n  Name: %s\n  Command: %s\n", out.Name, out.Command)
	return err
}

func feedbackPromptCommands(ctx context.Context, queries *sqlc.Queries, projectID int64, configured []string) ([]string, error) {
	commands := append([]string{}, configured...)
	projectCommands, err := queries.ListFeedbackCommandsByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project feedback commands: %w", err)
	}
	for _, command := range projectCommands {
		commands = append(commands, formatFeedbackPromptCommand(command))
	}
	return commands, nil
}

func formatFeedbackPromptCommand(command sqlc.FeedbackCommand) string {
	return fmt.Sprintf("%s: %s", command.Name, command.Command)
}
