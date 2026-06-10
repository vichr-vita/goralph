package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	gitrepo "goralph/internal/git"

	"github.com/spf13/cobra"
)

// Outcome is a named command outcome.
type Outcome string

const (
	OutcomeSuccess                 Outcome = "success"
	OutcomeGenericError            Outcome = "generic_error"
	OutcomeValidationError         Outcome = "validation_error"
	OutcomeNoEligibleTasks         Outcome = "no_eligible_tasks"
	OutcomeDirtyGit                Outcome = "dirty_git"
	OutcomeActiveRunExists         Outcome = "active_run_exists"
	OutcomeRunnerFailed            Outcome = "runner_failed"
	OutcomeFeedbackFailed          Outcome = "feedback_failed"
	OutcomeTaskBlockedOrFailedStop Outcome = "task_blocked_or_failed_stop"
)

var outcomeExitCodes = map[Outcome]int{
	OutcomeSuccess:                 0,
	OutcomeGenericError:            1,
	OutcomeValidationError:         2,
	OutcomeNoEligibleTasks:         3,
	OutcomeDirtyGit:                4,
	OutcomeActiveRunExists:         5,
	OutcomeRunnerFailed:            6,
	OutcomeFeedbackFailed:          7,
	OutcomeTaskBlockedOrFailedStop: 8,
}

type outcomeError struct {
	outcome Outcome
	err     error
}

func (e *outcomeError) Error() string {
	return e.err.Error()
}

func (e *outcomeError) Unwrap() error {
	return e.err
}

func (e *outcomeError) Outcome() Outcome {
	return e.outcome
}

func withOutcome(outcome Outcome, err error) error {
	if err == nil {
		return nil
	}
	return &outcomeError{outcome: outcome, err: err}
}

func outcomeErrorf(outcome Outcome, format string, args ...any) error {
	return withOutcome(outcome, fmt.Errorf(format, args...))
}

func OutcomeForError(err error) Outcome {
	if err == nil {
		return OutcomeSuccess
	}
	var with interface{ Outcome() Outcome }
	if errors.As(err, &with) {
		return with.Outcome()
	}
	if errors.Is(err, gitrepo.ErrDirtyWorktree) {
		return OutcomeDirtyGit
	}
	if looksLikeCommandValidationError(err) {
		return OutcomeValidationError
	}
	var active *activeRunExistsError
	if errors.As(err, &active) {
		return OutcomeActiveRunExists
	}
	var stale *staleRunRequiresForceError
	if errors.As(err, &stale) {
		return OutcomeActiveRunExists
	}
	return OutcomeGenericError
}

func looksLikeCommandValidationError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "unknown command ") ||
		strings.Contains(message, "unknown flag: ") ||
		strings.Contains(message, "invalid argument ") ||
		strings.Contains(message, "requires at least ") ||
		strings.Contains(message, "accepts ")
}

func ExitCodeForOutcome(outcome Outcome) int {
	if code, ok := outcomeExitCodes[outcome]; ok {
		return code
	}
	return outcomeExitCodes[OutcomeGenericError]
}

func ExitCodeForError(err error) int {
	return ExitCodeForOutcome(OutcomeForError(err))
}

func WriteErrorJSON(w io.Writer, err error) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(map[string]any{
		"ok":      false,
		"outcome": OutcomeForError(err),
		"error":   err.Error(),
	})
}

func CommandWantsJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flag("json")
	return flag != nil && flag.Value.String() == "true"
}
