package prd

import "goralph/internal/db"

// Task is the persisted task shape needed for PRD export.
type Task struct {
	Category    string
	Description string
	Steps       []string
	Status      string
}

// Item is one PRD JSON item.
type Item struct {
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Passes      bool     `json:"passes"`
}

// ExportTasks maps persisted task statuses into PRD pass booleans.
func ExportTasks(tasks []Task) ([]Item, error) {
	items := make([]Item, 0, len(tasks))
	for _, task := range tasks {
		if err := db.ValidateTaskStatus(task.Status); err != nil {
			return nil, err
		}
		items = append(items, Item{
			Category:    task.Category,
			Description: task.Description,
			Steps:       append([]string(nil), task.Steps...),
			Passes:      db.TaskStatusPasses(task.Status),
		})
	}
	return items, nil
}
