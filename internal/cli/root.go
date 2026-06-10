package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"goralph/internal/config"
	"goralph/internal/db"
	"goralph/internal/db/sqlc"
	gitrepo "goralph/internal/git"

	"github.com/spf13/cobra"
)

type settingsContextKey struct{}
type projectContextKey struct{}
type jsonOutputContextKey struct{}

// NewRootCommand creates the root goralph command.
func NewRootCommand() *cobra.Command {
	var cfgFile string
	var dbPath string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "goralph",
		Short: "Run Ralph loops for Go projects",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			settings, err := config.Load(cfgFile, dbPath)
			if err != nil {
				return err
			}

			ctx := context.WithValue(cmd.Context(), settingsContextKey{}, settings)
			ctx = context.WithValue(ctx, jsonOutputContextKey{}, jsonOutput)
			cmd.SetContext(ctx)
			if isProjectlessCommand(cmd) {
				return nil
			}

			ctx, err = prepareProjectContext(ctx, settings.DBPath)
			if err != nil {
				return err
			}
			cmd.SetContext(ctx)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	cmd.PersistentFlags().StringVar(&dbPath, "db", "", "SQLite database path")
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit JSON output")
	cmd.AddCommand(newDBCommand())
	cmd.AddCommand(newProjectCommand())
	cmd.AddCommand(newPRDCommand())
	cmd.AddCommand(newTaskCommand())
	cmd.AddCommand(newProgressCommand())
	cmd.AddCommand(newFeedbackCommand())
	cmd.AddCommand(newRunCommand())

	return cmd
}

func migrateDatabase(ctx context.Context, path string) error {
	database, err := db.Open(path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		return err
	}

	return nil
}

func prepareProjectContext(ctx context.Context, dbPath string) (context.Context, error) {
	projectRoot, err := currentProjectRoot()
	if err != nil {
		return ctx, err
	}

	database, err := db.Open(dbPath)
	if err != nil {
		return ctx, fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database); err != nil {
		return ctx, err
	}

	project, err := ensureProject(ctx, sqlc.New(database), projectRoot)
	if err != nil {
		return ctx, err
	}

	return context.WithValue(ctx, projectContextKey{}, project), nil
}

func currentProjectRoot() (string, error) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	root, err := gitrepo.FindRoot(cwd)
	if err != nil {
		if errors.Is(err, gitrepo.ErrRootNotFound) {
			return "", fmt.Errorf("resolve current project: no git root found from %s", cwd)
		}
		return "", fmt.Errorf("resolve current project: %w", err)
	}
	return root, nil
}

func ensureProject(ctx context.Context, queries *sqlc.Queries, rootPath string) (sqlc.Project, error) {
	project, err := queries.GetProjectByRootPath(ctx, rootPath)
	if err == nil {
		return project, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return sqlc.Project{}, fmt.Errorf("load project for git root %s: %w", rootPath, err)
	}

	project, err = queries.CreateProject(ctx, sqlc.CreateProjectParams{
		Name:     filepath.Base(rootPath),
		RootPath: rootPath,
	})
	if err == nil {
		return project, nil
	}

	project, getErr := queries.GetProjectByRootPath(ctx, rootPath)
	if getErr == nil {
		return project, nil
	}
	return sqlc.Project{}, fmt.Errorf("create project for git root %s: %w", rootPath, err)
}
