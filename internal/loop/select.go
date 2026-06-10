package loop

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"goralph/internal/db/sqlc"
)

// TaskSelection is the outcome of looking for agent-selectable work.
type TaskSelection struct {
	Task    sqlc.Task
	HasTask bool
}

// SelectEligibleTask returns the oldest task ready for an agent.
// Pending tasks and failed retry tasks are eligible. Passed, blocked, and
// in-progress tasks are not eligible for new selection.
func SelectEligibleTask(ctx context.Context, queries *sqlc.Queries, projectID int64) (TaskSelection, error) {
	task, err := queries.GetNextEligibleTaskByProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TaskSelection{}, nil
		}
		return TaskSelection{}, fmt.Errorf("select eligible task: %w", err)
	}

	return TaskSelection{Task: task, HasTask: true}, nil
}
