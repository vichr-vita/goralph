package db

import "fmt"

// TaskStatus is the set of task lifecycle values persisted in the database.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusPassed     TaskStatus = "passed"
	TaskStatusFailed     TaskStatus = "failed"
)

var validTaskStatuses = map[string]struct{}{
	string(TaskStatusPending):    {},
	string(TaskStatusInProgress): {},
	string(TaskStatusBlocked):    {},
	string(TaskStatusPassed):     {},
	string(TaskStatusFailed):     {},
}

// IsValidTaskStatus reports whether status is accepted by the data layer.
func IsValidTaskStatus(status string) bool {
	_, ok := validTaskStatuses[status]
	return ok
}

// ValidateTaskStatus rejects statuses that cannot be stored in task.status.
func ValidateTaskStatus(status string) error {
	if IsValidTaskStatus(status) {
		return nil
	}
	return fmt.Errorf("unknown task status %q", status)
}

// TaskStatusPasses maps persisted task status to PRD passes.
func TaskStatusPasses(status string) bool {
	return status == string(TaskStatusPassed)
}
