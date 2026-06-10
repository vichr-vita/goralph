package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/vichr-vita/goralph/internal/db"
	"github.com/vichr-vita/goralph/internal/db/sqlc"

	"github.com/spf13/cobra"
)

type projectOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	RootPath    string `json:"root_path"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage the current project",
	}

	cmd.AddCommand(newProjectInfoCommand())
	cmd.AddCommand(newProjectInitCommand())

	return cmd
}

func newProjectInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}
			return writeProject(cmd, project, "Project")
		},
	}
}

func newProjectInitCommand() *cobra.Command {
	var name string
	var description string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFromContext(cmd.Context())
			if err != nil {
				return err
			}

			if cmd.Flags().Changed("name") {
				if name == "" {
					return errors.New("project name cannot be empty")
				}
				project.Name = name
			}
			if cmd.Flags().Changed("description") {
				project.Description = description
			}

			settings, err := settingsFromContext(cmd.Context())
			if err != nil {
				return err
			}
			updated, err := updateProject(cmd.Context(), settings.DBPath, project)
			if err != nil {
				return err
			}
			return writeProject(cmd, updated, "Initialized project")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project name")
	cmd.Flags().StringVar(&description, "description", "", "project description")

	return cmd
}

func updateProject(ctx context.Context, dbPath string, project sqlc.Project) (sqlc.Project, error) {
	database, err := db.Open(dbPath)
	if err != nil {
		return sqlc.Project{}, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	updated, err := sqlc.New(database).UpdateProject(ctx, sqlc.UpdateProjectParams{
		Name:        project.Name,
		Description: project.Description,
		ID:          project.ID,
	})
	if err != nil {
		return sqlc.Project{}, fmt.Errorf("update project: %w", err)
	}
	return updated, nil
}

func projectFromContext(ctx context.Context) (sqlc.Project, error) {
	project, ok := ctx.Value(projectContextKey{}).(sqlc.Project)
	if !ok {
		return sqlc.Project{}, errors.New("current project not loaded")
	}
	return project, nil
}

func jsonOutputFromContext(ctx context.Context) bool {
	jsonOutput, _ := ctx.Value(jsonOutputContextKey{}).(bool)
	return jsonOutput
}

func writeProject(cmd *cobra.Command, project sqlc.Project, title string) error {
	out := projectOutput{
		ID:          project.ID,
		Name:        project.Name,
		RootPath:    project.RootPath,
		Description: project.Description,
		CreatedAt:   project.CreatedAt,
		UpdatedAt:   project.UpdatedAt,
	}

	if jsonOutputFromContext(cmd.Context()) {
		return writeJSON(cmd, out)
	}

	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n  ID: %d\n  Name: %s\n  Root: %s\n  Description: %s\n  Created: %s\n  Updated: %s\n", title, project.ID, project.Name, project.RootPath, project.Description, project.CreatedAt, project.UpdatedAt)
	return err
}
