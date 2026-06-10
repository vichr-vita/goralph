package cli

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"
)

type pathOutput struct {
	Path string `json:"path"`
}

type databaseActionOutput struct {
	OK     bool   `json:"ok"`
	Action string `json:"action"`
	Path   string `json:"path"`
}

type fileActionOutput struct {
	OK     bool   `json:"ok"`
	Action string `json:"action"`
	File   string `json:"file"`
}

type messageOutput struct {
	OK      bool    `json:"ok"`
	Outcome Outcome `json:"outcome,omitempty"`
	Status  string  `json:"status"`
	Message string  `json:"message,omitempty"`
	Runs    int     `json:"runs,omitempty"`
}

type runSessionCommandOutput struct {
	OK         bool   `json:"ok"`
	Action     string `json:"action"`
	RunID      int64  `json:"run_id"`
	SessionRef string `json:"session_ref"`
	File       string `json:"file,omitempty"`
}

func writeJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(withJSONOutcome(value, OutcomeSuccess))
}

func withJSONOutcome(value any, outcome Outcome) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return value
	}
	addOutcome(decoded, string(outcome))
	return decoded
}

func addOutcome(value any, outcome string) {
	switch typed := value.(type) {
	case map[string]any:
		if _, exists := typed["outcome"]; !exists {
			typed["outcome"] = outcome
		}
		for _, child := range typed {
			addOutcome(child, outcome)
		}
	case []any:
		for _, child := range typed {
			addOutcome(child, outcome)
		}
	}
}

func jsonModeWriter(cmd *cobra.Command, stdout io.Writer) io.Writer {
	if jsonOutputFromContext(cmd.Context()) {
		return cmd.ErrOrStderr()
	}
	return stdout
}
