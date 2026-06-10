package cli

import (
	"fmt"

	"goralph/internal/prd"

	"github.com/spf13/cobra"
)

func newPRDCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prd",
		Short: "Manage PRD JSON files",
	}

	cmd.AddCommand(newPRDValidateCommand())

	return cmd
}

func newPRDValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Validate a PRD JSON array file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prd.ValidateFile(args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "validated %s\n", args[0])
			return err
		},
	}
}

func isPRDValidateCommand(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Name() != "validate" {
		return false
	}
	parent := cmd.Parent()
	return parent != nil && parent.Name() == "prd"
}
