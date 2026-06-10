package loop

import (
	"fmt"
	"strings"
)

// PromptContract describes the context Ralph gives to an implementation agent.
type PromptContract struct {
	ProjectName      string
	ProjectRootPath  string
	ForcedTask       *PromptTask
	AssignedTask     *PromptTask
	EligibleTasks    []PromptTask
	FeedbackCommands []string
}

// PromptTask describes one task in the agent prompt contract.
type PromptTask struct {
	ID             int64
	Category       string
	Description    string
	Status         string
	ProgressReport string
	Steps          []string
	LatestProgress []PromptProgress
}

// PromptProgress describes recent progress included with a task.
type PromptProgress struct {
	Summary   string
	CreatedAt string
}

// GenerateAgentPrompt renders the Ralph loop agent prompt contract.
func GenerateAgentPrompt(contract PromptContract) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Ralph loop agent prompt contract\n\n")
	fmt.Fprintf(&b, "Project:\n")
	fmt.Fprintf(&b, "- Name: %s\n", emptyPromptValue(contract.ProjectName))
	fmt.Fprintf(&b, "- Root path: %s\n\n", emptyPromptValue(contract.ProjectRootPath))

	if contract.ForcedTask != nil {
		fmt.Fprintf(&b, "Forced task from --task:\n")
		writePromptTask(&b, *contract.ForcedTask)
		fmt.Fprintf(&b, "\n")
	} else if contract.AssignedTask != nil {
		fmt.Fprintf(&b, "Assigned task:\n")
		writePromptTask(&b, *contract.AssignedTask)
		fmt.Fprintf(&b, "\n")
	} else {
		fmt.Fprintf(&b, "Eligible tasks, highest priority first. Choose exactly one highest-priority task:\n")
		if len(contract.EligibleTasks) == 0 {
			fmt.Fprintf(&b, "- (none)\n")
		}
		for _, task := range contract.EligibleTasks {
			writePromptTask(&b, task)
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "Configured feedback commands:\n")
	if len(contract.FeedbackCommands) == 0 {
		fmt.Fprintf(&b, "- (none)\n")
	} else {
		for _, command := range contract.FeedbackCommands {
			fmt.Fprintf(&b, "- %s\n", command)
		}
	}

	fmt.Fprintf(&b, "\nRequired agent behavior:\n")
	fmt.Fprintf(&b, "- Work on only one feature.\n")
	fmt.Fprintf(&b, "- If multiple eligible tasks appear, choose the first task unless recent progress shows it is blocked.\n")
	fmt.Fprintf(&b, "- Before work, call `goralph task start <task-id>`.\n")
	fmt.Fprintf(&b, "- During or after work, call `goralph progress add --task <task-id> --summary \"<summary>\"`.\n")
	fmt.Fprintf(&b, "- Before passing task, run relevant configured feedback commands.\n")
	fmt.Fprintf(&b, "- At end, call exactly one: `goralph task pass <task-id>`, `goralph task fail <task-id> --reason \"<reason>\"`, or `goralph task block <task-id> --reason \"<reason>\"`.\n")
	fmt.Fprintf(&b, "- Commit the feature.\n")
	fmt.Fprintf(&b, "- When all work is complete, output `<promise>COMPLETE</promise>`.\n")

	return b.String()
}

func writePromptTask(b *strings.Builder, task PromptTask) {
	fmt.Fprintf(b, "- Task ID: %d\n", task.ID)
	fmt.Fprintf(b, "  Category: %s\n", task.Category)
	fmt.Fprintf(b, "  Description: %s\n", task.Description)
	fmt.Fprintf(b, "  Status: %s\n", task.Status)
	fmt.Fprintf(b, "  Steps:\n")
	if len(task.Steps) == 0 {
		fmt.Fprintf(b, "    - (none)\n")
	} else {
		for index, step := range task.Steps {
			fmt.Fprintf(b, "    %d. %s\n", index+1, step)
		}
	}
	fmt.Fprintf(b, "  Latest progress context:\n")
	if task.ProgressReport != "" {
		fmt.Fprintf(b, "    - Progress report: %s\n", task.ProgressReport)
	}
	if len(task.LatestProgress) == 0 {
		if task.ProgressReport == "" {
			fmt.Fprintf(b, "    - (none)\n")
		}
		return
	}
	for _, progress := range task.LatestProgress {
		if progress.CreatedAt == "" {
			fmt.Fprintf(b, "    - %s\n", progress.Summary)
			continue
		}
		fmt.Fprintf(b, "    - %s: %s\n", progress.CreatedAt, progress.Summary)
	}
}

func emptyPromptValue(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
