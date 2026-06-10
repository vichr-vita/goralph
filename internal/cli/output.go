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
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Runs    int    `json:"runs,omitempty"`
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
	return encoder.Encode(value)
}

func jsonModeWriter(cmd *cobra.Command, stdout io.Writer) io.Writer {
	if jsonOutputFromContext(cmd.Context()) {
		return cmd.ErrOrStderr()
	}
	return stdout
}
