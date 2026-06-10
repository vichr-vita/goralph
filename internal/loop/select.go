package loop

import (
	"context"
	"fmt"

	"github.com/vichr-vita/goralph/internal/db"
	"github.com/vichr-vita/goralph/internal/db/sqlc"
)

// TaskSelection is the outcome of looking for agent-selectable work.
type TaskSelection struct {
	Task    sqlc.Task
	HasTask bool
}

// SelectEligibleTasks returns agent-selectable tasks in priority order.
// Pending tasks and failed retry tasks are eligible. Passed, blocked, and
// in-progress tasks are not eligible for new selection.
func SelectEligibleTasks(ctx context.Context, queries *sqlc.Queries, projectID int64) ([]sqlc.Task, error) {
	tasks, err := queries.ListTasksByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list eligible tasks: %w", err)
	}

	eligible := make([]sqlc.Task, 0, len(tasks))
	for _, task := range tasks {
		if isEligibleStatus(task.Status) {
			eligible = append(eligible, task)
		}
	}
	return eligible, nil
}

// SelectEligibleTask returns the highest-priority task ready for an agent.
func SelectEligibleTask(ctx context.Context, queries *sqlc.Queries, projectID int64) (TaskSelection, error) {
	eligible, err := SelectEligibleTasks(ctx, queries, projectID)
	if err != nil {
		return TaskSelection{}, err
	}
	if len(eligible) == 0 {
		return TaskSelection{}, nil
	}
	return TaskSelection{Task: eligible[0], HasTask: true}, nil
}

func isEligibleStatus(status string) bool {
	return status == string(db.TaskStatusPending) || status == string(db.TaskStatusFailed)
}
