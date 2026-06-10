package prd

import "testing"

func TestExportTasksMapsPassedStatusToPassesTrue(t *testing.T) {
	items, err := ExportTasks([]Task{
		{Category: "database", Description: "pending task", Steps: []string{"one"}, Status: "pending"},
		{Category: "database", Description: "in-progress task", Status: "in_progress"},
		{Category: "database", Description: "blocked task", Status: "blocked"},
		{Category: "database", Description: "passed task", Status: "passed"},
		{Category: "database", Description: "failed task", Status: "failed"},
	})
	if err != nil {
		t.Fatalf("ExportTasks: %v", err)
	}

	wantPasses := []bool{false, false, false, true, false}
	if len(items) != len(wantPasses) {
		t.Fatalf("items length = %d, want %d", len(items), len(wantPasses))
	}
	for i, want := range wantPasses {
		if items[i].Passes != want {
			t.Fatalf("items[%d].Passes = %v, want %v", i, items[i].Passes, want)
		}
	}
	if got := items[0].Steps[0]; got != "one" {
		t.Fatalf("steps[0] = %q, want one", got)
	}
}

func TestExportTasksRejectsUnknownStatus(t *testing.T) {
	_, err := ExportTasks([]Task{{Status: "unknown"}})
	if err == nil {
		t.Fatalf("ExportTasks succeeded, want error")
	}
}
